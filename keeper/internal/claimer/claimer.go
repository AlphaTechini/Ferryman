// Package claimer drives Ferry Out exits to delivery. Per exit it locates the Outbox message, waits for
// confirmation, verifies the explorer's claim calldata, and then runs ExitFactory.claimAndSweep.
//
// Stages: intent -> confirmed -> claimed -> delivered. "claimed" means a claim transaction was sent but
// has not succeeded yet; it is processed like "confirmed" on the next pass, and isSwept makes every step
// idempotent across restarts.
package claimer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// recoveryWindow is how many HyperEVM blocks back the recovery scan looks for an already executed claim.
const recoveryWindow = 200_000

// Errors. Use errors.Is.
var (
	ErrNoCalldata = errors.New("claimer: no claim calldata available")
	ErrRefused    = errors.New("claimer: refused unsafe claim calldata")
)

// Explorer is the subset of the explorer client the claimer uses.
type Explorer interface {
	WithdrawalsByTx(ctx context.Context, txHash string) ([]explorer.Withdrawal, error)
	Claim(ctx context.Context, id int64) (explorer.ClaimData, error)
}

// Sender sends HyperEVM transactions (implemented by *chain.Sender).
type Sender interface {
	Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error)
}

// Settings configures the claimer.
type Settings struct {
	ExitFactory common.Address
	Outbox      common.Address
	Interval    time.Duration
	LogRange    uint64
	Maturity    time.Duration // estimate until an exit matures (25 minutes)
}

// Claimer processes exits.
type Claimer struct {
	s   Settings
	el  chain.Reader // Elysium, for the L2ToL1Tx receipt fallback
	hy  chain.Reader // HyperEVM, for isSwept, recovery logs and the head
	ex  Explorer
	tx  Sender // nil = read-only
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	mu sync.Mutex
}

// New creates a claimer. tx may be nil (read-only: stages are tracked, nothing is sent).
func New(s Settings, el, hy chain.Reader, ex Explorer, tx Sender, st *store.Store, log *slog.Logger) *Claimer {
	if s.Maturity == 0 {
		s.Maturity = 25 * time.Minute
	}
	if s.LogRange == 0 {
		s.LogRange = 5000
	}
	return &Claimer{s: s, el: el, hy: hy, ex: ex, tx: tx, st: st, log: log, now: time.Now}
}

// Run processes all exits every Interval until ctx is cancelled.
func (c *Claimer) Run(ctx context.Context) {
	t := time.NewTicker(c.s.Interval)
	defer t.Stop()
	for {
		if err := c.RunOnce(ctx); err != nil {
			c.log.Warn("claimer pass failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce advances every undelivered exit by one step. Per-exit errors are logged and do not stop the pass.
func (c *Claimer) RunOnce(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.s.ExitFactory == (common.Address{}) {
		return nil
	}
	for _, e := range c.st.Exits() {
		if e.Stage == "delivered" {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		updated, err := c.Step(ctx, e)
		if err != nil {
			c.log.Warn("exit step failed", "receiver", e.Receiver, "stage", e.Stage, "err", err)
		}
		if !reflect.DeepEqual(updated, e) {
			if perr := c.persist(updated); perr != nil {
				return perr
			}
		}
	}
	return nil
}

func (c *Claimer) persist(e model.Exit) error {
	return c.st.Update(func(st *store.State) { st.Exits[store.Key(e.Receiver)] = e })
}

// Step advances one exit and returns its new state. Errors mean "try again later" unless the calldata
// is refused, which is also returned as an error.
func (c *Claimer) Step(ctx context.Context, e model.Exit) (model.Exit, error) {
	receiver := common.HexToAddress(e.Receiver)
	salt := common.HexToHash(e.Salt)

	// 1. Find the Outbox message id.
	if e.MessageID == nil {
		id, err := c.locateMessage(ctx, e)
		if err != nil {
			return e, err
		}
		if id == nil {
			return e, nil
		}
		e.MessageID = id
	}

	// 2. Already delivered on HyperEVM? Idempotency across restarts.
	swept, err := c.isSwept(ctx, salt)
	if err != nil {
		return e, err
	}
	if swept {
		now := c.now().Unix()
		e.Stage = "delivered"
		e.DeliveredAt = &now
		return e, nil
	}

	// 3. Confirmation and claim calldata.
	data, err := c.ex.Claim(ctx, *e.MessageID)
	var calldata []byte
	switch {
	case errors.Is(err, explorer.ErrUnconfirmed):
		return e, nil
	case errors.Is(err, explorer.ErrNotFound):
		c.log.Warn("explorer does not know message", "message_id", *e.MessageID)
		return e, nil
	case errors.Is(err, explorer.ErrExecuted):
		calldata, err = c.recoverCalldata(ctx, receiver)
		if err != nil {
			c.log.Warn("claim already executed and calldata not recoverable", "receiver", e.Receiver, "err", err)
			return e, nil
		}
	case err != nil:
		return e, fmt.Errorf("claim %d: %w", *e.MessageID, err)
	default:
		calldata, err = c.verifyClaim(data, receiver)
		if err != nil {
			return e, err
		}
	}
	if e.Stage == "intent" {
		now := c.now().Unix()
		e.Stage = "confirmed"
		e.MaturesAt = &now
	}

	// 4. Claim and sweep.
	if c.tx == nil {
		return e, nil
	}
	params, err := ToABIExitParams(e.Params)
	if err != nil {
		return e, err
	}
	calldataSweep, err := abis.PackClaimAndSweep(calldata, params)
	if err != nil {
		return e, err
	}
	res, err := c.tx.Send(ctx, chain.TxRequest{
		To:   c.s.ExitFactory,
		Data: calldataSweep,
		OnSent: func(h common.Hash) {
			hash := h.Hex()
			e.Stage = "claimed"
			e.ClaimTx = &hash
			if perr := c.persist(e); perr != nil {
				c.log.Error("persist claimed state", "err", perr)
			}
		},
	})
	if err != nil {
		if errors.Is(err, chain.ErrSimulation) {
			// Not claimable yet, or already swept by someone else. Re-checked on the next pass.
			c.log.Info("claimAndSweep simulation failed, will retry", "receiver", e.Receiver, "err", err)
			if e.Stage == "claimed" {
				e.Stage = "confirmed"
			}
			return e, nil
		}
		if e.Stage == "claimed" {
			e.Stage = "confirmed"
		}
		return e, fmt.Errorf("claimAndSweep: %w", err)
	}
	if res.DryRun {
		c.log.Info("dry run: claimAndSweep simulated", "receiver", e.Receiver)
		return e, nil
	}
	now := c.now().Unix()
	hash := res.Hash.Hex()
	e.Stage = "delivered"
	e.ClaimTx = &hash
	e.DeliveredAt = &now
	c.log.Info("exit delivered", "receiver", e.Receiver, "claim_tx", hash)
	return e, nil
}

// verifyClaim checks the explorer calldata: the Outbox address must be the configured one, the
// selector must be executeTransaction, and the payout target must be the receiver.
func (c *Claimer) verifyClaim(data explorer.ClaimData, receiver common.Address) ([]byte, error) {
	if c.s.Outbox == (common.Address{}) {
		return nil, fmt.Errorf("%w: outbox not configured", ErrRefused)
	}
	if !strings.EqualFold(data.Outbox(), c.s.Outbox.Hex()) {
		return nil, fmt.Errorf("%w: outbox %s is not %s", ErrRefused, data.Outbox(), c.s.Outbox.Hex())
	}
	calldata, err := decodeHex(data.Calldata)
	if err != nil {
		return nil, fmt.Errorf("%w: calldata: %v", ErrRefused, err)
	}
	call, err := abis.DecodeOutboxCall(calldata)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	if call.To != receiver {
		return nil, fmt.Errorf("%w: payout target %s is not receiver %s", ErrRefused, call.To.Hex(), receiver.Hex())
	}
	return calldata, nil
}

// locateMessage finds the message id for an exit: withdrawals of the intent transaction whose
// destination is the receiver, with the L2ToL1Tx receipt log as a fallback.
func (c *Claimer) locateMessage(ctx context.Context, e model.Exit) (*int64, error) {
	receiver := common.HexToAddress(e.Receiver)
	items, err := c.ex.WithdrawalsByTx(ctx, e.ElysiumTx)
	if err != nil {
		return nil, fmt.Errorf("withdrawals %s: %w", e.ElysiumTx, err)
	}
	for _, it := range items {
		if strings.EqualFold(it.DestinationAddressHash, receiver.Hex()) {
			id := it.ID
			return &id, nil
		}
	}
	rc, err := c.el.TransactionReceipt(ctx, common.HexToHash(e.ElysiumTx))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("receipt: %w", err)
	}
	for _, l := range rc.Logs {
		if len(l.Topics) == 0 || l.Topics[0] != abis.TopicL2ToL1Tx {
			continue
		}
		w, err := abis.DecodeL2ToL1Tx(*l)
		if err != nil || w.Destination != receiver {
			continue
		}
		id := w.Position.Int64()
		return &id, nil
	}
	return nil, nil
}

// isSwept reads ExitFactory.isSwept(salt).
func (c *Claimer) isSwept(ctx context.Context, salt common.Hash) (bool, error) {
	data, err := abis.PackIsSwept(salt)
	if err != nil {
		return false, err
	}
	to := c.s.ExitFactory
	ret, err := c.hy.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		return false, fmt.Errorf("isSwept: %w", err)
	}
	return abis.UnpackBool(abis.ExitFactory, "isSwept", ret)
}

// recoverCalldata finds the Outbox claim for receiver that another party already executed. It scans
// recent OutBoxTransactionExecuted logs for the receiver and unwraps the transaction input.
func (c *Claimer) recoverCalldata(ctx context.Context, receiver common.Address) ([]byte, error) {
	head, err := c.hy.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	var from uint64
	if head > recoveryWindow {
		from = head - recoveryWindow
	}
	recvTopic := common.BytesToHash(receiver.Bytes())
	to := head
	for to >= from {
		start := from
		if to-from >= c.s.LogRange {
			start = to - c.s.LogRange + 1
		}
		logs, err := c.hy.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(start),
			ToBlock:   new(big.Int).SetUint64(to),
			Addresses: []common.Address{c.s.Outbox},
			Topics:    [][]common.Hash{{abis.TopicOutboxExecuted}, {recvTopic}},
		})
		if err != nil {
			return nil, err
		}
		for i := len(logs) - 1; i >= 0; i-- {
			tx, _, err := c.hy.TransactionByHash(ctx, logs[i].TxHash)
			if err != nil {
				continue
			}
			inner, err := abis.UnwrapOutboxCalldata(tx.Data())
			if err != nil {
				continue
			}
			call, err := abis.DecodeOutboxCall(inner)
			if err == nil && call.To == receiver {
				return inner, nil
			}
		}
		if start == 0 || start <= from {
			break
		}
		to = start - 1
	}
	return nil, ErrNoCalldata
}

// ToABIExitParams converts stored exit parameters to the ABI binding.
func ToABIExitParams(p model.ExitParams) (abis.ExitParams, error) {
	parse := func(name, s string) (*big.Int, error) {
		v, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return nil, fmt.Errorf("exit params %s: %q is not a decimal", name, s)
		}
		return v, nil
	}
	nonce, err := parse("nonce", p.Nonce)
	if err != nil {
		return abis.ExitParams{}, err
	}
	amount, err := parse("amount", p.Amount)
	if err != nil {
		return abis.ExitParams{}, err
	}
	tip, err := parse("claimTip", p.ClaimTip)
	if err != nil {
		return abis.ExitParams{}, err
	}
	return abis.ExitParams{
		ChainId:   new(big.Int).SetUint64(p.ChainID),
		Desk:      common.HexToAddress(p.Desk),
		Sender:    common.HexToAddress(p.Sender),
		Nonce:     nonce,
		Amount:    amount,
		ClaimTip:  tip,
		Recipient: common.HexToAddress(p.Recipient),
		ToCore:    p.ToCore,
	}, nil
}

func decodeHex(s string) ([]byte, error) {
	return hexutil.Decode(strings.TrimSpace(s))
}

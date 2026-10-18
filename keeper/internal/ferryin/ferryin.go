// Package ferryin tracks Ferry In tickets: a HyperEVM Inbox.createRetryableTicket that calls
// FerryIn.buy on Elysium. Status is derived from the HyperEVM transaction, the explorer's to-rollup
// message and the Elysium completion receipt (Bought or Refunded). Failed auto-redeems can be retried
// with ArbRetryableTx.redeem when the ticket id is known.
package ferryin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// Errors. Use errors.Is.
var (
	ErrNotFound    = errors.New("ferry in: transaction not found")
	ErrNotFerryIn  = errors.New("ferry in: transaction is not a Ferry In ticket")
	ErrTxFailed    = errors.New("ferry in: HyperEVM transaction failed, no ticket was created")
	ErrNotDeployed = errors.New("ferry in: FerryIn is not deployed on this keeper")
)

// arbRetryableTx is the ArbRetryableTx precompile on Elysium.
var arbRetryableTx = common.HexToAddress("0x000000000000000000000000000000000000006E")

const (
	stuckAfter   = 15 * time.Minute
	scanCooldown = 10 * time.Second
	scanMaxAge   = 24 * time.Hour
	scanMaxPages = 20
)

// ToRollup is the explorer subset used here.
type ToRollup interface {
	ToRollup(ctx context.Context, cursor *int64) (explorer.MessagesPage, error)
}

// Sender sends Elysium transactions (implemented by *chain.Sender). Nil means read-only.
type Sender interface {
	Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error)
}

// Settings configures the tracker.
type Settings struct {
	Inbox   common.Address
	FerryIn common.Address
}

// Tracker answers Ferry In status queries and retries failed redeems.
type Tracker struct {
	s   Settings
	hy  chain.Reader // HyperEVM: the ticket transaction and receipt
	el  chain.Reader // Elysium: the completion receipt
	ex  ToRollup
	tx  Sender // Elysium, for redeem; nil = read-only
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	lastScan map[string]time.Time // per tx: last to-rollup scan that did not find the message
	redeemMu sync.Mutex
}

// New creates a tracker.
func New(s Settings, hy, el chain.Reader, ex ToRollup, tx Sender, st *store.Store, log *slog.Logger) *Tracker {
	return &Tracker{s: s, hy: hy, el: el, ex: ex, tx: tx, st: st, log: log, now: time.Now, lastScan: map[string]time.Time{}}
}

// Enabled reports whether Ferry In is deployed.
func (t *Tracker) Enabled() bool {
	return t.s.FerryIn != (common.Address{}) && t.s.Inbox != (common.Address{})
}

// Status returns the current status of a Ferry In ticket. Final stages (bought, refunded) are cached.
func (t *Tracker) Status(ctx context.Context, txHash string) (model.FerryInStatus, error) {
	hash := common.HexToHash(txHash)
	key := store.Key(hash.Hex())
	prev, hasPrev := t.st.FerryIn(key)
	if hasPrev && (prev.Status.Stage == "bought" || prev.Status.Stage == "refunded") {
		return prev.Status, nil
	}

	tx, pending, err := t.hy.TransactionByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return model.FerryInStatus{}, ErrNotFound
		}
		return model.FerryInStatus{}, fmt.Errorf("hyperevm tx: %w", err)
	}
	if tx.To() == nil || *tx.To() != t.s.Inbox {
		return model.FerryInStatus{}, ErrNotFerryIn
	}
	ticket, err := abis.DecodeCreateRetryableTicket(tx.Data())
	if err != nil || ticket.To != t.s.FerryIn {
		return model.FerryInStatus{}, ErrNotFerryIn
	}
	token, recipient, _, err := abis.DecodeBuy(ticket.Data)
	if err != nil {
		return model.FerryInStatus{}, ErrNotFerryIn
	}

	base := model.FerryInStatus{
		HyperEVMTx: hash.Hex(),
		Stage:      "submitted",
		Recipient:  recipient.Hex(),
		HypeIn:     ticket.L2CallValue.String(),
		Token:      strPtr(token.Hex()),
		UpdatedAt:  t.now().Unix(),
	}
	if hasPrev {
		base.MessageID = prev.Status.MessageID
		base.ElysiumTx = prev.Status.ElysiumTx
		base.AmountOut = prev.Status.AmountOut
		if prev.Status.Stage == "redeemed" || prev.Status.Stage == "redeem_failed" {
			base.Stage = prev.Status.Stage
		}
	}
	if pending {
		return t.save(key, prev, hasPrev, base), nil
	}

	receipt, err := t.hy.TransactionReceipt(ctx, hash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return t.save(key, prev, hasPrev, base), nil
		}
		return model.FerryInStatus{}, fmt.Errorf("hyperevm receipt: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return model.FerryInStatus{}, ErrTxFailed
	}

	msg, err := t.findToRollup(ctx, hash)
	if err != nil {
		t.log.Warn("ferry in: explorer lookup failed", "tx", hash.Hex(), "err", err)
		return base, nil
	}
	if msg == nil {
		return t.save(key, prev, hasPrev, base), nil
	}
	id := msg.ID
	base.MessageID = &id
	if !(hasPrev && prev.Status.Stage == "redeemed") {
		base.Stage = "relayed"
	}

	if !msg.Completed() {
		if base.Stage != "redeemed" {
			if at, ok := msg.Time(); ok && t.now().Sub(at) > stuckAfter {
				base.Stage = "redeem_failed"
			}
		}
		return t.save(key, prev, hasPrev, base), nil
	}

	completion := *msg.CompletionTransactionHash
	base.ElysiumTx = &completion
	rc, err := t.el.TransactionReceipt(ctx, common.HexToHash(completion))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return t.save(key, prev, hasPrev, base), nil
		}
		return model.FerryInStatus{}, fmt.Errorf("elysium receipt: %w", err)
	}
	if rc.Status != types.ReceiptStatusSuccessful {
		base.Stage = "redeem_failed"
		return t.save(key, prev, hasPrev, base), nil
	}
	for _, l := range rc.Logs {
		if l.Address != t.s.FerryIn || len(l.Topics) == 0 {
			continue
		}
		switch l.Topics[0] {
		case abis.TopicBought:
			_, out, err := abis.DecodeBought(*l)
			if err != nil {
				continue
			}
			base.Stage = "bought"
			base.AmountOut = strPtr(out.String())
		case abis.TopicRefunded:
			base.Stage = "refunded"
			base.AmountOut = nil
		}
	}
	return t.save(key, prev, hasPrev, base), nil
}

// findToRollup looks for the to-rollup message created by hash. Negative scans are rate limited per tx.
func (t *Tracker) findToRollup(ctx context.Context, hash common.Hash) (*explorer.Message, error) {
	key := store.Key(hash.Hex())
	t.mu.Lock()
	if last, ok := t.lastScan[key]; ok && t.now().Sub(last) < scanCooldown {
		t.mu.Unlock()
		return nil, nil
	}
	t.mu.Unlock()

	now := t.now()
	var cursor *int64
	for page := 0; page < scanMaxPages; page++ {
		p, err := t.ex.ToRollup(ctx, cursor)
		if err != nil {
			return nil, err
		}
		stop := false
		for i := range p.Items {
			m := p.Items[i]
			if strings.EqualFold(m.OriginationTransactionHash, hash.Hex()) {
				return &m, nil
			}
			if at, ok := m.Time(); ok && now.Sub(at) > scanMaxAge {
				stop = true
				break
			}
		}
		if stop || p.NextPageParams == nil || len(p.Items) == 0 {
			break
		}
		next := p.NextPageParams.ID
		cursor = &next
	}
	t.mu.Lock()
	t.lastScan[key] = t.now()
	t.mu.Unlock()
	return nil, nil
}

// save writes the record when its observable state changed and returns the status.
func (t *Tracker) save(key string, prev store.FerryInRecord, hasPrev bool, next model.FerryInStatus) model.FerryInStatus {
	changed := !hasPrev || prev.Status.Stage != next.Stage || !sameInt64(prev.Status.MessageID, next.MessageID) ||
		!sameStr(prev.Status.ElysiumTx, next.ElysiumTx) || !sameStr(prev.Status.AmountOut, next.AmountOut)
	if !changed {
		return prev.Status
	}
	rec := prev
	rec.Status = next
	if err := t.st.Update(func(st *store.State) { st.FerryIn[key] = rec }); err != nil {
		t.log.Error("ferry in: persist", "tx", key, "err", err)
	}
	return next
}

// RedeemFailed retries ArbRetryableTx.redeem for tickets in redeem_failed whose ticket id is known.
// Best effort: it needs a ticket id in the record and a signing key.
func (t *Tracker) RedeemFailed(ctx context.Context) error {
	if t.tx == nil {
		return nil
	}
	t.redeemMu.Lock()
	defer t.redeemMu.Unlock()
	for _, rec := range t.st.FerryInRecords() {
		if rec.Status.Stage != "redeem_failed" || rec.TicketID == "" || rec.RedeemTx != "" {
			continue
		}
		data, err := abis.PackRedeem(common.HexToHash(rec.TicketID))
		if err != nil {
			return err
		}
		res, err := t.tx.Send(ctx, chain.TxRequest{To: arbRetryableTx, Data: data})
		if err != nil {
			t.log.Warn("ferry in: redeem failed", "tx", rec.Status.HyperEVMTx, "err", err)
			continue
		}
		if res.DryRun {
			continue
		}
		key := store.Key(rec.Status.HyperEVMTx)
		hash := res.Hash.Hex()
		next := rec
		next.RedeemTx = hash
		next.Status.Stage = "redeemed"
		next.Status.UpdatedAt = t.now().Unix()
		if err := t.st.Update(func(st *store.State) { st.FerryIn[key] = next }); err != nil {
			return err
		}
		t.log.Info("ferry in: redeem sent", "tx", rec.Status.HyperEVMTx, "redeem_tx", hash)
	}
	return nil
}

// SetTicketID records the retryable ticket id for a Ferry In record so RedeemFailed can act on it.
func (t *Tracker) SetTicketID(hyperevmTx, ticketID string) error {
	key := store.Key(hyperevmTx)
	return t.st.Update(func(st *store.State) {
		rec, ok := st.FerryIn[key]
		if !ok {
			rec = store.FerryInRecord{Status: model.FerryInStatus{HyperEVMTx: hyperevmTx, Stage: "submitted"}}
		}
		rec.TicketID = ticketID
		st.FerryIn[key] = rec
	})
}

func strPtr(s string) *string { return &s }

func sameInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

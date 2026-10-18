// Package chain holds the RPC connections (Elysium and HyperEVM) and the Sender that signs and
// submits transactions. Every transaction is eth_call-simulated first.
package chain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Reader is the subset of the JSON-RPC API the keeper uses. *ethclient.Client implements it.
type Reader interface {
	ChainID(ctx context.Context) (*big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error)
	TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
}

var _ Reader = (*ethclient.Client)(nil)

// Chains holds the two RPC connections. Dialing does not contact the node, so an unreachable RPC
// does not fail start-up; calls fail later and the caller decides.
type Chains struct {
	Elysium                 Reader
	HyperEVM                Reader
	ElysiumURL, HyperEVMURL string
}

// Dial opens both connections.
func Dial(ctx context.Context, elysiumRPC, hyperevmRPC string) (*Chains, error) {
	el, err := ethclient.DialContext(ctx, elysiumRPC)
	if err != nil {
		return nil, fmt.Errorf("dial elysium: %w", err)
	}
	hy, err := ethclient.DialContext(ctx, hyperevmRPC)
	if err != nil {
		return nil, fmt.Errorf("dial hyperevm: %w", err)
	}
	return &Chains{Elysium: el, HyperEVM: hy, ElysiumURL: elysiumRPC, HyperEVMURL: hyperevmRPC}, nil
}

// ErrSimulation is returned when the eth_call simulation of a transaction fails. Nothing was sent.
var ErrSimulation = errors.New("chain: simulation failed")

// ErrReverted is returned when a sent transaction is mined with a failed status.
var ErrReverted = errors.New("chain: transaction reverted")

// TxRequest describes one transaction the keeper wants to send.
type TxRequest struct {
	To    common.Address
	Data  []byte
	Value *big.Int // nil means zero
	// OnSent is called with the hash once the transaction is accepted by the RPC, before the
	// receipt is awaited. Callers use it to persist "sent" state.
	OnSent func(hash common.Hash)
}

// SendResult describes a completed send.
type SendResult struct {
	DryRun   bool // only simulated; nothing was sent
	Hash     common.Hash
	Receipt  *types.Receipt
	GasSpent *big.Int // gasUsed * effectiveGasPrice
}

// Sender signs and submits transactions with the keeper key on one chain. Sends are serialized
// (one nonce at a time). It tracks the total gas spent.
type Sender struct {
	backend   Reader
	chainID   *big.Int
	key       *ecdsa.PrivateKey
	addr      common.Address
	dryRun    bool
	log       *slog.Logger
	pollEvery time.Duration
	timeout   time.Duration

	mu       sync.Mutex
	gasSpent *big.Int
}

// SenderOption tunes a Sender.
type SenderOption func(*Sender)

// WithPollInterval sets the receipt polling interval (tests use a few milliseconds).
func WithPollInterval(d time.Duration) SenderOption { return func(s *Sender) { s.pollEvery = d } }

// WithReceiptTimeout sets how long a send waits for its receipt.
func WithReceiptTimeout(d time.Duration) SenderOption { return func(s *Sender) { s.timeout = d } }

// ParseKey parses a hex private key (with or without 0x).
func ParseKey(hexKey string) (*ecdsa.PrivateKey, error) {
	h := strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	return crypto.HexToECDSA(h)
}

// NewSender creates a sender for one chain. In dry-run mode Send simulates and returns DryRun.
func NewSender(backend Reader, chainID *big.Int, key *ecdsa.PrivateKey, dryRun bool, log *slog.Logger, opts ...SenderOption) *Sender {
	s := &Sender{
		backend:   backend,
		chainID:   chainID,
		key:       key,
		addr:      crypto.PubkeyToAddress(key.PublicKey),
		dryRun:    dryRun,
		log:       log,
		pollEvery: 2 * time.Second,
		timeout:   5 * time.Minute,
		gasSpent:  new(big.Int),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Address returns the keeper address.
func (s *Sender) Address() common.Address { return s.addr }

// DryRun reports whether the sender only simulates.
func (s *Sender) DryRun() bool { return s.dryRun }

// GasSpent returns the total gas cost paid so far in wei.
func (s *Sender) GasSpent() *big.Int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return new(big.Int).Set(s.gasSpent)
}

// Send simulates the call from the keeper address, then (unless dry-run) signs, submits and waits
// for the receipt. A failed simulation returns ErrSimulation and sends nothing.
func (s *Sender) Send(ctx context.Context, req TxRequest) (*SendResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value := req.Value
	if value == nil {
		value = new(big.Int)
	}
	to := req.To
	call := ethereum.CallMsg{From: s.addr, To: &to, Data: req.Data, Value: value}

	if _, err := s.backend.CallContract(ctx, call, nil); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSimulation, err)
	}
	if s.dryRun {
		return &SendResult{DryRun: true, GasSpent: new(big.Int)}, nil
	}

	gas, err := s.backend.EstimateGas(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("%w: estimate: %v", ErrSimulation, err)
	}
	gasLimit := gas + gas/5 + 10_000 // 20% headroom

	nonce, err := s.backend.PendingNonceAt(ctx, s.addr)
	if err != nil {
		return nil, fmt.Errorf("chain: nonce: %w", err)
	}

	hash, err := s.submit(ctx, req, value, gasLimit, nonce)
	if err != nil {
		return nil, err
	}
	if req.OnSent != nil {
		req.OnSent(hash)
	}
	s.log.Info("tx sent", "chain_id", s.chainID, "hash", hash.Hex(), "to", to.Hex())

	receipt, err := s.waitReceipt(ctx, hash)
	if err != nil {
		return &SendResult{Hash: hash}, err
	}
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	s.gasSpent.Add(s.gasSpent, gasCost)
	res := &SendResult{Hash: hash, Receipt: receipt, GasSpent: gasCost}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return res, fmt.Errorf("%w: %s", ErrReverted, hash.Hex())
	}
	return res, nil
}

// submit builds a transaction (EIP-1559 when the chain reports a base fee, legacy otherwise),
// signs it and sends it. If the node rejects the EIP-1559 type, it retries once as legacy.
func (s *Sender) submit(ctx context.Context, req TxRequest, value *big.Int, gasLimit, nonce uint64) (common.Hash, error) {
	to := req.To
	head, err := s.backend.HeaderByNumber(ctx, nil)
	if err == nil && head.BaseFee != nil {
		tip, tErr := s.backend.SuggestGasTipCap(ctx)
		if tErr != nil || tip == nil || tip.Sign() == 0 {
			tip = big.NewInt(1_000_000_000)
		}
		feeCap := new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)
		hash, sErr := s.sign(ctx, &types.DynamicFeeTx{
			ChainID: s.chainID, Nonce: nonce, GasTipCap: tip, GasFeeCap: feeCap,
			Gas: gasLimit, To: &to, Value: value, Data: req.Data,
		})
		if sErr == nil || !isTypeRejection(sErr) {
			return hash, sErr
		}
		s.log.Warn("node rejected EIP-1559 transaction, retrying as legacy", "err", sErr)
	}
	gp, err := s.backend.SuggestGasPrice(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("chain: gas price: %w", err)
	}
	return s.sign(ctx, &types.LegacyTx{
		Nonce: nonce, GasPrice: gp, Gas: gasLimit, To: &to, Value: value, Data: req.Data,
	})
}

func (s *Sender) sign(ctx context.Context, inner types.TxData) (common.Hash, error) {
	signed, err := types.SignNewTx(s.key, types.LatestSignerForChainID(s.chainID), inner)
	if err != nil {
		return common.Hash{}, fmt.Errorf("chain: sign: %w", err)
	}
	if err := s.backend.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, fmt.Errorf("chain: send: %w", err)
	}
	return signed.Hash(), nil
}

func isTypeRejection(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "transaction type") || strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "eip-1559") || strings.Contains(msg, "eip1559")
}

func (s *Sender) waitReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	deadline := time.Now().Add(s.timeout)
	for {
		r, err := s.backend.TransactionReceipt(ctx, hash)
		if err == nil && r != nil {
			return r, nil
		}
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			s.log.Debug("receipt poll", "hash", hash.Hex(), "err", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("chain: no receipt for %s after %s", hash.Hex(), s.timeout)
		}
		t := time.NewTimer(s.pollEvery)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

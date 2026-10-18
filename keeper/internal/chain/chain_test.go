package chain

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// fakeChain implements the Reader methods the Sender uses. Other methods panic through the nil embed.
type fakeChain struct {
	Reader
	mu        sync.Mutex
	callErr   error
	estErr    error
	rejectDyn bool
	revert    bool
	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
	chainID   *big.Int
	calls     int
}

func (f *fakeChain) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil, f.callErr
}

func (f *fakeChain) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	return 100_000, f.estErr
}

func (f *fakeChain) PendingNonceAt(ctx context.Context, a common.Address) (uint64, error) {
	return 7, nil
}

func (f *fakeChain) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(1), BaseFee: big.NewInt(1_000_000_000)}, nil
}

func (f *fakeChain) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return big.NewInt(100_000_000), nil
}

func (f *fakeChain) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(2_000_000_000), nil
}

func (f *fakeChain) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rejectDyn && tx.Type() == types.DynamicFeeTxType {
		return errors.New("transaction type not supported")
	}
	f.sent = append(f.sent, tx)
	if f.receipts == nil {
		f.receipts = map[common.Hash]*types.Receipt{}
	}
	status := uint64(types.ReceiptStatusSuccessful)
	if f.revert {
		status = types.ReceiptStatusFailed
	}
	f.receipts[tx.Hash()] = &types.Receipt{
		Status: status, GasUsed: 50_000, EffectiveGasPrice: big.NewInt(1_500_000_000), TxHash: tx.Hash(),
	}
	return nil
}

func (f *fakeChain) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.receipts[h]
	if !ok {
		return nil, ethereum.NotFound
	}
	return r, nil
}

func newSender(t *testing.T, f *fakeChain, dry bool) (*Sender, common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s := NewSender(f, big.NewInt(99801), key, dry, slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		WithPollInterval(time.Millisecond), WithReceiptTimeout(2*time.Second))
	return s, s.Address()
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

var target = common.HexToAddress("0x000000000000000000000000000000000000dEaD")

func TestSimulationFailureSendsNothing(t *testing.T) {
	f := &fakeChain{callErr: errors.New("execution reverted: AlreadySwept")}
	s, _ := newSender(t, f, false)
	_, err := s.Send(context.Background(), TxRequest{To: target, Data: []byte{1}})
	if !errors.Is(err, ErrSimulation) {
		t.Fatalf("err = %v, want ErrSimulation", err)
	}
	if len(f.sent) != 0 {
		t.Fatal("a failed simulation must not send")
	}
}

func TestDryRunSimulatesOnly(t *testing.T) {
	f := &fakeChain{}
	s, _ := newSender(t, f, true)
	res, err := s.Send(context.Background(), TxRequest{To: target, Data: []byte{1}})
	if err != nil || !res.DryRun {
		t.Fatalf("dry run: res=%+v err=%v", res, err)
	}
	if len(f.sent) != 0 || f.calls != 1 {
		t.Fatalf("dry run sent %d txs after %d simulations", len(f.sent), f.calls)
	}
}

func TestSendSignsRecoversSenderAndTracksGas(t *testing.T) {
	f := &fakeChain{}
	s, from := newSender(t, f, false)
	var sentHash common.Hash
	res, err := s.Send(context.Background(), TxRequest{
		To: target, Data: []byte{0xab}, Value: big.NewInt(5),
		OnSent: func(h common.Hash) { sentHash = h },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("sent %d txs", len(f.sent))
	}
	tx := f.sent[0]
	if sentHash != tx.Hash() || res.Hash != tx.Hash() {
		t.Fatal("OnSent and result hash must match the transaction")
	}
	signer := types.LatestSignerForChainID(big.NewInt(99801))
	got, err := types.Sender(signer, tx)
	if err != nil || got != from {
		t.Fatalf("recovered sender %s, want %s (%v)", got.Hex(), from.Hex(), err)
	}
	if tx.Nonce() != 7 || tx.To() == nil || *tx.To() != target || tx.Value().Int64() != 5 {
		t.Fatalf("tx fields wrong: nonce=%d", tx.Nonce())
	}
	if tx.Type() != types.DynamicFeeTxType {
		t.Fatalf("expected EIP-1559 tx, got type %d", tx.Type())
	}
	want := new(big.Int).Mul(big.NewInt(50_000), big.NewInt(1_500_000_000))
	if res.GasSpent.Cmp(want) != 0 || s.GasSpent().Cmp(want) != 0 {
		t.Fatalf("gas spent = %s / %s, want %s", res.GasSpent, s.GasSpent(), want)
	}
}

func TestRevertedReceiptIsAnError(t *testing.T) {
	f := &fakeChain{revert: true}
	s, _ := newSender(t, f, false)
	res, err := s.Send(context.Background(), TxRequest{To: target})
	if !errors.Is(err, ErrReverted) {
		t.Fatalf("err = %v, want ErrReverted", err)
	}
	if res == nil || res.Receipt == nil {
		t.Fatal("reverted send should still return the receipt")
	}
}

func TestLegacyFallbackWhenNodeRejectsDynamicFee(t *testing.T) {
	f := &fakeChain{rejectDyn: true}
	s, _ := newSender(t, f, false)
	if _, err := s.Send(context.Background(), TxRequest{To: target}); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].Type() != types.LegacyTxType {
		t.Fatalf("expected a legacy transaction after rejection, got type %d", f.sent[0].Type())
	}
}

func TestParseKey(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	hexKey := common.Bytes2Hex(crypto.FromECDSA(key))
	for _, in := range []string{hexKey, "0x" + hexKey} {
		got, err := ParseKey(in)
		if err != nil || crypto.PubkeyToAddress(got.PublicKey) != crypto.PubkeyToAddress(key.PublicKey) {
			t.Fatalf("ParseKey(%q) failed: %v", in, err)
		}
	}
	if _, err := ParseKey("0x1234"); err == nil {
		t.Fatal("short key must fail")
	}
}

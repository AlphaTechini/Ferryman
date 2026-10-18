package claimer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

var (
	outboxAddr = common.HexToAddress("0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060")
	factory    = common.HexToAddress("0x00000000000000000000000000000000000000fa")
	receiver   = common.HexToAddress("0x7777777777777777777777777777777777777777")
	attacker   = common.HexToAddress("0x6666666666666666666666666666666666666666")
	recipient  = common.HexToAddress("0x00000000000000000000000000000000000000b2")
)

type fakeExplorer struct {
	withdrawals []explorer.Withdrawal
	claim       explorer.ClaimData
	claimErr    error
}

func (f *fakeExplorer) WithdrawalsByTx(ctx context.Context, tx string) ([]explorer.Withdrawal, error) {
	return f.withdrawals, nil
}

func (f *fakeExplorer) Claim(ctx context.Context, id int64) (explorer.ClaimData, error) {
	return f.claim, f.claimErr
}

// fakeHyper answers isSwept (CallContract), BlockNumber and FilterLogs/TransactionByHash for recovery.
type fakeHyper struct {
	chain.Reader
	swept   bool
	head    uint64
	logs    []types.Log
	txInput []byte
}

func (f *fakeHyper) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	return abis.ExitFactory.Methods["isSwept"].Outputs.Pack(f.swept)
}

func (f *fakeHyper) BlockNumber(ctx context.Context) (uint64, error) { return f.head, nil }

func (f *fakeHyper) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return f.logs, nil
}

func (f *fakeHyper) TransactionByHash(ctx context.Context, h common.Hash) (*types.Transaction, bool, error) {
	return types.NewTx(&types.LegacyTx{Data: f.txInput}), false, nil
}

// fakeSender records sends. OnSent is invoked like the real Sender does.
type fakeSender struct {
	sends  []chain.TxRequest
	dryRun bool
	simErr bool
}

func (f *fakeSender) Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error) {
	f.sends = append(f.sends, req)
	if f.simErr {
		return nil, chain.ErrSimulation
	}
	if f.dryRun {
		return &chain.SendResult{DryRun: true}, nil
	}
	h := common.HexToHash("0xc1a1")
	if req.OnSent != nil {
		req.OnSent(h)
	}
	return &chain.SendResult{Hash: h, Receipt: &types.Receipt{Status: 1}, GasSpent: big.NewInt(1)}, nil
}

func innerCalldata(t *testing.T, to common.Address) []byte {
	t.Helper()
	b, err := abis.Outbox.Pack("executeTransaction", [][32]byte{}, big.NewInt(1), common.Address{}, to,
		big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(1_000), []byte{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func claimOK(t *testing.T, to, outbox common.Address) explorer.ClaimData {
	return explorer.ClaimData{Calldata: hexutil.Encode(innerCalldata(t, to)), OutboxAddressHash: outbox.Hex()}
}

func testExit(t *testing.T) model.Exit {
	t.Helper()
	p := abis.ExitParams{
		ChainId: big.NewInt(99801), Desk: common.HexToAddress("0xd1"), Sender: common.HexToAddress("0xa1"),
		Nonce: big.NewInt(0), Amount: big.NewInt(1_000_000), ClaimTip: big.NewInt(100), Recipient: recipient,
	}
	salt, err := abis.ExitSalt(p)
	if err != nil {
		t.Fatal(err)
	}
	return model.Exit{
		Receiver: receiver.Hex(), Salt: salt.Hex(), Stage: "intent", ElysiumTx: "0xtx1", StartedAt: 1000,
		Params: model.ExitParams{ChainID: 99801, Desk: "0x00000000000000000000000000000000000000d1",
			Sender: "0x00000000000000000000000000000000000000a1", Nonce: "0", Amount: "1000000",
			ClaimTip: "100", Recipient: recipient.Hex()},
	}
}

func newClaimer(t *testing.T, ex *fakeExplorer, hy *fakeHyper, tx *fakeSender, e model.Exit) (*Claimer, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/s.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *store.State) { s.Exits[store.Key(e.Receiver)] = e }); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var sender Sender
	if tx != nil {
		sender = tx
	}
	c := New(Settings{ExitFactory: factory, Outbox: outboxAddr, Interval: 1}, &fakeEl{}, hy, ex, sender, st, log)
	return c, st
}

type fakeEl struct{ chain.Reader }

func (fakeEl) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	return nil, ethereum.NotFound
}

func withdrawalFor(id int64) explorer.Withdrawal {
	return explorer.Withdrawal{ID: id, Status: "confirmed", DestinationAddressHash: receiver.Hex()}
}

func stored(t *testing.T, st *store.Store) model.Exit {
	t.Helper()
	e, ok := st.Exit(receiver.Hex())
	if !ok {
		t.Fatal("exit missing from store")
	}
	return e
}

func TestHappyPathDelivers(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, receiver, outboxAddr)}
	tx := &fakeSender{}
	c, st := newClaimer(t, ex, &fakeHyper{}, tx, e)

	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := stored(t, st)
	if got.Stage != "delivered" || got.MessageID == nil || *got.MessageID != 77 {
		t.Fatalf("stage=%s messageId=%v", got.Stage, got.MessageID)
	}
	if got.ClaimTx == nil || got.DeliveredAt == nil {
		t.Fatal("claimTx and deliveredAt must be set")
	}
	if len(tx.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(tx.sends))
	}
	req := tx.sends[0]
	if req.To != factory || !hasSelector(req.Data, abis.ExitFactory.Methods["claimAndSweep"].ID) {
		t.Fatal("must call ExitFactory.claimAndSweep")
	}
	inner, err := abis.UnwrapOutboxCalldata(req.Data)
	if err != nil {
		t.Fatal(err)
	}
	call, _ := abis.DecodeOutboxCall(inner)
	if call.To != receiver {
		t.Fatal("sweep must carry the receiver as payout target")
	}
}

func TestUnconfirmedWaits(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claimErr: explorer.ErrUnconfirmed}
	tx := &fakeSender{}
	c, st := newClaimer(t, ex, &fakeHyper{}, tx, e)
	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := stored(t, st)
	if got.Stage != "intent" || len(tx.sends) != 0 || got.MessageID == nil {
		t.Fatalf("stage=%s sends=%d", got.Stage, len(tx.sends))
	}
}

func TestAlreadySweptIsIdempotent(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, receiver, outboxAddr)}
	tx := &fakeSender{}
	c, st := newClaimer(t, ex, &fakeHyper{swept: true}, tx, e)
	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stored(t, st); got.Stage != "delivered" || len(tx.sends) != 0 {
		t.Fatalf("stage=%s sends=%d", got.Stage, len(tx.sends))
	}
}

func TestRefusesCalldataPayingAnotherAddress(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, attacker, outboxAddr)}
	tx := &fakeSender{}
	c, _ := newClaimer(t, ex, &fakeHyper{}, tx, e)
	if _, err := c.Step(context.Background(), e); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	if len(tx.sends) != 0 {
		t.Fatal("refused calldata must never be sent")
	}
}

func TestRefusesWrongOutboxAndSelector(t *testing.T) {
	e := testExit(t)
	tx := &fakeSender{}

	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, receiver, attacker)}
	c, _ := newClaimer(t, ex, &fakeHyper{}, tx, e)
	if _, err := c.Step(context.Background(), e); !errors.Is(err, ErrRefused) {
		t.Fatalf("wrong outbox: err = %v", err)
	}

	bad := explorer.ClaimData{Calldata: hexutil.Encode(append([]byte{0xde, 0xad, 0xbe, 0xef}, innerCalldata(t, receiver)[4:]...)), OutboxAddressHash: outboxAddr.Hex()}
	ex2 := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: bad}
	c2, _ := newClaimer(t, ex2, &fakeHyper{}, tx, e)
	if _, err := c2.Step(context.Background(), e); !errors.Is(err, ErrRefused) {
		t.Fatalf("wrong selector: err = %v", err)
	}
	if len(tx.sends) != 0 {
		t.Fatal("nothing may be sent for refused data")
	}
}

func TestSimulationFailureKeepsConfirmedAndRetries(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, receiver, outboxAddr)}
	tx := &fakeSender{simErr: true}
	c, st := newClaimer(t, ex, &fakeHyper{}, tx, e)
	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stored(t, st); got.Stage != "confirmed" || got.ClaimTx != nil {
		t.Fatalf("stage=%s claimTx=%v", got.Stage, got.ClaimTx)
	}
}

func TestDryRunAndReadOnlyDoNotSend(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claim: claimOK(t, receiver, outboxAddr)}

	tx := &fakeSender{dryRun: true}
	c, st := newClaimer(t, ex, &fakeHyper{}, tx, e)
	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stored(t, st); got.Stage != "confirmed" {
		t.Fatalf("dry run stage=%s", got.Stage)
	}

	c2, st2 := newClaimer(t, ex, &fakeHyper{}, nil, e)
	if err := c2.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stored(t, st2); got.Stage != "confirmed" || got.MessageID == nil {
		t.Fatalf("read-only stage=%s", got.Stage)
	}
}

func TestAlreadyExecutedRecoversCalldataFromChain(t *testing.T) {
	e := testExit(t)
	ex := &fakeExplorer{withdrawals: []explorer.Withdrawal{withdrawalFor(77)}, claimErr: explorer.ErrExecuted}
	inner := innerCalldata(t, receiver)
	topic := common.BytesToHash(receiver.Bytes())
	hy := &fakeHyper{
		head:    500,
		logs:    []types.Log{{Topics: []common.Hash{abis.TopicOutboxExecuted, topic}, TxHash: common.HexToHash("0x9")}},
		txInput: inner,
	}
	tx := &fakeSender{}
	c, st := newClaimer(t, ex, hy, tx, e)
	if err := c.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stored(t, st); got.Stage != "delivered" {
		t.Fatalf("stage=%s, want delivered via recovered calldata", got.Stage)
	}
	if len(tx.sends) != 1 {
		t.Fatalf("sends=%d", len(tx.sends))
	}
}

func hasSelector(data []byte, sel []byte) bool {
	return len(data) >= 4 && string(data[:4]) == string(sel)
}

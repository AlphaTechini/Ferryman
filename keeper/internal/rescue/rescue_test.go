package rescue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"
	"time"

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
	outbox   = common.HexToAddress("0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060")
	owner    = common.HexToAddress("0x00000000000000000000000000000000000000c1")
	attacker = common.HexToAddress("0x00000000000000000000000000000000000000ee")
	fixedNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
)

type fakeEx struct {
	mu          sync.Mutex
	pages       []explorer.MessagesPage // page i is returned for cursor pages[i-1].next (or nil for i=0)
	withdrawals map[string][]explorer.Withdrawal
	claims      map[int64]claimResp
	claimCalls  int
	fromCalls   int
}

type claimResp struct {
	data explorer.ClaimData
	err  error
}

func (f *fakeEx) FromRollup(ctx context.Context, cursor *int64) (explorer.MessagesPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fromCalls++
	idx := 0
	if cursor != nil {
		idx = -1
		for i, p := range f.pages {
			if p.NextPageParams != nil && p.NextPageParams.ID == *cursor {
				idx = i + 1
			}
		}
		if idx < 0 || idx >= len(f.pages) {
			return explorer.MessagesPage{}, nil
		}
	}
	if idx >= len(f.pages) {
		return explorer.MessagesPage{}, nil
	}
	return f.pages[idx], nil
}

func (f *fakeEx) WithdrawalsByTx(ctx context.Context, tx string) ([]explorer.Withdrawal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.withdrawals[tx], nil
}

func (f *fakeEx) Claim(ctx context.Context, id int64) (explorer.ClaimData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	c, ok := f.claims[id]
	if !ok {
		return explorer.ClaimData{}, explorer.ErrNotFound
	}
	return c.data, c.err
}

type fakeTx struct {
	mu    sync.Mutex
	sends []chain.TxRequest
}

func (f *fakeTx) Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, req)
	h := common.HexToHash(fmt.Sprintf("0x%064x", len(f.sends)))
	if req.OnSent != nil {
		req.OnSent(h)
	}
	return &chain.SendResult{Hash: h, Receipt: &types.Receipt{Status: 1}, GasSpent: big.NewInt(1000)}, nil
}

func calldataTo(t *testing.T, to common.Address, value int64) (string, []byte) {
	t.Helper()
	b, err := abis.Outbox.Pack("executeTransaction", [][32]byte{}, big.NewInt(1), common.Address{}, to,
		big.NewInt(1), big.NewInt(1), big.NewInt(1), big.NewInt(value), []byte{})
	if err != nil {
		t.Fatal(err)
	}
	return hexutil.Encode(b), b
}

func msg(id int64, origin string, status string, ageHours float64, completed bool) explorer.Message {
	ts := fixedNow.Add(-time.Duration(ageHours * float64(time.Hour))).Format("2006-01-02T15:04:05.000000Z")
	m := explorer.Message{
		ID: id, OriginationAddressHash: attacker.Hex(), OriginationTransactionHash: origin,
		OriginationTimestamp: &ts, Status: status,
	}
	if completed {
		c := "0xdone"
		m.CompletionTransactionHash = &c
	}
	return m
}

func newRescuer(t *testing.T, ex *fakeEx, tx *fakeTx, maxPer int) (*Rescuer, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/s.json")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var sender Sender
	if tx != nil {
		sender = tx
	}
	r := New(Settings{Enabled: true, Outbox: outbox, MinAge: 24 * time.Hour, Lookback: 30 * 24 * time.Hour,
		MaxPerRun: maxPer, PageLimit: 20}, ex, sender, st, log)
	r.now = func() time.Time { return fixedNow }
	return r, st
}

func TestRunClaimsEligibleToOriginalDestination(t *testing.T) {
	raw, _ := calldataTo(t, owner, 1000)
	ex := &fakeEx{
		pages:       []explorer.MessagesPage{{Items: []explorer.Message{msg(573261, "0xaaaa", "confirmed", 48, false)}}},
		withdrawals: map[string][]explorer.Withdrawal{"0xaaaa": {{ID: 573261, DestinationAddressHash: owner.Hex(), Callvalue: "1000", Status: "confirmed"}}},
		claims:      map[int64]claimResp{573261: {data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}},
	}
	tx := &fakeTx{}
	r, st := newRescuer(t, ex, tx, 10)
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tx.sends) != 1 || tx.sends[0].To != outbox || hexutil.Encode(tx.sends[0].Data) != raw {
		t.Fatal("calldata must be sent unchanged to the configured outbox")
	}
	stats := st.Rescue()
	if stats.Withdrawals != 1 || stats.Owners != 1 || stats.HypeReturned != "1000" || stats.GasSpent != "1000" {
		t.Fatalf("stats wrong: %+v", stats)
	}
	if len(stats.Recent) != 1 || stats.Recent[0].Owner != owner.Hex() || stats.Recent[0].Owner == attacker.Hex() {
		t.Fatalf("owner must be the withdrawal destination, not the L2 caller: %+v", stats.Recent)
	}
	if _, ok := st.Processed(store.MessageKey(573261)); !ok {
		t.Fatal("message must be marked processed")
	}
}

func TestSelectionFilter(t *testing.T) {
	raw, _ := calldataTo(t, owner, 1)
	ok := claimResp{data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}
	ex := &fakeEx{
		pages: []explorer.MessagesPage{{Items: []explorer.Message{
			msg(1, "0x01", "relayed", 48, true),    // relayed: skip
			msg(2, "0x02", "sent", 48, false),      // not confirmed: skip
			msg(3, "0x03", "confirmed", 48, true),  // completed: skip
			msg(4, "0x04", "confirmed", 1, false),  // too young: skip
			msg(5, "0x05", "confirmed", 48, false), // eligible but ours (exit below)
			msg(6, "0x06", "confirmed", 48, false), // eligible but already processed
			msg(7, "0x07", "confirmed", 48, false), // eligible: claimed
		}}},
		withdrawals: map[string][]explorer.Withdrawal{
			"0x07": {{ID: 7, DestinationAddressHash: owner.Hex(), Callvalue: "1", Status: "confirmed"}},
			"0x06": {{ID: 6, DestinationAddressHash: owner.Hex(), Callvalue: "1", Status: "confirmed"}},
			"0x05": {{ID: 5, DestinationAddressHash: owner.Hex(), Callvalue: "1", Status: "confirmed"}},
		},
		claims: map[int64]claimResp{5: ok, 6: ok, 7: ok},
	}
	tx := &fakeTx{}
	r, st := newRescuer(t, ex, tx, 10)
	_ = st.Update(func(s *store.State) {
		s.Exits["0xe"] = model.Exit{Receiver: "0xe", ElysiumTx: "0x05", Stage: "confirmed"}
		s.Processed[store.MessageKey(6)] = store.Processed{Outcome: "claimed"}
	})
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tx.sends) != 1 {
		t.Fatalf("sends = %d, want exactly message 7", len(tx.sends))
	}
	if ex.claimCalls != 1 {
		t.Fatalf("claim endpoint called %d times, want 1", ex.claimCalls)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	raw, _ := calldataTo(t, owner, 5)
	ex := &fakeEx{
		pages:       []explorer.MessagesPage{{Items: []explorer.Message{msg(9, "0x09", "confirmed", 50, false)}}},
		withdrawals: map[string][]explorer.Withdrawal{"0x09": {{ID: 9, DestinationAddressHash: owner.Hex(), Callvalue: "5"}}},
		claims:      map[int64]claimResp{9: {data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}},
	}
	tx := &fakeTx{}
	r, _ := newRescuer(t, ex, tx, 10)
	for i := 0; i < 3; i++ {
		if err := r.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(tx.sends) != 1 {
		t.Fatalf("sends = %d after 3 runs, want 1", len(tx.sends))
	}
}

func TestRefusesCalldataNotPayingDestination(t *testing.T) {
	raw, _ := calldataTo(t, attacker, 5)
	ex := &fakeEx{
		pages:       []explorer.MessagesPage{{Items: []explorer.Message{msg(10, "0x10", "confirmed", 50, false)}}},
		withdrawals: map[string][]explorer.Withdrawal{"0x10": {{ID: 10, DestinationAddressHash: owner.Hex(), Callvalue: "5"}}},
		claims:      map[int64]claimResp{10: {data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}},
	}
	tx := &fakeTx{}
	r, st := newRescuer(t, ex, tx, 10)
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tx.sends) != 0 {
		t.Fatal("calldata paying someone else must not be sent")
	}
	if p, ok := st.Processed(store.MessageKey(10)); !ok || p.Outcome != "refused" {
		t.Fatalf("refused message should be recorded: %+v %v", p, ok)
	}
}

func TestRefusesWrongOutbox(t *testing.T) {
	raw, _ := calldataTo(t, owner, 5)
	ex := &fakeEx{
		pages:       []explorer.MessagesPage{{Items: []explorer.Message{msg(11, "0x11", "confirmed", 50, false)}}},
		withdrawals: map[string][]explorer.Withdrawal{"0x11": {{ID: 11, DestinationAddressHash: owner.Hex(), Callvalue: "5"}}},
		claims:      map[int64]claimResp{11: {data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: attacker.Hex()}}},
	}
	tx := &fakeTx{}
	r, _ := newRescuer(t, ex, tx, 10)
	_ = r.RunOnce(context.Background())
	if len(tx.sends) != 0 {
		t.Fatal("an outbox other than the configured one must never be called")
	}
}

func TestUnconfirmedAndSimulationFailureAreRetried(t *testing.T) {
	raw, _ := calldataTo(t, owner, 5)
	ex := &fakeEx{
		pages:       []explorer.MessagesPage{{Items: []explorer.Message{msg(12, "0x12", "confirmed", 50, false)}}},
		withdrawals: map[string][]explorer.Withdrawal{"0x12": {{ID: 12, DestinationAddressHash: owner.Hex(), Callvalue: "5"}}},
		claims:      map[int64]claimResp{12: {err: explorer.ErrUnconfirmed}},
	}
	tx := &fakeTx{}
	r, st := newRescuer(t, ex, tx, 10)
	_ = r.RunOnce(context.Background())
	if _, ok := st.Processed(store.MessageKey(12)); ok || len(tx.sends) != 0 {
		t.Fatal("unconfirmed must stay unprocessed and unsent")
	}
	ex.claims[12] = claimResp{data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}
	_ = r.RunOnce(context.Background())
	if len(tx.sends) != 1 {
		t.Fatalf("expected a retry to send, sends=%d", len(tx.sends))
	}
}

func TestMaxPerRunCap(t *testing.T) {
	raw, _ := calldataTo(t, owner, 5)
	var items []explorer.Message
	wd := map[string][]explorer.Withdrawal{}
	claims := map[int64]claimResp{}
	for i := 0; i < 5; i++ {
		id := int64(200 + i)
		h := fmt.Sprintf("0x%02x", i)
		items = append(items, msg(id, h, "confirmed", 50, false))
		wd[h] = []explorer.Withdrawal{{ID: id, DestinationAddressHash: owner.Hex(), Callvalue: "5"}}
		claims[id] = claimResp{data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}}
	}
	ex := &fakeEx{pages: []explorer.MessagesPage{{Items: items}}, withdrawals: wd, claims: claims}
	tx := &fakeTx{}
	r, _ := newRescuer(t, ex, tx, 2)
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(tx.sends) != 2 {
		t.Fatalf("sends = %d, want 2 (RESCUE_MAX_PER_RUN)", len(tx.sends))
	}
}

func TestLookupFiltersByDestinationAndCaches(t *testing.T) {
	raw, _ := calldataTo(t, owner, 5)
	_ = raw
	ex := &fakeEx{
		pages: []explorer.MessagesPage{{Items: []explorer.Message{
			msg(20, "0x20", "confirmed", 50, false),
			msg(21, "0x21", "sent", 2, false),
			msg(22, "0x22", "relayed", 50, true),
		}}},
		withdrawals: map[string][]explorer.Withdrawal{
			"0x20": {{ID: 20, DestinationAddressHash: owner.Hex(), Callvalue: "7"}},
			"0x21": {{ID: 21, DestinationAddressHash: owner.Hex(), Callvalue: "9"}},
			"0x22": {{ID: 22, DestinationAddressHash: owner.Hex(), Callvalue: "9"}},
		},
	}
	r, _ := newRescuer(t, ex, &fakeTx{}, 10)
	got, err := r.Lookup(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 stuck items (sent+confirmed), got %+v", got)
	}
	if got[0].MessageID != 20 || !got[0].Claimable || got[0].Amount != "7" || got[1].Claimable {
		t.Fatalf("lookup items wrong: %+v", got)
	}
	none, _ := r.Lookup(context.Background(), attacker)
	if len(none) != 0 || none == nil {
		t.Fatal("no matches must be an empty, non-nil list")
	}
	calls := ex.fromCalls
	_, _ = r.Lookup(context.Background(), owner)
	if ex.fromCalls != calls {
		t.Fatal("from-rollup scan must be cached for the TTL")
	}
}

func TestClaimForMeStatuses(t *testing.T) {
	raw, _ := calldataTo(t, owner, 42)
	ex := &fakeEx{claims: map[int64]claimResp{
		30: {data: explorer.ClaimData{Calldata: raw, OutboxAddressHash: outbox.Hex()}},
		31: {err: explorer.ErrUnconfirmed},
		32: {err: explorer.ErrExecuted},
	}}
	tx := &fakeTx{}
	r, st := newRescuer(t, ex, tx, 10)

	res, err := r.ClaimForMe(context.Background(), 30)
	if err != nil || res.Status != StatusClaimed || res.TxHash == nil {
		t.Fatalf("claimed: %+v %v", res, err)
	}
	if st.Rescue().Withdrawals != 1 {
		t.Fatal("claim-for-me must update the counter")
	}
	for _, id := range []int64{31, 32, 99} {
		res, err := r.ClaimForMe(context.Background(), id)
		if err != nil || res.Status != StatusNotClaimable || res.TxHash != nil {
			t.Fatalf("id %d: %+v %v", id, res, err)
		}
	}

	ro, _ := newRescuer(t, ex, nil, 10)
	if _, err := ro.ClaimForMe(context.Background(), 30); !errors.Is(err, ErrDisabled) {
		t.Fatalf("read-only claim-for-me: err=%v, want ErrDisabled", err)
	}
}

func TestStatsStartEmpty(t *testing.T) {
	r, _ := newRescuer(t, &fakeEx{}, nil, 10)
	s := r.Stats()
	if s.Withdrawals != 0 || s.Owners != 0 || s.HypeReturned != "0" || s.Recent == nil && len(s.Recent) != 0 {
		t.Fatalf("stats: %+v", s)
	}
}

package store

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphatechini/ferryman/keeper/internal/model"
)

func TestRoundTripSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tok := model.Token{Address: "0x7D29d8047B905000459c0E80c34a26CeedcB47b2", Symbol: "FERRY", LaunchedAt: 100, LaunchBlock: 5}
	tip := int64(123)
	exit := model.Exit{Receiver: "0x0000000000000000000000000000000000000ABC", Stage: "confirmed", StartedAt: 50, MaturesAt: &tip}
	if err := s.Update(func(st *State) {
		st.Tokens[Key(tok.Address)] = tok
		st.Exits[Key(exit.Receiver)] = exit
		st.Cursors["elysium.pools"] = 4242
		st.Registered[Key(tok.Address)] = true
	}); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := again.Token(tok.Address)
	if !ok || got.Symbol != "FERRY" || got.LaunchBlock != 5 {
		t.Fatalf("token lost: %+v %v", got, ok)
	}
	e, ok := again.Exit(exit.Receiver)
	if !ok || e.MaturesAt == nil || *e.MaturesAt != 123 {
		t.Fatalf("exit lost: %+v", e)
	}
	if c, ok := again.Cursor("elysium.pools"); !ok || c != 4242 {
		t.Fatalf("cursor lost: %d %v", c, ok)
	}
	if !again.IsRegistered(tok.Address) {
		t.Fatal("registration flag lost")
	}
	if _, ok := again.Cursor("missing"); ok {
		t.Fatal("unknown cursor must report ok=false")
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Update(func(st *State) { st.Cursors["x"] = uint64(i) }); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("unexpected files after atomic writes: %v", names)
	}
}

func TestCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("corrupt state must not be silently replaced")
	}
}

func TestRecordRescueAccumulatesAndCapsRecent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	owners := []string{"0x01", "0x02", "0x01"}
	for i, owner := range owners {
		rec := model.RescueRecent{MessageID: int64(100 + i), Owner: owner, Amount: "1000", ClaimTx: "0xtx", ClaimedAt: 9}
		if err := s.RecordRescue(rec, owner, big.NewInt(10), 9); err != nil {
			t.Fatal(err)
		}
	}
	st := s.Rescue()
	if st.Withdrawals != 3 || st.Owners != 2 || st.HypeReturned != "3000" || st.GasSpent != "30" {
		t.Fatalf("stats wrong: %+v", st)
	}
	if len(st.Recent) != 3 || st.Recent[0].MessageID != 102 {
		t.Fatalf("recent must be newest first: %+v", st.Recent)
	}
	for i := 0; i < RecentLimit+5; i++ {
		_ = s.RecordRescue(model.RescueRecent{MessageID: int64(1000 + i), Owner: "0x9", Amount: "1"}, "0x9", big.NewInt(0), 1)
	}
	if n := len(s.Rescue().Recent); n != RecentLimit {
		t.Fatalf("recent list length %d, want %d", n, RecentLimit)
	}
	if _, ok := s.Processed(MessageKey(102)); !ok {
		t.Fatal("rescued message must be marked processed")
	}
}

// Package store persists keeper state in one JSON file. Writes go to a temp file that is fsynced and
// renamed over the old file, so a crash leaves either the old or the new state, never a torn file.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alphatechini/ferryman/keeper/internal/model"
)

// Current state format version.
const Version = 1

// FerryInRecord is a tracked Ferry In ticket plus keeper-only bookkeeping.
type FerryInRecord struct {
	Status   model.FerryInStatus `json:"status"`
	TicketID string              `json:"ticketId,omitempty"`
	RedeemTx string              `json:"redeemTx,omitempty"`
}

// Processed records that a message id (or other key) was handled, so restarts skip it.
type Processed struct {
	Outcome string `json:"outcome"`
	Tx      string `json:"tx,omitempty"`
	At      int64  `json:"at"`
}

// RescueState is the persisted Rescue counter.
type RescueState struct {
	Withdrawals  int                  `json:"withdrawals"`
	Owners       map[string]bool      `json:"owners"`
	HypeReturned string               `json:"hypeReturned"`
	GasSpent     string               `json:"gasSpent"`
	LastRunAt    int64                `json:"lastRunAt"`
	Recent       []model.RescueRecent `json:"recent"`
}

// State is the whole persisted document. Map keys are lowercase addresses or hashes.
type State struct {
	Version    int                      `json:"version"`
	Tokens     map[string]model.Token   `json:"tokens"`
	Registered map[string]bool          `json:"registered"`
	Exits      map[string]model.Exit    `json:"exits"`
	FerryIn    map[string]FerryInRecord `json:"ferryIn"`
	Rescue     RescueState              `json:"rescue"`
	Processed  map[string]Processed     `json:"processed"`
	Cursors    map[string]uint64        `json:"cursors"`
}

// RecentLimit is how many Rescue claims the recent list keeps.
const RecentLimit = 50

// Store is the concurrency-safe state holder.
type Store struct {
	path string
	mu   sync.RWMutex
	st   State
}

// Key normalises an address or hash for use as a map key.
func Key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Open loads the state file, or starts empty when it does not exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	s.st = newState()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &s.st); err != nil {
		return nil, fmt.Errorf("store: parse %s: %w", path, err)
	}
	normalize(&s.st)
	return s, nil
}

func newState() State {
	return State{
		Version:    Version,
		Tokens:     map[string]model.Token{},
		Registered: map[string]bool{},
		Exits:      map[string]model.Exit{},
		FerryIn:    map[string]FerryInRecord{},
		Rescue:     RescueState{Owners: map[string]bool{}, HypeReturned: "0", GasSpent: "0"},
		Processed:  map[string]Processed{},
		Cursors:    map[string]uint64{},
	}
}

func normalize(st *State) {
	d := newState()
	if st.Tokens == nil {
		st.Tokens = d.Tokens
	}
	if st.Registered == nil {
		st.Registered = d.Registered
	}
	if st.Exits == nil {
		st.Exits = d.Exits
	}
	if st.FerryIn == nil {
		st.FerryIn = d.FerryIn
	}
	if st.Processed == nil {
		st.Processed = d.Processed
	}
	if st.Cursors == nil {
		st.Cursors = d.Cursors
	}
	if st.Rescue.Owners == nil {
		st.Rescue.Owners = map[string]bool{}
	}
	if st.Rescue.HypeReturned == "" {
		st.Rescue.HypeReturned = "0"
	}
	if st.Rescue.GasSpent == "" {
		st.Rescue.GasSpent = "0"
	}
	st.Version = Version
}

// Path returns the state file path.
func (s *Store) Path() string { return s.path }

// View runs fn under a read lock. fn must not retain maps or modify the state.
func (s *Store) View(fn func(st *State)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(&s.st)
}

// Update runs fn under the write lock and then persists the state atomically. fn may mutate the state.
func (s *Store) Update(fn func(st *State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.st)
	return s.saveLocked()
}

// Save persists the current state.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(&s.st, "", "  ")
	if err != nil {
		return fmt.Errorf("store: encode: %w", err)
	}
	return WriteFileAtomic(s.path, b, 0o600)
}

// WriteFileAtomic writes data to a temp file in the same directory, fsyncs it and renames it over path.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("store: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("store: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("store: fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("store: close temp: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return fmt.Errorf("store: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("store: rename: %w", err)
	}
	return nil
}

// ---- typed accessors (copies, safe to use after the call) ----

// Tokens returns all tokens, newest launch first.
func (s *Store) Tokens() []model.Token {
	var out []model.Token
	s.View(func(st *State) {
		out = make([]model.Token, 0, len(st.Tokens))
		for _, t := range st.Tokens {
			out = append(out, t)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].LaunchedAt != out[j].LaunchedAt {
			return out[i].LaunchedAt > out[j].LaunchedAt
		}
		return out[i].LaunchBlock > out[j].LaunchBlock
	})
	return out
}

// Token returns one token by address.
func (s *Store) Token(addr string) (model.Token, bool) {
	var t model.Token
	var ok bool
	s.View(func(st *State) { t, ok = st.Tokens[Key(addr)] })
	return t, ok
}

// IsRegistered reports whether FerryIn has a pool key for the token (as recorded by the keeper).
func (s *Store) IsRegistered(addr string) bool {
	var ok bool
	s.View(func(st *State) { ok = st.Registered[Key(addr)] })
	return ok
}

// Exit returns the exit keyed by its HyperEVM receiver.
func (s *Store) Exit(receiver string) (model.Exit, bool) {
	var e model.Exit
	var ok bool
	s.View(func(st *State) { e, ok = st.Exits[Key(receiver)] })
	return e, ok
}

// Exits returns all exits, newest first.
func (s *Store) Exits() []model.Exit {
	var out []model.Exit
	s.View(func(st *State) {
		out = make([]model.Exit, 0, len(st.Exits))
		for _, e := range st.Exits {
			out = append(out, e)
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

// ExitByElysiumTx returns the exit whose intent transaction is tx.
func (s *Store) ExitByElysiumTx(tx string) (model.Exit, bool) {
	var e model.Exit
	var ok bool
	s.View(func(st *State) {
		for _, x := range st.Exits {
			if Key(x.ElysiumTx) == Key(tx) {
				e, ok = x, true
				return
			}
		}
	})
	return e, ok
}

// FerryIn returns a Ferry In record by HyperEVM transaction hash.
func (s *Store) FerryIn(tx string) (FerryInRecord, bool) {
	var r FerryInRecord
	var ok bool
	s.View(func(st *State) { r, ok = st.FerryIn[Key(tx)] })
	return r, ok
}

// FerryInRecords returns all Ferry In records.
func (s *Store) FerryInRecords() []FerryInRecord {
	var out []FerryInRecord
	s.View(func(st *State) {
		for _, r := range st.FerryIn {
			out = append(out, r)
		}
	})
	return out
}

// Processed returns the processed record for a key such as "msg:573261".
func (s *Store) Processed(key string) (Processed, bool) {
	var p Processed
	var ok bool
	s.View(func(st *State) { p, ok = st.Processed[key] })
	return p, ok
}

// Cursor returns the last scanned block of a stream. ok is false when the stream was never scanned.
func (s *Store) Cursor(name string) (uint64, bool) {
	var v uint64
	var ok bool
	s.View(func(st *State) { v, ok = st.Cursors[name] })
	return v, ok
}

// Rescue returns the public Rescue counter.
func (s *Store) Rescue() model.RescueStats {
	var out model.RescueStats
	s.View(func(st *State) {
		out = model.RescueStats{
			Withdrawals:  st.Rescue.Withdrawals,
			Owners:       len(st.Rescue.Owners),
			HypeReturned: st.Rescue.HypeReturned,
			GasSpent:     st.Rescue.GasSpent,
			LastRunAt:    st.Rescue.LastRunAt,
			Recent:       append([]model.RescueRecent{}, st.Rescue.Recent...),
		}
	})
	return out
}

// RecordRescue adds one successful Rescue claim to the counter and the recent list, marks the message
// processed, and persists. gasSpent is the gas cost of this claim in wei.
func (s *Store) RecordRescue(rec model.RescueRecent, owner string, gasSpent *big.Int, now int64) error {
	return s.Update(func(st *State) {
		st.Rescue.Withdrawals++
		st.Rescue.Owners[Key(owner)] = true
		st.Rescue.HypeReturned = addDecimal(st.Rescue.HypeReturned, rec.Amount)
		st.Rescue.GasSpent = addDecimal(st.Rescue.GasSpent, gasSpent.String())
		st.Rescue.LastRunAt = now
		st.Rescue.Recent = append([]model.RescueRecent{rec}, st.Rescue.Recent...)
		if len(st.Rescue.Recent) > RecentLimit {
			st.Rescue.Recent = st.Rescue.Recent[:RecentLimit]
		}
		st.Processed[MessageKey(rec.MessageID)] = Processed{Outcome: "claimed", Tx: rec.ClaimTx, At: now}
	})
}

// MessageKey is the processed-set key for an Outbox message id.
func MessageKey(id int64) string { return fmt.Sprintf("msg:%d", id) }

func addDecimal(a, b string) string {
	x, ok1 := new(big.Int).SetString(a, 10)
	y, ok2 := new(big.Int).SetString(b, 10)
	if !ok1 {
		x = new(big.Int)
	}
	if !ok2 {
		y = new(big.Int)
	}
	return new(big.Int).Add(x, y).String()
}

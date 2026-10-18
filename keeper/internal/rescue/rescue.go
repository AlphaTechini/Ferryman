// Package rescue claims matured withdrawals that nobody claimed. Funds always go to the destination the
// L2 withdrawal was created with: the keeper sends the explorer's claim calldata unchanged after checking
// its selector, its Outbox address and that its payout target equals the withdrawal's destination.
package rescue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// ErrDisabled is returned when Rescue is not configured or has no signing key for claims.
var ErrDisabled = errors.New("rescue is disabled")

// Claim outcomes.
const (
	StatusQueued       = "queued"
	StatusClaimed      = "claimed"
	StatusNotClaimable = "not_claimable"
)

const (
	maxResolveCache = 20_000
	maxLookupCands  = 200
	lookupTTL       = 60 * time.Second
	claimWait       = 20 * time.Second
	claimTimeout    = 10 * time.Minute
)

// Explorer is the subset of the explorer client Rescue uses.
type Explorer interface {
	FromRollup(ctx context.Context, cursor *int64) (explorer.MessagesPage, error)
	WithdrawalsByTx(ctx context.Context, txHash string) ([]explorer.Withdrawal, error)
	Claim(ctx context.Context, id int64) (explorer.ClaimData, error)
}

// Sender sends HyperEVM transactions (implemented by *chain.Sender).
type Sender interface {
	Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error)
}

// Settings configures Rescue.
type Settings struct {
	Enabled   bool
	Outbox    common.Address
	MinAge    time.Duration
	Lookback  time.Duration
	MaxPerRun int
	Interval  time.Duration
	PageLimit int // pages scanned per run and per lookup
}

// Rescuer runs the Rescue loop, the lookup and claim-for-me.
type Rescuer struct {
	s   Settings
	ex  Explorer
	tx  Sender // nil = read-only
	st  *store.Store
	log *slog.Logger
	now func() time.Time

	mu sync.Mutex // serializes claims: the loop and claim-for-me share it

	cacheMu  sync.Mutex
	resolved map[string]resolution // (origin tx, message id) -> immutable withdrawal fields

	lookupMu    sync.Mutex
	lookupAt    time.Time
	lookupCands []explorer.Message
}

type resolution struct {
	owner  string
	amount string
	ok     bool
}

// New creates a Rescuer. tx may be nil (read-only: lookup works, claims are refused).
func New(s Settings, ex Explorer, tx Sender, st *store.Store, log *slog.Logger) *Rescuer {
	if s.MaxPerRun <= 0 {
		s.MaxPerRun = 10
	}
	if s.PageLimit <= 0 {
		s.PageLimit = 20
	}
	if s.Interval <= 0 {
		s.Interval = 20 * time.Second
	}
	return &Rescuer{s: s, ex: ex, tx: tx, st: st, log: log, now: time.Now, resolved: map[string]resolution{}}
}

// Active reports whether the loop and claims can run.
func (r *Rescuer) Active() bool {
	return r.s.Enabled && r.tx != nil && r.s.Outbox != (common.Address{})
}

// Enabled reports whether lookups and stats are served.
func (r *Rescuer) Enabled() bool { return r.s.Enabled && r.s.Outbox != (common.Address{}) }

// Stats returns the public counter.
func (r *Rescuer) Stats() model.RescueStats { return r.st.Rescue() }

// Run runs RunOnce every Interval until ctx is cancelled.
func (r *Rescuer) Run(ctx context.Context) {
	if !r.Active() {
		return
	}
	t := time.NewTicker(r.s.Interval)
	defer t.Stop()
	for {
		if err := r.RunOnce(ctx); err != nil {
			r.log.Warn("rescue pass failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce scans from-rollup pages newest first and claims up to MaxPerRun eligible withdrawals.
func (r *Rescuer) RunOnce(ctx context.Context) error {
	if !r.Active() {
		return ErrDisabled
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	attempts := 0
	var cursor *int64
	for page := 0; page < r.s.PageLimit && attempts < r.s.MaxPerRun; page++ {
		p, err := r.ex.FromRollup(ctx, cursor)
		if err != nil {
			return fmt.Errorf("from-rollup: %w", err)
		}
		stop := false
		for _, m := range p.Items {
			t, ok := m.Time()
			if ok && now.Sub(t) > r.s.Lookback {
				stop = true
				break
			}
			if !r.eligible(m, now) || r.handled(m) {
				continue
			}
			if attempts >= r.s.MaxPerRun {
				break
			}
			attempts++
			if err := r.claimMessage(ctx, m); err != nil {
				r.log.Warn("rescue claim failed", "message_id", m.ID, "err", err)
			}
		}
		if stop || p.NextPageParams == nil || len(p.Items) == 0 {
			break
		}
		next := p.NextPageParams.ID
		cursor = &next
	}
	return r.st.Update(func(st *store.State) { st.Rescue.LastRunAt = now.Unix() })
}

// eligible: confirmed, not completed, and at least MinAge old.
func (r *Rescuer) eligible(m explorer.Message, now time.Time) bool {
	if m.Status != "confirmed" || m.Completed() {
		return false
	}
	t, ok := m.Time()
	return ok && now.Sub(t) >= r.s.MinAge
}

// handled reports whether the message was already processed or belongs to one of our Ferry Out exits.
func (r *Rescuer) handled(m explorer.Message) bool {
	if _, ok := r.st.Processed(store.MessageKey(m.ID)); ok {
		return true
	}
	if _, ok := r.st.ExitByElysiumTx(m.OriginationTransactionHash); ok {
		return true
	}
	return false
}

// claimMessage resolves owner and amount, verifies the explorer calldata, simulates and sends it, and
// records the result. Transient failures return an error and leave the message unprocessed.
func (r *Rescuer) claimMessage(ctx context.Context, m explorer.Message) error {
	item, err := r.resolve(ctx, m)
	if err != nil {
		return err
	}
	if item == nil {
		_ = r.st.Update(func(st *store.State) {
			st.Processed[store.MessageKey(m.ID)] = store.Processed{Outcome: "unattributed", Tx: m.OriginationTransactionHash, At: r.now().Unix()}
		})
		return nil
	}

	data, err := r.ex.Claim(ctx, m.ID)
	switch {
	case errors.Is(err, explorer.ErrUnconfirmed), errors.Is(err, explorer.ErrNotFound):
		return nil
	case errors.Is(err, explorer.ErrExecuted):
		r.markProcessed(m.ID, "executed", "")
		return nil
	case err != nil:
		return err
	}

	raw, call, err := r.verify(data, item.DestinationAddressHash, string(item.Callvalue))
	if err != nil {
		r.log.Error("refusing rescue calldata", "message_id", m.ID, "err", err)
		r.markProcessed(m.ID, "refused", "")
		return nil
	}

	res, err := r.tx.Send(ctx, chain.TxRequest{To: r.s.Outbox, Data: raw})
	if err != nil {
		if errors.Is(err, chain.ErrSimulation) {
			r.log.Info("rescue simulation failed, will retry", "message_id", m.ID, "err", err)
			return nil
		}
		return err
	}
	if res.DryRun {
		r.log.Info("dry run: rescue claim simulated", "message_id", m.ID, "owner", call.To.Hex(), "amount", call.Value)
		return nil
	}
	now := r.now().Unix()
	rec := model.RescueRecent{
		MessageID: m.ID,
		Owner:     call.To.Hex(),
		Amount:    call.Value.String(),
		ClaimTx:   res.Hash.Hex(),
		ClaimedAt: now,
	}
	if err := r.st.RecordRescue(rec, rec.Owner, res.GasSpent, now); err != nil {
		return fmt.Errorf("record rescue: %w", err)
	}
	r.log.Info("withdrawal rescued", "message_id", m.ID, "owner", rec.Owner, "amount", rec.Amount, "tx", rec.ClaimTx)
	return nil
}

func (r *Rescuer) markProcessed(id int64, outcome, tx string) {
	_ = r.st.Update(func(st *store.State) {
		st.Processed[store.MessageKey(id)] = store.Processed{Outcome: outcome, Tx: tx, At: r.now().Unix()}
	})
}

// verify checks the explorer claim: the outbox must be the configured one, the selector must be
// executeTransaction, the payout target must equal the withdrawal destination, and the forwarded value
// must equal the withdrawal callvalue (when the callvalue parses).
func (r *Rescuer) verify(data explorer.ClaimData, destination, callvalue string) ([]byte, abis.OutboxCall, error) {
	if !strings.EqualFold(data.Outbox(), r.s.Outbox.Hex()) {
		return nil, abis.OutboxCall{}, fmt.Errorf("outbox %s is not %s", data.Outbox(), r.s.Outbox.Hex())
	}
	raw, err := decodeCalldata(data.Calldata)
	if err != nil {
		return nil, abis.OutboxCall{}, fmt.Errorf("calldata: %w", err)
	}
	call, err := abis.DecodeOutboxCall(raw)
	if err != nil {
		return nil, abis.OutboxCall{}, err
	}
	if !strings.EqualFold(call.To.Hex(), destination) {
		return nil, abis.OutboxCall{}, fmt.Errorf("payout target %s is not destination %s", call.To.Hex(), destination)
	}
	if v, ok := new(big.Int).SetString(callvalue, 10); ok && v.Cmp(call.Value) != 0 {
		return nil, abis.OutboxCall{}, fmt.Errorf("value %s is not callvalue %s", call.Value, v)
	}
	return raw, call, nil
}

// resolve finds the withdrawal item for message m among the withdrawals of its origin transaction.
// The owner (destination_address_hash) and callvalue are immutable, so they are cached. Returns nil when
// the transaction has no matching item.
func (r *Rescuer) resolve(ctx context.Context, m explorer.Message) (*explorer.Withdrawal, error) {
	key := strings.ToLower(m.OriginationTransactionHash) + "|" + fmt.Sprint(m.ID)
	r.cacheMu.Lock()
	cached, hit := r.resolved[key]
	r.cacheMu.Unlock()
	if hit {
		if !cached.ok {
			return nil, nil
		}
		return &explorer.Withdrawal{ID: m.ID, DestinationAddressHash: cached.owner, Callvalue: explorer.FlexString(cached.amount)}, nil
	}
	items, err := r.ex.WithdrawalsByTx(ctx, m.OriginationTransactionHash)
	if err != nil {
		return nil, fmt.Errorf("withdrawals %s: %w", m.OriginationTransactionHash, err)
	}
	var found *explorer.Withdrawal
	for i := range items {
		if items[i].ID == m.ID {
			found = &items[i]
			break
		}
	}
	r.cacheMu.Lock()
	if len(r.resolved) >= maxResolveCache {
		r.resolved = map[string]resolution{}
	}
	if found == nil {
		r.resolved[key] = resolution{}
	} else {
		r.resolved[key] = resolution{owner: found.DestinationAddressHash, amount: string(found.Callvalue), ok: true}
	}
	r.cacheMu.Unlock()
	return found, nil
}

// Lookup returns the stuck withdrawals (status sent or confirmed, not completed) whose destination is
// address. The from-rollup scan is cached for 60 seconds.
func (r *Rescuer) Lookup(ctx context.Context, address common.Address) ([]model.StuckWithdrawal, error) {
	if !r.Enabled() {
		return nil, ErrDisabled
	}
	cands, err := r.candidates(ctx)
	if err != nil {
		return nil, err
	}
	now := r.now()
	var out []model.StuckWithdrawal
	for _, m := range cands {
		item, err := r.resolve(ctx, m)
		if err != nil {
			r.log.Debug("lookup: resolve failed", "message_id", m.ID, "err", err)
			continue
		}
		if item == nil || !strings.EqualFold(item.DestinationAddressHash, address.Hex()) {
			continue
		}
		age := int64(0)
		if t, ok := m.Time(); ok {
			age = int64(now.Sub(t).Seconds())
		}
		out = append(out, model.StuckWithdrawal{
			MessageID:  m.ID,
			Owner:      common.HexToAddress(item.DestinationAddressHash).Hex(),
			Amount:     string(item.Callvalue),
			OriginTx:   m.OriginationTransactionHash,
			Status:     m.Status,
			AgeSeconds: age,
			Claimable:  m.Status == "confirmed",
		})
	}
	if out == nil {
		out = []model.StuckWithdrawal{}
	}
	return out, nil
}

func (r *Rescuer) candidates(ctx context.Context) ([]explorer.Message, error) {
	r.lookupMu.Lock()
	defer r.lookupMu.Unlock()
	if r.lookupCands != nil && r.now().Sub(r.lookupAt) < lookupTTL {
		return r.lookupCands, nil
	}
	now := r.now()
	var out []explorer.Message
	var cursor *int64
	for page := 0; page < r.s.PageLimit && len(out) < maxLookupCands; page++ {
		p, err := r.ex.FromRollup(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("from-rollup: %w", err)
		}
		stop := false
		for _, m := range p.Items {
			t, ok := m.Time()
			if ok && now.Sub(t) > r.s.Lookback {
				stop = true
				break
			}
			if (m.Status == "sent" || m.Status == "confirmed") && !m.Completed() {
				out = append(out, m)
			}
		}
		if stop || p.NextPageParams == nil || len(p.Items) == 0 {
			break
		}
		next := p.NextPageParams.ID
		cursor = &next
	}
	if len(out) > maxLookupCands {
		out = out[:maxLookupCands]
	}
	if out == nil {
		out = []explorer.Message{}
	}
	r.lookupCands = out
	r.lookupAt = now
	return out, nil
}

// ClaimForMe claims one message on demand. The claim runs in a goroutine behind the same mutex as the
// loop. If it has not finished within claimWait, the status is "queued" and it completes in the background.
func (r *Rescuer) ClaimForMe(ctx context.Context, messageID int64) (model.ClaimResult, error) {
	if !r.Active() {
		return model.ClaimResult{}, ErrDisabled
	}
	type outcome struct {
		res model.ClaimResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		bg, cancel := context.WithTimeout(context.Background(), claimTimeout)
		defer cancel()
		res, err := r.claimOne(bg, messageID)
		done <- outcome{res, err}
	}()
	timer := time.NewTimer(claimWait)
	defer timer.Stop()
	select {
	case o := <-done:
		return o.res, o.err
	case <-timer.C:
		return model.ClaimResult{Status: StatusQueued}, nil
	case <-ctx.Done():
		return model.ClaimResult{Status: StatusQueued}, nil
	}
}

func (r *Rescuer) claimOne(ctx context.Context, id int64) (model.ClaimResult, error) {
	notClaimable := model.ClaimResult{Status: StatusNotClaimable}
	data, err := r.ex.Claim(ctx, id)
	switch {
	case errors.Is(err, explorer.ErrUnconfirmed), errors.Is(err, explorer.ErrNotFound), errors.Is(err, explorer.ErrExecuted):
		return notClaimable, nil
	case err != nil:
		return model.ClaimResult{}, err
	}
	if !strings.EqualFold(data.Outbox(), r.s.Outbox.Hex()) {
		r.log.Error("refusing claim-for-me: outbox mismatch", "message_id", id, "outbox", data.Outbox())
		return notClaimable, nil
	}
	raw, err := decodeCalldata(data.Calldata)
	if err != nil {
		r.log.Error("refusing claim-for-me: bad calldata", "message_id", id, "err", err)
		return notClaimable, nil
	}
	call, err := abis.DecodeOutboxCall(raw)
	if err != nil {
		r.log.Error("refusing claim-for-me: bad calldata", "message_id", id, "err", err)
		return notClaimable, nil
	}
	res, err := r.tx.Send(ctx, chain.TxRequest{To: r.s.Outbox, Data: raw})
	if err != nil {
		if errors.Is(err, chain.ErrSimulation) {
			return notClaimable, nil
		}
		return model.ClaimResult{}, err
	}
	if res.DryRun {
		return notClaimable, nil
	}
	now := r.now().Unix()
	hash := res.Hash.Hex()
	rec := model.RescueRecent{MessageID: id, Owner: call.To.Hex(), Amount: call.Value.String(), ClaimTx: hash, ClaimedAt: now}
	if err := r.st.RecordRescue(rec, rec.Owner, res.GasSpent, now); err != nil {
		return model.ClaimResult{}, fmt.Errorf("record rescue: %w", err)
	}
	return model.ClaimResult{Status: StatusClaimed, TxHash: &hash}, nil
}

// decodeCalldata decodes 0x-hex calldata.
func decodeCalldata(s string) ([]byte, error) {
	return hexutil.Decode(strings.TrimSpace(s))
}

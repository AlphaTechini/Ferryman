// Package indexer builds the token list from Ascend launchpad pools and tracks Ferry Out exit intents
// on Elysium. Scans run in LOG_RANGE chunks behind head by Confirmations blocks, and each chunk's
// cursor is persisted only after the chunk is stored, so a restart re-scans at most one chunk.
package indexer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// Stream names for the persisted cursors.
const (
	streamTokens = "elysium.pools"
	streamExits  = "elysium.exits"
)

// maxMetaFailures is how many times a pool's metadata lookup may fail before the pool is skipped.
const maxMetaFailures = 5

// maxRegisterFailures is how many simulated registrations may fail before a token is skipped.
const maxRegisterFailures = 5

// ExitWindow is the estimated time until an exit matures (25 minutes on testnet).
const ExitWindow = 25 * time.Minute

// Settings configures the indexer.
type Settings struct {
	Launchpad       common.Address
	CLPoolManager   common.Address // zero disables token indexing
	ExitDesk        common.Address // zero disables exit indexing
	FerryIn         common.Address // zero disables token registration
	QuoteCurrencies []common.Address
	OurLaunchers    []common.Address
	IndexFromBlock  uint64
	LogRange        uint64
	Confirmations   uint64
	Interval        time.Duration
	RegisterPerTick int
}

// Transactor sends a transaction (implemented by *chain.Sender). Nil means read-only.
type Transactor interface {
	Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error)
}

// Indexer scans Elysium logs into the store.
type Indexer struct {
	s   Settings
	el  chain.Reader
	st  *store.Store
	tx  Transactor
	log *slog.Logger
	now func() time.Time

	ours map[common.Address]bool

	mu           sync.Mutex
	chainID      *big.Int
	quoteSymbols map[common.Address]string
	metaFailures map[string]int
	regFailures  map[string]int
	lastErr      string
	lastSync     time.Time
}

// New creates an indexer.
func New(s Settings, el chain.Reader, st *store.Store, tx Transactor, log *slog.Logger) *Indexer {
	ix := &Indexer{
		s: s, el: el, st: st, tx: tx, log: log, now: time.Now,
		ours:         map[common.Address]bool{},
		quoteSymbols: map[common.Address]string{},
		metaFailures: map[string]int{},
		regFailures:  map[string]int{},
	}
	for _, a := range s.OurLaunchers {
		ix.ours[a] = true
	}
	if ix.s.Confirmations == 0 {
		ix.s.Confirmations = 3
	}
	if ix.s.LogRange == 0 {
		ix.s.LogRange = 5000
	}
	if ix.s.RegisterPerTick <= 0 {
		ix.s.RegisterPerTick = 5
	}
	return ix
}

// Run syncs immediately and then every Interval until ctx is cancelled.
func (ix *Indexer) Run(ctx context.Context) {
	t := time.NewTicker(ix.s.Interval)
	defer t.Stop()
	for {
		ix.SyncOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// LastError returns the most recent sync error text, or "".
func (ix *Indexer) LastError() string {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.lastErr
}

// SyncOnce runs one pass: pools, exits, then pending registrations. Errors are logged and recorded.
func (ix *Indexer) SyncOnce(ctx context.Context) {
	var errs []string
	if ix.s.CLPoolManager != (common.Address{}) {
		if err := ix.syncTokens(ctx); err != nil {
			errs = append(errs, "tokens: "+err.Error())
		}
		if err := ix.registerPending(ctx); err != nil {
			errs = append(errs, "register: "+err.Error())
		}
	}
	if ix.s.ExitDesk != (common.Address{}) {
		if err := ix.syncExits(ctx); err != nil {
			errs = append(errs, "exits: "+err.Error())
		}
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.lastErr = strings.Join(errs, "; ")
	ix.lastSync = ix.now()
	if ix.lastErr != "" {
		ix.log.Warn("indexer sync incomplete", "err", ix.lastErr)
	}
}

// ---- scanning ----

// scan walks [cursor+1, head-confirmations] in LOG_RANGE chunks and persists the cursor after each chunk.
func (ix *Indexer) scan(ctx context.Context, stream string, addr common.Address, topic0 common.Hash,
	handle func(ctx context.Context, logs []types.Log) error) error {
	head, err := ix.el.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if head < ix.s.Confirmations {
		return nil
	}
	target := head - ix.s.Confirmations
	from := ix.s.IndexFromBlock
	if c, ok := ix.st.Cursor(stream); ok {
		from = c + 1
	}
	for from <= target {
		to := from + ix.s.LogRange - 1
		if to < from || to > target {
			to = target
		}
		logs, err := ix.el.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(from),
			ToBlock:   new(big.Int).SetUint64(to),
			Addresses: []common.Address{addr},
			Topics:    [][]common.Hash{{topic0}},
		})
		if err != nil {
			return fmt.Errorf("getLogs %d-%d: %w", from, to, err)
		}
		if err := handle(ctx, logs); err != nil {
			return fmt.Errorf("handle %d-%d: %w", from, to, err)
		}
		if err := ix.st.Update(func(st *store.State) { st.Cursors[stream] = to }); err != nil {
			return fmt.Errorf("persist cursor: %w", err)
		}
		from = to + 1
	}
	return nil
}

// blockTimes caches block timestamps for one sync pass.
type blockTimes map[uint64]uint64

func (ix *Indexer) blockTime(ctx context.Context, cache blockTimes, n uint64) (uint64, error) {
	if t, ok := cache[n]; ok {
		return t, nil
	}
	h, err := ix.el.HeaderByNumber(ctx, new(big.Int).SetUint64(n))
	if err != nil {
		return 0, fmt.Errorf("header %d: %w", n, err)
	}
	cache[n] = h.Time
	return h.Time, nil
}

// ---- tokens ----

func (ix *Indexer) syncTokens(ctx context.Context) error {
	cache := blockTimes{}
	return ix.scan(ctx, streamTokens, ix.s.CLPoolManager, abis.TopicInitialize, func(ctx context.Context, logs []types.Log) error {
		for _, l := range logs {
			if err := ix.handleInitialize(ctx, cache, l); err != nil {
				return err
			}
		}
		return nil
	})
}

// handleInitialize records one Ascend pool. It returns an error only for transient RPC failures,
// which stop the chunk so it is retried; permanent problems are logged and skipped.
func (ix *Indexer) handleInitialize(ctx context.Context, cache blockTimes, l types.Log) error {
	ev, err := abis.DecodeInitialize(l)
	if err != nil {
		ix.log.Warn("skip malformed Initialize", "tx", l.TxHash.Hex(), "err", err)
		return nil
	}
	if ev.Hooks != ix.s.Launchpad {
		return nil
	}
	token, quote, ok := ix.pickToken(ev)
	if !ok {
		ix.log.Debug("skip Ascend pool with unknown quote currency", "tx", l.TxHash.Hex())
		return nil
	}
	if _, exists := ix.st.Token(token.Hex()); exists {
		return nil
	}
	failKey := token.Hex() + "|" + l.TxHash.Hex()
	fail := func(what string, err error) error {
		ix.mu.Lock()
		ix.metaFailures[failKey]++
		n := ix.metaFailures[failKey]
		ix.mu.Unlock()
		if n >= maxMetaFailures {
			ix.log.Warn("skipping token after repeated failures", "token", token.Hex(), "what", what, "err", err)
			return nil
		}
		return fmt.Errorf("%s for %s: %w", what, token.Hex(), err)
	}

	name, symbol, decimals, err := ix.erc20Meta(ctx, token)
	if err != nil {
		return fail("metadata", err)
	}
	launcher, err := ix.launcher(ctx, l.TxHash)
	if err != nil {
		return fail("launcher", err)
	}
	ts, err := ix.blockTime(ctx, cache, l.BlockNumber)
	if err != nil {
		return fail("block time", err)
	}
	qsym := ix.quoteSymbol(ctx, quote)

	rec := model.Token{
		Address:       token.Hex(),
		Name:          name,
		Symbol:        symbol,
		Decimals:      decimals,
		Launcher:      launcher.Hex(),
		LaunchTx:      l.TxHash.Hex(),
		LaunchBlock:   l.BlockNumber,
		LaunchedAt:    int64(ts),
		QuoteCurrency: quote.Hex(),
		QuoteSymbol:   qsym,
		PoolKey: model.PoolKey{
			Currency0:   ev.Currency0.Hex(),
			Currency1:   ev.Currency1.Hex(),
			Hooks:       ev.Hooks.Hex(),
			PoolManager: ev.PoolManager.Hex(),
			Fee:         ev.Fee,
			Parameters:  common.BytesToHash(ev.Parameters[:]).Hex(),
		},
		PriceHype: nil,
		OurLaunch: ix.ours[launcher],
	}
	ix.log.Info("token indexed", "token", rec.Symbol, "address", rec.Address, "launcher", rec.Launcher, "our_launch", rec.OurLaunch)
	return ix.st.Update(func(st *store.State) { st.Tokens[store.Key(rec.Address)] = rec })
}

func (ix *Indexer) pickToken(ev abis.InitializeEvent) (token, quote common.Address, ok bool) {
	isQuote := func(a common.Address) bool {
		for _, q := range ix.s.QuoteCurrencies {
			if q == a {
				return true
			}
		}
		return false
	}
	var zero common.Address
	switch {
	case isQuote(ev.Currency0) && !isQuote(ev.Currency1):
		token, quote = ev.Currency1, ev.Currency0
	case isQuote(ev.Currency1) && !isQuote(ev.Currency0):
		token, quote = ev.Currency0, ev.Currency1
	default:
		return zero, zero, false
	}
	if token == zero {
		return zero, zero, false
	}
	return token, quote, true
}

func (ix *Indexer) erc20Meta(ctx context.Context, token common.Address) (name, symbol string, decimals uint8, err error) {
	call := func(method string, out any) error {
		data, err := abis.ERC20.Pack(method)
		if err != nil {
			return err
		}
		ret, err := ix.el.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
		if err != nil {
			return err
		}
		vals, err := abis.ERC20.Unpack(method, ret)
		if err != nil {
			return err
		}
		switch p := out.(type) {
		case *string:
			*p = vals[0].(string)
		case *uint8:
			*p = vals[0].(uint8)
		}
		return nil
	}
	if err = call("name", &name); err != nil {
		return
	}
	if err = call("symbol", &symbol); err != nil {
		return
	}
	err = call("decimals", &decimals)
	return
}

func (ix *Indexer) quoteSymbol(ctx context.Context, quote common.Address) string {
	ix.mu.Lock()
	if s, ok := ix.quoteSymbols[quote]; ok {
		ix.mu.Unlock()
		return s
	}
	ix.mu.Unlock()
	var sym string
	data, _ := abis.ERC20.Pack("symbol")
	if ret, err := ix.el.CallContract(ctx, ethereum.CallMsg{To: &quote, Data: data}, nil); err == nil {
		if vals, err := abis.ERC20.Unpack("symbol", ret); err == nil {
			sym, _ = vals[0].(string)
		}
	}
	if sym == "" {
		sym = quote.Hex()[:6] + "…" + quote.Hex()[len(quote.Hex())-4:]
	}
	ix.mu.Lock()
	ix.quoteSymbols[quote] = sym
	ix.mu.Unlock()
	return sym
}

// launcher returns the sender of the Initialize transaction.
func (ix *Indexer) launcher(ctx context.Context, txHash common.Hash) (common.Address, error) {
	if err := ix.ensureChainID(ctx); err != nil {
		return common.Address{}, err
	}
	tx, _, err := ix.el.TransactionByHash(ctx, txHash)
	if err != nil {
		return common.Address{}, err
	}
	from, err := types.Sender(types.LatestSignerForChainID(ix.chainID), tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("recover sender: %w", err)
	}
	return from, nil
}

func (ix *Indexer) ensureChainID(ctx context.Context) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.chainID != nil {
		return nil
	}
	id, err := ix.el.ChainID(ctx)
	if err != nil {
		return err
	}
	ix.chainID = id
	return nil
}

// registerPending registers unregistered tokens with FerryIn. A token whose pool key is already set
// on FerryIn is only recorded. Otherwise registerToken is simulated and sent (read-only mode skips it).
func (ix *Indexer) registerPending(ctx context.Context) error {
	if ix.s.FerryIn == (common.Address{}) {
		return nil
	}
	var pending []model.Token
	ix.st.View(func(st *store.State) {
		for k, t := range st.Tokens {
			if !st.Registered[k] {
				pending = append(pending, t)
			}
		}
	})
	sort.Slice(pending, func(i, j int) bool { return pending[i].LaunchedAt < pending[j].LaunchedAt })

	done := 0
	for _, t := range pending {
		if done >= ix.s.RegisterPerTick {
			break
		}
		token := common.HexToAddress(t.Address)
		data, err := abis.PackPoolKeyOf(token)
		if err != nil {
			return err
		}
		ret, err := ix.el.CallContract(ctx, ethereum.CallMsg{To: &ix.s.FerryIn, Data: data}, nil)
		if err != nil {
			return fmt.Errorf("poolKeyOf %s: %w", token.Hex(), err)
		}
		key, err := abis.UnpackPoolKey(ret)
		if err != nil {
			return fmt.Errorf("decode poolKeyOf: %w", err)
		}
		if !abis.IsZeroPoolKey(key) {
			_ = ix.st.Update(func(st *store.State) { st.Registered[store.Key(t.Address)] = true })
			continue
		}
		if ix.tx == nil {
			continue
		}
		done++
		ix.mu.Lock()
		failed := ix.regFailures[store.Key(t.Address)] >= maxRegisterFailures
		ix.mu.Unlock()
		if failed {
			continue
		}
		pk, err := toABIPoolKey(t.PoolKey)
		if err != nil {
			return err
		}
		calldata, err := abis.PackRegisterToken(pk)
		if err != nil {
			return err
		}
		res, err := ix.tx.Send(ctx, chain.TxRequest{To: ix.s.FerryIn, Data: calldata})
		if err != nil {
			ix.mu.Lock()
			ix.regFailures[store.Key(t.Address)]++
			ix.mu.Unlock()
			if errors.Is(err, chain.ErrSimulation) {
				ix.log.Warn("registerToken simulation failed", "token", t.Address, "err", err)
				continue
			}
			return fmt.Errorf("registerToken %s: %w", t.Address, err)
		}
		if res.DryRun {
			continue
		}
		ix.log.Info("token registered with FerryIn", "token", t.Address, "tx", res.Hash.Hex())
		_ = ix.st.Update(func(st *store.State) { st.Registered[store.Key(t.Address)] = true })
	}
	return nil
}

func toABIPoolKey(k model.PoolKey) (abis.PoolKey, error) {
	return abis.PoolKey{
		Currency0:   common.HexToAddress(k.Currency0),
		Currency1:   common.HexToAddress(k.Currency1),
		Hooks:       common.HexToAddress(k.Hooks),
		PoolManager: common.HexToAddress(k.PoolManager),
		Fee:         new(big.Int).SetUint64(uint64(k.Fee)),
		Parameters:  common.HexToHash(k.Parameters),
	}, nil
}

// ---- exits ----

func (ix *Indexer) syncExits(ctx context.Context) error {
	cache := blockTimes{}
	return ix.scan(ctx, streamExits, ix.s.ExitDesk, abis.TopicExitIntent, func(ctx context.Context, logs []types.Log) error {
		for _, l := range logs {
			ev, err := abis.DecodeExitIntent(l)
			if err != nil {
				ix.log.Warn("skip malformed ExitIntent", "tx", l.TxHash.Hex(), "err", err)
				continue
			}
			salt, err := abis.ExitSalt(ev.Params)
			if err != nil || salt != ev.Salt {
				ix.log.Warn("skip ExitIntent with salt mismatch", "tx", l.TxHash.Hex())
				continue
			}
			if _, exists := ix.st.Exit(ev.Receiver.Hex()); exists {
				continue
			}
			ts, err := ix.blockTime(ctx, cache, l.BlockNumber)
			if err != nil {
				return err
			}
			started := int64(ts)
			matures := started + int64(ExitWindow/time.Second)
			rec := model.Exit{
				Receiver:  ev.Receiver.Hex(),
				Salt:      ev.Salt.Hex(),
				Params:    toModelParams(ev.Params),
				Stage:     "intent",
				ElysiumTx: l.TxHash.Hex(),
				StartedAt: started,
				MaturesAt: &matures,
			}
			ix.log.Info("exit intent indexed", "receiver", rec.Receiver, "sender", rec.Params.Sender, "amount", rec.Params.Amount)
			if err := ix.st.Update(func(st *store.State) { st.Exits[store.Key(rec.Receiver)] = rec }); err != nil {
				return err
			}
		}
		return nil
	})
}

func toModelParams(p abis.ExitParams) model.ExitParams {
	return model.ExitParams{
		ChainID:   p.ChainId.Uint64(),
		Desk:      p.Desk.Hex(),
		Sender:    p.Sender.Hex(),
		Nonce:     p.Nonce.String(),
		Amount:    p.Amount.String(),
		ClaimTip:  p.ClaimTip.String(),
		Recipient: p.Recipient.Hex(),
		ToCore:    p.ToCore,
	}
}

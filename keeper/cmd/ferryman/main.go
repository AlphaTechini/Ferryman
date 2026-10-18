// Command ferryman runs the Ferryman keeper: the Elysium indexer, the quote API, the Ferry Out claimer,
// the Rescue loop, the Ferry In tracker and the HTTP API, in one process.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/alphatechini/ferryman/keeper/internal/api"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/claimer"
	"github.com/alphatechini/ferryman/keeper/internal/config"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/ferryin"
	"github.com/alphatechini/ferryman/keeper/internal/indexer"
	"github.com/alphatechini/ferryman/keeper/internal/quote"
	"github.com/alphatechini/ferryman/keeper/internal/rescue"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ferryman:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "", "listen address (overrides LISTEN_ADDR)")
	deployments := flag.String("deployments", "", "deployments JSON (overrides DEPLOYMENTS)")
	dataDir := flag.String("data-dir", "", "state directory (overrides DATA_DIR)")
	dryRun := flag.Bool("dry-run", false, "simulate transactions only (overrides DRY_RUN)")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "addr":
			_ = os.Setenv("LISTEN_ADDR", *addr)
		case "deployments":
			_ = os.Setenv("DEPLOYMENTS", *deployments)
		case "data-dir":
			_ = os.Setenv("DATA_DIR", *dataDir)
		case "dry-run":
			_ = os.Setenv("DRY_RUN", fmt.Sprint(*dryRun))
		}
	})

	log := newLogger(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		log.Warn("configuration", "warning", w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(filepath.Join(cfg.DataDir, "state.json"))
	if err != nil {
		return err
	}
	chains, err := chain.Dial(ctx, cfg.ElysiumRPC, cfg.HyperEVMRPC)
	if err != nil {
		return err
	}
	probe(ctx, log, "elysium", cfg.ElysiumRPC, chains.Elysium, config.ElysiumChainID)
	probe(ctx, log, "hyperevm", cfg.HyperEVMRPC, chains.HyperEVM, config.HyperEVMChainID)

	// Signers. A nil *chain.Sender must never be stored in an interface, so the interfaces stay nil.
	var keeper common.Address
	var elTx indexer.Transactor
	var hyTx claimer.Sender
	var hyTxRescue rescue.Sender
	var elTxFerry ferryin.Sender
	if cfg.KeeperKey != "" {
		key, err := chain.ParseKey(cfg.KeeperKey)
		if err != nil {
			return fmt.Errorf("KEEPER_PRIVATE_KEY: %w", err)
		}
		elSender := chain.NewSender(chains.Elysium, new(big.Int).SetUint64(config.ElysiumChainID), key, cfg.DryRun, log)
		hySender := chain.NewSender(chains.HyperEVM, new(big.Int).SetUint64(config.HyperEVMChainID), key, cfg.DryRun, log)
		keeper = elSender.Address()
		elTx, elTxFerry = elSender, elSender
		hyTx, hyTxRescue = hySender, hySender
		log.Info("signer loaded", "keeper", keeper.Hex(), "dry_run", cfg.DryRun)
	} else {
		log.Info("no signing key: read-only mode (API and indexing only)")
	}

	ex := explorer.New(cfg.ExplorerAPI)

	ix := indexer.New(indexer.Settings{
		Launchpad:       cfg.Launchpad,
		CLPoolManager:   cfg.CLPoolManager,
		ExitDesk:        cfg.Contracts.ExitDesk,
		FerryIn:         cfg.Contracts.FerryIn,
		QuoteCurrencies: cfg.QuoteCurrencies,
		OurLaunchers:    cfg.OurLaunchers,
		IndexFromBlock:  cfg.IndexFromBlock,
		LogRange:        cfg.LogRange,
		Confirmations:   config.Confirmations,
		Interval:        cfg.PollInterval,
	}, chains.Elysium, st, elTx, log)

	quoter := quote.New(quote.Settings{
		FerryIn:            cfg.Contracts.FerryIn,
		Inbox:              cfg.Contracts.Inbox,
		FeeBps:             config.FeeBps,
		DefaultSlippageBps: config.DefaultSlippageBps,
		Keeper:             keeper,
	}, chains.Elysium, chains.HyperEVM, st, log)

	tracker := ferryin.New(ferryin.Settings{
		Inbox:   cfg.Contracts.Inbox,
		FerryIn: cfg.Contracts.FerryIn,
	}, chains.HyperEVM, chains.Elysium, ex, elTxFerry, st, log)

	clm := claimer.New(claimer.Settings{
		ExitFactory: cfg.Contracts.ExitFactory,
		Outbox:      cfg.Contracts.Outbox,
		Interval:    cfg.PollInterval,
		LogRange:    cfg.LogRange,
		Maturity:    25 * time.Minute,
	}, chains.Elysium, chains.HyperEVM, ex, hyTx, st, log)

	rs := rescue.New(rescue.Settings{
		Enabled:   cfg.RescueEnabled && cfg.Contracts.Outbox != (common.Address{}),
		Outbox:    cfg.Contracts.Outbox,
		MinAge:    cfg.RescueMinAge,
		Lookback:  cfg.RescueLookback,
		MaxPerRun: cfg.RescueMaxPerRun,
		Interval:  cfg.PollInterval,
	}, ex, hyTxRescue, st, log)

	var wg sync.WaitGroup
	loop := func(name string, fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("loop started", "loop", name)
			fn(ctx)
			log.Info("loop stopped", "loop", name)
		}()
	}
	loop("indexer", ix.Run)
	loop("claimer", clm.Run)
	loop("rescue", rs.Run)
	if elTxFerry != nil && tracker.Enabled() {
		loop("ferry-in-redeem", func(ctx context.Context) {
			t := time.NewTicker(cfg.PollInterval)
			defer t.Stop()
			for {
				if err := tracker.RedeemFailed(ctx); err != nil {
					log.Warn("ferry in redeem pass", "err", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		})
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.Handler(api.Deps{
			Config:       cfg,
			Store:        st,
			Quote:        quoter,
			FerryIn:      tracker,
			Rescue:       rs,
			ElysiumHead:  headFunc(chains.Elysium),
			HyperEVMHead: headFunc(chains.HyperEVM),
			Keeper:       keeper,
			Log:          log,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("http listening", "addr", cfg.ListenAddr)

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
			wg.Wait()
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", "err", err)
	}
	wg.Wait()
	log.Info("stopped")
	return nil
}

// probe checks the chain id once at start-up. Failures are logged, never fatal.
func probe(ctx context.Context, log *slog.Logger, name, url string, r chain.Reader, want uint64) {
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id, err := r.ChainID(pctx)
	if err != nil {
		log.Warn("rpc unreachable at start-up (continuing)", "chain", name, "rpc", url, "err", err)
		return
	}
	if id.Uint64() != want {
		log.Warn("rpc chain id mismatch", "chain", name, "got", id.String(), "want", want)
		return
	}
	log.Info("rpc ok", "chain", name, "chain_id", id.String())
}

// headFunc returns a block-number getter with a short timeout, used by /api/health.
func headFunc(r chain.Reader) func(context.Context) (uint64, error) {
	return func(ctx context.Context) (uint64, error) {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return r.BlockNumber(c)
	}
}

func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

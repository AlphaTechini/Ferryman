// Package api serves the keeper HTTP API (docs/INTERFACES.md section 3). JSON everywhere, CORS *,
// errors as {"error": "..."}, big numbers as decimal strings, addresses checksummed.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/alphatechini/ferryman/keeper/internal/config"
	"github.com/alphatechini/ferryman/keeper/internal/ferryin"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/quote"
	"github.com/alphatechini/ferryman/keeper/internal/rescue"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// Quoter is the Ferry In quote service.
type Quoter interface {
	Quote(ctx context.Context, token common.Address, amountIn *big.Int, slippageBps int) (model.Quote, error)
	Enabled() bool
	DefaultSlippageBps() int
}

// FerryInTracker answers Ferry In status queries.
type FerryInTracker interface {
	Status(ctx context.Context, hyperevmTx string) (model.FerryInStatus, error)
	Enabled() bool
}

// RescueService is the Rescue component.
type RescueService interface {
	Stats() model.RescueStats
	Lookup(ctx context.Context, address common.Address) ([]model.StuckWithdrawal, error)
	ClaimForMe(ctx context.Context, messageID int64) (model.ClaimResult, error)
	Enabled() bool
}

// Deps are the handler dependencies.
type Deps struct {
	Config         *config.Config
	Store          *store.Store
	Quote          Quoter
	FerryIn        FerryInTracker
	Rescue         RescueService
	ElysiumHead    func(ctx context.Context) (uint64, error)
	HyperEVMHead   func(ctx context.Context) (uint64, error)
	Keeper         common.Address // zero when read-only
	Log            *slog.Logger
	RequestTimeout time.Duration
}

// Server wires the routes.
type server struct {
	d Deps
}

var hashRe = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)

// Handler returns the HTTP handler with CORS, panic recovery and request timeouts.
func Handler(d Deps) http.Handler {
	if d.RequestTimeout <= 0 {
		d.RequestTimeout = 25 * time.Second
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	s := &server{d: d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/config", s.configHandler)
	mux.HandleFunc("GET /api/tokens", s.tokens)
	mux.HandleFunc("GET /api/quote", s.quote)
	mux.HandleFunc("GET /api/ferry-in/{hyperevmTxHash}", s.ferryIn)
	mux.HandleFunc("GET /api/exits", s.exitsByAddress)
	mux.HandleFunc("GET /api/exits/{receiver}", s.exit)
	mux.HandleFunc("GET /api/rescue/stats", s.rescueStats)
	mux.HandleFunc("GET /api/rescue/lookup", s.rescueLookup)
	mux.HandleFunc("POST /api/rescue/claim", s.rescueClaim)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return withMiddleware(d, mux)
}

func withMiddleware(d Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				d.Log.Error("panic in handler", "path", r.URL.Path, "panic", fmt.Sprint(rec))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		ctx, cancel := context.WithTimeout(r.Context(), d.RequestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
		d.Log.Debug("request", "method", r.Method, "path", r.URL.Path, "took", time.Since(start))
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func validAddress(s string) (common.Address, bool) {
	if !common.IsHexAddress(s) {
		return common.Address{}, false
	}
	return common.HexToAddress(s), true
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// ---- health and config ----

type healthResp struct {
	OK            bool    `json:"ok"`
	ElysiumBlock  *uint64 `json:"elysiumBlock"`
	HyperEVMBlock *uint64 `json:"hyperevmBlock"`
	Keeper        *string `json:"keeper"`
	DryRun        bool    `json:"dryRun"`
	Error         string  `json:"error,omitempty"`
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	resp := healthResp{DryRun: s.d.Config.DryRun}
	if s.d.Keeper != (common.Address{}) {
		k := s.d.Keeper.Hex()
		resp.Keeper = &k
	}
	var errs []string
	if s.d.ElysiumHead != nil {
		if h, err := s.d.ElysiumHead(r.Context()); err != nil {
			errs = append(errs, "elysium: "+err.Error())
		} else {
			resp.ElysiumBlock = &h
		}
	}
	if s.d.HyperEVMHead != nil {
		if h, err := s.d.HyperEVMHead(r.Context()); err != nil {
			errs = append(errs, "hyperevm: "+err.Error())
		} else {
			resp.HyperEVMBlock = &h
		}
	}
	resp.OK = len(errs) == 0
	resp.Error = strings.Join(errs, "; ")
	writeJSON(w, http.StatusOK, resp)
}

type chainInfo struct {
	ChainID  uint64 `json:"chainId"`
	RPC      string `json:"rpc"`
	Explorer string `json:"explorer"`
}

type contractsInfo struct {
	FerryIn     string `json:"ferryIn"`
	ExitDesk    string `json:"exitDesk"`
	ExitFactory string `json:"exitFactory"`
	FeeJar      string `json:"feeJar"`
	Inbox       string `json:"inbox"`
	Outbox      string `json:"outbox"`
}

type configResp struct {
	Elysium            chainInfo     `json:"elysium"`
	HyperEVM           chainInfo     `json:"hyperevm"`
	Contracts          contractsInfo `json:"contracts"`
	FeeBps             uint64        `json:"feeBps"`
	DefaultSlippageBps int           `json:"defaultSlippageBps"`
	DefaultClaimTip    string        `json:"defaultClaimTip"`
}

func (s *server) configHandler(w http.ResponseWriter, r *http.Request) {
	c := s.d.Config
	writeJSON(w, http.StatusOK, configResp{
		Elysium:  chainInfo{ChainID: config.ElysiumChainID, RPC: c.ElysiumRPC, Explorer: config.ElysiumExplorerURL},
		HyperEVM: chainInfo{ChainID: config.HyperEVMChainID, RPC: c.HyperEVMRPC, Explorer: config.HyperEVMExplorerURL},
		Contracts: contractsInfo{
			FerryIn:     c.Contracts.FerryIn.Hex(),
			ExitDesk:    c.Contracts.ExitDesk.Hex(),
			ExitFactory: c.Contracts.ExitFactory.Hex(),
			FeeJar:      c.Contracts.FeeJar.Hex(),
			Inbox:       c.Contracts.Inbox.Hex(),
			Outbox:      c.Contracts.Outbox.Hex(),
		},
		FeeBps:             config.FeeBps,
		DefaultSlippageBps: config.DefaultSlippageBps,
		DefaultClaimTip:    c.DefaultClaimTip.String(),
	})
}

// ---- tokens and quote ----

func (s *server) tokens(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, nonNil(s.d.Store.Tokens()))
}

func (s *server) quote(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	token, ok := validAddress(q.Get("token"))
	if !ok {
		writeError(w, http.StatusBadRequest, "token must be a hex address")
		return
	}
	amountIn, ok := new(big.Int).SetString(q.Get("amountIn"), 10)
	if !ok || amountIn.Sign() <= 0 {
		writeError(w, http.StatusBadRequest, "amountIn must be a positive decimal integer (wei)")
		return
	}
	slippage := s.d.Quote.DefaultSlippageBps()
	if raw := q.Get("slippageBps"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 || v > 5000 {
			writeError(w, http.StatusBadRequest, "slippageBps must be an integer between 0 and 5000")
			return
		}
		slippage = v
	}
	if !s.d.Quote.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "ferry in is not deployed on this keeper")
		return
	}
	res, err := s.d.Quote.Quote(r.Context(), token, amountIn, slippage)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, quote.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, quote.ErrUnknownToken):
		writeError(w, http.StatusNotFound, "token is not indexed")
	case errors.Is(err, quote.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.d.Log.Warn("quote failed", "err", err)
		writeError(w, http.StatusBadGateway, "quote failed: "+err.Error())
	}
}

// ---- ferry in ----

func (s *server) ferryIn(w http.ResponseWriter, r *http.Request) {
	tx := r.PathValue("hyperevmTxHash")
	if !hashRe.MatchString(tx) {
		writeError(w, http.StatusBadRequest, "hyperevmTxHash must be a 0x-prefixed 32-byte hash")
		return
	}
	if !s.d.FerryIn.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "ferry in is not deployed on this keeper")
		return
	}
	st, err := s.d.FerryIn.Status(r.Context(), tx)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, st)
	case errors.Is(err, ferryin.ErrNotFound), errors.Is(err, ferryin.ErrNotFerryIn):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ferryin.ErrTxFailed):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		s.d.Log.Warn("ferry in status failed", "tx", tx, "err", err)
		writeError(w, http.StatusBadGateway, "ferry in status failed: "+err.Error())
	}
}

// ---- exits ----

func (s *server) exitsByAddress(w http.ResponseWriter, r *http.Request) {
	addr, ok := validAddress(r.URL.Query().Get("address"))
	if !ok {
		writeError(w, http.StatusBadRequest, "address query parameter must be a hex address")
		return
	}
	var out []model.Exit
	for _, e := range s.d.Store.Exits() {
		if strings.EqualFold(e.Params.Sender, addr.Hex()) || strings.EqualFold(e.Params.Recipient, addr.Hex()) {
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, nonNil(out))
}

func (s *server) exit(w http.ResponseWriter, r *http.Request) {
	addr, ok := validAddress(r.PathValue("receiver"))
	if !ok {
		writeError(w, http.StatusBadRequest, "receiver must be a hex address")
		return
	}
	e, found := s.d.Store.Exit(addr.Hex())
	if !found {
		writeError(w, http.StatusNotFound, "exit not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// ---- rescue ----

func (s *server) rescueStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.d.Rescue.Stats())
}

func (s *server) rescueLookup(w http.ResponseWriter, r *http.Request) {
	addr, ok := validAddress(r.URL.Query().Get("address"))
	if !ok {
		writeError(w, http.StatusBadRequest, "address query parameter must be a hex address")
		return
	}
	if !s.d.Rescue.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "rescue is disabled")
		return
	}
	items, err := s.d.Rescue.Lookup(r.Context(), addr)
	if err != nil {
		if errors.Is(err, rescue.ErrDisabled) {
			writeError(w, http.StatusServiceUnavailable, "rescue is disabled")
			return
		}
		writeError(w, http.StatusBadGateway, "lookup failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nonNil(items))
}

type claimRequest struct {
	MessageID int64 `json:"messageId"`
}

func (s *server) rescueClaim(w http.ResponseWriter, r *http.Request) {
	var body claimRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON {\"messageId\": <integer>}")
		return
	}
	if body.MessageID <= 0 {
		writeError(w, http.StatusBadRequest, "messageId must be a positive integer")
		return
	}
	res, err := s.d.Rescue.ClaimForMe(r.Context(), body.MessageID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, rescue.ErrDisabled):
		writeError(w, http.StatusServiceUnavailable, "rescue is disabled or read-only")
	default:
		s.d.Log.Warn("rescue claim failed", "message_id", body.MessageID, "err", err)
		writeError(w, http.StatusBadGateway, "claim failed: "+err.Error())
	}
}

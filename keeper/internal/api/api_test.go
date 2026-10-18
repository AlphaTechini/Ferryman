package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/alphatechini/ferryman/keeper/internal/config"
	"github.com/alphatechini/ferryman/keeper/internal/ferryin"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/quote"
	"github.com/alphatechini/ferryman/keeper/internal/rescue"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

const (
	tokenAddr = "0x00000000000000000000000000000000000000aA"
	userAddr  = "0x00000000000000000000000000000000000000Bb"
	receiver  = "0x7777777777777777777777777777777777777777"
	txHash    = "0x1111111111111111111111111111111111111111111111111111111111111111"
)

type fakeQuote struct {
	enabled bool
	err     error
	last    *big.Int
}

func (f *fakeQuote) Quote(ctx context.Context, token common.Address, amountIn *big.Int, slippage int) (model.Quote, error) {
	if f.err != nil {
		return model.Quote{}, f.err
	}
	return model.Quote{Token: token.Hex(), AmountIn: amountIn.String(), Route: []string{"HYPE", "USDC", "ALP"},
		Retryable: model.Retryable{GasLimit: "1500000"}, SlippageBps: slippage}, nil
}
func (f *fakeQuote) Enabled() bool           { return f.enabled }
func (f *fakeQuote) DefaultSlippageBps() int { return 150 }

type fakeFerry struct {
	enabled bool
	err     error
}

func (f *fakeFerry) Status(ctx context.Context, tx string) (model.FerryInStatus, error) {
	if f.err != nil {
		return model.FerryInStatus{}, f.err
	}
	id := int64(9)
	return model.FerryInStatus{HyperEVMTx: tx, Stage: "relayed", MessageID: &id, Recipient: userAddr, HypeIn: "1"}, nil
}
func (f *fakeFerry) Enabled() bool { return f.enabled }

type fakeRescue struct {
	enabled  bool
	claimErr error
}

func (f *fakeRescue) Stats() model.RescueStats {
	return model.RescueStats{Withdrawals: 1, Owners: 1, HypeReturned: "5", GasSpent: "2", Recent: []model.RescueRecent{{MessageID: 1}}}
}
func (f *fakeRescue) Lookup(ctx context.Context, a common.Address) ([]model.StuckWithdrawal, error) {
	return []model.StuckWithdrawal{{MessageID: 3, Owner: a.Hex(), Amount: "1", Status: "confirmed", Claimable: true}}, nil
}
func (f *fakeRescue) ClaimForMe(ctx context.Context, id int64) (model.ClaimResult, error) {
	if f.claimErr != nil {
		return model.ClaimResult{}, f.claimErr
	}
	h := "0xabc"
	return model.ClaimResult{Status: rescue.StatusClaimed, TxHash: &h}, nil
}
func (f *fakeRescue) Enabled() bool { return f.enabled }

type harness struct {
	srv   *httptest.Server
	st    *store.Store
	quote *fakeQuote
	ferry *fakeFerry
	resc  *fakeRescue
}

func newHarness(t *testing.T, mut func(*Deps)) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	pid := int64(555)
	ts := int64(1_700_000_000)
	matures := ts + 1500
	if err := st.Update(func(s *store.State) {
		s.Tokens[store.Key(tokenAddr)] = model.Token{
			Address: tokenAddr, Name: "Alpha", Symbol: "ALP", Decimals: 18, Launcher: userAddr, LaunchTx: txHash,
			LaunchBlock: 10, LaunchedAt: ts, QuoteCurrency: "0x7D29d8047B905000459c0E80c34a26CeedcB47b2", QuoteSymbol: "USDC",
			PoolKey:   model.PoolKey{Currency0: tokenAddr, Currency1: userAddr, Hooks: userAddr, PoolManager: userAddr, Fee: 3000, Parameters: txHash},
			OurLaunch: true,
		}
		s.Exits[store.Key(receiver)] = model.Exit{
			Receiver: receiver, Salt: txHash, Stage: "confirmed", ElysiumTx: txHash, MessageID: &pid, StartedAt: ts, MaturesAt: &matures,
			Params: model.ExitParams{ChainID: 99801, Desk: userAddr, Sender: userAddr, Nonce: "0", Amount: "1000", ClaimTip: "1", Recipient: receiver},
		}
	}); err != nil {
		t.Fatal(err)
	}
	h := &harness{st: st, quote: &fakeQuote{enabled: true}, ferry: &fakeFerry{enabled: true}, resc: &fakeRescue{enabled: true}}
	deps := Deps{
		Config: &config.Config{ElysiumRPC: "https://e", HyperEVMRPC: "https://h", DefaultClaimTip: big.NewInt(500),
			Contracts: config.Contracts{FerryIn: common.HexToAddress(userAddr)}},
		Store: st, Quote: h.quote, FerryIn: h.ferry, Rescue: h.resc,
		ElysiumHead:  func(ctx context.Context) (uint64, error) { return 100, nil },
		HyperEVMHead: func(ctx context.Context) (uint64, error) { return 200, nil },
		Keeper:       common.HexToAddress(userAddr),
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mut != nil {
		mut(&deps)
	}
	h.srv = httptest.NewServer(Handler(deps))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) get(t *testing.T, path string) (int, map[string]any, []any) {
	t.Helper()
	resp, err := http.Get(h.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var obj map[string]any
	var arr []any
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(body, &arr); err != nil {
			t.Fatalf("bad JSON array for %s: %v (%s)", path, err, body)
		}
	} else if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("bad JSON for %s: %v (%s)", path, err, body)
	}
	return resp.StatusCode, obj, arr
}

func keysOf(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func wantKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := keysOf(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s keys = %v, want %v", what, got, want)
	}
}

func TestHealthShape(t *testing.T) {
	h := newHarness(t, nil)
	code, body, _ := h.get(t, "/api/health")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	wantKeys(t, "health", body, "ok", "elysiumBlock", "hyperevmBlock", "keeper", "dryRun")
	if body["ok"] != true || body["elysiumBlock"].(float64) != 100 || body["hyperevmBlock"].(float64) != 200 {
		t.Fatalf("health = %v", body)
	}
	if body["keeper"] != common.HexToAddress(userAddr).Hex() {
		t.Fatalf("keeper = %v", body["keeper"])
	}
}

func TestHealthReportsRPCErrorAsOkFalse(t *testing.T) {
	h := newHarness(t, func(d *Deps) {
		d.ElysiumHead = func(ctx context.Context) (uint64, error) { return 0, errors.New("down") }
	})
	_, body, _ := h.get(t, "/api/health")
	if body["ok"] != false || !strings.Contains(body["error"].(string), "down") || body["elysiumBlock"] != nil {
		t.Fatalf("health with failure = %v", body)
	}
}

func TestConfigShape(t *testing.T) {
	h := newHarness(t, nil)
	_, body, _ := h.get(t, "/api/config")
	wantKeys(t, "config", body, "elysium", "hyperevm", "contracts", "feeBps", "defaultSlippageBps", "defaultClaimTip")
	wantKeys(t, "elysium", body["elysium"].(map[string]any), "chainId", "rpc", "explorer")
	wantKeys(t, "contracts", body["contracts"].(map[string]any), "ferryIn", "exitDesk", "exitFactory", "feeJar", "inbox", "outbox")
	if body["defaultClaimTip"] != "500" || body["feeBps"].(float64) != 30 || body["defaultSlippageBps"].(float64) != 150 {
		t.Fatalf("config values: %v", body)
	}
}

func TestTokensShapeAndEmptyList(t *testing.T) {
	h := newHarness(t, nil)
	_, _, arr := h.get(t, "/api/tokens")
	if len(arr) != 1 {
		t.Fatalf("tokens = %d", len(arr))
	}
	tok := arr[0].(map[string]any)
	wantKeys(t, "token", tok, "address", "name", "symbol", "decimals", "launcher", "launchTx", "launchBlock",
		"launchedAt", "quoteCurrency", "quoteSymbol", "poolKey", "priceHype", "ourLaunch")
	wantKeys(t, "poolKey", tok["poolKey"].(map[string]any), "currency0", "currency1", "hooks", "poolManager", "fee", "parameters")
	if tok["priceHype"] != nil || tok["ourLaunch"] != true {
		t.Fatalf("token values wrong: %v", tok)
	}

	empty := newHarness(t, func(d *Deps) {})
	if err := empty.st.Update(func(s *store.State) { s.Tokens = map[string]model.Token{} }); err != nil {
		t.Fatal(err)
	}
	resp, _ := http.Get(empty.srv.URL + "/api/tokens")
	b, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("empty list must be [], got %s", b)
	}
}

func TestQuoteValidationAndShape(t *testing.T) {
	h := newHarness(t, nil)
	cases := map[string]int{
		"/api/quote?token=nope&amountIn=1":                                400,
		"/api/quote?token=" + tokenAddr + "&amountIn=0":                   400,
		"/api/quote?token=" + tokenAddr + "&amountIn=x":                   400,
		"/api/quote?token=" + tokenAddr + "&amountIn=1&slippageBps=99999": 400,
	}
	for path, want := range cases {
		if code, body, _ := h.get(t, path); code != want || body["error"] == nil {
			t.Fatalf("%s: status %d body %v, want %d with error", path, code, body, want)
		}
	}
	code, body, _ := h.get(t, "/api/quote?token="+tokenAddr+"&amountIn=1000&slippageBps=150")
	if code != 200 {
		t.Fatalf("quote status %d: %v", code, body)
	}
	wantKeys(t, "quote", body, "token", "amountIn", "fee", "amountOut", "minOut", "slippageBps", "priceImpactBps",
		"route", "simulatedAtBlock", "retryable")
	wantKeys(t, "retryable", body["retryable"].(map[string]any), "gasLimit", "maxFeePerGas", "maxSubmissionCost", "totalValue")
	if body["amountIn"] != "1000" {
		t.Fatalf("amountIn must be a decimal string: %v", body["amountIn"])
	}

	h.quote.err = quote.ErrUnknownToken
	if code, _, _ := h.get(t, "/api/quote?token="+tokenAddr+"&amountIn=1"); code != 404 {
		t.Fatalf("unknown token status %d", code)
	}
	h.quote.err = nil
	h.quote.enabled = false
	if code, _, _ := h.get(t, "/api/quote?token="+tokenAddr+"&amountIn=1"); code != 503 {
		t.Fatalf("disabled ferry in status %d, want 503", code)
	}
}

func TestFerryInStatusShapeAndErrors(t *testing.T) {
	h := newHarness(t, nil)
	if code, _, _ := h.get(t, "/api/ferry-in/0x1234"); code != 400 {
		t.Fatalf("bad hash status %d", code)
	}
	code, body, _ := h.get(t, "/api/ferry-in/"+txHash)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	wantKeys(t, "ferry-in", body, "hyperevmTx", "stage", "messageId", "elysiumTx", "token", "recipient", "hypeIn", "amountOut", "updatedAt")
	h.ferry.err = ferryin.ErrNotFound
	if code, _, _ := h.get(t, "/api/ferry-in/"+txHash); code != 404 {
		t.Fatalf("not found status %d", code)
	}
	h.ferry.err = ferryin.ErrTxFailed
	if code, _, _ := h.get(t, "/api/ferry-in/"+txHash); code != 422 {
		t.Fatalf("failed tx status %d", code)
	}
	h.ferry.err = nil
	h.ferry.enabled = false
	if code, _, _ := h.get(t, "/api/ferry-in/"+txHash); code != 503 {
		t.Fatalf("disabled status %d", code)
	}
}

func TestExitsFilterAndShape(t *testing.T) {
	h := newHarness(t, nil)
	if code, _, _ := h.get(t, "/api/exits"); code != 400 {
		t.Fatalf("missing address status %d", code)
	}
	if code, _, _ := h.get(t, "/api/exits?address=bad"); code != 400 {
		t.Fatalf("bad address status %d", code)
	}
	_, _, arr := h.get(t, "/api/exits?address="+strings.ToLower(userAddr))
	if len(arr) != 1 {
		t.Fatalf("sender filter returned %d", len(arr))
	}
	_, _, none := h.get(t, "/api/exits?address=0x0000000000000000000000000000000000000001")
	if none == nil || len(none) != 0 {
		t.Fatalf("no match must be []: %v", none)
	}
	_, body, _ := h.get(t, "/api/exits/"+receiver)
	wantKeys(t, "exit", body, "receiver", "salt", "params", "stage", "elysiumTx", "messageId", "startedAt", "maturesAt", "claimTx", "deliveredAt")
	wantKeys(t, "params", body["params"].(map[string]any), "chainId", "desk", "sender", "nonce", "amount", "claimTip", "recipient", "toCore")
	if body["params"].(map[string]any)["chainId"].(float64) != 99801 {
		t.Fatal("params.chainId must be a number")
	}
	if code, _, _ := h.get(t, "/api/exits/0x0000000000000000000000000000000000000002"); code != 404 {
		t.Fatalf("missing exit status %d", code)
	}
	if code, _, _ := h.get(t, "/api/exits/nope"); code != 400 {
		t.Fatalf("bad receiver status %d", code)
	}
}

func TestRescueStatsAndLookup(t *testing.T) {
	h := newHarness(t, nil)
	_, body, _ := h.get(t, "/api/rescue/stats")
	wantKeys(t, "rescue stats", body, "withdrawals", "owners", "hypeReturned", "gasSpent", "lastRunAt", "recent")
	wantKeys(t, "recent item", body["recent"].([]any)[0].(map[string]any), "messageId", "owner", "amount", "claimTx", "claimedAt")

	_, _, arr := h.get(t, "/api/rescue/lookup?address="+userAddr)
	if len(arr) != 1 {
		t.Fatalf("lookup returned %d", len(arr))
	}
	wantKeys(t, "stuck", arr[0].(map[string]any), "messageId", "owner", "amount", "originTx", "status", "ageSeconds", "claimable")
	if code, _, _ := h.get(t, "/api/rescue/lookup?address=zz"); code != 400 {
		t.Fatalf("bad address status %d", code)
	}
	h.resc.enabled = false
	if code, _, _ := h.get(t, "/api/rescue/lookup?address="+userAddr); code != 503 {
		t.Fatalf("disabled lookup status %d", code)
	}
}

func post(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestRescueClaimShapeAndValidation(t *testing.T) {
	h := newHarness(t, nil)
	code, body := post(t, h.srv.URL+"/api/rescue/claim", `{"messageId": 573261}`)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	wantKeys(t, "claim result", body, "status", "txHash")
	if body["status"] != "claimed" || body["txHash"] != "0xabc" {
		t.Fatalf("claim result: %v", body)
	}
	for _, bad := range []string{`{"messageId": 0}`, `{"messageId": -1}`, `not json`, `{"messageId": "x"}`, ``} {
		if code, _ := post(t, h.srv.URL+"/api/rescue/claim", bad); code != 400 {
			t.Fatalf("body %q: status %d, want 400", bad, code)
		}
	}
	h.resc.claimErr = rescue.ErrDisabled
	if code, _ := post(t, h.srv.URL+"/api/rescue/claim", `{"messageId": 1}`); code != 503 {
		t.Fatalf("read-only claim status %d, want 503", code)
	}
}

func TestCORSAndJSONErrors(t *testing.T) {
	h := newHarness(t, nil)
	resp, err := http.Get(h.srv.URL + "/api/tokens")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("CORS header missing on GET")
	}
	req, _ := http.NewRequest(http.MethodOptions, h.srv.URL+"/api/quote", nil)
	pre, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	pre.Body.Close()
	if pre.StatusCode != http.StatusNoContent || pre.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight: %d %v", pre.StatusCode, pre.Header)
	}
	code, body, _ := h.get(t, "/api/nope")
	if code != 404 || body["error"] == nil {
		t.Fatalf("unknown route must be a JSON error, got %d %v", code, body)
	}
}

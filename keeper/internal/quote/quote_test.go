package quote

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

var (
	ferry = common.HexToAddress("0x00000000000000000000000000000000000000f1")
	inbox = common.HexToAddress("0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0")
	token = common.HexToAddress("0x00000000000000000000000000000000000000aa")
)

// elysium answers FerryIn.quote. The rate is 1:1 up to 1e15 wei and 0.9 above, and estimates fail.
type elysium struct {
	chain.Reader
	head uint64
}

func (e *elysium) BlockNumber(ctx context.Context) (uint64, error) { return e.head, nil }

func (e *elysium) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (e *elysium) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	return 0, errors.New("estimate unavailable from placeholder sender")
}

func (e *elysium) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if string(call.Data[:4]) != string(abis.FerryIn.Methods["quote"].ID) {
		return nil, errors.New("unexpected call")
	}
	args, err := abis.FerryIn.Methods["quote"].Inputs.Unpack(call.Data[4:])
	if err != nil {
		return nil, err
	}
	in := args[1].(*big.Int)
	out := new(big.Int).Set(in)
	if in.Cmp(big.NewInt(1_000_000_000_000_000)) > 0 {
		out.Mul(out, big.NewInt(9))
		out.Div(out, big.NewInt(10))
	}
	return abis.FerryIn.Methods["quote"].Outputs.Pack(out)
}

// hyperevm answers the Inbox submission fee and serves a base fee.
type hyperevm struct {
	chain.Reader
	fee     *big.Int
	baseFee *big.Int
}

func (h *hyperevm) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if *call.To != inbox {
		return nil, errors.New("wrong target")
	}
	return abis.Inbox.Methods["calculateRetryableSubmissionFee"].Outputs.Pack(h.fee)
}

func (h *hyperevm) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	return &types.Header{Number: n, BaseFee: h.baseFee}, nil
}

func newService(t *testing.T, ferryAddr common.Address, el *elysium, hy *hyperevm) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(s *store.State) {
		s.Tokens[store.Key(token.Hex())] = model.Token{Address: token.Hex(), Symbol: "ALP", QuoteSymbol: "USDC"}
	}); err != nil {
		t.Fatal(err)
	}
	return New(Settings{FerryIn: ferryAddr, Inbox: inbox, FeeBps: 30, DefaultSlippageBps: 150}, el, hy, st,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestQuoteNumbers(t *testing.T) {
	svc := newService(t, ferry, &elysium{head: 777}, &hyperevm{fee: big.NewInt(1000), baseFee: big.NewInt(1)})
	amountIn := big.NewInt(1_000_000_000_000_000_000) // 1 HYPE
	q, err := svc.Quote(context.Background(), token, amountIn, 150)
	if err != nil {
		t.Fatal(err)
	}
	if q.AmountOut != "900000000000000000" {
		t.Fatalf("amountOut = %s", q.AmountOut)
	}
	if q.MinOut != "886500000000000000" { // 0.9 * (1 - 1.5%)
		t.Fatalf("minOut = %s", q.MinOut)
	}
	if q.Fee != "3000000000000000" { // 30 bps of 1 HYPE
		t.Fatalf("fee = %s", q.Fee)
	}
	if q.SimulatedAtBlock != 777 || q.SlippageBps != 150 {
		t.Fatalf("block/slippage wrong: %+v", q)
	}
	if q.PriceImpactBps != 1000 {
		t.Fatalf("priceImpactBps = %d, want 1000 (rate drops 10%% at this size)", q.PriceImpactBps)
	}
	if len(q.Route) != 3 || q.Route[0] != "HYPE" || q.Route[1] != "USDC" || q.Route[2] != "ALP" {
		t.Fatalf("route = %v", q.Route)
	}
	r := q.Retryable
	if r.GasLimit != "1500000" || r.MaxFeePerGas != "2000000000" {
		t.Fatalf("gas params: %+v", r)
	}
	if r.MaxSubmissionCost != "4000" { // 4 * inbox fee (100 bytes, baseFee 0)
		t.Fatalf("maxSubmissionCost = %s", r.MaxSubmissionCost)
	}
	want := new(big.Int).Add(amountIn, big.NewInt(4000))
	want.Add(want, new(big.Int).Mul(big.NewInt(1_500_000), big.NewInt(2_000_000_000)))
	if r.TotalValue != want.String() {
		t.Fatalf("totalValue = %s, want %s", r.TotalValue, want)
	}
}

func TestSubmissionFeeFallbackFormula(t *testing.T) {
	// No inbox configured: (1400 + 6*100) * baseFee, times the factor 4 applied by the caller.
	hy := &hyperevm{baseFee: big.NewInt(10)}
	svc := newService(t, ferry, &elysium{head: 1}, hy)
	svc.s.Inbox = common.Address{}
	fee, err := svc.submissionFee(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if fee.Int64() != 2000*10 {
		t.Fatalf("fallback fee = %s, want 20000", fee)
	}
}

func TestErrors(t *testing.T) {
	hy := &hyperevm{fee: big.NewInt(1), baseFee: big.NewInt(1)}
	svc := newService(t, ferry, &elysium{head: 1}, hy)
	if _, err := svc.Quote(context.Background(), common.HexToAddress("0x1"), big.NewInt(1), 150); !errors.Is(err, ErrUnknownToken) {
		t.Fatalf("unknown token: %v", err)
	}
	if _, err := svc.Quote(context.Background(), token, big.NewInt(0), 150); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero amount: %v", err)
	}
	if _, err := svc.Quote(context.Background(), token, big.NewInt(1), 9999); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad slippage: %v", err)
	}
	off := newService(t, common.Address{}, &elysium{head: 1}, hy)
	if _, err := off.Quote(context.Background(), token, big.NewInt(1), 150); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("not deployed: %v", err)
	}
	if off.Enabled() {
		t.Fatal("service must report disabled")
	}
}

// Package quote produces Ferry In quotes: a FerryIn.quote simulation on Elysium, the slippage floor,
// and the retryable ticket gas parameters for the HyperEVM submission.
package quote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/model"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

// Errors returned by Quote. Use errors.Is.
var (
	ErrNotConfigured = errors.New("ferry in is not deployed on this keeper")
	ErrUnknownToken  = errors.New("token is not indexed")
	ErrInvalidInput  = errors.New("invalid quote input")
)

const (
	minGasLimit      = 1_500_000
	submissionFactor = 4
	maxFeeFactor     = 2
	bpsDenominator   = 10_000
	minImpactProbe   = 1_000_000_000_000 // 1e12 wei
)

// Placeholder addresses used for gas estimation when no keeper address is configured.
var placeholderRecipient = common.HexToAddress("0x0000000000000000000000000000000000000001")
var placeholderSender = common.HexToAddress("0x000000000000000000000000000000000000dEaD")

// Settings configures the quote service.
type Settings struct {
	FerryIn            common.Address // zero disables quotes (503)
	Inbox              common.Address // HyperEVM Inbox, zero means fallback formula
	FeeBps             uint64
	DefaultSlippageBps int
	Keeper             common.Address // optional estimate sender (should hold HYPE)
}

// Service quotes Ferry In buys.
type Service struct {
	s   Settings
	el  chain.Reader
	hy  chain.Reader
	st  *store.Store
	log *slog.Logger
}

// New creates a quote service.
func New(s Settings, el, hy chain.Reader, st *store.Store, log *slog.Logger) *Service {
	return &Service{s: s, el: el, hy: hy, st: st, log: log}
}

// Enabled reports whether FerryIn is deployed.
func (s *Service) Enabled() bool { return s.s.FerryIn != (common.Address{}) }

// DefaultSlippageBps returns the default slippage used when none is given.
func (s *Service) DefaultSlippageBps() int { return s.s.DefaultSlippageBps }

// FeeBps returns the protocol fee in basis points.
func (s *Service) FeeBps() uint64 { return s.s.FeeBps }

// Quote returns a Ferry In quote for buying token with amountIn wei of HYPE.
func (s *Service) Quote(ctx context.Context, token common.Address, amountIn *big.Int, slippageBps int) (model.Quote, error) {
	if !s.Enabled() {
		return model.Quote{}, ErrNotConfigured
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return model.Quote{}, fmt.Errorf("%w: amountIn must be positive", ErrInvalidInput)
	}
	if slippageBps < 0 || slippageBps > 5000 {
		return model.Quote{}, fmt.Errorf("%w: slippageBps must be between 0 and 5000", ErrInvalidInput)
	}
	t, ok := s.st.Token(token.Hex())
	if !ok {
		return model.Quote{}, ErrUnknownToken
	}

	block, err := s.el.BlockNumber(ctx)
	if err != nil {
		return model.Quote{}, fmt.Errorf("elysium head: %w", err)
	}
	amountOut, err := s.quoteOut(ctx, token, amountIn)
	if err != nil {
		return model.Quote{}, fmt.Errorf("quote simulation: %w", err)
	}
	minOut := new(big.Int).Mul(amountOut, big.NewInt(int64(bpsDenominator-slippageBps)))
	minOut.Div(minOut, big.NewInt(bpsDenominator))

	fee := new(big.Int).Mul(amountIn, new(big.Int).SetUint64(s.s.FeeBps))
	fee.Div(fee, big.NewInt(bpsDenominator))

	retry, err := s.retryable(ctx, token, minOut, amountIn)
	if err != nil {
		return model.Quote{}, err
	}

	return model.Quote{
		Token:            token.Hex(),
		AmountIn:         amountIn.String(),
		Fee:              fee.String(),
		AmountOut:        amountOut.String(),
		MinOut:           minOut.String(),
		SlippageBps:      slippageBps,
		PriceImpactBps:   s.priceImpactBps(ctx, token, amountIn, amountOut),
		Route:            []string{"HYPE", t.QuoteSymbol, t.Symbol},
		SimulatedAtBlock: block,
		Retryable:        retry,
	}, nil
}

func (s *Service) quoteOut(ctx context.Context, token common.Address, amountIn *big.Int) (*big.Int, error) {
	data, err := abis.PackQuote(token, amountIn)
	if err != nil {
		return nil, err
	}
	to := s.s.FerryIn
	ret, err := s.el.CallContract(ctx, ethereum.CallMsg{From: s.sender(), To: &to, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	return abis.UnpackUint(abis.FerryIn, "quote", ret)
}

// priceImpactBps compares the quote rate with a tiny probe (amountIn/1000, at least 1e12 wei).
// Best effort: 0 when the probe is not meaningful or fails.
func (s *Service) priceImpactBps(ctx context.Context, token common.Address, amountIn, amountOut *big.Int) int {
	probe := new(big.Int).Div(amountIn, big.NewInt(1000))
	if probe.Cmp(big.NewInt(minImpactProbe)) < 0 {
		probe = big.NewInt(minImpactProbe)
	}
	if probe.Cmp(amountIn) >= 0 {
		return 0
	}
	tinyOut, err := s.quoteOut(ctx, token, probe)
	if err != nil || tinyOut.Sign() == 0 {
		return 0
	}
	expected := new(big.Int).Mul(tinyOut, amountIn)
	expected.Div(expected, probe)
	if expected.Sign() == 0 || expected.Cmp(amountOut) <= 0 {
		return 0
	}
	diff := new(big.Int).Sub(expected, amountOut)
	diff.Mul(diff, big.NewInt(bpsDenominator))
	diff.Div(diff, expected)
	if !diff.IsInt64() || diff.Int64() > 1_000_000 {
		return 0
	}
	return int(diff.Int64())
}

// retryable computes the gas parameters for the Inbox.createRetryableTicket call (RESEARCH implications 5).
func (s *Service) retryable(ctx context.Context, token common.Address, minOut, amountIn *big.Int) (model.Retryable, error) {
	gasPrice, err := s.el.SuggestGasPrice(ctx)
	if err != nil {
		return model.Retryable{}, fmt.Errorf("elysium gas price: %w", err)
	}
	maxFee := new(big.Int).Mul(gasPrice, big.NewInt(maxFeeFactor))

	buy, err := abis.PackBuy(token, placeholderRecipient, minOut)
	if err != nil {
		return model.Retryable{}, err
	}
	gasLimit := big.NewInt(minGasLimit)
	to := s.s.FerryIn
	est, err := s.el.EstimateGas(ctx, ethereum.CallMsg{From: s.sender(), To: &to, Data: buy, Value: amountIn})
	if err != nil {
		s.log.Debug("buy gas estimate failed, using floor", "err", err)
	} else {
		scaled := new(big.Int).Mul(new(big.Int).SetUint64(est), big.NewInt(3))
		scaled.Div(scaled, big.NewInt(2))
		if scaled.Cmp(gasLimit) > 0 {
			gasLimit = scaled
		}
	}

	subFee, err := s.submissionFee(ctx, len(buy))
	if err != nil {
		return model.Retryable{}, err
	}
	maxSubmission := new(big.Int).Mul(subFee, big.NewInt(submissionFactor))

	total := new(big.Int).Set(amountIn)
	total.Add(total, maxSubmission)
	total.Add(total, new(big.Int).Mul(gasLimit, maxFee))

	return model.Retryable{
		GasLimit:          gasLimit.String(),
		MaxFeePerGas:      maxFee.String(),
		MaxSubmissionCost: maxSubmission.String(),
		TotalValue:        total.String(),
	}, nil
}

// submissionFee returns the Inbox submission fee for calldata of the given length (before the factor).
func (s *Service) submissionFee(ctx context.Context, dataLen int) (*big.Int, error) {
	if s.s.Inbox != (common.Address{}) {
		data, err := abis.PackSubmissionFee(big.NewInt(int64(dataLen)), big.NewInt(0))
		if err == nil {
			inbox := s.s.Inbox
			ret, cErr := s.hy.CallContract(ctx, ethereum.CallMsg{To: &inbox, Data: data}, nil)
			if cErr == nil {
				if fee, uErr := abis.UnpackUint(abis.Inbox, "calculateRetryableSubmissionFee", ret); uErr == nil {
					return fee, nil
				}
			}
		}
		s.log.Debug("inbox submission fee call failed, using formula")
	}
	baseFee, err := s.hyperBaseFee(ctx)
	if err != nil {
		return nil, fmt.Errorf("hyperevm base fee: %w", err)
	}
	fee := new(big.Int).Mul(big.NewInt(int64(1400+6*dataLen)), baseFee)
	return fee, nil
}

func (s *Service) hyperBaseFee(ctx context.Context) (*big.Int, error) {
	h, err := s.hy.HeaderByNumber(ctx, nil)
	if err == nil && h.BaseFee != nil {
		return h.BaseFee, nil
	}
	return s.hy.SuggestGasPrice(ctx)
}

func (s *Service) sender() common.Address {
	if s.s.Keeper != (common.Address{}) {
		return s.s.Keeper
	}
	return placeholderSender
}

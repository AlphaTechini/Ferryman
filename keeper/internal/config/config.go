// Package config loads the keeper configuration from environment variables and the deployments file.
// Missing contract addresses (zero or absent) disable the dependent feature and add a warning; they
// never stop the process.
package config

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Fixed network constants.
const (
	ElysiumChainID      uint64 = 99801
	HyperEVMChainID     uint64 = 998
	Confirmations       uint64 = 3 // reads stay this many blocks behind head
	FeeBps              uint64 = 30
	DefaultSlippageBps  int    = 150
	ElysiumExplorerURL         = "https://elysium.kinetiq.xyz/testnet-explorer"
	HyperEVMExplorerURL        = "https://testnet.purrsec.com"
	DefaultClaimTipWei         = "500000000000000" // 0.0005 HYPE
	defaultElysiumRPC          = "https://testnet-rpc.elysium.kinetiq.xyz"
	defaultHyperEVMRPC         = "https://rpc.hyperliquid-testnet.xyz/evm"
	defaultExplorerAPI         = "https://testnet-api.elysium.kinetiq.xyz/api/v2"
	defaultLaunchpad           = "0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d"
	canonicalUSDC              = "0x7D29d8047B905000459c0E80c34a26CeedcB47b2"
	mockUSDC                   = "0x84Cc00D13173f7016Bbab22A736dBf75A154cFE2"
)

// Contracts are the deployed addresses. The zero address means "not deployed".
type Contracts struct {
	FerryIn     common.Address
	ExitDesk    common.Address
	FeeJar      common.Address
	SwapAdapter common.Address
	WHYPE       common.Address
	ExitFactory common.Address
	Inbox       common.Address
	Outbox      common.Address
}

// Config is the fully resolved keeper configuration.
type Config struct {
	ElysiumRPC      string
	HyperEVMRPC     string
	ExplorerAPI     string
	KeeperKey       string // raw hex, empty means read-only
	DryRun          bool
	ListenAddr      string
	DataDir         string
	DeploymentsPath string

	Launchpad       common.Address
	CLPoolManager   common.Address
	QuoteCurrencies []common.Address
	OurLaunchers    []common.Address

	IndexFromBlock  uint64
	LogRange        uint64
	PollInterval    time.Duration
	DefaultClaimTip *big.Int

	RescueEnabled   bool
	RescueMinAge    time.Duration
	RescueLookback  time.Duration
	RescueMaxPerRun int

	Contracts Contracts

	// Warnings collects non-fatal configuration problems for the log.
	Warnings []string
}

// deploymentsFile mirrors deployments/testnet.json.
type deploymentsFile struct {
	Elysium struct {
		ChainID     uint64 `json:"chainId"`
		FerryIn     string `json:"ferryIn"`
		ExitDesk    string `json:"exitDesk"`
		FeeJar      string `json:"feeJar"`
		SwapAdapter string `json:"swapAdapter"`
		WHYPE       string `json:"whype"`
	} `json:"elysium"`
	HyperEVM struct {
		ChainID     uint64 `json:"chainId"`
		ExitFactory string `json:"exitFactory"`
		Inbox       string `json:"inbox"`
		Outbox      string `json:"outbox"`
	} `json:"hyperevm"`
}

// Load reads configuration. getenv is os.Getenv in production and a map lookup in tests.
func Load(getenv func(string) string) (*Config, error) {
	var errs []string
	addErr := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	c := &Config{}

	c.ElysiumRPC = strOr(getenv("ELYSIUM_RPC"), defaultElysiumRPC)
	c.HyperEVMRPC = strOr(getenv("HYPEREVM_RPC"), defaultHyperEVMRPC)
	c.ExplorerAPI = strOr(getenv("EXPLORER_API"), defaultExplorerAPI)
	c.KeeperKey = strings.TrimSpace(getenv("KEEPER_PRIVATE_KEY"))
	c.ListenAddr = strOr(getenv("LISTEN_ADDR"), ":8787")
	c.DataDir = strOr(getenv("DATA_DIR"), "./data")
	c.DeploymentsPath = strOr(getenv("DEPLOYMENTS"), "../deployments/testnet.json")

	var err error
	if c.DryRun, err = boolEnv(getenv("DRY_RUN"), false); err != nil {
		addErr("DRY_RUN: %v", err)
	}
	if c.RescueEnabled, err = boolEnv(getenv("RESCUE_ENABLED"), true); err != nil {
		addErr("RESCUE_ENABLED: %v", err)
	}
	c.Launchpad, err = addressEnv(getenv("LAUNCHPAD"), defaultLaunchpad)
	if err != nil {
		addErr("LAUNCHPAD: %v", err)
	}
	c.CLPoolManager, err = addressEnv(getenv("CL_POOL_MANAGER"), "")
	if err != nil {
		addErr("CL_POOL_MANAGER: %v", err)
	}
	if c.IndexFromBlock, err = uintEnv(getenv("INDEX_FROM_BLOCK"), 0); err != nil {
		addErr("INDEX_FROM_BLOCK: %v", err)
	}
	if c.LogRange, err = uintEnv(getenv("LOG_RANGE"), 5000); err != nil || c.LogRange == 0 {
		addErr("LOG_RANGE must be a positive integer")
	}
	if c.RescueMaxPerRun, err = intEnv(getenv("RESCUE_MAX_PER_RUN"), 10); err != nil || c.RescueMaxPerRun <= 0 {
		addErr("RESCUE_MAX_PER_RUN must be a positive integer")
	}
	if c.PollInterval, err = durationEnv(getenv("POLL_INTERVAL"), 20*time.Second); err != nil {
		addErr("POLL_INTERVAL: %v", err)
	}
	if c.RescueMinAge, err = durationEnv(getenv("RESCUE_MIN_AGE"), 24*time.Hour); err != nil {
		addErr("RESCUE_MIN_AGE: %v", err)
	}
	if c.RescueLookback, err = durationEnv(getenv("RESCUE_LOOKBACK"), 30*24*time.Hour); err != nil {
		addErr("RESCUE_LOOKBACK: %v", err)
	}
	tipStr := strOr(getenv("DEFAULT_CLAIM_TIP_WEI"), DefaultClaimTipWei)
	tip, ok := new(big.Int).SetString(tipStr, 10)
	if !ok || tip.Sign() < 0 {
		addErr("DEFAULT_CLAIM_TIP_WEI must be a non-negative integer (wei)")
	}
	c.DefaultClaimTip = tip

	if c.OurLaunchers, err = addressList(getenv("OUR_LAUNCHERS")); err != nil {
		addErr("OUR_LAUNCHERS: %v", err)
	}
	quotes := []common.Address{common.HexToAddress(canonicalUSDC), common.HexToAddress(mockUSDC)}
	extra, err := addressList(getenv("QUOTE_CURRENCIES"))
	if err != nil {
		addErr("QUOTE_CURRENCIES: %v", err)
	}
	c.QuoteCurrencies = dedupe(append(quotes, extra...))

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid configuration: %s", strings.Join(errs, "; "))
	}

	if err := c.loadDeployments(); err != nil {
		c.Warnings = append(c.Warnings, err.Error())
	}
	c.warnDisabled()
	return c, nil
}

func (c *Config) loadDeployments() error {
	b, err := os.ReadFile(c.DeploymentsPath)
	if err != nil {
		return fmt.Errorf("deployments file %s not readable (%v): all contract addresses are zero", c.DeploymentsPath, err)
	}
	var d deploymentsFile
	if err := json.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("deployments file %s: %v: all contract addresses are zero", c.DeploymentsPath, err)
	}
	var errs []string
	parse := func(name, v string) common.Address {
		a, err := parseDeployedAddress(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
		return a
	}
	c.Contracts = Contracts{
		FerryIn:     parse("elysium.ferryIn", d.Elysium.FerryIn),
		ExitDesk:    parse("elysium.exitDesk", d.Elysium.ExitDesk),
		FeeJar:      parse("elysium.feeJar", d.Elysium.FeeJar),
		SwapAdapter: parse("elysium.swapAdapter", d.Elysium.SwapAdapter),
		WHYPE:       parse("elysium.whype", d.Elysium.WHYPE),
		ExitFactory: parse("hyperevm.exitFactory", d.HyperEVM.ExitFactory),
		Inbox:       parse("hyperevm.inbox", d.HyperEVM.Inbox),
		Outbox:      parse("hyperevm.outbox", d.HyperEVM.Outbox),
	}
	if len(errs) > 0 {
		return fmt.Errorf("deployments file %s: %s", c.DeploymentsPath, strings.Join(errs, "; "))
	}
	return nil
}

// warnDisabled records one warning per feature that will be disabled by configuration.
func (c *Config) warnDisabled() {
	w := func(format string, a ...any) { c.Warnings = append(c.Warnings, fmt.Sprintf(format, a...)) }
	if c.KeeperKey == "" {
		w("KEEPER_PRIVATE_KEY is empty: running read-only (no claims, no registrations, no redeems)")
	}
	if c.DryRun {
		w("DRY_RUN is on: transactions are simulated only")
	}
	if c.CLPoolManager == (common.Address{}) {
		w("CL_POOL_MANAGER is not set: token indexing is disabled")
	}
	if c.Contracts.ExitDesk == (common.Address{}) {
		w("elysium.exitDesk is zero: exit indexing is disabled")
	}
	if c.Contracts.ExitFactory == (common.Address{}) {
		w("hyperevm.exitFactory is zero: Ferry Out claims are disabled")
	}
	if c.Contracts.FerryIn == (common.Address{}) {
		w("elysium.ferryIn is zero: Ferry In quotes return 503 and token registration is disabled")
	}
	if c.Contracts.Outbox == (common.Address{}) {
		w("hyperevm.outbox is zero: Rescue and Ferry Out claims are disabled")
	}
	if c.Contracts.Inbox == (common.Address{}) {
		w("hyperevm.inbox is zero: retryable gas uses the fallback formula")
	}
	if !c.RescueEnabled {
		w("RESCUE_ENABLED is false")
	}
}

// RescueActive reports whether the Rescue loop can run.
func (c *Config) RescueActive() bool {
	return c.RescueEnabled && c.KeeperKey != "" && c.Contracts.Outbox != (common.Address{})
}

// ---- helpers ----

func strOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

func boolEnv(v string, def bool) (bool, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	return strconv.ParseBool(strings.TrimSpace(v))
}

func uintEnv(v string, def uint64) (uint64, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	return strconv.ParseUint(strings.TrimSpace(v), 10, 64)
}

func intEnv(v string, def int) (int, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	return strconv.Atoi(strings.TrimSpace(v))
}

func durationEnv(v string, def time.Duration) (time.Duration, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	return time.ParseDuration(strings.TrimSpace(v))
}

func addressEnv(v, def string) (common.Address, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		s = def
	}
	if s == "" {
		return common.Address{}, nil
	}
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("%q is not a hex address", s)
	}
	return common.HexToAddress(s), nil
}

func addressList(v string) ([]common.Address, error) {
	var out []common.Address
	for _, part := range strings.Split(v, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if !common.IsHexAddress(p) {
			return nil, fmt.Errorf("%q is not a hex address", p)
		}
		out = append(out, common.HexToAddress(p))
	}
	return out, nil
}

// parseDeployedAddress accepts "" and "0x0" as the zero address.
func parseDeployedAddress(s string) (common.Address, error) {
	t := strings.TrimSpace(s)
	if t == "" || t == "0x0" {
		return common.Address{}, nil
	}
	if !common.IsHexAddress(t) {
		return common.Address{}, fmt.Errorf("%q is not a hex address", t)
	}
	return common.HexToAddress(t), nil
}

func dedupe(in []common.Address) []common.Address {
	seen := map[common.Address]bool{}
	var out []common.Address
	for _, a := range in {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

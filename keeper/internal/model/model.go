// Package model holds the JSON wire types of the keeper HTTP API. The shapes follow
// docs/INTERFACES.md section 3 exactly; the store persists the same types.
package model

// PoolKey is the Infinity CL pool key (field order follows PoolKey.sol).
type PoolKey struct {
	Currency0   string `json:"currency0"`
	Currency1   string `json:"currency1"`
	Hooks       string `json:"hooks"`
	PoolManager string `json:"poolManager"`
	Fee         uint32 `json:"fee"`
	Parameters  string `json:"parameters"`
}

// Token is an Ascend launch discovered from CLPoolManager Initialize events.
type Token struct {
	Address       string  `json:"address"`
	Name          string  `json:"name"`
	Symbol        string  `json:"symbol"`
	Decimals      uint8   `json:"decimals"`
	Launcher      string  `json:"launcher"`
	LaunchTx      string  `json:"launchTx"`
	LaunchBlock   uint64  `json:"launchBlock"`
	LaunchedAt    int64   `json:"launchedAt"`
	QuoteCurrency string  `json:"quoteCurrency"`
	QuoteSymbol   string  `json:"quoteSymbol"`
	PoolKey       PoolKey `json:"poolKey"`
	PriceHype     *string `json:"priceHype"`
	OurLaunch     bool    `json:"ourLaunch"`
}

// Retryable holds the gas parameters for a Ferry In retryable ticket (wei decimal strings).
type Retryable struct {
	GasLimit          string `json:"gasLimit"`
	MaxFeePerGas      string `json:"maxFeePerGas"`
	MaxSubmissionCost string `json:"maxSubmissionCost"`
	TotalValue        string `json:"totalValue"`
}

// Quote is a Ferry In quote simulated on Elysium.
type Quote struct {
	Token            string    `json:"token"`
	AmountIn         string    `json:"amountIn"`
	Fee              string    `json:"fee"`
	AmountOut        string    `json:"amountOut"`
	MinOut           string    `json:"minOut"`
	SlippageBps      int       `json:"slippageBps"`
	PriceImpactBps   int       `json:"priceImpactBps"`
	Route            []string  `json:"route"`
	SimulatedAtBlock uint64    `json:"simulatedAtBlock"`
	Retryable        Retryable `json:"retryable"`
}

// FerryInStatus tracks one HyperEVM ticket that buys a token on Elysium.
type FerryInStatus struct {
	HyperEVMTx string  `json:"hyperevmTx"`
	Stage      string  `json:"stage"`
	MessageID  *int64  `json:"messageId"`
	ElysiumTx  *string `json:"elysiumTx"`
	Token      *string `json:"token"`
	Recipient  string  `json:"recipient"`
	HypeIn     string  `json:"hypeIn"`
	AmountOut  *string `json:"amountOut"`
	UpdatedAt  int64   `json:"updatedAt"`
}

// ExitParams mirrors the Solidity ExitParams struct (chainId is a number, the rest are decimal strings).
type ExitParams struct {
	ChainID   uint64 `json:"chainId"`
	Desk      string `json:"desk"`
	Sender    string `json:"sender"`
	Nonce     string `json:"nonce"`
	Amount    string `json:"amount"`
	ClaimTip  string `json:"claimTip"`
	Recipient string `json:"recipient"`
	ToCore    bool   `json:"toCore"`
}

// Exit is one Ferry Out exit, keyed by its HyperEVM receiver.
type Exit struct {
	Receiver    string     `json:"receiver"`
	Salt        string     `json:"salt"`
	Params      ExitParams `json:"params"`
	Stage       string     `json:"stage"`
	ElysiumTx   string     `json:"elysiumTx"`
	MessageID   *int64     `json:"messageId"`
	StartedAt   int64      `json:"startedAt"`
	MaturesAt   *int64     `json:"maturesAt"`
	ClaimTx     *string    `json:"claimTx"`
	DeliveredAt *int64     `json:"deliveredAt"`
}

// RescueRecent is one entry of the recent Rescue claims list.
type RescueRecent struct {
	MessageID int64  `json:"messageId"`
	Owner     string `json:"owner"`
	Amount    string `json:"amount"`
	ClaimTx   string `json:"claimTx"`
	ClaimedAt int64  `json:"claimedAt"`
}

// RescueStats is the public Rescue counter.
type RescueStats struct {
	Withdrawals  int            `json:"withdrawals"`
	Owners       int            `json:"owners"`
	HypeReturned string         `json:"hypeReturned"`
	GasSpent     string         `json:"gasSpent"`
	LastRunAt    int64          `json:"lastRunAt"`
	Recent       []RescueRecent `json:"recent"`
}

// StuckWithdrawal is a Rescue lookup result for one address.
type StuckWithdrawal struct {
	MessageID  int64  `json:"messageId"`
	Owner      string `json:"owner"`
	Amount     string `json:"amount"`
	OriginTx   string `json:"originTx"`
	Status     string `json:"status"`
	AgeSeconds int64  `json:"ageSeconds"`
	Claimable  bool   `json:"claimable"`
}

// ClaimResult is the answer of POST /api/rescue/claim.
type ClaimResult struct {
	Status string  `json:"status"`
	TxHash *string `json:"txHash"`
}

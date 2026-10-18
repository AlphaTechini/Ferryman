package indexer

import (
	"context"
	"crypto/ecdsa"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

var (
	launchpad = common.HexToAddress("0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d")
	otherHook = common.HexToAddress("0x9999999999999999999999999999999999999999")
	pm        = common.HexToAddress("0x2222222222222222222222222222222222222222")
	desk      = common.HexToAddress("0x00000000000000000000000000000000000000d5")
	ferry     = common.HexToAddress("0x00000000000000000000000000000000000000f1")
	usdc      = common.HexToAddress("0x7D29d8047B905000459c0E80c34a26CeedcB47b2")
	tokenA    = common.HexToAddress("0x00000000000000000000000000000000000000aa")
	tokenB    = common.HexToAddress("0x00000000000000000000000000000000000000bb")
	tokenC    = common.HexToAddress("0x00000000000000000000000000000000000000cc")
	chainID   = big.NewInt(99801)
)

// fakeEL serves logs, headers, transactions and ERC20/poolKeyOf calls.
type fakeEL struct {
	chain.Reader
	head    uint64
	logs    []types.Log
	txs     map[common.Hash]*types.Transaction
	erc20   map[common.Address][2]string
	poolKey map[common.Address]abis.PoolKey
}

func (f *fakeEL) ChainID(ctx context.Context) (*big.Int, error) { return chainID, nil }

func (f *fakeEL) BlockNumber(ctx context.Context) (uint64, error) { return f.head, nil }

func (f *fakeEL) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	return &types.Header{Number: n, Time: 1_700_000_000 + n.Uint64()}, nil
}

func (f *fakeEL) TransactionByHash(ctx context.Context, h common.Hash) (*types.Transaction, bool, error) {
	return f.txs[h], false, nil
}

func (f *fakeEL) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	var out []types.Log
	for _, l := range f.logs {
		if l.BlockNumber < q.FromBlock.Uint64() || l.BlockNumber > q.ToBlock.Uint64() {
			continue
		}
		if len(q.Addresses) > 0 && !containsAddr(q.Addresses, l.Address) {
			continue
		}
		if len(q.Topics) > 0 && len(q.Topics[0]) > 0 && !containsHash(q.Topics[0], l.Topics[0]) {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (f *fakeEL) CallContract(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
	to := *call.To
	sel := call.Data[:4]
	switch {
	case string(sel) == string(abis.ERC20.Methods["name"].ID):
		return abis.ERC20.Methods["name"].Outputs.Pack(f.erc20[to][0])
	case string(sel) == string(abis.ERC20.Methods["symbol"].ID):
		return abis.ERC20.Methods["symbol"].Outputs.Pack(f.erc20[to][1])
	case string(sel) == string(abis.ERC20.Methods["decimals"].ID):
		return abis.ERC20.Methods["decimals"].Outputs.Pack(uint8(18))
	case string(sel) == string(abis.FerryIn.Methods["poolKeyOf"].ID):
		token := common.BytesToAddress(call.Data[4+12 : 4+32])
		key, ok := f.poolKey[token]
		if !ok {
			key = abis.PoolKey{Fee: big.NewInt(0)}
		}
		return abis.FerryIn.Methods["poolKeyOf"].Outputs.Pack(key)
	}
	return nil, errString("unexpected call")
}

type errString string

func (e errString) Error() string { return string(e) }

func containsAddr(list []common.Address, a common.Address) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

func containsHash(list []common.Hash, h common.Hash) bool {
	for _, x := range list {
		if x == h {
			return true
		}
	}
	return false
}

type fakeTx struct {
	mu    sync.Mutex
	sends []chain.TxRequest
	dry   bool
}

func (f *fakeTx) Send(ctx context.Context, req chain.TxRequest) (*chain.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, req)
	return &chain.SendResult{DryRun: f.dry, Hash: common.HexToHash("0x1")}, nil
}

func initLog(t *testing.T, block uint64, hooks, c0, c1 common.Address, txHash common.Hash) types.Log {
	t.Helper()
	data, err := abis.CLPoolManager.Events["Initialize"].Inputs.NonIndexed().Pack(
		hooks, big.NewInt(3000), common.HexToHash("0x3c00"), big.NewInt(1), big.NewInt(-60))
	if err != nil {
		t.Fatal(err)
	}
	return types.Log{
		Address:     pm,
		BlockNumber: block,
		TxHash:      txHash,
		Topics: []common.Hash{abis.TopicInitialize, common.HexToHash("0x01"),
			common.BytesToHash(c0.Bytes()), common.BytesToHash(c1.Bytes())},
		Data: data,
	}
}

func signedTx(t *testing.T, key *ecdsa.PrivateKey) *types.Transaction {
	t.Helper()
	tx, err := types.SignNewTx(key, types.LatestSignerForChainID(chainID), &types.LegacyTx{Nonce: 1, GasPrice: big.NewInt(1), Gas: 21000})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func newIndexer(t *testing.T, el *fakeEL, tx Transactor, ours []common.Address) (*Indexer, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/s.json")
	if err != nil {
		t.Fatal(err)
	}
	ix := New(Settings{
		Launchpad: launchpad, CLPoolManager: pm, ExitDesk: desk, FerryIn: ferry,
		QuoteCurrencies: []common.Address{usdc}, OurLaunchers: ours,
		LogRange: 5000, Confirmations: 3, RegisterPerTick: 10,
	}, el, st, tx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return ix, st
}

func TestOnlyLaunchpadPoolsWithKnownQuoteAreIndexed(t *testing.T) {
	key, _ := crypto.GenerateKey()
	launcher := crypto.PubkeyToAddress(key.PublicKey)
	txA := common.HexToHash("0xa1")
	txB := common.HexToHash("0xb1")
	txC := common.HexToHash("0xc1")
	el := &fakeEL{
		head: 100,
		logs: []types.Log{
			initLog(t, 10, launchpad, tokenA, usdc, txA),   // genuine Ascend pool
			initLog(t, 11, otherHook, tokenB, usdc, txB),   // look-alike: different hook
			initLog(t, 12, launchpad, tokenC, tokenB, txC), // launchpad hook but no known quote
		},
		txs:   map[common.Hash]*types.Transaction{txA: signedTx(t, key)},
		erc20: map[common.Address][2]string{tokenA: {"Alpha", "ALP"}},
	}
	ix, st := newIndexer(t, el, nil, []common.Address{launcher})
	ix.SyncOnce(context.Background())

	toks := st.Tokens()
	if len(toks) != 1 {
		t.Fatalf("indexed %d tokens, want 1: %+v", len(toks), toks)
	}
	tok := toks[0]
	if tok.Address != tokenA.Hex() || tok.Symbol != "ALP" || tok.QuoteCurrency != usdc.Hex() {
		t.Fatalf("token wrong: %+v", tok)
	}
	if tok.Launcher != launcher.Hex() || !tok.OurLaunch || tok.LaunchBlock != 10 || tok.LaunchedAt != 1_700_000_010 {
		t.Fatalf("launcher/time wrong: %+v", tok)
	}
	if tok.PoolKey.Hooks != launchpad.Hex() || tok.PoolKey.PoolManager != pm.Hex() || tok.PoolKey.Fee != 3000 {
		t.Fatalf("pool key wrong: %+v", tok.PoolKey)
	}
	if tok.PriceHype != nil {
		t.Fatal("priceHype is best effort and must be null here")
	}
}

func TestResyncIsIdempotentAndCursorAdvances(t *testing.T) {
	key, _ := crypto.GenerateKey()
	txA := common.HexToHash("0xa1")
	el := &fakeEL{
		head:  100,
		logs:  []types.Log{initLog(t, 10, launchpad, tokenA, usdc, txA)},
		txs:   map[common.Hash]*types.Transaction{txA: signedTx(t, key)},
		erc20: map[common.Address][2]string{tokenA: {"Alpha", "ALP"}},
	}
	ix, st := newIndexer(t, el, nil, nil)
	ix.SyncOnce(context.Background())
	ix.SyncOnce(context.Background())
	if n := len(st.Tokens()); n != 1 {
		t.Fatalf("tokens = %d after two syncs", n)
	}
	if c, ok := st.Cursor(streamTokens); !ok || c != 97 {
		t.Fatalf("cursor = %d (%v), want 97 (head-3)", c, ok)
	}
	if tok, _ := st.Token(tokenA.Hex()); tok.OurLaunch {
		t.Fatal("no OUR_LAUNCHERS configured: must not be ours")
	}
}

func TestExitIntentIndexed(t *testing.T) {
	p := abis.ExitParams{
		ChainId: big.NewInt(99801), Desk: desk, Sender: common.HexToAddress("0xa1"), Nonce: big.NewInt(4),
		Amount: big.NewInt(2_000), ClaimTip: big.NewInt(100), Recipient: common.HexToAddress("0xb2"),
	}
	salt, err := abis.ExitSalt(p)
	if err != nil {
		t.Fatal(err)
	}
	receiver := common.HexToAddress("0x7777777777777777777777777777777777777777")
	data, err := abis.ExitDesk.Events["ExitIntent"].Inputs.NonIndexed().Pack(p)
	if err != nil {
		t.Fatal(err)
	}
	txHash := common.HexToHash("0xe1")
	el := &fakeEL{head: 50, logs: []types.Log{{
		Address: desk, BlockNumber: 20, TxHash: txHash,
		Topics: []common.Hash{abis.TopicExitIntent, salt, common.BytesToHash(receiver.Bytes()), common.BytesToHash(p.Recipient.Bytes())},
		Data:   data,
	}}}
	ix, st := newIndexer(t, el, nil, nil)
	ix.SyncOnce(context.Background())
	e, ok := st.Exit(receiver.Hex())
	if !ok {
		t.Fatal("exit not indexed")
	}
	if e.Stage != "intent" || e.ElysiumTx != txHash.Hex() || e.StartedAt != 1_700_000_020 || e.Params.Nonce != "4" || e.Params.Amount != "2000" {
		t.Fatalf("exit wrong: %+v", e)
	}
	if e.MaturesAt == nil || *e.MaturesAt != 1_700_000_020+25*60 {
		t.Fatal("maturesAt must be startedAt + 25 minutes")
	}
	if e.Salt != salt.Hex() {
		t.Fatal("salt mismatch")
	}
}

func TestFerryRegistrationSendsRegisterTokenOnce(t *testing.T) {
	key, _ := crypto.GenerateKey()
	txA := common.HexToHash("0xa1")
	el := &fakeEL{
		head:    100,
		logs:    []types.Log{initLog(t, 10, launchpad, tokenA, usdc, txA)},
		txs:     map[common.Hash]*types.Transaction{txA: signedTx(t, key)},
		erc20:   map[common.Address][2]string{tokenA: {"Alpha", "ALP"}},
		poolKey: map[common.Address]abis.PoolKey{}, // unregistered: zero key
	}
	tx := &fakeTx{}
	ix, st := newIndexer(t, el, tx, nil)
	ix.SyncOnce(context.Background())
	ix.SyncOnce(context.Background())
	if len(tx.sends) != 1 {
		t.Fatalf("registerToken sends = %d, want 1", len(tx.sends))
	}
	if tx.sends[0].To != ferry || string(tx.sends[0].Data[:4]) != string(abis.FerryIn.Methods["registerToken"].ID) {
		t.Fatal("must call FerryIn.registerToken")
	}
	if !st.IsRegistered(tokenA.Hex()) {
		t.Fatal("registration must be recorded")
	}
}

func TestRegistrationSkippedWhenAlreadySetOrReadOnly(t *testing.T) {
	key, _ := crypto.GenerateKey()
	txA := common.HexToHash("0xa1")
	registered := abis.PoolKey{Currency0: tokenA, Currency1: usdc, Hooks: launchpad, PoolManager: pm, Fee: big.NewInt(3000), Parameters: common.HexToHash("0x3c00")}
	el := &fakeEL{
		head:    100,
		logs:    []types.Log{initLog(t, 10, launchpad, tokenA, usdc, txA)},
		txs:     map[common.Hash]*types.Transaction{txA: signedTx(t, key)},
		erc20:   map[common.Address][2]string{tokenA: {"Alpha", "ALP"}},
		poolKey: map[common.Address]abis.PoolKey{tokenA: registered},
	}
	tx := &fakeTx{}
	ix, st := newIndexer(t, el, tx, nil)
	ix.SyncOnce(context.Background())
	if len(tx.sends) != 0 || !st.IsRegistered(tokenA.Hex()) {
		t.Fatalf("already registered token must not be re-sent (sends=%d)", len(tx.sends))
	}

	el2 := &fakeEL{
		head:    100,
		logs:    []types.Log{initLog(t, 10, launchpad, tokenB, usdc, txA)},
		txs:     map[common.Hash]*types.Transaction{txA: signedTx(t, key)},
		erc20:   map[common.Address][2]string{tokenB: {"Beta", "BET"}},
		poolKey: map[common.Address]abis.PoolKey{},
	}
	ix2, st2 := newIndexer(t, el2, nil, nil) // read-only: no transactor
	ix2.SyncOnce(context.Background())
	if st2.IsRegistered(tokenB.Hex()) {
		t.Fatal("read-only mode must not mark tokens registered")
	}
}

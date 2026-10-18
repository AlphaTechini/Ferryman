// Package abis embeds the minimal ABI fragments the keeper uses and provides typed
// encode/decode helpers around them. ABIs are kept small on purpose: only the functions
// and events that the keeper calls or reads.
package abis

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const tupleExitParams = `{"type":"tuple","name":"p","components":[
 {"name":"chainId","type":"uint256"},{"name":"desk","type":"address"},{"name":"sender","type":"address"},
 {"name":"nonce","type":"uint256"},{"name":"amount","type":"uint256"},{"name":"claimTip","type":"uint256"},
 {"name":"recipient","type":"address"},{"name":"toCore","type":"bool"}]}`

const tuplePoolKey = `{"type":"tuple","name":"key","components":[
 {"name":"currency0","type":"address"},{"name":"currency1","type":"address"},{"name":"hooks","type":"address"},
 {"name":"poolManager","type":"address"},{"name":"fee","type":"uint24"},{"name":"parameters","type":"bytes32"}]}`

// ExitDesk (Elysium).
const exitDeskJSON = `[
 {"type":"event","name":"ExitIntent","anonymous":false,"inputs":[
   {"name":"salt","type":"bytes32","indexed":true},{"name":"receiver","type":"address","indexed":true},
   {"name":"recipient","type":"address","indexed":true},{"name":"params","type":"tuple","indexed":false,"components":` + tupleComponents + `}]}
]`

// ExitFactory (HyperEVM).
const exitFactoryJSON = `[
 {"type":"function","name":"claimAndSweep","stateMutability":"nonpayable","inputs":[
   {"name":"outboxCalldata","type":"bytes"},` + tupleExitParams + `],"outputs":[{"name":"paid","type":"uint256"}]},
 {"type":"function","name":"sweep","stateMutability":"nonpayable","inputs":[` + tupleExitParams + `],"outputs":[]},
 {"type":"function","name":"receiverFor","stateMutability":"view","inputs":[` + tupleExitParams + `],"outputs":[{"name":"","type":"address"}]},
 {"type":"function","name":"isSwept","stateMutability":"view","inputs":[{"name":"salt","type":"bytes32"}],"outputs":[{"name":"","type":"bool"}]}
]`

// FerryIn (Elysium).
const ferryInJSON = `[
 {"type":"function","name":"buy","stateMutability":"payable","inputs":[
   {"name":"token","type":"address"},{"name":"recipient","type":"address"},{"name":"minOut","type":"uint256"}],"outputs":[]},
 {"type":"function","name":"quote","stateMutability":"nonpayable","inputs":[
   {"name":"token","type":"address"},{"name":"hypeIn","type":"uint256"}],"outputs":[{"name":"amountOut","type":"uint256"}]},
 {"type":"function","name":"registerToken","stateMutability":"nonpayable","inputs":[` + tuplePoolKey + `],"outputs":[]},
 {"type":"function","name":"poolKeyOf","stateMutability":"view","inputs":[{"name":"token","type":"address"}],"outputs":[` + tuplePoolKey + `]},
 {"type":"event","name":"Bought","anonymous":false,"inputs":[
   {"name":"token","type":"address","indexed":true},{"name":"recipient","type":"address","indexed":true},
   {"name":"hypeIn","type":"uint256","indexed":false},{"name":"fee","type":"uint256","indexed":false},
   {"name":"amountOut","type":"uint256","indexed":false}]},
 {"type":"event","name":"Refunded","anonymous":false,"inputs":[
   {"name":"token","type":"address","indexed":true},{"name":"recipient","type":"address","indexed":true},
   {"name":"hypeIn","type":"uint256","indexed":false},{"name":"reason","type":"bytes","indexed":false}]},
 {"type":"event","name":"TokenRegistered","anonymous":false,"inputs":[
   {"name":"token","type":"address","indexed":true},{"name":"quoteCurrency","type":"address","indexed":true},
   {"name":"key","type":"tuple","indexed":false,"components":` + tupleComponentsPoolKey + `}]}
]`

// Outbox (HyperEVM): executeTransaction is the claim call, OutBoxTransactionExecuted is emitted on success.
const outboxJSON = `[
 {"type":"function","name":"executeTransaction","stateMutability":"nonpayable","inputs":[
   {"name":"proof","type":"bytes32[]"},{"name":"index","type":"uint256"},{"name":"l2Sender","type":"address"},
   {"name":"to","type":"address"},{"name":"l2Block","type":"uint256"},{"name":"l1Block","type":"uint256"},
   {"name":"l2Timestamp","type":"uint256"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"}],"outputs":[]},
 {"type":"event","name":"OutBoxTransactionExecuted","anonymous":false,"inputs":[
   {"name":"to","type":"address","indexed":true},{"name":"l2Sender","type":"address","indexed":true},
   {"name":"zero","type":"uint256","indexed":true},{"name":"transactionIndex","type":"uint256","indexed":false}]}
]`

// Inbox (HyperEVM): retryable ticket creation and the submission-fee helper.
const inboxJSON = `[
 {"type":"function","name":"createRetryableTicket","stateMutability":"payable","inputs":[
   {"name":"to","type":"address"},{"name":"l2CallValue","type":"uint256"},{"name":"maxSubmissionCost","type":"uint256"},
   {"name":"excessFeeRefundAddress","type":"address"},{"name":"callValueRefundAddress","type":"address"},
   {"name":"gasLimit","type":"uint256"},{"name":"maxFeePerGas","type":"uint256"},{"name":"data","type":"bytes"}],
   "outputs":[{"name":"","type":"uint256"}]},
 {"type":"function","name":"calculateRetryableSubmissionFee","stateMutability":"view","inputs":[
   {"name":"dataLength","type":"uint256"},{"name":"baseFee","type":"uint256"}],"outputs":[{"name":"","type":"uint256"}]},
 {"type":"event","name":"InboxMessageDelivered","anonymous":false,"inputs":[
   {"name":"messageNum","type":"uint256","indexed":true},{"name":"data","type":"bytes","indexed":false}]}
]`

// ArbRetryableTx (Elysium, precompile 0x6E).
const arbRetryableTxJSON = `[
 {"type":"function","name":"redeem","stateMutability":"nonpayable","inputs":[{"name":"ticketId","type":"bytes32"}],"outputs":[{"name":"","type":"bytes32"}]},
 {"type":"event","name":"TicketCreated","anonymous":false,"inputs":[{"name":"ticketId","type":"bytes32","indexed":true}]}
]`

// ArbSys (Elysium, precompile 0x64): L2ToL1Tx carries the withdrawal message id in `position`.
const arbSysJSON = `[
 {"type":"event","name":"L2ToL1Tx","anonymous":false,"inputs":[
   {"name":"caller","type":"address","indexed":false},{"name":"destination","type":"address","indexed":true},
   {"name":"hash","type":"uint256","indexed":true},{"name":"position","type":"uint256","indexed":true},
   {"name":"arbBlockNum","type":"uint256","indexed":false},{"name":"ethBlockNum","type":"uint256","indexed":false},
   {"name":"timestamp","type":"uint256","indexed":false},{"name":"callvalue","type":"uint256","indexed":false},
   {"name":"data","type":"bytes","indexed":false}]}
]`

// CLPoolManager (Elysium): only the Initialize event. Hooks and fee are data, currencies and id are topics.
const clPoolManagerJSON = `[
 {"type":"event","name":"Initialize","anonymous":false,"inputs":[
   {"name":"id","type":"bytes32","indexed":true},{"name":"currency0","type":"address","indexed":true},
   {"name":"currency1","type":"address","indexed":true},{"name":"hooks","type":"address","indexed":false},
   {"name":"fee","type":"uint24","indexed":false},{"name":"parameters","type":"bytes32","indexed":false},
   {"name":"sqrtPriceX96","type":"uint160","indexed":false},{"name":"tick","type":"int24","indexed":false}]}
]`

// ERC20 metadata getters.
const erc20JSON = `[
 {"type":"function","name":"name","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"string"}]},
 {"type":"function","name":"symbol","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"string"}]},
 {"type":"function","name":"decimals","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint8"}]}
]`

const tupleComponents = `[
 {"name":"chainId","type":"uint256"},{"name":"desk","type":"address"},{"name":"sender","type":"address"},
 {"name":"nonce","type":"uint256"},{"name":"amount","type":"uint256"},{"name":"claimTip","type":"uint256"},
 {"name":"recipient","type":"address"},{"name":"toCore","type":"bool"}]`

const tupleComponentsPoolKey = `[
 {"name":"currency0","type":"address"},{"name":"currency1","type":"address"},{"name":"hooks","type":"address"},
 {"name":"poolManager","type":"address"},{"name":"fee","type":"uint24"},{"name":"parameters","type":"bytes32"}]`

// Parsed ABIs. Parsing happens once at start-up; a malformed fragment panics, and the tests catch it.
var (
	ExitDesk       = mustParse("ExitDesk", exitDeskJSON)
	ExitFactory    = mustParse("ExitFactory", exitFactoryJSON)
	FerryIn        = mustParse("FerryIn", ferryInJSON)
	Outbox         = mustParse("Outbox", outboxJSON)
	Inbox          = mustParse("Inbox", inboxJSON)
	ArbRetryableTx = mustParse("ArbRetryableTx", arbRetryableTxJSON)
	ArbSys         = mustParse("ArbSys", arbSysJSON)
	CLPoolManager  = mustParse("CLPoolManager", clPoolManagerJSON)
	ERC20          = mustParse("ERC20", erc20JSON)
)

func mustParse(name, src string) abi.ABI {
	a, err := abi.JSON(strings.NewReader(src))
	if err != nil {
		panic(fmt.Sprintf("abis: parse %s: %v", name, err))
	}
	return a
}

// Event topic0 values.
var (
	TopicExitIntent     = ExitDesk.Events["ExitIntent"].ID
	TopicInitialize     = CLPoolManager.Events["Initialize"].ID
	TopicBought         = FerryIn.Events["Bought"].ID
	TopicRefunded       = FerryIn.Events["Refunded"].ID
	TopicOutboxExecuted = Outbox.Events["OutBoxTransactionExecuted"].ID
	TopicL2ToL1Tx       = ArbSys.Events["L2ToL1Tx"].ID
	TopicInboxDelivered = Inbox.Events["InboxMessageDelivered"].ID
)

// OutboxExecuteSelector is Outbox.executeTransaction(bytes32[],uint256,address,address,uint256,uint256,uint256,uint256,bytes).
var OutboxExecuteSelector = Outbox.Methods["executeTransaction"].ID

// ExitParams is the Go binding of the Solidity ExitParams struct.
type ExitParams struct {
	ChainId   *big.Int
	Desk      common.Address
	Sender    common.Address
	Nonce     *big.Int
	Amount    *big.Int
	ClaimTip  *big.Int
	Recipient common.Address
	ToCore    bool
}

// PoolKey is the Go binding of the Infinity CL PoolKey struct.
type PoolKey struct {
	Currency0   common.Address
	Currency1   common.Address
	Hooks       common.Address
	PoolManager common.Address
	Fee         *big.Int // uint24 is bound as *big.Int by go-ethereum
	Parameters  [32]byte
}

// tupleArgs wraps the ExitParams tuple type so it can be packed on its own.
var tupleArgs = func() abi.Arguments {
	t := ExitFactory.Methods["receiverFor"].Inputs[0].Type
	return abi.Arguments{{Type: t}}
}()

// ExitSalt returns keccak256(abi.encode(p)). ExitParams is fully static, so the encoding is the
// eight 32-byte words in struct order, which is what a single-argument tuple pack produces.
func ExitSalt(p ExitParams) (common.Hash, error) {
	enc, err := tupleArgs.Pack(p)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(enc), nil
}

// PackClaimAndSweep builds ExitFactory.claimAndSweep(outboxCalldata, p) calldata.
func PackClaimAndSweep(outboxCalldata []byte, p ExitParams) ([]byte, error) {
	return ExitFactory.Pack("claimAndSweep", outboxCalldata, p)
}

// PackIsSwept builds ExitFactory.isSwept(salt) calldata.
func PackIsSwept(salt common.Hash) ([]byte, error) {
	return ExitFactory.Pack("isSwept", salt)
}

// UnpackBool decodes a single bool return value.
func UnpackBool(a abi.ABI, method string, ret []byte) (bool, error) {
	out, err := a.Unpack(method, ret)
	if err != nil {
		return false, err
	}
	b, ok := out[0].(bool)
	if !ok {
		return false, errors.New("abis: not a bool")
	}
	return b, nil
}

// PackRedeem builds ArbRetryableTx.redeem(ticketId) calldata.
func PackRedeem(ticketID common.Hash) ([]byte, error) {
	return ArbRetryableTx.Pack("redeem", ticketID)
}

// PackBuy builds FerryIn.buy(token, recipient, minOut) calldata.
func PackBuy(token, recipient common.Address, minOut *big.Int) ([]byte, error) {
	return FerryIn.Pack("buy", token, recipient, minOut)
}

// PackQuote builds FerryIn.quote(token, hypeIn) calldata.
func PackQuote(token common.Address, hypeIn *big.Int) ([]byte, error) {
	return FerryIn.Pack("quote", token, hypeIn)
}

// PackPoolKeyOf builds FerryIn.poolKeyOf(token) calldata.
func PackPoolKeyOf(token common.Address) ([]byte, error) {
	return FerryIn.Pack("poolKeyOf", token)
}

// PackRegisterToken builds FerryIn.registerToken(key) calldata.
func PackRegisterToken(key PoolKey) ([]byte, error) {
	return FerryIn.Pack("registerToken", key)
}

// UnpackPoolKey decodes the poolKeyOf return value.
func UnpackPoolKey(ret []byte) (PoolKey, error) {
	out, err := FerryIn.Unpack("poolKeyOf", ret)
	if err != nil {
		return PoolKey{}, err
	}
	return toPoolKey(out[0])
}

func toPoolKey(v any) (PoolKey, error) {
	k, ok := abi.ConvertType(v, new(PoolKey)).(*PoolKey)
	if !ok {
		return PoolKey{}, errors.New("abis: pool key conversion failed")
	}
	return *k, nil
}

// IsZeroPoolKey reports whether a pool key is the empty value returned for unregistered tokens.
func IsZeroPoolKey(k PoolKey) bool {
	return k.Currency0 == (common.Address{}) && k.Currency1 == (common.Address{}) &&
		k.Hooks == (common.Address{}) && k.PoolManager == (common.Address{}) &&
		(k.Fee == nil || k.Fee.Sign() == 0) && k.Parameters == [32]byte{}
}

// BuyCalldataLength is the length of FerryIn.buy calldata: a 4-byte selector and three static words.
func BuyCalldataLength() int {
	return 4 + 3*32
}

// PackSubmissionFee packs Inbox.calculateRetryableSubmissionFee(dataLength, baseFee) calldata.
func PackSubmissionFee(dataLength, baseFee *big.Int) ([]byte, error) {
	return Inbox.Pack("calculateRetryableSubmissionFee", dataLength, baseFee)
}

// UnpackUint decodes a single uint256 return value.
func UnpackUint(a abi.ABI, method string, ret []byte) (*big.Int, error) {
	out, err := a.Unpack(method, ret)
	if err != nil {
		return nil, err
	}
	v, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("abis: %s did not return uint256", method)
	}
	return v, nil
}

// OutboxCall is the decoded Outbox.executeTransaction call.
type OutboxCall struct {
	To    common.Address
	Value *big.Int
}

// DecodeOutboxCall checks the selector and decodes the payout target and value. Use the decoded
// `To` (never byte offsets: the proof array is dynamic).
func DecodeOutboxCall(calldata []byte) (OutboxCall, error) {
	if len(calldata) < 4 {
		return OutboxCall{}, errors.New("abis: calldata too short")
	}
	if !bytes.Equal(calldata[:4], OutboxExecuteSelector) {
		return OutboxCall{}, fmt.Errorf("abis: selector %x is not Outbox.executeTransaction", calldata[:4])
	}
	args, err := Outbox.Methods["executeTransaction"].Inputs.Unpack(calldata[4:])
	if err != nil {
		return OutboxCall{}, fmt.Errorf("abis: decode executeTransaction: %w", err)
	}
	to, ok1 := args[3].(common.Address)
	value, ok2 := args[7].(*big.Int)
	if !ok1 || !ok2 {
		return OutboxCall{}, errors.New("abis: unexpected executeTransaction argument types")
	}
	return OutboxCall{To: to, Value: value}, nil
}

// BalanceDelta splits a packed Infinity BalanceDelta (int256) into its two int128 halves:
// amount0 is the upper 128 bits, amount1 the lower 128 bits, both sign-extended.
func BalanceDelta(packed *big.Int) (amount0, amount1 *big.Int) {
	mod := new(big.Int).Lsh(big.NewInt(1), 256)
	u := new(big.Int).Set(packed)
	if u.Sign() < 0 {
		u.Add(u, mod)
	}
	half := new(big.Int).Lsh(big.NewInt(1), 128)
	mask := new(big.Int).Sub(half, big.NewInt(1))
	hi := new(big.Int).Rsh(u, 128)
	lo := new(big.Int).And(u, mask)
	return signExtend128(hi), signExtend128(lo)
}

func signExtend128(v *big.Int) *big.Int {
	half := new(big.Int).Lsh(big.NewInt(1), 127)
	full := new(big.Int).Lsh(big.NewInt(1), 128)
	if v.Cmp(half) >= 0 {
		return new(big.Int).Sub(v, full)
	}
	return new(big.Int).Set(v)
}

// ExitIntentEvent is a decoded ExitDesk.ExitIntent log.
type ExitIntentEvent struct {
	Salt      common.Hash
	Receiver  common.Address
	Recipient common.Address
	Params    ExitParams
}

// DecodeExitIntent decodes an ExitIntent log.
func DecodeExitIntent(l types.Log) (ExitIntentEvent, error) {
	if len(l.Topics) != 4 || l.Topics[0] != TopicExitIntent {
		return ExitIntentEvent{}, errors.New("abis: not an ExitIntent log")
	}
	vals, err := ExitDesk.Events["ExitIntent"].Inputs.NonIndexed().Unpack(l.Data)
	if err != nil {
		return ExitIntentEvent{}, err
	}
	p, ok := abi.ConvertType(vals[0], new(ExitParams)).(*ExitParams)
	if !ok {
		return ExitIntentEvent{}, errors.New("abis: exit params conversion failed")
	}
	return ExitIntentEvent{
		Salt:      l.Topics[1],
		Receiver:  common.BytesToAddress(l.Topics[2].Bytes()),
		Recipient: common.BytesToAddress(l.Topics[3].Bytes()),
		Params:    *p,
	}, nil
}

// InitializeEvent is a decoded CLPoolManager.Initialize log (only the parts the keeper uses).
type InitializeEvent struct {
	PoolManager common.Address // the emitter
	Currency0   common.Address
	Currency1   common.Address
	Hooks       common.Address
	Fee         uint32
	Parameters  [32]byte
}

// DecodeInitialize decodes Initialize(bytes32 indexed id, address indexed currency0,
// address indexed currency1, address hooks, uint24 fee, bytes32 parameters, uint160, int24).
func DecodeInitialize(l types.Log) (InitializeEvent, error) {
	if len(l.Topics) != 4 || l.Topics[0] != TopicInitialize {
		return InitializeEvent{}, errors.New("abis: not an Initialize log")
	}
	vals, err := CLPoolManager.Events["Initialize"].Inputs.NonIndexed().Unpack(l.Data)
	if err != nil {
		return InitializeEvent{}, err
	}
	hooks, ok1 := vals[0].(common.Address)
	feeBig, ok2 := vals[1].(*big.Int)
	params, ok3 := vals[2].([32]byte)
	if !ok1 || !ok2 || !ok3 {
		return InitializeEvent{}, errors.New("abis: unexpected Initialize argument types")
	}
	fee := uint32(feeBig.Uint64())
	return InitializeEvent{
		PoolManager: l.Address,
		Currency0:   common.BytesToAddress(l.Topics[2].Bytes()),
		Currency1:   common.BytesToAddress(l.Topics[3].Bytes()),
		Hooks:       hooks,
		Fee:         fee,
		Parameters:  params,
	}, nil
}

// DecodeBought returns the recipient and the amountOut field of a FerryIn.Bought log.
func DecodeBought(l types.Log) (recipient common.Address, amountOut *big.Int, err error) {
	if len(l.Topics) != 3 || l.Topics[0] != TopicBought {
		return common.Address{}, nil, errors.New("abis: not a Bought log")
	}
	vals, err := FerryIn.Events["Bought"].Inputs.NonIndexed().Unpack(l.Data)
	if err != nil {
		return common.Address{}, nil, err
	}
	out, ok := vals[2].(*big.Int)
	if !ok {
		return common.Address{}, nil, errors.New("abis: unexpected Bought argument types")
	}
	return common.BytesToAddress(l.Topics[2].Bytes()), out, nil
}

// DecodeOutboxExecuted returns the payout target of an OutBoxTransactionExecuted log.
func DecodeOutboxExecuted(l types.Log) (common.Address, error) {
	if len(l.Topics) != 4 || l.Topics[0] != TopicOutboxExecuted {
		return common.Address{}, errors.New("abis: not an OutBoxTransactionExecuted log")
	}
	return common.BytesToAddress(l.Topics[1].Bytes()), nil
}

// L2ToL1Withdrawal is the part of an L2ToL1Tx log the claimer needs.
type L2ToL1Withdrawal struct {
	Destination common.Address
	Position    *big.Int // the Outbox message id
}

// DecodeL2ToL1Tx decodes the indexed fields of an ArbSys L2ToL1Tx log.
func DecodeL2ToL1Tx(l types.Log) (L2ToL1Withdrawal, error) {
	if len(l.Topics) != 4 || l.Topics[0] != TopicL2ToL1Tx {
		return L2ToL1Withdrawal{}, errors.New("abis: not an L2ToL1Tx log")
	}
	return L2ToL1Withdrawal{
		Destination: common.BytesToAddress(l.Topics[1].Bytes()),
		Position:    new(big.Int).SetBytes(l.Topics[3].Bytes()),
	}, nil
}

// InboxTicket is the decoded createRetryableTicket call on HyperEVM.
type InboxTicket struct {
	To                     common.Address
	L2CallValue            *big.Int
	MaxSubmissionCost      *big.Int
	ExcessFeeRefundAddress common.Address
	CallValueRefundAddress common.Address
	GasLimit               *big.Int
	MaxFeePerGas           *big.Int
	Data                   []byte
}

// DecodeCreateRetryableTicket decodes Inbox.createRetryableTicket calldata.
func DecodeCreateRetryableTicket(calldata []byte) (InboxTicket, error) {
	m := Inbox.Methods["createRetryableTicket"]
	if len(calldata) < 4 || !bytes.Equal(calldata[:4], m.ID) {
		return InboxTicket{}, errors.New("abis: not createRetryableTicket")
	}
	args, err := m.Inputs.Unpack(calldata[4:])
	if err != nil {
		return InboxTicket{}, err
	}
	return InboxTicket{
		To:                     args[0].(common.Address),
		L2CallValue:            args[1].(*big.Int),
		MaxSubmissionCost:      args[2].(*big.Int),
		ExcessFeeRefundAddress: args[3].(common.Address),
		CallValueRefundAddress: args[4].(common.Address),
		GasLimit:               args[5].(*big.Int),
		MaxFeePerGas:           args[6].(*big.Int),
		Data:                   args[7].([]byte),
	}, nil
}

// DecodeBuy decodes FerryIn.buy(token, recipient, minOut) calldata.
func DecodeBuy(calldata []byte) (token, recipient common.Address, minOut *big.Int, err error) {
	m := FerryIn.Methods["buy"]
	if len(calldata) < 4 || !bytes.Equal(calldata[:4], m.ID) {
		return common.Address{}, common.Address{}, nil, errors.New("abis: not FerryIn.buy")
	}
	args, err := m.Inputs.Unpack(calldata[4:])
	if err != nil {
		return common.Address{}, common.Address{}, nil, err
	}
	return args[0].(common.Address), args[1].(common.Address), args[2].(*big.Int), nil
}

// UnwrapOutboxCalldata returns the Outbox.executeTransaction calldata that a transaction carried. It
// accepts a direct executeTransaction call, or an ExitFactory.claimAndSweep call (whose first argument
// is that calldata). Used to recover a claim another party already executed.
func UnwrapOutboxCalldata(input []byte) ([]byte, error) {
	if len(input) < 4 {
		return nil, errors.New("abis: input too short")
	}
	if bytes.Equal(input[:4], OutboxExecuteSelector) {
		return input, nil
	}
	m := ExitFactory.Methods["claimAndSweep"]
	if !bytes.Equal(input[:4], m.ID) {
		return nil, fmt.Errorf("abis: selector %x is neither executeTransaction nor claimAndSweep", input[:4])
	}
	args, err := m.Inputs.Unpack(input[4:])
	if err != nil {
		return nil, err
	}
	inner, ok := args[0].([]byte)
	if !ok {
		return nil, errors.New("abis: claimAndSweep outbox argument is not bytes")
	}
	return inner, nil
}

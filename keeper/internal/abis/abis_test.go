package abis

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func addr(s string) common.Address { return common.HexToAddress(s) }

func sampleParams() ExitParams {
	return ExitParams{
		ChainId:   big.NewInt(99801),
		Desk:      addr("0x00000000000000000000000000000000000000d1"),
		Sender:    addr("0x00000000000000000000000000000000000000a1"),
		Nonce:     big.NewInt(3),
		Amount:    big.NewInt(2_000_000_000_000_000_000),
		ClaimTip:  big.NewInt(500_000_000_000_000),
		Recipient: addr("0x00000000000000000000000000000000000000b2"),
		ToCore:    true,
	}
}

func executeCalldata(t *testing.T, to common.Address) []byte {
	t.Helper()
	proof := [][32]byte{{1}, {2}}
	data, err := Outbox.Pack("executeTransaction", proof, big.NewInt(7), addr("0x00000000000000000000000000000000000000cc"),
		to, big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(42), []byte{0xde, 0xad})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestOutboxSelectorIsCanonical(t *testing.T) {
	want := []byte{0x08, 0x63, 0x5a, 0x95}
	if !bytes.Equal(OutboxExecuteSelector, want) {
		t.Fatalf("selector = %x, want 08635a95", OutboxExecuteSelector)
	}
}

func TestDecodeOutboxCallExtractsTo(t *testing.T) {
	to := addr("0x5555555555555555555555555555555555555555")
	call, err := DecodeOutboxCall(executeCalldata(t, to))
	if err != nil {
		t.Fatal(err)
	}
	if call.To != to {
		t.Fatalf("To = %s, want %s", call.To.Hex(), to.Hex())
	}
	if call.Value.Int64() != 42 {
		t.Fatalf("Value = %s, want 42", call.Value)
	}
}

func TestDecodeOutboxCallRejectsWrongSelector(t *testing.T) {
	to := addr("0x5555555555555555555555555555555555555555")
	other, err := ExitFactory.Pack("isSwept", common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOutboxCall(other); err == nil {
		t.Fatal("expected selector rejection for isSwept calldata")
	}
	if _, err := DecodeOutboxCall([]byte{0x08}); err == nil {
		t.Fatal("expected rejection of short calldata")
	}
	bad := executeCalldata(t, to)
	bad[0] ^= 0xff
	if _, err := DecodeOutboxCall(bad); err == nil {
		t.Fatal("expected rejection of a flipped selector")
	}
}

func TestUnwrapOutboxCalldata(t *testing.T) {
	to := addr("0x5555555555555555555555555555555555555555")
	inner := executeCalldata(t, to)
	direct, err := UnwrapOutboxCalldata(inner)
	if err != nil || !bytes.Equal(direct, inner) {
		t.Fatalf("direct unwrap failed: %v", err)
	}
	wrapped, err := PackClaimAndSweep(inner, sampleParams())
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapOutboxCalldata(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, inner) {
		t.Fatal("claimAndSweep unwrap did not return the inner outbox calldata")
	}
	if _, err := UnwrapOutboxCalldata(wrapped[:4]); err != nil {
		// selector-only claimAndSweep has no decodable arguments; an error is the right answer
	} else {
		t.Fatal("expected error for truncated claimAndSweep")
	}
}

func TestExitSaltMatchesManualEncoding(t *testing.T) {
	p := sampleParams()
	salt, err := ExitSalt(p)
	if err != nil {
		t.Fatal(err)
	}
	// abi.encode of a static struct is eight 32-byte words in field order.
	word := func(v *big.Int) []byte { return common.LeftPadBytes(v.Bytes(), 32) }
	addrWord := func(a common.Address) []byte { return common.LeftPadBytes(a.Bytes(), 32) }
	bool01 := func(b bool) []byte {
		if b {
			return word(big.NewInt(1))
		}
		return word(big.NewInt(0))
	}
	var enc []byte
	enc = append(enc, word(p.ChainId)...)
	enc = append(enc, addrWord(p.Desk)...)
	enc = append(enc, addrWord(p.Sender)...)
	enc = append(enc, word(p.Nonce)...)
	enc = append(enc, word(p.Amount)...)
	enc = append(enc, word(p.ClaimTip)...)
	enc = append(enc, addrWord(p.Recipient)...)
	enc = append(enc, bool01(p.ToCore)...)
	if want := crypto.Keccak256Hash(enc); salt != want {
		t.Fatalf("salt = %s, want %s", salt.Hex(), want.Hex())
	}
	p2 := p
	p2.Nonce = big.NewInt(4)
	salt2, _ := ExitSalt(p2)
	if salt2 == salt {
		t.Fatal("salt must change with every field")
	}
}

func TestBalanceDeltaSplit(t *testing.T) {
	cases := []struct{ a0, a1 int64 }{
		{0, 0}, {5, 7}, {-5, 7}, {5, -7}, {-5, -7}, {1 << 40, -(1 << 50)},
	}
	for _, c := range cases {
		packed := new(big.Int).Lsh(big.NewInt(c.a0), 128)
		low := big.NewInt(c.a1)
		if low.Sign() < 0 {
			low.Add(low, new(big.Int).Lsh(big.NewInt(1), 128))
		}
		packed.Add(packed, low)
		if packed.Sign() < 0 {
			// int256 encoding of a negative value
			packed.Add(packed, new(big.Int).Lsh(big.NewInt(1), 256))
		}
		// convert back to the signed int256 the helper receives
		signed := new(big.Int).Set(packed)
		if signed.Bit(255) == 1 {
			signed.Sub(signed, new(big.Int).Lsh(big.NewInt(1), 256))
		}
		a0, a1 := BalanceDelta(signed)
		if a0.Int64() != c.a0 || a1.Int64() != c.a1 {
			t.Fatalf("BalanceDelta(%d,%d) = (%s,%s)", c.a0, c.a1, a0, a1)
		}
	}
}

func TestDecodeExitIntentRoundTrip(t *testing.T) {
	p := sampleParams()
	salt, err := ExitSalt(p)
	if err != nil {
		t.Fatal(err)
	}
	receiver := addr("0x7777777777777777777777777777777777777777")
	data, err := ExitDesk.Events["ExitIntent"].Inputs.NonIndexed().Pack(p)
	if err != nil {
		t.Fatal(err)
	}
	log := types.Log{Topics: []common.Hash{
		TopicExitIntent, salt, common.BytesToHash(receiver.Bytes()), common.BytesToHash(p.Recipient.Bytes()),
	}, Data: data}
	ev, err := DecodeExitIntent(log)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Receiver != receiver || ev.Recipient != p.Recipient || ev.Salt != salt {
		t.Fatalf("indexed fields wrong: %+v", ev)
	}
	if ev.Params.Amount.Cmp(p.Amount) != 0 || ev.Params.ClaimTip.Cmp(p.ClaimTip) != 0 || !ev.Params.ToCore || ev.Params.Nonce.Int64() != 3 {
		t.Fatalf("params wrong: %+v", ev.Params)
	}
	got, err := ExitSalt(ev.Params)
	if err != nil || got != salt {
		t.Fatalf("recomputed salt mismatch: %v", err)
	}
}

func TestDecodeInitializeAndHooks(t *testing.T) {
	hooks := addr("0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d")
	pm := addr("0x2222222222222222222222222222222222222222")
	params := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000003c00")
	data, err := CLPoolManager.Events["Initialize"].Inputs.NonIndexed().Pack(hooks, big.NewInt(3000), params, big.NewInt(12345), big.NewInt(-60))
	if err != nil {
		t.Fatal(err)
	}
	c0 := addr("0x0000000000000000000000000000000000000000")
	c1 := addr("0x7D29d8047B905000459c0E80c34a26CeedcB47b2")
	log := types.Log{
		Address: pm,
		Topics:  []common.Hash{TopicInitialize, common.HexToHash("0x01"), common.BytesToHash(c0.Bytes()), common.BytesToHash(c1.Bytes())},
		Data:    data,
	}
	ev, err := DecodeInitialize(log)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Hooks != hooks || ev.PoolManager != pm || ev.Currency0 != c0 || ev.Currency1 != c1 || ev.Fee != 3000 {
		t.Fatalf("decoded initialize wrong: %+v", ev)
	}
	if common.BytesToHash(ev.Parameters[:]) != params {
		t.Fatal("parameters mismatch")
	}
	if _, err := DecodeInitialize(types.Log{Topics: []common.Hash{{}}}); err == nil {
		t.Fatal("expected rejection of non-Initialize log")
	}
}

func TestPoolKeyRoundTrip(t *testing.T) {
	want := PoolKey{
		Currency0:   addr("0x0000000000000000000000000000000000000001"),
		Currency1:   addr("0x0000000000000000000000000000000000000002"),
		Hooks:       addr("0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d"),
		PoolManager: addr("0x2222222222222222222222222222222222222222"),
		Fee:         big.NewInt(3000),
		Parameters:  common.HexToHash("0x3c00"),
	}
	ret, err := FerryIn.Methods["poolKeyOf"].Outputs.Pack(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnpackPoolKey(ret)
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency0 != want.Currency0 || got.Currency1 != want.Currency1 || got.Hooks != want.Hooks ||
		got.PoolManager != want.PoolManager || got.Fee.Cmp(want.Fee) != 0 || got.Parameters != want.Parameters {
		t.Fatalf("pool key round trip mismatch: %+v", got)
	}
	if IsZeroPoolKey(got) || !IsZeroPoolKey(PoolKey{}) {
		t.Fatal("IsZeroPoolKey wrong")
	}
}

func TestBuyCalldataLengthMatchesPack(t *testing.T) {
	data, err := PackBuy(addr("0x1"), addr("0x2"), big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != BuyCalldataLength() || BuyCalldataLength() != 100 {
		t.Fatalf("buy calldata len %d, const %d", len(data), BuyCalldataLength())
	}
	token, recipient, minOut, err := DecodeBuy(data)
	if err != nil || token != addr("0x1") || recipient != addr("0x2") || minOut.Int64() != 1 {
		t.Fatalf("DecodeBuy: %v", err)
	}
}

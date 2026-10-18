package ferryin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphatechini/ferryman/keeper/internal/abis"
	"github.com/alphatechini/ferryman/keeper/internal/chain"
	"github.com/alphatechini/ferryman/keeper/internal/explorer"
	"github.com/alphatechini/ferryman/keeper/internal/store"
)

var (
	inbox = common.HexToAddress("0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0")
	ferry = common.HexToAddress("0x00000000000000000000000000000000000000f1")
	token = common.HexToAddress("0x00000000000000000000000000000000000000aa")
	user  = common.HexToAddress("0x00000000000000000000000000000000000000b2")
)

type fakeHY struct {
	chain.Reader
	tx      *types.Transaction
	pending bool
	receipt *types.Receipt
}

func (f *fakeHY) TransactionByHash(ctx context.Context, h common.Hash) (*types.Transaction, bool, error) {
	if f.tx == nil {
		return nil, false, ethereum.NotFound
	}
	return f.tx, f.pending, nil
}

func (f *fakeHY) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if f.receipt == nil {
		return nil, ethereum.NotFound
	}
	return f.receipt, nil
}

type fakeExplorer struct {
	msgs []explorer.Message
}

func (f *fakeExplorer) ToRollup(ctx context.Context, cursor *int64) (explorer.MessagesPage, error) {
	return explorer.MessagesPage{Items: f.msgs}, nil
}

func ticketTx(t *testing.T, to common.Address) *types.Transaction {
	t.Helper()
	buy, err := abis.PackBuy(token, user, big.NewInt(12345))
	if err != nil {
		t.Fatal(err)
	}
	data, err := abis.Inbox.Methods["createRetryableTicket"].Inputs.Pack(
		ferry, big.NewInt(1_000_000), big.NewInt(4000), user, user, big.NewInt(1_500_000), big.NewInt(2_000_000_000), buy)
	if err != nil {
		t.Fatal(err)
	}
	calldata := append(append([]byte{}, abis.Inbox.Methods["createRetryableTicket"].ID...), data...)
	return types.NewTx(&types.LegacyTx{To: &to, Data: calldata, Value: big.NewInt(1)})
}

func newTracker(t *testing.T, hy *fakeHY, ex *fakeExplorer) *Tracker {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(Settings{Inbox: inbox, FerryIn: ferry}, hy, &fakeHY{}, ex, nil, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

const hash = "0x2222222222222222222222222222222222222222222222222222222222222222"

func TestPendingTicketIsSubmittedAndDecoded(t *testing.T) {
	hy := &fakeHY{tx: ticketTx(t, inbox), pending: true}
	tr := newTracker(t, hy, &fakeExplorer{})
	st, err := tr.Status(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if st.Stage != "submitted" || st.Recipient != user.Hex() || st.HypeIn != "1000000" || st.Token == nil || *st.Token != token.Hex() {
		t.Fatalf("pending status wrong: %+v", st)
	}
}

func TestNonTicketTransactionIsRejected(t *testing.T) {
	hy := &fakeHY{tx: ticketTx(t, common.HexToAddress("0x9")), pending: true}
	tr := newTracker(t, hy, &fakeExplorer{})
	if _, err := tr.Status(context.Background(), hash); !errors.Is(err, ErrNotFerryIn) {
		t.Fatalf("err = %v, want ErrNotFerryIn", err)
	}
	if _, err := newTracker(t, &fakeHY{}, &fakeExplorer{}).Status(context.Background(), hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown tx: err = %v, want ErrNotFound", err)
	}
}

func TestRelayedWithoutCompletionBecomesRedeemFailedAfter15Min(t *testing.T) {
	old := time.Now().Add(-30 * time.Minute).UTC().Format("2006-01-02T15:04:05.000000Z")
	msg := explorer.Message{ID: 42, OriginationTransactionHash: hash, OriginationTimestamp: &old, Status: "confirmed"}
	hy := &fakeHY{tx: ticketTx(t, inbox), receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}}
	tr := newTracker(t, hy, &fakeExplorer{msgs: []explorer.Message{msg}})
	st, err := tr.Status(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if st.Stage != "redeem_failed" || st.MessageID == nil || *st.MessageID != 42 {
		t.Fatalf("status = %+v, want redeem_failed with messageId 42", st)
	}
	again, _ := tr.Status(context.Background(), hash)
	if again.Stage != "redeem_failed" {
		t.Fatal("stage must persist across polls")
	}
}

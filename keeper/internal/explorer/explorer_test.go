package explorer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const fromRollupPage1 = `{
 "items": [
  {"id": 573261, "origination_address_hash": "0x1111111111111111111111111111111111111111",
   "origination_transaction_hash": "0xaaaa", "origination_timestamp": "2026-10-16T12:34:56.000000Z",
   "origination_transaction_block_number": 1234567, "completion_transaction_hash": null, "status": "confirmed"},
  {"id": 573260, "origination_address_hash": "0x2222222222222222222222222222222222222222",
   "origination_transaction_hash": "0xbbbb", "origination_timestamp": "2026-10-17T01:00:00.000000Z",
   "origination_transaction_block_number": null, "completion_transaction_hash": "0xcccc", "status": "relayed"}
 ],
 "next_page_params": {"id": 573260}
}`

const fromRollupPage2 = `{"items": [
  {"id": 573100, "origination_address_hash": "0x3333333333333333333333333333333333333333",
   "origination_transaction_hash": "0xdddd", "origination_timestamp": null,
   "origination_transaction_block_number": null, "completion_transaction_hash": null, "status": "sent"}
 ], "next_page_params": null}`

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL+"/api/v2", WithRetry(3, time.Millisecond)), srv
}

func TestFromRollupPagination(t *testing.T) {
	var seen []string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Path != "/api/v2/arbitrum/messages/from-rollup" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("id") == "" {
			_, _ = w.Write([]byte(fromRollupPage1))
			return
		}
		_, _ = w.Write([]byte(fromRollupPage2))
	})
	ctx := context.Background()
	p1, err := c.FromRollup(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Items) != 2 || p1.NextPageParams == nil || p1.NextPageParams.ID != 573260 {
		t.Fatalf("page 1 parsed wrong: %+v", p1)
	}
	m := p1.Items[0]
	if m.ID != 573261 || m.Status != "confirmed" || m.Completed() {
		t.Fatalf("item 0 wrong: %+v", m)
	}
	if ts, ok := m.Time(); !ok || ts.Year() != 2026 || ts.Day() != 16 {
		t.Fatalf("timestamp not parsed: %v %v", ts, ok)
	}
	if !p1.Items[1].Completed() {
		t.Fatal("item 1 should be completed")
	}
	cursor := p1.NextPageParams.ID
	p2, err := c.FromRollup(ctx, &cursor)
	if err != nil {
		t.Fatal(err)
	}
	if p2.NextPageParams != nil || len(p2.Items) != 1 {
		t.Fatalf("page 2 parsed wrong: %+v", p2)
	}
	if _, ok := p2.Items[0].Time(); ok {
		t.Fatal("null timestamp must report ok=false")
	}
	if seen[len(seen)-1] != "/api/v2/arbitrum/messages/from-rollup?id=573260" {
		t.Fatalf("cursor not sent: %v", seen)
	}
}

func TestClaimBothOutboxFieldNames(t *testing.T) {
	cases := map[string]string{
		`{"calldata":"0x08635a95","outbox_address_hash":"0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060"}`: "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060",
		`{"calldata":"0x08635a95","outbox_address":"0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060"}`:      "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060",
	}
	for body, want := range cases {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		d, err := c.Claim(context.Background(), 573261)
		if err != nil {
			t.Fatal(err)
		}
		if d.Outbox() != want || d.Calldata != "0x08635a95" {
			t.Fatalf("Outbox() = %q, want %q", d.Outbox(), want)
		}
	}
}

func TestClaimErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unconfirmed", 400, `{"message":"withdrawal is unconfirmed yet"}`, ErrUnconfirmed},
		{"executed", 400, `{"message":"withdrawal was executed already"}`, ErrExecuted},
		{"not found", 404, `{"message":"cannot find requested withdrawal"}`, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.Claim(context.Background(), 1)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Fatalf("4xx must not be retried, got %d calls", n)
			}
		})
	}
}

func TestOtherBadRequestIsPlainError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"message":"something else"}`))
	})
	_, err := c.Claim(context.Background(), 1)
	if err == nil || errors.Is(err, ErrUnconfirmed) || errors.Is(err, ErrExecuted) || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected a generic error, got %v", err)
	}
}

func TestRetriesTransientServerErrors(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"calldata":"0x08635a95","outbox_address_hash":"0x1"}`))
	})
	d, err := c.Claim(context.Background(), 5)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if d.Calldata != "0x08635a95" || atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("calls=%d data=%+v", calls, d)
	}
}

func TestWithdrawalsByTxParsesStringAndNumericCallvalue(t *testing.T) {
	body := `{"items":[
	 {"id":573261,"status":"confirmed","caller_address_hash":"0x1","destination_address_hash":"0x2222222222222222222222222222222222222222","callvalue":"2000000000000000000","data":"0x","completion_transaction_hash":null},
	 {"id":573262,"status":"relayed","caller_address_hash":"0x1","destination_address_hash":"0x3333333333333333333333333333333333333333","callvalue":5,"data":"0x","completion_transaction_hash":"0xabc"}
	]}`
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/arbitrum/messages/withdrawals/0xtx" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	})
	items, err := c.WithdrawalsByTx(context.Background(), "0xtx")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || string(items[0].Callvalue) != "2000000000000000000" || string(items[1].Callvalue) != "5" {
		t.Fatalf("items wrong: %+v", items)
	}
	if items[0].CompletionTransactionHash != nil || items[1].CompletionTransactionHash == nil {
		t.Fatal("completion hashes wrong")
	}
}

func TestWithdrawalsUnknownTxIsEmpty(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	items, err := c.WithdrawalsByTx(context.Background(), "0xnope")
	if err != nil || len(items) != 0 {
		t.Fatalf("expected empty list, got %v %v", items, err)
	}
}

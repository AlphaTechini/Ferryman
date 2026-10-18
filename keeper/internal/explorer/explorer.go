// Package explorer is a client for the Elysium Blockscout (Arbitrum-flavoured) API: message pages,
// withdrawal lookups by transaction and ready-made claim calldata.
package explorer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Errors returned by Claim. Use errors.Is.
var (
	// ErrUnconfirmed: the withdrawal is not yet confirmed (400 "unconfirmed yet"). Retry later.
	ErrUnconfirmed = errors.New("explorer: withdrawal unconfirmed")
	// ErrExecuted: the withdrawal was already executed on HyperEVM (400 "executed already").
	ErrExecuted = errors.New("explorer: withdrawal already executed")
	// ErrNotFound: the explorer does not know this message id (404).
	ErrNotFound = errors.New("explorer: message not found")
)

// Message is one item of the from-rollup or to-rollup lists.
type Message struct {
	ID                         int64   `json:"id"`
	OriginationAddressHash     string  `json:"origination_address_hash"`
	OriginationTransactionHash string  `json:"origination_transaction_hash"`
	OriginationTimestamp       *string `json:"origination_timestamp"`
	OriginationBlockNumber     *int64  `json:"origination_transaction_block_number"`
	CompletionTransactionHash  *string `json:"completion_transaction_hash"`
	Status                     string  `json:"status"`
}

// Time parses origination_timestamp. ok is false when the field is missing or unparsable.
func (m Message) Time() (time.Time, bool) {
	if m.OriginationTimestamp == nil {
		return time.Time{}, false
	}
	return ParseTimestamp(*m.OriginationTimestamp)
}

// Completed reports whether the message has a completion (relay) transaction.
func (m Message) Completed() bool {
	return m.CompletionTransactionHash != nil && *m.CompletionTransactionHash != ""
}

// PageParam is the cursor returned in next_page_params.
type PageParam struct {
	ID int64 `json:"id"`
}

// MessagesPage is one page of a messages list.
type MessagesPage struct {
	Items          []Message  `json:"items"`
	NextPageParams *PageParam `json:"next_page_params"`
}

// Withdrawal is one item of the withdrawals-by-transaction list.
type Withdrawal struct {
	ID                        int64      `json:"id"`
	Status                    string     `json:"status"`
	CallerAddressHash         string     `json:"caller_address_hash"`
	DestinationAddressHash    string     `json:"destination_address_hash"`
	Callvalue                 FlexString `json:"callvalue"`
	Data                      string     `json:"data"`
	CompletionTransactionHash *string    `json:"completion_transaction_hash"`
}

// ClaimData is the ready-made Outbox.executeTransaction calldata for one message.
type ClaimData struct {
	Calldata          string `json:"calldata"`
	OutboxAddressHash string `json:"outbox_address_hash"`
	OutboxAddress     string `json:"outbox_address"`
}

// Outbox returns whichever outbox field the explorer filled (current builds use outbox_address_hash).
func (c ClaimData) Outbox() string {
	if c.OutboxAddressHash != "" {
		return c.OutboxAddressHash
	}
	return c.OutboxAddress
}

// FlexString accepts a JSON string or number and stores its text form.
type FlexString string

// UnmarshalJSON implements json.Unmarshaler.
func (f *FlexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = FlexString(v)
		return nil
	}
	*f = FlexString(s)
	return nil
}

// ParseTimestamp parses explorer timestamps (RFC 3339, with or without fractional seconds).
func ParseTimestamp(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000000Z07:00", "2006-01-02 15:04:05.000000Z07"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Client talks to the Blockscout API. It is safe for concurrent use.
type Client struct {
	base     string
	http     *http.Client
	attempts int
	backoff  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithRetry sets the number of attempts and the base backoff for transient failures.
func WithRetry(attempts int, backoff time.Duration) Option {
	return func(c *Client) { c.attempts = attempts; c.backoff = backoff }
}

// New returns a client for a base such as https://testnet-api.elysium.kinetiq.xyz/api/v2.
func New(base string, opts ...Option) *Client {
	c := &Client{
		base:     strings.TrimRight(base, "/"),
		http:     &http.Client{Timeout: 20 * time.Second},
		attempts: 3,
		backoff:  500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// FromRollup returns one page of outgoing (L2 to L1) messages, newest first. A nil cursor is the first page.
func (c *Client) FromRollup(ctx context.Context, cursor *int64) (MessagesPage, error) {
	return c.messages(ctx, "from-rollup", cursor)
}

// ToRollup returns one page of incoming (L1 to L2, retryable) messages, newest first.
func (c *Client) ToRollup(ctx context.Context, cursor *int64) (MessagesPage, error) {
	return c.messages(ctx, "to-rollup", cursor)
}

func (c *Client) messages(ctx context.Context, dir string, cursor *int64) (MessagesPage, error) {
	q := url.Values{}
	if cursor != nil {
		q.Set("id", strconv.FormatInt(*cursor, 10))
	}
	var page MessagesPage
	if err := c.getJSON(ctx, "/arbitrum/messages/"+dir, q, &page); err != nil {
		return MessagesPage{}, err
	}
	return page, nil
}

// WithdrawalsByTx lists the withdrawals emitted by one L2 transaction. Unknown transactions give an empty list.
func (c *Client) WithdrawalsByTx(ctx context.Context, txHash string) ([]Withdrawal, error) {
	var resp struct {
		Items []Withdrawal `json:"items"`
	}
	err := c.getJSON(ctx, "/arbitrum/messages/withdrawals/"+url.PathEscape(txHash), nil, &resp)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// Claim returns the claim calldata for a message. ErrUnconfirmed, ErrExecuted and ErrNotFound are
// returned as typed errors; other 4xx responses are plain errors and are not retried.
func (c *Client) Claim(ctx context.Context, id int64) (ClaimData, error) {
	var out ClaimData
	err := c.getJSON(ctx, "/arbitrum/messages/claim/"+strconv.FormatInt(id, 10), nil, &out)
	return out, err
}

// apiError is a non-2xx response with its body message.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("explorer: HTTP %d: %s", e.status, e.msg) }

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	full := c.base + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	var lastErr error
	for attempt := 0; attempt < c.attempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, c.backoff<<(attempt-1)); err != nil {
				return err
			}
		}
		status, body, err := c.do(ctx, full)
		if err != nil {
			lastErr = err
			continue
		}
		if status >= 500 || status == http.StatusTooManyRequests {
			lastErr = &apiError{status: status, msg: messageOf(body)}
			continue
		}
		return interpret(status, body, out)
	}
	return fmt.Errorf("explorer: %s: %w", path, lastErr)
}

func (c *Client) do(ctx context.Context, full string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func interpret(status int, body []byte, out any) error {
	switch {
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusBadRequest:
		msg := strings.ToLower(messageOf(body))
		switch {
		case strings.Contains(msg, "unconfirmed"):
			return ErrUnconfirmed
		case strings.Contains(msg, "executed"):
			return ErrExecuted
		}
		return &apiError{status: status, msg: messageOf(body)}
	case status < 200 || status > 299:
		return &apiError{status: status, msg: messageOf(body)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("explorer: decode response: %w", err)
	}
	return nil
}

func messageOf(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

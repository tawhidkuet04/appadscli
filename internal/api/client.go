// Package api is the Apple Ads Platform API v1 client: bearer auth,
// ad-account context header, rate-limit-aware transport (token bucket +
// 429 backoff), and the query/selector pagination pattern.
//
// Contract (validated against the live api.ads.apple.com/v1):
//   - Base URL https://api.ads.apple.com/v1/
//   - Ad-account-scoped requests send "X-AP-Context: adAccountId=<id>;"
//   - GET /me, /acls, /orgs/{id}, /advertiser-resources, POST /ad-accounts
//     omit the context header
//   - Successful responses wrap the payload in {"result": …} and paged ones
//     add {"pagination": {offset, pageSize, totalCount}}
//   - Failures arrive as {"error": {code, message, details[]}} or
//     {"errors": [...]} — change-history serves them under HTTP 200
//   - List endpoints are POST {resource}/query with a Selector body:
//     {fields, filters[{field, operator, value}], sorting[{field, order}],
//     pagination{offset, pageSize}}
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/appadscli/appadscli/internal/auth"
	"github.com/appadscli/appadscli/internal/config"
)

// DefaultBase is the Apple Ads Platform API v1 base URL (no trailing slash;
// all paths below start with /v1/).
const DefaultBase = "https://api.ads.apple.com"

// Client is a configured Apple Ads API client bound to one ad account.
type Client struct {
	Base      string
	AdAccount string // ad account id for X-AP-Context; empty for unscoped calls
	HTTP      *http.Client

	limiter *rateLimiter
}

// APIError is a non-2xx response with Apple's error payload attached.
type APIError struct {
	Status int
	Body   string
}

// errorDetail is one entry of Apple's error payload.
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Info    struct {
		Field string `json:"field"`
	} `json:"info"`
}

// errorPayload covers the error shapes v1 serves: a nested {"error": …} object
// with details, a bare {"errors": [...]} list, and — on some 404s — the error
// object flattened into the response root.
type errorPayload struct {
	Error struct {
		Code    string        `json:"code"`
		Message string        `json:"message"`
		Details []errorDetail `json:"details"`
	} `json:"error"`
	Errors  []errorDetail `json:"errors"`
	Message string        `json:"message"`
	Details []errorDetail `json:"details"`
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	var parsed errorPayload
	if json.Unmarshal([]byte(e.Body), &parsed) == nil {
		details := parsed.Error.Details
		if len(details) == 0 {
			details = parsed.Errors
		}
		if len(details) == 0 {
			details = parsed.Details
		}
		switch parts := detailMessages(details); {
		case len(parts) > 0:
			msg = strings.Join(parts, "; ")
		case parsed.Error.Message != "":
			msg = parsed.Error.Message
		case parsed.Message != "":
			msg = parsed.Message
		}
	}
	return fmt.Sprintf("apple ads api: HTTP %d: %s", e.Status, msg)
}

// detailMessages renders error details as "field: message" where Apple names a field.
func detailMessages(details []errorDetail) []string {
	parts := make([]string, 0, len(details))
	for _, d := range details {
		p := d.Message
		if p == "" {
			p = d.Code
		}
		if p == "" {
			continue
		}
		if d.Info.Field != "" {
			p = d.Info.Field + ": " + p
		}
		parts = append(parts, p)
	}
	return parts
}

// IsNotFound reports whether err is an APIError with status 404.
func IsNotFound(err error) bool {
	ae, ok := err.(*APIError)
	return ok && ae.Status == 404
}

// New builds a client. account=="" falls back to the config default; account
// stays optional for unscoped endpoints (me, acls, orgs).
func New(cfg *config.Config, account string) *Client {
	base := DefaultBase
	if cfg != nil && cfg.APIBase != "" {
		base = cfg.APIBase
	}
	if env := os.Getenv("APPADSCLI_API_BASE"); env != "" {
		base = env
	}
	if account == "" && cfg != nil {
		account = cfg.DefaultAccount
	}
	return &Client{
		Base:      strings.TrimRight(base, "/"),
		AdAccount: account,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		limiter:   newRateLimiter(),
	}
}

// RequireAccount errors early for ad-account-scoped commands.
func (c *Client) RequireAccount() error {
	if c.AdAccount == "" {
		return fmt.Errorf("no ad account set — pass --account <id> or run `appadscli accounts use <id>` (find ids with `appadscli accounts list`)")
	}
	return nil
}

// Get issues a GET and decodes the `result` envelope into out (if non-nil).
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, "GET", path, nil, out)
}

// Post issues a POST with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, "POST", path, body, out)
}

// Put issues a PUT with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, "PUT", path, body, out)
}

// Delete issues a DELETE.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.do(ctx, "DELETE", path, nil, nil)
}

// Envelope is Apple's standard response wrapper: the payload under `result`
// plus page counters on list endpoints.
type Envelope struct {
	Result     json.RawMessage `json:"result"`
	Pagination *PageInfo       `json:"pagination"`
}

// PageInfo is the page counter block v1 returns alongside a list result.
type PageInfo struct {
	Offset     int `json:"offset"`
	PageSize   int `json:"pageSize"`
	TotalCount int `json:"totalCount"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	env, err := c.DoRaw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out != nil && env != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decoding %s %s: %w", method, path, err)
		}
	}
	return nil
}

// DoRaw executes one API call with retries and returns the raw envelope.
func (c *Client) DoRaw(ctx context.Context, method, path string, body any) (*Envelope, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.Base + path
	}
	const maxAttempts = 5
	for attempt := 1; ; attempt++ {
		c.limiter.wait(ctx)
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return nil, err
		}
		tok, err := auth.AccessToken(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		req.Header.Set("Accept", "application/json")
		if c.AdAccount != "" && !unscopedPath(path) {
			req.Header.Set("X-AP-Context", "adAccountId="+c.AdAccount+";")
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if attempt < maxAttempts {
				sleepCtx(ctx, backoff(attempt))
				continue
			}
			return nil, err
		}
		c.limiter.observe(resp)
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == 429 || resp.StatusCode >= 500:
			if attempt < maxAttempts {
				d := backoff(attempt)
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil {
						d = time.Duration(secs) * time.Second
					}
				}
				sleepCtx(ctx, d)
				continue
			}
			return nil, &APIError{Status: resp.StatusCode, Body: string(rb)}
		case resp.StatusCode >= 400:
			return nil, &APIError{Status: resp.StatusCode, Body: string(rb)}
		}
		if len(rb) == 0 {
			return nil, nil
		}
		return parseEnvelope(resp.StatusCode, rb)
	}
}

// parseEnvelope unwraps `result`, and surfaces the error payloads Apple also
// serves under 2xx (change-history does this) as APIError.
func parseEnvelope(status int, rb []byte) (*Envelope, error) {
	var body struct {
		Result     json.RawMessage `json:"result"`
		Pagination *PageInfo       `json:"pagination"`
		Error      json.RawMessage `json:"error"`
		Errors     json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(rb, &body); err != nil {
		// Not an enveloped response; treat the whole body as the payload.
		return &Envelope{Result: rb}, nil
	}
	if isPresent(body.Error) || isPresent(body.Errors) {
		return nil, &APIError{Status: status, Body: string(rb)}
	}
	if body.Result == nil {
		return &Envelope{Result: rb}, nil
	}
	return &Envelope{Result: body.Result, Pagination: body.Pagination}, nil
}

// isPresent reports whether a raw field carries a value (absent and JSON null don't).
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// unscopedPath lists the v1 endpoints that must omit X-AP-Context.
func unscopedPath(path string) bool {
	p := strings.SplitN(path, "?", 2)[0]
	switch {
	case p == "/v1/me", p == "/v1/acls", p == "/v1/advertiser-resources":
		return true
	case strings.HasPrefix(p, "/v1/orgs/"):
		return true
	case p == "/v1/ad-accounts": // POST create
		return true
	}
	return false
}

func backoff(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Selector is the v1 query body: field projection, filters, sort, pagination.
type Selector struct {
	Fields     []string    `json:"fields,omitempty"`
	Filters    []Filter    `json:"filters,omitempty"`
	Sorting    []Sort      `json:"sorting,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Filter narrows a query request. Value is a scalar for EQUALS/LIKE and a list
// for IN/BETWEEN; which operators a field accepts is per-field (LIKE is the
// substring match, EQUALS and IN are universal).
type Filter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // EQUALS, IN, LIKE, BETWEEN, GREATER_THAN, ...
	Value    any    `json:"value"`
}

// Sort orders a query request.
type Sort struct {
	Field string `json:"field"`
	Order string `json:"order"` // ASC | DESC
}

// Pagination bounds a query request. PageSize maxes out at 1000.
type Pagination struct {
	Offset   int `json:"offset"`
	PageSize int `json:"pageSize"`
}

// Query POSTs {path}/query with a selector and paginates until `limit`
// items (0 = all), returning raw JSON items.
func (c *Client) Query(ctx context.Context, path string, sel *Selector, limit int) ([]json.RawMessage, error) {
	const page = 200
	if sel == nil {
		sel = &Selector{}
	}
	var items []json.RawMessage
	offset := 0
	for {
		want := page
		if limit > 0 && limit-len(items) < page {
			want = limit - len(items)
		}
		sel.Pagination = &Pagination{Offset: offset, PageSize: want}
		env, err := c.DoRaw(ctx, "POST", path, sel)
		if err != nil {
			return nil, err
		}
		batch, done := pageItems(env)
		items = append(items, batch...)
		if done {
			return items, nil
		}
		offset += len(batch)
		if len(batch) < want || (limit > 0 && len(items) >= limit) {
			return items, nil
		}
		if env.Pagination != nil && env.Pagination.TotalCount > 0 && offset >= env.Pagination.TotalCount {
			return items, nil
		}
	}
}

// pageItems decodes one page of results. done reports that paging must stop:
// the response was empty or carried a single object instead of a list.
func pageItems(env *Envelope) (batch []json.RawMessage, done bool) {
	if env == nil || len(env.Result) == 0 {
		return nil, true
	}
	if err := json.Unmarshal(env.Result, &batch); err != nil {
		return []json.RawMessage{env.Result}, true
	}
	return batch, false
}

// GetPaged fetches GET list endpoints page by page until `limit` items (0 = all).
func (c *Client) GetPaged(ctx context.Context, path string, limit int) ([]json.RawMessage, error) {
	const page = 200
	var items []json.RawMessage
	offset := 0
	for {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		want := page
		if limit > 0 && limit-len(items) < page {
			want = limit - len(items)
		}
		env, err := c.DoRaw(ctx, "GET", fmt.Sprintf("%s%slimit=%d&offset=%d", path, sep, want, offset), nil)
		if err != nil {
			return nil, err
		}
		batch, done := pageItems(env)
		items = append(items, batch...)
		if done {
			return items, nil
		}
		offset += len(batch)
		if len(batch) < want || (limit > 0 && len(items) >= limit) {
			return items, nil
		}
		if env.Pagination != nil && env.Pagination.TotalCount > 0 && offset >= env.Pagination.TotalCount {
			return items, nil
		}
	}
}

// EqFilter is shorthand for a selector with a single EQUALS filter.
func EqFilter(field string, value any) *Selector {
	return &Selector{Filters: []Filter{{Field: field, Operator: "EQUALS", Value: value}}}
}

// BulkBody wraps records for the bulk-create/bulk-update endpoints, which take
// {"items": [...]} rather than a bare array.
func BulkBody(items []map[string]any) map[string]any {
	return map[string]any{"items": items}
}

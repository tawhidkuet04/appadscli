// Package api is the Apple Ads Platform API v1 client: bearer auth,
// ad-account context header, rate-limit-aware transport (token bucket +
// 429 backoff), and the query/selector pagination pattern.
//
// Contract (validated against Apple's docs and shipping v1 clients):
//   - Base URL https://api.ads.apple.com/v1/
//   - Ad-account-scoped requests send "X-AP-Context: adAccountId=<id>;"
//   - GET /me, /acls, /orgs/{id}, /advertiser-resources, POST /ad-accounts
//     omit the context header
//   - List endpoints are POST {resource}/query with a Selector body
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

	"github.com/tawhidkuet04/appadscli/internal/auth"
	"github.com/tawhidkuet04/appadscli/internal/config"
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

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	var parsed struct {
		Error struct {
			Errors []struct {
				MessageCode string `json:"messageCode"`
				Message     string `json:"message"`
				Field       string `json:"field"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &parsed) == nil && len(parsed.Error.Errors) > 0 {
		parts := make([]string, 0, len(parsed.Error.Errors))
		for _, er := range parsed.Error.Errors {
			p := er.Message
			if er.Field != "" {
				p = er.Field + ": " + p
			}
			parts = append(parts, p)
		}
		msg = strings.Join(parts, "; ")
	}
	return fmt.Sprintf("apple ads api: HTTP %d: %s", e.Status, msg)
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

// Get issues a GET and decodes the `data` envelope into out (if non-nil).
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

// Envelope is Apple's standard response wrapper.
type Envelope struct {
	Data       json.RawMessage `json:"data"`
	Pagination *struct {
		TotalResults int `json:"totalResults"`
		StartIndex   int `json:"startIndex"`
		ItemsPerPage int `json:"itemsPerPage"`
	} `json:"pagination"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	env, err := c.DoRaw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out != nil && env != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
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
		var env Envelope
		if err := json.Unmarshal(rb, &env); err != nil || len(env.Data) == 0 {
			// Not an enveloped response; treat the whole body as data.
			return &Envelope{Data: rb}, nil
		}
		return &env, nil
	}
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

// Selector is the v1 query body: filter conditions, sort, and pagination.
type Selector struct {
	Fields     []string    `json:"fields,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	OrderBy    []OrderBy   `json:"orderBy,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Condition filters a query request.
type Condition struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"` // EQUALS, IN, CONTAINS_ANY, STARTSWITH, ...
	Values   []string `json:"values"`
}

// OrderBy sorts a query request.
type OrderBy struct {
	Field     string `json:"field"`
	SortOrder string `json:"sortOrder"` // ASCENDING | DESCENDING
}

// Pagination bounds a query request.
type Pagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
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
		sel.Pagination = &Pagination{Offset: offset, Limit: want}
		env, err := c.DoRaw(ctx, "POST", path, sel)
		if err != nil {
			return nil, err
		}
		if env == nil || len(env.Data) == 0 {
			return items, nil
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(env.Data, &batch); err != nil {
			return []json.RawMessage{env.Data}, nil
		}
		items = append(items, batch...)
		offset += len(batch)
		if len(batch) < want || (limit > 0 && len(items) >= limit) {
			return items, nil
		}
		if env.Pagination != nil && offset >= env.Pagination.TotalResults {
			return items, nil
		}
	}
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
		if env == nil || len(env.Data) == 0 {
			return items, nil
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(env.Data, &batch); err != nil {
			return []json.RawMessage{env.Data}, nil
		}
		items = append(items, batch...)
		offset += len(batch)
		if len(batch) < want || (limit > 0 && len(items) >= limit) {
			return items, nil
		}
		if env.Pagination != nil && offset >= env.Pagination.TotalResults {
			return items, nil
		}
	}
}

// EqCond is shorthand for a single EQUALS condition.
func EqCond(field, value string) *Selector {
	return &Selector{Conditions: []Condition{{Field: field, Operator: "EQUALS", Values: []string{value}}}}
}

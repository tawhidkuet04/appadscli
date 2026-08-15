package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"30d":        now.AddDate(0, 0, -30),
		"4w":         now.AddDate(0, 0, -28),
		"2m":         now.AddDate(0, -2, 0),
		"24h":        now.Add(-24 * time.Hour),
		"2026-01-02": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		"":           now.AddDate(0, 0, -30),
	}
	for in, want := range cases {
		got, err := ParseSince(in, now)
		if err != nil {
			t.Errorf("ParseSince(%q) error: %v", in, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("ParseSince(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseSince("banana", now); err == nil {
		t.Error("expected error for garbage input")
	}
}

func TestFlattenRows(t *testing.T) {
	row := json.RawMessage(`{
		"metadata": {"campaignId": 42, "campaignName": "brand-us"},
		"total": {"impressions": 100, "taps": 10,
			"localSpend": {"amount": "12.34", "currency": "USD"}}
	}`)
	out := flattenRows([]json.RawMessage{row})
	if len(out) != 1 {
		t.Fatalf("got %d rows", len(out))
	}
	if got := Field(out[0], "campaignName"); got != "brand-us" {
		t.Errorf("campaignName = %q", got)
	}
	if got := Field(out[0], "localSpend"); got != "12.34" {
		t.Errorf("localSpend = %q, want flattened amount", got)
	}
	if got := FloatField(out[0], "impressions"); got != 100 {
		t.Errorf("impressions = %v", got)
	}
}

func TestFieldAndMoney(t *testing.T) {
	raw := json.RawMessage(`{"a":{"b":{"amount":"1.50","currency":"USD"}},"n":3,"s":"x","t":true}`)
	if got := Money(raw, "a.b"); got != "1.50 USD" {
		t.Errorf("Money = %q", got)
	}
	if got := Field(raw, "n"); got != "3" {
		t.Errorf("int field = %q", got)
	}
	if got := Field(raw, "missing.path"); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestUnscopedPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/v1/me": true, "/v1/acls": true, "/v1/orgs/123": true,
		"/v1/campaigns/query": false, "/v1/ad-accounts": true, "/v1/ad-accounts/9": false,
	} {
		if got := unscopedPath(path); got != want {
			t.Errorf("unscopedPath(%q) = %v, want %v", path, got, want)
		}
	}
}

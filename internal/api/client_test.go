package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseEnvelopeResult(t *testing.T) {
	// /v1/acls and friends nest the payload under result, alongside success/info.
	body := []byte(`{"success":true,"result":{"acls":[{"roles":["API Campaign Manager"]}]},"info":{}}`)
	env, err := parseEnvelope(200, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		ACLs []json.RawMessage `json:"acls"`
	}
	if err := json.Unmarshal(env.Result, &out); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if len(out.ACLs) != 1 {
		t.Errorf("got %d acls, want 1", len(out.ACLs))
	}
}

func TestParseEnvelopePagination(t *testing.T) {
	body := []byte(`{"result":[{"id":1}],"pagination":{"offset":0,"pageSize":1,"totalCount":7}}`)
	env, err := parseEnvelope(200, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Pagination == nil || env.Pagination.TotalCount != 7 {
		t.Fatalf("pagination = %+v, want totalCount 7", env.Pagination)
	}
	batch, done := pageItems(env)
	if done || len(batch) != 1 {
		t.Errorf("pageItems = %d items, done=%v", len(batch), done)
	}
}

func TestParseEnvelopeErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"detail with field": {400,
			`{"error":{"code":"VALIDATION_ERROR","details":[{"code":"REQUIRED_VALUE_FIELD","message":"Field 'name' is required","info":{"field":"name"}}]}}`,
			"name: Field 'name' is required"},
		"bare errors list": {400,
			`{"errors":[{"code":"REQUEST_UNRECOGNIZED_PROPERTY","message":"Unrecognized field [startTime].","info":{"field":"startTime"}}],"errorCount":1}`,
			"startTime: Unrecognized field [startTime]."},
		"message only": {400,
			`{"error":{"code":"VALIDATION_ERROR","message":"Request validation failed"}}`,
			"Request validation failed"},
		// change-history reports failures under HTTP 200.
		"error under 200": {200,
			`{"pagination":null,"result":null,"error":{"code":"VALIDATION_ERROR","message":"Event Time Range required","details":[{"code":"INVALID_VALUE_FIELD","message":"Invalid/Unsupported value of eventTime","info":{"field":"eventTime"}}]}}`,
			"eventTime: Invalid/Unsupported value of eventTime"},
	}
	for name, tc := range cases {
		env, err := parseEnvelope(tc.status, []byte(tc.body))
		if err == nil {
			t.Errorf("%s: got envelope %+v, want error", name, env)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %q, want it to contain %q", name, err, tc.want)
		}
	}
}

func TestAPIErrorFlattened(t *testing.T) {
	// Some 404s put the error object at the response root instead of under "error".
	err := &APIError{Status: 404, Body: `{"code":"DATA_NOT_FOUND","message":"Data not found id","details":[]}`}
	if !strings.Contains(err.Error(), "Data not found id") {
		t.Errorf("error = %q, want the API message", err)
	}
}

func TestParseEnvelopeUnwrapped(t *testing.T) {
	// A body with no result key is handed through whole rather than dropped.
	body := []byte(`{"command":"BULK_OPERATIONS","success":true}`)
	env, err := parseEnvelope(200, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(env.Result) != string(body) {
		t.Errorf("result = %s, want the whole body", env.Result)
	}
}

func TestSelectorEncoding(t *testing.T) {
	sel := &Selector{
		Filters:    []Filter{{Field: "campaignId", Operator: "EQUALS", Value: "42"}},
		Sorting:    []Sort{{Field: "localSpend", Order: "DESC"}},
		Pagination: &Pagination{Offset: 0, PageSize: 200},
	}
	b, err := json.Marshal(sel)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"filters":[{"field":"campaignId","operator":"EQUALS","value":"42"}],"sorting":[{"field":"localSpend","order":"DESC"}],"pagination":{"offset":0,"pageSize":200}}`
	if string(b) != want {
		t.Errorf("selector = %s\nwant       %s", b, want)
	}
}

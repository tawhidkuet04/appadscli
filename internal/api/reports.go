package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ReportRequest is the reporting query body shared by all report endpoints.
type ReportRequest struct {
	TimeRange  *TimeRange  `json:"timeRange"`
	Fields     []string    `json:"fields,omitempty"`
	Filters    []Filter    `json:"filters,omitempty"`
	Sorting    []Sort      `json:"sorting,omitempty"`
	GroupBy    []string    `json:"groupBy,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// TimeRange is the required reporting window. Granularity splits rows per
// period (HOURLY|DAILY|WEEKLY|MONTHLY, and the window has to be short enough
// for the period picked); empty means totals only.
type TimeRange struct {
	Start       string `json:"start"` // YYYY-MM-DD
	End         string `json:"end"`   // YYYY-MM-DD
	Granularity string `json:"granularity,omitempty"`
	TimeZone    string `json:"timeZone,omitempty"` // ORTZ|UTC
}

// ParseSince turns "30d", "7d", "24h", or a YYYY-MM-DD date into a start time.
func ParseSince(since string, now time.Time) (time.Time, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return now.AddDate(0, 0, -30), nil
	}
	if t, err := time.Parse("2006-01-02", since); err == nil {
		return t, nil
	}
	if len(since) > 1 {
		n, err := strconv.Atoi(since[:len(since)-1])
		if err == nil {
			switch since[len(since)-1] {
			case 'd':
				return now.AddDate(0, 0, -n), nil
			case 'w':
				return now.AddDate(0, 0, -7*n), nil
			case 'm':
				return now.AddDate(0, -n, 0), nil
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse --since %q (use 30d, 4w, 24h, or YYYY-MM-DD)", since)
}

// NewReportRequest builds a request covering [since, today].
func NewReportRequest(since string, granularity string) (*ReportRequest, error) {
	now := time.Now()
	start, err := ParseSince(since, now)
	if err != nil {
		return nil, err
	}
	req := &ReportRequest{
		TimeRange: &TimeRange{
			Start:       start.Format("2006-01-02"),
			End:         now.Format("2006-01-02"),
			Granularity: strings.ToUpper(granularity),
			TimeZone:    "ORTZ",
		},
		Sorting:    []Sort{{Field: "localSpend", Order: "DESC"}},
		Pagination: &Pagination{Offset: 0, PageSize: 1000},
	}
	return req, nil
}

// RunReport POSTs a report query and returns flattened rows: metadata fields
// merged with total (or per-granularity) metrics, money objects reduced to
// plain amounts.
func (c *Client) RunReport(ctx context.Context, path string, req *ReportRequest) ([]json.RawMessage, error) {
	env, err := c.DoRaw(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}
	if env == nil || len(env.Result) == 0 {
		return nil, nil
	}
	// Expected shape: {"result":{"rows":[{metadata, totalMetrics, granularMetrics}]}}
	var wrapper struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(env.Result, &wrapper); err == nil {
		return flattenRows(wrapper.Rows), nil
	}
	// Fallback: the result is already an array of rows.
	var rows []json.RawMessage
	if err := json.Unmarshal(env.Result, &rows); err == nil {
		return flattenRows(rows), nil
	}
	return []json.RawMessage{env.Result}, nil
}

func flattenRows(rows []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		var row map[string]json.RawMessage
		if err := json.Unmarshal(r, &row); err != nil {
			out = append(out, r)
			continue
		}
		flat := map[string]any{}
		mergeObj(flat, row["metadata"])
		if g, ok := row["granularMetrics"]; ok {
			// Per-period rows: emit one flat row per period.
			var periods []json.RawMessage
			if json.Unmarshal(g, &periods) == nil && len(periods) > 0 {
				for _, p := range periods {
					pf := map[string]any{}
					for k, v := range flat {
						pf[k] = v
					}
					mergeObj(pf, p)
					if b, err := json.Marshal(flattenMoney(pf)); err == nil {
						out = append(out, b)
					}
				}
				continue
			}
		}
		mergeObj(flat, row["totalMetrics"])
		if len(flat) == 0 {
			out = append(out, r)
			continue
		}
		if b, err := json.Marshal(flattenMoney(flat)); err == nil {
			out = append(out, b)
		}
	}
	return out
}

func mergeObj(dst map[string]any, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		for k, v := range m {
			dst[k] = v
		}
	}
}

// flattenMoney reduces {"amount":"1.5","currency":"USD"} values to "1.50".
func flattenMoney(m map[string]any) map[string]any {
	for k, v := range m {
		if obj, ok := v.(map[string]any); ok {
			if amt, ok := obj["amount"]; ok {
				if _, hasCur := obj["currency"]; hasCur {
					m[k] = amt
				}
			}
		}
	}
	return m
}

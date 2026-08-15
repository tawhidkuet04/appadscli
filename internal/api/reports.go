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
	StartTime                  string    `json:"startTime"`
	EndTime                    string    `json:"endTime"`
	Granularity                string    `json:"granularity,omitempty"` // HOURLY|DAILY|WEEKLY|MONTHLY
	TimeZone                   string    `json:"timeZone,omitempty"`    // ORTZ|UTC
	Selector                   *Selector `json:"selector,omitempty"`
	GroupBy                    []string  `json:"groupBy,omitempty"`
	ReturnRowTotals            bool      `json:"returnRowTotals"`
	ReturnGrandTotals          bool      `json:"returnGrandTotals"`
	ReturnRecordsWithNoMetrics bool      `json:"returnRecordsWithNoMetrics"`
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
		StartTime:       start.Format("2006-01-02"),
		EndTime:         now.Format("2006-01-02"),
		TimeZone:        "ORTZ",
		ReturnRowTotals: false,
		Selector: &Selector{
			OrderBy:    []OrderBy{{Field: "localSpend", SortOrder: "DESCENDING"}},
			Pagination: &Pagination{Offset: 0, Limit: 1000},
		},
	}
	if granularity != "" {
		req.Granularity = strings.ToUpper(granularity)
		// Row totals are unavailable when granularity is set.
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
	if env == nil || len(env.Data) == 0 {
		return nil, nil
	}
	// Expected shape: {"reportingDataResponse":{"row":[{metadata, total|granularity}]}}
	var wrapper struct {
		ReportingDataResponse struct {
			Row []json.RawMessage `json:"row"`
		} `json:"reportingDataResponse"`
	}
	if err := json.Unmarshal(env.Data, &wrapper); err == nil && len(wrapper.ReportingDataResponse.Row) > 0 {
		return flattenRows(wrapper.ReportingDataResponse.Row), nil
	}
	// Fallback: data is already an array of rows.
	var rows []json.RawMessage
	if err := json.Unmarshal(env.Data, &rows); err == nil {
		return flattenRows(rows), nil
	}
	return []json.RawMessage{env.Data}, nil
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
		if g, ok := row["granularity"]; ok {
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
		mergeObj(flat, row["total"])
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

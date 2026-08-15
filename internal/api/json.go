package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Field extracts a dotted path (e.g. "budgetAmount.amount") from a raw JSON
// object and renders it as a display string. Missing paths return "".
func Field(raw json.RawMessage, path string) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v, ok = m[part]
		if !ok {
			return ""
		}
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', 2, 64)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// Money renders Apple's {"amount":"1.50","currency":"USD"} objects.
func Money(raw json.RawMessage, path string) string {
	amt := Field(raw, path+".amount")
	cur := Field(raw, path+".currency")
	if amt == "" {
		return ""
	}
	if cur == "" {
		return amt
	}
	return amt + " " + cur
}

// Table projects raw items into rows given column specs "Header=json.path".
func Table(items []json.RawMessage, cols []string) (headers []string, rows [][]string) {
	paths := make([]string, len(cols))
	headers = make([]string, len(cols))
	for i, c := range cols {
		parts := strings.SplitN(c, "=", 2)
		headers[i] = parts[0]
		if len(parts) == 2 {
			paths[i] = parts[1]
		} else {
			paths[i] = parts[0]
		}
	}
	for _, it := range items {
		row := make([]string, len(cols))
		for i, p := range paths {
			if strings.HasPrefix(p, "$money:") {
				row[i] = Money(it, strings.TrimPrefix(p, "$money:"))
			} else {
				row[i] = Field(it, p)
			}
		}
		rows = append(rows, row)
	}
	return headers, rows
}

// FloatField parses a numeric field, returning 0 when absent.
func FloatField(raw json.RawMessage, path string) float64 {
	f, _ := strconv.ParseFloat(Field(raw, path), 64)
	return f
}

// FmtUSD formats a float as a money string with 2 decimals.
func FmtUSD(v float64) string { return fmt.Sprintf("%.2f", v) }

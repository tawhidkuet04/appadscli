// Package output renders command results as table, json, csv, or markdown.
// Resolution order: --output flag > ADASTRA_OUTPUT env > config default >
// auto (TTY → table, pipe/CI → json).
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Format is an output format name.
type Format string

const (
	Auto     Format = ""
	Table    Format = "table"
	JSON     Format = "json"
	CSV      Format = "csv"
	Markdown Format = "markdown"
)

// Renderer holds the resolved format and destination.
type Renderer struct {
	Format Format
	W      io.Writer
}

// New resolves the effective format. flagVal comes from --output; def from config.
func New(flagVal, def string) *Renderer {
	f := Format(flagVal)
	if f == Auto {
		if env := os.Getenv("ADASTRA_OUTPUT"); env != "" {
			f = Format(env)
		} else if def != "" {
			f = Format(def)
		} else if isTTY() {
			f = Table
		} else {
			f = JSON
		}
	}
	return &Renderer{Format: f, W: os.Stdout}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// JSON always emits raw JSON regardless of format — for commands whose
// natural shape is a document rather than rows.
func (r *Renderer) JSON(v any) error {
	enc := json.NewEncoder(r.W)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Rows renders tabular data. headers name the columns; each row is a slice of
// cells. rawJSON, when non-nil, is emitted verbatim for json output (so json
// consumers get full API objects, not the table projection).
func (r *Renderer) Rows(headers []string, rows [][]string, rawJSON any) error {
	switch r.Format {
	case JSON:
		if rawJSON != nil {
			return r.JSON(rawJSON)
		}
		out := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			m := map[string]string{}
			for i, h := range headers {
				if i < len(row) {
					m[strings.ToLower(h)] = row[i]
				}
			}
			out = append(out, m)
		}
		return r.JSON(out)
	case CSV:
		w := csv.NewWriter(r.W)
		if err := w.Write(headers); err != nil {
			return err
		}
		if err := w.WriteAll(rows); err != nil {
			return err
		}
		w.Flush()
		return w.Error()
	case Markdown:
		fmt.Fprintln(r.W, "| "+strings.Join(headers, " | ")+" |")
		seps := make([]string, len(headers))
		for i := range seps {
			seps[i] = "---"
		}
		fmt.Fprintln(r.W, "| "+strings.Join(seps, " | ")+" |")
		for _, row := range rows {
			fmt.Fprintln(r.W, "| "+strings.Join(row, " | ")+" |")
		}
		return nil
	default: // table
		return renderTable(r.W, headers, rows)
	}
}

func renderTable(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && utf8.RuneCountInString(c) > widths[i] {
				widths[i] = utf8.RuneCountInString(c)
			}
		}
	}
	pad := func(s string, n int) string {
		return s + strings.Repeat(" ", n-utf8.RuneCountInString(s))
	}
	cells := make([]string, len(headers))
	for i, h := range headers {
		cells[i] = pad(strings.ToUpper(h), widths[i])
	}
	fmt.Fprintln(w, strings.TrimRight(strings.Join(cells, "  "), " "))
	for _, row := range rows {
		for i := range cells {
			c := ""
			if i < len(row) {
				c = row[i]
			}
			cells[i] = pad(c, widths[i])
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(cells, "  "), " "))
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no rows)")
	}
	return nil
}

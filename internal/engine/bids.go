package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/tawhidkuet04/appadscli/internal/api"
	"github.com/tawhidkuet04/appadscli/internal/store"
)

// BidAdjustOpts tunes keyword bids in an ad group toward a CPA or ROAS target.
type BidAdjustOpts struct {
	AdGroupID    string
	TargetCPA    float64 // bid down when observed CPA above target, up when below
	TargetROAS   float64 // e.g. 1.5 = 150%; needs RevenueCat data
	MaxChangePct float64 // cap per-run change, e.g. 20
	Since        string
	MinTaps      float64 // ignore keywords with fewer taps (noise)
}

// BidChange is one proposed/applied bid move.
type BidChange struct {
	KeywordID string  `json:"keywordId"`
	Text      string  `json:"text"`
	OldBid    float64 `json:"oldBid"`
	NewBid    float64 `json:"newBid"`
	CPA       float64 `json:"cpa,omitempty"`
	ROAS      float64 `json:"roas,omitempty"`
	Installs  float64 `json:"installs"`
	Reason    string  `json:"reason"`
}

// BidPlan computes bid changes from the keyword report without mutating.
func BidPlan(ctx context.Context, c *api.Client, st *store.Store, o BidAdjustOpts) ([]BidChange, error) {
	req, err := api.NewReportRequest(o.Since, "")
	if err != nil {
		return nil, err
	}
	req.Selector.Conditions = []api.Condition{{
		Field: "adGroupId", Operator: "EQUALS", Values: []string{o.AdGroupID},
	}}
	rows, err := c.RunReport(ctx, "/v1/reports/apps/keywords/query", req)
	if err != nil {
		return nil, err
	}
	var roasByKw map[string]float64
	if o.TargetROAS > 0 {
		roasByKw, err = roasBySearchTermKeyword(st)
		if err != nil {
			return nil, fmt.Errorf("ROAS mode needs RevenueCat data — run `appadscli rc ingest` first: %w", err)
		}
	}
	maxF := o.MaxChangePct / 100
	var changes []BidChange
	for _, r := range rows {
		taps := api.FloatField(r, "taps")
		if taps < o.MinTaps {
			continue
		}
		kwID := api.Field(r, "keywordId")
		oldBid := api.FloatField(r, "bidAmount")
		if oldBid == 0 {
			continue
		}
		installs := api.FloatField(r, "totalInstalls")
		spend := api.FloatField(r, "localSpend")
		var cpa float64
		if installs > 0 {
			cpa = spend / installs
		}
		var ratio float64
		var reason string
		ch := BidChange{KeywordID: kwID, Text: api.Field(r, "keyword"), OldBid: oldBid, CPA: cpa, Installs: installs}
		if o.TargetROAS > 0 {
			roas := 0.0
			if spend > 0 {
				roas = roasByKw[kwID] / spend
			}
			ch.ROAS = roas
			if roas == 0 && installs == 0 {
				ratio = 1 - maxF // no signal, cool down
				reason = "no installs and no revenue — bid down"
			} else {
				ratio = clamp(roas/o.TargetROAS, 1-maxF, 1+maxF)
				reason = fmt.Sprintf("ROAS %.2f vs target %.2f", roas, o.TargetROAS)
			}
		} else {
			switch {
			case installs == 0 && spend > o.TargetCPA:
				ratio = 1 - maxF
				reason = fmt.Sprintf("%.2f spent, 0 installs — bid down", spend)
			case cpa == 0:
				continue // no spend yet, leave alone
			default:
				ratio = clamp(o.TargetCPA/cpa, 1-maxF, 1+maxF)
				reason = fmt.Sprintf("CPA %.2f vs target %.2f", cpa, o.TargetCPA)
			}
		}
		newBid := math.Round(oldBid*ratio*100) / 100
		if newBid == oldBid || newBid <= 0 {
			continue
		}
		ch.NewBid = newBid
		ch.Reason = reason
		changes = append(changes, ch)
	}
	return changes, nil
}

// BidApply pushes bid changes via keywords/bulk-update.
func BidApply(ctx context.Context, c *api.Client, st *store.Store, changes []BidChange, currency string) (json.RawMessage, error) {
	var body []map[string]any
	for _, ch := range changes {
		body = append(body, map[string]any{
			"id":        json.Number(ch.KeywordID),
			"bidAmount": map[string]string{"amount": api.FmtUSD(ch.NewBid), "currency": currency},
		})
	}
	var out json.RawMessage
	if err := c.Post(ctx, "/v1/keywords/bulk-update", body, &out); err != nil {
		return nil, err
	}
	for _, ch := range changes {
		_ = st.LogMutation("keyword", ch.KeywordID, "bid",
			fmt.Sprintf("%.2f→%.2f (%s)", ch.OldBid, ch.NewBid, ch.Reason))
	}
	return out, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// roasBySearchTermKeyword sums RevenueCat proceeds by ASA keywordId.
func roasBySearchTermKeyword(st *store.Store) (map[string]float64, error) {
	rows, err := st.DB.Query(`SELECT keyword_id, SUM(proceeds) FROM rc_transactions
		WHERE keyword_id IS NOT NULL AND keyword_id != '' GROUP BY keyword_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	n := 0
	for rows.Next() {
		var kw string
		var sum float64
		if err := rows.Scan(&kw, &sum); err != nil {
			return nil, err
		}
		out[kw] = sum
		n++
	}
	if n == 0 {
		return out, fmt.Errorf("no RevenueCat transactions with keyword attribution in local store")
	}
	return out, rows.Err()
}

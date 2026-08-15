package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tawhidkuet04/asacli/internal/api"
	"github.com/tawhidkuet04/asacli/internal/store"
)

// HarvestOpts configures the harvest loop.
type HarvestOpts struct {
	DiscoveryCampaign string  // campaign id mined for search terms
	TargetCampaign    string  // campaign id receiving exact-match winners
	Since             string  // report window, e.g. "30d"
	MinInstalls       float64 // promote terms with at least this many installs
	MaxCPA            float64 // …and CPA at or under this (0 = ignore)
	AutoNegate        bool    // negate wasteful terms in discovery
	WasteTaps         float64 // taps with zero installs ≥ this → wasteful
	BidFactor         float64 // promoted keyword bid = observed CPT × factor
	RankBy            string  // "installs" (default) or "roas" (needs rc data)
}

// HarvestAction is one proposed/executed change.
type HarvestAction struct {
	Action     string  `json:"action"` // promote | negate-in-discovery | negate-waste
	SearchTerm string  `json:"searchTerm"`
	Installs   float64 `json:"installs"`
	Taps       float64 `json:"taps"`
	Spend      float64 `json:"spend"`
	CPA        float64 `json:"cpa,omitempty"`
	ROAS       float64 `json:"roas,omitempty"`
	Bid        float64 `json:"bid,omitempty"`
	Reason     string  `json:"reason"`
}

// HarvestPlan computes the harvest actions without mutating anything.
func HarvestPlan(ctx context.Context, c *api.Client, st *store.Store, o HarvestOpts) ([]HarvestAction, error) {
	req, err := api.NewReportRequest(o.Since, "")
	if err != nil {
		return nil, err
	}
	req.Selector.Conditions = []api.Condition{{
		Field: "campaignId", Operator: "EQUALS", Values: []string{o.DiscoveryCampaign},
	}}
	rows, err := c.RunReport(ctx, "/v1/reports/apps/searchterms/query", req)
	if err != nil {
		return nil, err
	}
	already, err := st.HarvestedTerms()
	if err != nil {
		return nil, err
	}
	var roasByTerm map[string]float64
	if strings.EqualFold(o.RankBy, "roas") {
		roasByTerm, _ = roasBySearchTermKeyword(st)
	}
	var actions []HarvestAction
	for _, r := range rows {
		term := strings.TrimSpace(api.Field(r, "searchTermText"))
		if term == "" {
			continue
		}
		installs := api.FloatField(r, "totalInstalls")
		taps := api.FloatField(r, "taps")
		spend := api.FloatField(r, "localSpend")
		var cpa float64
		if installs > 0 {
			cpa = spend / installs
		}
		var avgCPT float64
		if taps > 0 {
			avgCPT = spend / taps
		}
		switch {
		case installs >= o.MinInstalls && (o.MaxCPA == 0 || cpa <= o.MaxCPA):
			if already[term] {
				continue // promoted in an earlier run
			}
			bid := avgCPT * o.BidFactor
			if bid <= 0 {
				bid = 1.00
			}
			a := HarvestAction{
				Action: "promote", SearchTerm: term, Installs: installs, Taps: taps,
				Spend: spend, CPA: cpa, Bid: bid,
				Reason: fmt.Sprintf("%.0f installs at %.2f CPA — promote to exact in target, negate in discovery", installs, cpa),
			}
			if roasByTerm != nil {
				a.ROAS = roasByTerm[keyForTerm(r)]
			}
			actions = append(actions, a)
		case o.AutoNegate && installs == 0 && taps >= o.WasteTaps:
			actions = append(actions, HarvestAction{
				Action: "negate-waste", SearchTerm: term, Installs: 0, Taps: taps, Spend: spend,
				Reason: fmt.Sprintf("%.0f taps, 0 installs, %.2f spent — negate in discovery", taps, spend),
			})
		}
	}
	return actions, nil
}

func keyForTerm(r json.RawMessage) string {
	return api.Field(r, "keywordId")
}

// HarvestApply executes the actions: bulk-create exact keywords in the
// target's ad group, and exact negatives in the discovery campaign.
func HarvestApply(ctx context.Context, c *api.Client, st *store.Store, o HarvestOpts, actions []HarvestAction, currency string) (map[string]any, error) {
	// Find the target campaign's first ad group.
	targetAG, err := firstAdGroup(ctx, c, o.TargetCampaign)
	if err != nil {
		return nil, fmt.Errorf("target campaign: %w", err)
	}
	var creates, negatives []map[string]any
	for _, a := range actions {
		switch a.Action {
		case "promote":
			creates = append(creates, map[string]any{
				"adGroupId": json.Number(targetAG),
				"text":      a.SearchTerm,
				"matchType": "EXACT",
				"bidAmount": map[string]string{"amount": api.FmtUSD(a.Bid), "currency": currency},
				"status":    "ACTIVE",
			})
			negatives = append(negatives, map[string]any{
				"campaignId": json.Number(o.DiscoveryCampaign),
				"text":       a.SearchTerm,
				"matchType":  "EXACT",
				"status":     "ACTIVE",
			})
		case "negate-waste":
			negatives = append(negatives, map[string]any{
				"campaignId": json.Number(o.DiscoveryCampaign),
				"text":       a.SearchTerm,
				"matchType":  "EXACT",
				"status":     "ACTIVE",
			})
		}
	}
	result := map[string]any{}
	if len(creates) > 0 {
		var out json.RawMessage
		if err := c.Post(ctx, "/v1/keywords/bulk-create", creates, &out); err != nil {
			return nil, fmt.Errorf("promoting keywords: %w", err)
		}
		result["promoted"] = out
	}
	if len(negatives) > 0 {
		var out json.RawMessage
		if err := c.Post(ctx, "/v1/negative-keywords/bulk-create", negatives, &out); err != nil {
			return nil, fmt.Errorf("adding negatives: %w", err)
		}
		result["negated"] = out
	}
	for _, a := range actions {
		action := "promote"
		if a.Action != "promote" {
			action = "negate"
		}
		_ = st.LogHarvest(a.SearchTerm, o.DiscoveryCampaign, o.TargetCampaign, action, a.Reason)
		_ = st.LogMutation("keyword", a.SearchTerm, action, "harvest")
	}
	result["actions"] = len(actions)
	return result, nil
}

func firstAdGroup(ctx context.Context, c *api.Client, campaignID string) (string, error) {
	items, err := c.Query(ctx, "/v1/adgroups/query", api.EqCond("campaignId", campaignID), 1)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("campaign %s has no ad groups", campaignID)
	}
	return api.Field(items[0], "id"), nil
}

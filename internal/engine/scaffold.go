// Package engine implements adastra's opinionated workflows: campaign
// scaffolding, the harvest loop, declarative bid rules, plan/apply, and the
// watch guardrail evaluator.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tawhidjoarder/adastra/internal/api"
	"github.com/tawhidjoarder/adastra/internal/store"
)

// ScaffoldOpts configures the best-practice campaign structure.
type ScaffoldOpts struct {
	AdamID      string
	Structure   []string // subset of brand, category, competitor, discovery
	DailyBudget float64
	Currency    string
	Countries   []string
	DefaultBid  float64
}

// ScaffoldCampaign is one planned campaign + ad group.
type ScaffoldCampaign struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	DailyBudget float64  `json:"dailyBudget"`
	Currency    string   `json:"currency"`
	Countries   []string `json:"countries"`
	AdGroupName string   `json:"adGroupName"`
	DefaultBid  float64  `json:"defaultBid"`
	SearchMatch bool     `json:"searchMatch"`
	Notes       string   `json:"notes"`
}

// Plan is the full scaffold proposal.
type Plan struct {
	AdamID    string             `json:"adamId"`
	Campaigns []ScaffoldCampaign `json:"campaigns"`
}

// ScaffoldPlan builds the proposal without touching the API.
func ScaffoldPlan(o ScaffoldOpts) *Plan {
	cc := strings.ToLower(strings.Join(o.Countries, "-"))
	p := &Plan{AdamID: o.AdamID}
	for _, kind := range o.Structure {
		kind = strings.TrimSpace(strings.ToLower(kind))
		sc := ScaffoldCampaign{
			Kind: kind, DailyBudget: o.DailyBudget, Currency: o.Currency,
			Countries: o.Countries, DefaultBid: o.DefaultBid,
			Name: fmt.Sprintf("%s-%s", kind, cc), AdGroupName: kind + "-exact",
		}
		switch kind {
		case "brand":
			sc.Notes = "add your app/brand name as exact-match keywords; bid low — you own this traffic"
		case "category":
			sc.Notes = "add researched category keywords exact-match (see `adastra aso research`)"
		case "competitor":
			sc.Notes = "add competitor app names exact-match; expect higher CPTs and lower conversion"
		case "discovery":
			sc.AdGroupName = kind + "-broad"
			sc.SearchMatch = true
			sc.Notes = "Search Match ON + broad match — mines new terms; harvest winners with `adastra harvest run`"
		default:
			sc.Notes = "custom campaign kind"
		}
		p.Campaigns = append(p.Campaigns, sc)
	}
	return p
}

// ScaffoldResult reports what was created.
type ScaffoldResult struct {
	Kind       string `json:"kind"`
	CampaignID string `json:"campaignId"`
	AdGroupID  string `json:"adGroupId"`
	Name       string `json:"name"`
	Notes      string `json:"notes"`
}

// ScaffoldApply creates the campaigns and ad groups from a plan.
func ScaffoldApply(ctx context.Context, c *api.Client, p *Plan) ([]ScaffoldResult, error) {
	st, _ := store.Open()
	if st != nil {
		defer st.Close()
	}
	var out []ScaffoldResult
	for _, sc := range p.Campaigns {
		campBody := map[string]any{
			"name":               sc.Name,
			"adamId":             json.Number(p.AdamID),
			"adChannelType":      "SEARCH",
			"supplySources":      []string{"APPSTORE_SEARCH_RESULTS"},
			"billingEvent":       "TAPS",
			"countriesOrRegions": sc.Countries,
			"dailyBudgetAmount":  map[string]string{"amount": api.FmtUSD(sc.DailyBudget), "currency": sc.Currency},
		}
		var camp json.RawMessage
		if err := c.Post(ctx, "/v1/campaigns", campBody, &camp); err != nil {
			return out, fmt.Errorf("creating %s campaign: %w", sc.Kind, err)
		}
		campID := api.Field(camp, "id")
		agBody := map[string]any{
			"campaignId":             json.Number(campID),
			"name":                   sc.AdGroupName,
			"defaultBidAmount":       map[string]string{"amount": api.FmtUSD(sc.DefaultBid), "currency": sc.Currency},
			"automatedKeywordsOptIn": sc.SearchMatch,
			"pricingModel":           "CPC",
		}
		var ag json.RawMessage
		if err := c.Post(ctx, "/v1/adgroups", agBody, &ag); err != nil {
			return out, fmt.Errorf("creating ad group for %s: %w", sc.Kind, err)
		}
		agID := api.Field(ag, "id")
		if st != nil {
			_ = st.LogMutation("campaign", campID, "create", "scaffold "+sc.Kind)
			_ = st.LogMutation("adgroup", agID, "create", "scaffold "+sc.Kind)
		}
		out = append(out, ScaffoldResult{Kind: sc.Kind, CampaignID: campID, AdGroupID: agID, Name: sc.Name, Notes: sc.Notes})
	}
	return out, nil
}

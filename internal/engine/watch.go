package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tawhidkuet04/adastra/internal/api"
	"github.com/tawhidkuet04/adastra/internal/store"
)

// Guardrails is the guardrails.json schema that drives `adastra watch`.
type Guardrails struct {
	MaxDailySpend   float64            `json:"maxDailySpend,omitempty"`
	MaxCPA          *MaxCPA            `json:"maxCpa,omitempty"`
	MinRoas         float64            `json:"minRoas,omitempty"`
	OptimizeFor     string             `json:"optimizeFor,omitempty"` // "cpa" | "roas"
	MaxBidChangePct float64            `json:"maxBidChangePct,omitempty"`
	NeverPause      []string           `json:"neverPause,omitempty"` // campaign names/ids
	Harvest         *GuardrailsHarvest `json:"harvest,omitempty"`
	Alerts          *GuardrailsAlerts  `json:"alerts,omitempty"`
	Autonomy        string             `json:"autonomy,omitempty"` // alert | propose | auto
}

// MaxCPA supports a default plus per-campaign overrides.
type MaxCPA struct {
	Default   float64            `json:"default"`
	Campaigns map[string]float64 `json:"campaigns,omitempty"`
}

// GuardrailsHarvest configures the automated harvest tick.
type GuardrailsHarvest struct {
	MinInstalls float64 `json:"minInstalls"`
	AutoNegate  bool    `json:"autoNegate"`
	Discovery   string  `json:"discovery,omitempty"` // campaign id or name
	PromoteTo   string  `json:"promoteTo,omitempty"` // campaign id or name
}

// GuardrailsAlerts configures alert thresholds.
type GuardrailsAlerts struct {
	ImpressionShareDropPct float64 `json:"impressionShareDropPct,omitempty"`
	RankDrop               int     `json:"rankDrop,omitempty"`
	Webhook                string  `json:"webhook,omitempty"`
}

// LoadGuardrails reads and validates a guardrails file.
func LoadGuardrails(path string) (*Guardrails, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Guardrails
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if g.Autonomy == "" {
		g.Autonomy = "alert"
	}
	switch g.Autonomy {
	case "alert", "propose", "auto":
	default:
		return nil, fmt.Errorf("autonomy must be alert, propose, or auto (got %q)", g.Autonomy)
	}
	return &g, nil
}

// Finding is one guardrail breach or observation from a watch tick.
type Finding struct {
	Severity string `json:"severity"` // alert | info
	Rule     string `json:"rule"`
	Entity   string `json:"entity"`
	Message  string `json:"message"`
}

// WatchResult is the full output of one watch tick.
type WatchResult struct {
	At        time.Time   `json:"at"`
	Findings  []Finding   `json:"findings"`
	Proposals *ChangePlan `json:"proposals,omitempty"`
	Applied   []string    `json:"applied,omitempty"`
}

// WatchTick evaluates guardrails against yesterday+today's campaign report
// and (per autonomy) alerts, proposes a plan, or applies capped fixes.
func WatchTick(ctx context.Context, c *api.Client, st *store.Store, g *Guardrails) (*WatchResult, error) {
	res := &WatchResult{At: time.Now()}
	req, err := api.NewReportRequest("2d", "")
	if err != nil {
		return nil, err
	}
	rows, err := c.RunReport(ctx, "/v1/reports/apps/campaigns/query", req)
	if err != nil {
		return nil, err
	}
	neverPause := map[string]bool{}
	for _, n := range g.NeverPause {
		neverPause[n] = true
	}
	plan := &ChangePlan{CreatedAt: time.Now(), Source: "watch", Account: c.AdAccount}

	var totalSpend float64
	for _, r := range rows {
		id := api.Field(r, "campaignId")
		name := api.Field(r, "campaignName")
		spend := api.FloatField(r, "localSpend")
		installs := api.FloatField(r, "totalInstalls")
		totalSpend += spend

		// per-campaign CPA breach
		if g.MaxCPA != nil {
			limit := g.MaxCPA.Default
			if v, ok := g.MaxCPA.Campaigns[name]; ok {
				limit = v
			} else if v, ok := g.MaxCPA.Campaigns[id]; ok {
				limit = v
			}
			if limit > 0 && spend > 0 {
				cpa := 0.0
				if installs > 0 {
					cpa = spend / installs
				}
				breach := (installs == 0 && spend > limit) || (cpa > limit)
				if breach {
					msg := fmt.Sprintf("CPA breach: spend %.2f, installs %.0f (limit %.2f)", spend, installs, limit)
					res.Findings = append(res.Findings, Finding{"alert", "maxCpa", name, msg})
					if !neverPause[name] && !neverPause[id] {
						body, _ := json.Marshal(map[string]string{"status": "PAUSED"})
						plan.Changes = append(plan.Changes, PlanChange{
							Description: "pause campaign " + name + " — " + msg,
							Method:      "PUT", Path: "/v1/campaigns/" + id, Body: body,
							EntityType: "campaign", EntityID: id,
						})
					}
				}
			}
		}
	}
	if g.MaxDailySpend > 0 && totalSpend > g.MaxDailySpend*2 { // 2-day window
		res.Findings = append(res.Findings, Finding{"alert", "maxDailySpend", "account",
			fmt.Sprintf("2-day spend %.2f exceeds 2×maxDailySpend (%.2f)", totalSpend, g.MaxDailySpend*2)})
	}

	// rank-drop guardrail from local tracking data
	if g.Alerts != nil && g.Alerts.RankDrop > 0 {
		tracked, err := st.TrackedKeywords()
		if err == nil {
			for _, t := range tracked {
				hist, err := st.RankHistory(t.AdamID, t.Keyword, t.Country, time.Now().AddDate(0, 0, -30))
				if err != nil || len(hist) < 2 {
					continue
				}
				prev, cur := hist[len(hist)-2].Rank, hist[len(hist)-1].Rank
				if prev > 0 && (cur == 0 || cur-prev >= g.Alerts.RankDrop) {
					res.Findings = append(res.Findings, Finding{"alert", "rankDrop", t.Keyword,
						fmt.Sprintf("organic rank dropped %d → %d [%s]", prev, cur, t.Country)})
				}
			}
		}
	}

	// harvest tick
	if g.Harvest != nil && g.Harvest.Discovery != "" && g.Harvest.PromoteTo != "" {
		actions, err := HarvestPlan(ctx, c, st, HarvestOpts{
			DiscoveryCampaign: g.Harvest.Discovery, TargetCampaign: g.Harvest.PromoteTo,
			Since: "30d", MinInstalls: g.Harvest.MinInstalls, AutoNegate: g.Harvest.AutoNegate,
			WasteTaps: 20, BidFactor: 1.1,
		})
		if err != nil {
			res.Findings = append(res.Findings, Finding{"info", "harvest", "harvest", "harvest check failed: " + err.Error()})
		}
		for _, a := range actions {
			res.Findings = append(res.Findings, Finding{"info", "harvest", a.SearchTerm, a.Reason})
		}
		if len(actions) > 0 && g.Autonomy != "alert" {
			plan.Changes = append(plan.Changes, harvestToPlanChanges(g.Harvest, actions)...)
		}
	}

	switch g.Autonomy {
	case "propose":
		if len(plan.Changes) > 0 {
			res.Proposals = plan
		}
	case "auto":
		if len(plan.Changes) > 0 {
			applied, err := ApplyPlan(ctx, c, st, plan)
			for _, a := range applied {
				res.Applied = append(res.Applied, fmt.Sprint(a["description"]))
			}
			if err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func harvestToPlanChanges(h *GuardrailsHarvest, actions []HarvestAction) []PlanChange {
	var out []PlanChange
	for _, a := range actions {
		switch a.Action {
		case "promote":
			// The literal keyword create needs the target ad group resolved at
			// apply time; watch encodes the negative (safe) and leaves promotion
			// to `adastra harvest run`, which resolves ad groups properly.
			body, _ := json.Marshal([]map[string]any{{
				"campaignId": json.Number(h.Discovery), "text": a.SearchTerm,
				"matchType": "EXACT", "status": "ACTIVE",
			}})
			out = append(out, PlanChange{
				Description: "negate promoted term in discovery: " + a.SearchTerm,
				Method:      "POST", Path: "/v1/negative-keywords/bulk-create", Body: body,
				EntityType: "negative-keyword", EntityID: a.SearchTerm,
			})
		case "negate-waste":
			body, _ := json.Marshal([]map[string]any{{
				"campaignId": json.Number(h.Discovery), "text": a.SearchTerm,
				"matchType": "EXACT", "status": "ACTIVE",
			}})
			out = append(out, PlanChange{
				Description: "negate wasteful term: " + a.SearchTerm + " — " + a.Reason,
				Method:      "POST", Path: "/v1/negative-keywords/bulk-create", Body: body,
				EntityType: "negative-keyword", EntityID: a.SearchTerm,
			})
		}
	}
	return out
}

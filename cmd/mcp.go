package cmd

import (
	"github.com/spf13/cobra"
	"github.com/tawhidjoarder/adastra/internal/mcpserver"
)

// mcpTools is the curated 1:1 command→tool mapping. Mutating tools require
// confirm=true; otherwise they run with --dry-run.
func mcpTools() []mcpserver.Tool {
	str := func(name, desc string) mcpserver.Flag {
		return mcpserver.Flag{Name: name, Type: "string", Description: desc}
	}
	num := func(name, desc string) mcpserver.Flag {
		return mcpserver.Flag{Name: name, Type: "number", Description: desc}
	}
	boolean := func(name, desc string) mcpserver.Flag {
		return mcpserver.Flag{Name: name, Type: "boolean", Description: desc}
	}
	return []mcpserver.Tool{
		{Name: "accounts_list", Description: "List accessible Apple Ads ad accounts", Argv: []string{"accounts", "list"}},
		{Name: "apps_search", Description: "Search App Store apps by name/keyword", Argv: []string{"apps", "search"},
			Positional: []string{"query"}, Flags: []mcpserver.Flag{str("country", "storefront e.g. us")}},
		{Name: "apps_eligibility", Description: "Check whether an app can run Apple Ads", Argv: []string{"apps", "eligibility"},
			Positional: []string{"adamId"}},
		{Name: "campaigns_list", Description: "List Apple Ads campaigns", Argv: []string{"campaigns", "list"},
			Flags: []mcpserver.Flag{str("name", "filter by name substring")}},
		{Name: "campaigns_get", Description: "Get one campaign's full details", Argv: []string{"campaigns", "get"},
			Positional: []string{"id"}},
		{Name: "campaigns_pause", Description: "Pause a campaign", Argv: []string{"campaigns", "pause"},
			Positional: []string{"id"}, Mutating: true},
		{Name: "campaigns_resume", Description: "Resume a paused campaign", Argv: []string{"campaigns", "resume"},
			Positional: []string{"id"}, Mutating: true},
		{Name: "campaigns_scaffold", Description: "Create the best-practice brand/category/competitor/discovery campaign structure",
			Argv: []string{"campaigns", "scaffold"}, Mutating: true,
			Flags: []mcpserver.Flag{str("app", "adamId (required)"), num("daily-budget", "per-campaign daily budget"),
				str("country", "storefronts csv"), num("default-bid", "default CPT bid"), str("structure", "which campaigns, e.g. brand,discovery")}},
		{Name: "adgroups_list", Description: "List ad groups", Argv: []string{"adgroups", "list"},
			Flags: []mcpserver.Flag{str("campaign", "campaign id")}},
		{Name: "keywords_list", Description: "List targeting keywords in an ad group or campaign", Argv: []string{"keywords", "list"},
			Flags: []mcpserver.Flag{str("adgroup", "ad group id"), str("campaign", "campaign id")}},
		{Name: "keywords_add", Description: "Add keywords to an ad group", Argv: []string{"keywords", "add"}, Mutating: true,
			Flags: []mcpserver.Flag{str("adgroup", "ad group id (required)"), str("terms", "comma-separated keywords"),
				str("match", "exact|broad"), num("bid", "CPT bid")}},
		{Name: "negatives_add", Description: "Add negative keywords", Argv: []string{"negatives", "add"}, Mutating: true,
			Flags: []mcpserver.Flag{str("adgroup", "ad group id"), str("campaign", "campaign id"),
				str("terms", "comma-separated keywords"), str("match", "exact|broad")}},
		{Name: "reports_campaigns", Description: "Campaign performance report (spend, taps, installs, CPA)",
			Argv: []string{"reports", "campaigns"}, Flags: []mcpserver.Flag{str("since", "e.g. 30d"), str("granularity", "daily|weekly")}},
		{Name: "reports_keywords", Description: "Keyword performance report", Argv: []string{"reports", "keywords"},
			Flags: []mcpserver.Flag{str("since", "e.g. 30d"), str("campaign", "campaign id"), str("adgroup", "ad group id")}},
		{Name: "reports_searchterms", Description: "Search terms report — what users actually typed",
			Argv: []string{"reports", "searchterms"}, Flags: []mcpserver.Flag{str("since", "e.g. 30d"), str("campaign", "campaign id")}},
		{Name: "insights_popularity", Description: "Apple's search-term popularity scores (1-5)",
			Argv: []string{"insights", "popularity"}, Flags: []mcpserver.Flag{str("terms", "comma-separated terms"), str("countries", "storefronts csv")}},
		{Name: "insights_impression_share", Description: "Impression share — who's squeezing you on which terms",
			Argv: []string{"insights", "impression-share"}, Flags: []mcpserver.Flag{str("app", "adamId"), str("since", "e.g. 7d")}},
		{Name: "aso_research", Description: "ASO research for a seed term: popularity, difficulty, top apps",
			Argv: []string{"aso", "research"}, Positional: []string{"term"},
			Flags: []mcpserver.Flag{str("country", "storefront"), boolean("expand", "fan out candidate terms")}},
		{Name: "aso_difficulty", Description: "Keyword difficulty (1-10) from the live top-10",
			Argv: []string{"aso", "difficulty"}, Positional: []string{"term"}, Flags: []mcpserver.Flag{str("country", "storefront")}},
		{Name: "aso_track_run", Description: "Snapshot organic ranks for all tracked keywords", Argv: []string{"aso", "track", "run"}},
		{Name: "aso_track_report", Description: "Rank history report", Argv: []string{"aso", "track", "report"},
			Flags: []mcpserver.Flag{str("since", "e.g. 30d")}},
		{Name: "aso_metadata_audit", Description: "Audit an app's visible metadata (char budgets, duplication)",
			Argv: []string{"aso", "metadata", "audit"}, Flags: []mcpserver.Flag{str("app", "adamId (required)"), str("country", "storefront")}},
		{Name: "aso_competitors_gap", Description: "Metadata keyword gap vs competitors",
			Argv: []string{"aso", "competitors", "gap"}, Flags: []mcpserver.Flag{str("app", "your adamId (required)"), str("vs", "competitor adamIds csv (required)")}},
		{Name: "reviews_list", Description: "Recent App Store reviews", Argv: []string{"aso", "reviews", "list"},
			Flags: []mcpserver.Flag{str("app", "adamId (required)"), str("country", "storefront"), str("stars", "filter e.g. 1,2")}},
		{Name: "harvest_run", Description: "The harvest loop: promote converting search terms to exact match, negate waste",
			Argv: []string{"harvest", "run"}, Mutating: true,
			Flags: []mcpserver.Flag{str("discovery", "discovery campaign id (required)"), str("target", "target campaign id (required)"),
				num("min-installs", "promotion threshold"), num("max-cpa", "CPA ceiling"), boolean("auto-negate", "negate wasteful terms")}},
		{Name: "bids_adjust", Description: "Adjust keyword bids toward a CPA or ROAS target (capped)",
			Argv: []string{"bids", "adjust"}, Mutating: true,
			Flags: []mcpserver.Flag{str("adgroup", "ad group id (required)"), num("target-cpa", "target cost per install"),
				num("target-roas", "target ROAS e.g. 1.5"), str("max-change", "cap e.g. 20%")}},
		{Name: "budget_pacing", Description: "Spend vs days-elapsed projection for a campaign",
			Argv: []string{"budget", "pacing"}, Flags: []mcpserver.Flag{str("campaign", "campaign id (required)")}},
		{Name: "reco_list", Description: "Apple's own budget/target-CPA recommendations", Argv: []string{"reco", "list"}},
		{Name: "watch_tick", Description: "Evaluate guardrails: CPA breaches, pacing, rank drops, harvest candidates",
			Argv: []string{"watch"}, Flags: []mcpserver.Flag{str("config", "path to guardrails.json")}},
		{Name: "roas_report", Description: "ROAS by campaign/adgroup/keyword (needs RevenueCat data)",
			Argv: []string{"roas", "report"}, Flags: []mcpserver.Flag{str("by", "campaign|adgroup|keyword"), str("since", "e.g. 30d")}},
		{Name: "dashboard", Description: "Account summary: spend, taps, installs, CPA by campaign",
			Argv: []string{"dashboard"}, Flags: []mcpserver.Flag{str("since", "e.g. 7d")}},
		{Name: "history", Description: "Change history audit trail", Argv: []string{"history"},
			Flags: []mcpserver.Flag{str("since", "e.g. 7d"), str("entity", "campaign|adgroup|keyword")}},
	}
}

func init() {
	mcpCmd := &cobra.Command{Use: "mcp", Short: "Model Context Protocol integration"}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Run the built-in MCP server (stdio) exposing adastra as agent tools",
		Long: `Exposes every major adastra command as an MCP tool over stdio.

Claude Code:      claude mcp add adastra -- adastra mcp serve
Claude Desktop:   add {"command":"adastra","args":["mcp","serve"]} to mcpServers

Mutating tools require the argument confirm=true; without it they run in
--dry-run mode and return the would-be changes. Reads run as-is.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.Serve(mcpTools(), Version)
		},
	}
	mcpCmd.AddCommand(serve)
	rootCmd.AddCommand(mcpCmd)
}

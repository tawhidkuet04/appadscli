package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
	"github.com/appadscli/appadscli/internal/engine"
)

var campaignCols = []string{
	"Id=id", "Name=name", "Status=status", "ServingStatus=servingStatus",
	"DailyBudget=$money:dailyBudgetAmount", "Countries=countriesOrRegions", "AdamId=adamId",
}

func init() {
	campaignsCmd := &cobra.Command{Use: "campaigns", Short: "Manage Apple Ads campaigns"}

	var limit int
	var name string
	list := &cobra.Command{
		Use:   "list",
		Short: "List campaigns",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if name != "" {
				sel.Conditions = []api.Condition{{Field: "name", Operator: "CONTAINS_ANY", Values: []string{name}}}
			}
			items, err := c.Query(cmd.Context(), "/v1/campaigns/query", sel, limit)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, campaignCols)
			return render().Rows(h, rows, items)
		},
	}
	list.Flags().IntVar(&limit, "limit", 0, "max results (0 = all)")
	list.Flags().StringVar(&name, "name", "", "filter by name substring")

	get := &cobra.Command{
		Use:   "get <id>",
		Short: "Campaign details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var camp json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/campaigns/"+args[0], &camp); err != nil {
				return err
			}
			return render().JSON(camp)
		},
	}

	var (
		createApp, createName, createCountries, createCurrency string
		createDaily                                            float64
	)
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a campaign (App Store search results)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			countries := strings.Split(strings.ToUpper(createCountries), ",")
			body := map[string]any{
				"name":               createName,
				"adamId":             mustInt(createApp),
				"adChannelType":      "SEARCH",
				"supplySources":      []string{"APPSTORE_SEARCH_RESULTS"},
				"billingEvent":       "TAPS",
				"countriesOrRegions": countries,
				"dailyBudgetAmount":  map[string]string{"amount": api.FmtUSD(createDaily), "currency": createCurrency},
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create campaign %q (%.2f %s/day, %s)", createName, createDaily, createCurrency, createCountries))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/campaigns", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	create.Flags().StringVar(&createApp, "app", "", "adamId of the app to promote (required)")
	create.Flags().StringVar(&createName, "name", "", "campaign name (required)")
	create.Flags().Float64Var(&createDaily, "daily-budget", 5, "daily budget amount")
	create.Flags().StringVar(&createCurrency, "currency", "USD", "budget currency")
	create.Flags().StringVar(&createCountries, "countries", "us", "comma-separated storefronts")
	_ = create.MarkFlagRequired("app")
	_ = create.MarkFlagRequired("name")
	addMutationFlags(create)

	var updDaily float64
	var updName string
	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update campaign name or daily budget",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			body := map[string]any{}
			desc := []string{}
			if updName != "" {
				body["name"] = updName
				desc = append(desc, "rename to "+updName)
			}
			if cmd.Flags().Changed("daily-budget") {
				cur, err := campaignCurrency(cmd, c, args[0])
				if err != nil {
					return err
				}
				body["dailyBudgetAmount"] = map[string]string{"amount": api.FmtUSD(updDaily), "currency": cur}
				desc = append(desc, fmt.Sprintf("set daily budget to %.2f %s", updDaily, cur))
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update — pass --name and/or --daily-budget")
			}
			ok, err := confirmOrAbort(cmd, "update campaign "+args[0]+": "+strings.Join(desc, ", "))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Put(cmd.Context(), "/v1/campaigns/"+args[0], body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	update.Flags().StringVar(&updName, "name", "", "new name")
	update.Flags().Float64Var(&updDaily, "daily-budget", 0, "new daily budget")
	addMutationFlags(update)

	pause := statusCmd("pause", "PAUSED")
	resume := statusCmd("resume", "ENABLED")

	del := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a campaign",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, "DELETE campaign "+args[0]+" (irreversible)")
			if err != nil || !ok {
				return err
			}
			if err := c.Delete(cmd.Context(), "/v1/campaigns/"+args[0]); err != nil {
				return err
			}
			fmt.Println("deleted", args[0])
			return nil
		},
	}
	addMutationFlags(del)

	scaffold := newScaffoldCmd()

	campaignsCmd.AddCommand(list, get, create, update, pause, resume, del, scaffold)
	rootCmd.AddCommand(campaignsCmd)
}

// statusCmd builds pause/resume as a status PUT.
func statusCmd(verb, status string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   verb + " <id>",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " a campaign",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, verb+" campaign "+args[0])
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Put(cmd.Context(), "/v1/campaigns/"+args[0], map[string]string{"status": status}, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	addMutationFlags(cmd)
	return cmd
}

func campaignCurrency(cmd *cobra.Command, c *api.Client, id string) (string, error) {
	var camp json.RawMessage
	if err := c.Get(cmd.Context(), "/v1/campaigns/"+id, &camp); err != nil {
		return "", err
	}
	cur := api.Field(camp, "dailyBudgetAmount.currency")
	if cur == "" {
		cur = api.Field(camp, "budgetAmount.currency")
	}
	if cur == "" {
		cur = "USD"
	}
	return cur, nil
}

func mustInt(s string) any {
	var n json.Number = json.Number(s)
	if _, err := n.Int64(); err == nil {
		return n
	}
	return s
}

func newScaffoldCmd() *cobra.Command {
	var (
		app, structure, countries, currency string
		daily, bid                          float64
	)
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "One-shot best-practice campaign structure (brand/category/competitor/discovery)",
		Long: `Create the canonical 4-campaign Apple Ads structure in one command:

  brand       — exact-match on your own app name; cheap, defends your brand
  category    — exact-match on category/feature keywords you researched
  competitor  — exact-match on competitor names
  discovery   — Search Match ON + broad match, mines new search terms

Each campaign gets one ad group with sensible defaults. Feed discovery's
winners into category/brand with ` + "`appadscli harvest run`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			plan := engine.ScaffoldPlan(engine.ScaffoldOpts{
				AdamID: app, Structure: strings.Split(structure, ","),
				DailyBudget: daily, Currency: currency,
				Countries: strings.Split(strings.ToUpper(countries), ","), DefaultBid: bid,
			})
			dry, _ := cmd.Flags().GetBool("dry-run")
			if dry {
				return render().JSON(plan)
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create %d campaigns + %d ad groups (%.2f %s/day total)",
				len(plan.Campaigns), len(plan.Campaigns), daily*float64(len(plan.Campaigns)), currency))
			if err != nil || !ok {
				return err
			}
			results, err := engine.ScaffoldApply(cmd.Context(), c, plan)
			if err != nil {
				return err
			}
			return render().JSON(results)
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "adamId of the app (required)")
	cmd.Flags().StringVar(&structure, "structure", "brand,category,competitor,discovery", "which campaigns to create")
	cmd.Flags().Float64Var(&daily, "daily-budget", 10, "daily budget per campaign")
	cmd.Flags().Float64Var(&bid, "default-bid", 1.00, "default CPT bid per ad group")
	cmd.Flags().StringVar(&currency, "currency", "USD", "budget currency")
	cmd.Flags().StringVar(&countries, "country", "us", "comma-separated storefronts")
	_ = cmd.MarkFlagRequired("app")
	addMutationFlags(cmd)
	return cmd
}

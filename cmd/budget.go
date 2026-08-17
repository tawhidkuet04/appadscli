package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

func init() {
	budgetCmd := &cobra.Command{Use: "budget", Short: "Shared budgets and spend pacing"}

	// pacing: spend vs days-elapsed projection for a campaign's current month.
	var pacingCampaign string
	pacing := &cobra.Command{
		Use:   "pacing",
		Short: "Spend vs. days-elapsed projection for a campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			if pacingCampaign == "" {
				return fmt.Errorf("pass --campaign <id>")
			}
			var camp json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/campaigns/"+pacingCampaign, &camp); err != nil {
				return err
			}
			daily := api.FloatField(camp, "dailyBudget.value.amount")
			now := time.Now()
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			req, err := api.NewReportRequest(monthStart.Format("2006-01-02"), "")
			if err != nil {
				return err
			}
			// Campaign reports key the campaign as `id`, not `campaignId`.
			req.Filters = []api.Filter{{Field: "id", Operator: "EQUALS", Value: pacingCampaign}}
			rows, err := c.RunReport(cmd.Context(), "/v1/reports/apps/campaigns/query", req)
			if err != nil {
				return err
			}
			var spent float64
			for _, r := range rows {
				spent += api.FloatField(r, "localSpend")
			}
			daysElapsed := now.Day()
			daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
			expected := daily * float64(daysElapsed)
			projected := spent / float64(daysElapsed) * float64(daysInMonth)
			out := map[string]any{
				"campaignId":       pacingCampaign,
				"campaignName":     api.Field(camp, "name"),
				"dailyBudget":      daily,
				"monthToDateSpend": spent,
				"expectedToDate":   expected,
				"pacePct":          pct(spent, expected),
				"projectedMonth":   projected,
				"budgetMonth":      daily * float64(daysInMonth),
				"daysElapsed":      daysElapsed,
				"daysInMonth":      daysInMonth,
			}
			return render().JSON(out)
		},
	}
	pacing.Flags().StringVar(&pacingCampaign, "campaign", "", "campaign id (required)")

	ordersCmd := &cobra.Command{Use: "orders", Short: "Shared budgets (budget orders)"}
	oList := &cobra.Command{
		Use:   "list",
		Short: "List shared budgets",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			items, err := c.Query(cmd.Context(), "/v1/shared-budgets/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Name=name", "Status=status", "Budget=$money:value", "StartTime=startTime", "EndTime=endTime",
			})
			return render().Rows(h, rows, items)
		},
	}

	var oName, oCurrency string
	var oAmount float64
	oCreate := &cobra.Command{
		Use:   "create",
		Short: "Create a shared budget",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create shared budget %q of %.2f %s", oName, oAmount, oCurrency))
			if err != nil || !ok {
				return err
			}
			body := map[string]any{
				"name":  oName,
				"value": map[string]string{"amount": api.FmtUSD(oAmount), "currency": oCurrency},
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/shared-budgets", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	oCreate.Flags().StringVar(&oName, "name", "", "budget name (required)")
	oCreate.Flags().Float64Var(&oAmount, "amount", 0, "budget amount (required)")
	oCreate.Flags().StringVar(&oCurrency, "currency", "USD", "currency")
	_ = oCreate.MarkFlagRequired("name")
	_ = oCreate.MarkFlagRequired("amount")
	addMutationFlags(oCreate)

	var uAmount float64
	oUpdate := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a shared budget amount",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			var cur json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/shared-budgets/"+args[0], &cur); err != nil {
				return err
			}
			curr := api.Field(cur, "value.currency")
			if curr == "" {
				curr = "USD"
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("update shared budget %s to %.2f %s", args[0], uAmount, curr))
			if err != nil || !ok {
				return err
			}
			body := map[string]any{"value": map[string]string{"amount": api.FmtUSD(uAmount), "currency": curr}}
			var out json.RawMessage
			if err := c.Put(cmd.Context(), "/v1/shared-budgets/"+args[0], body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	oUpdate.Flags().Float64Var(&uAmount, "amount", 0, "new amount (required)")
	_ = oUpdate.MarkFlagRequired("amount")
	addMutationFlags(oUpdate)

	ordersCmd.AddCommand(oList, oCreate, oUpdate)
	budgetCmd.AddCommand(pacing, ordersCmd)
	rootCmd.AddCommand(budgetCmd)
}

func pct(actual, expected float64) float64 {
	if expected == 0 {
		return 0
	}
	return actual / expected * 100
}

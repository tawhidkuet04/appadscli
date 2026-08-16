package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

func init() {
	adgroupsCmd := &cobra.Command{Use: "adgroups", Short: "Manage ad groups"}

	var campaignID string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List ad groups (optionally for one campaign)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if campaignID != "" {
				sel = api.EqCond("campaignId", campaignID)
			}
			items, err := c.Query(cmd.Context(), "/v1/adgroups/query", sel, limit)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "CampaignId=campaignId", "Name=name", "Status=status",
				"ServingStatus=servingStatus", "DefaultBid=$money:defaultBidAmount",
				"SearchMatch=automatedKeywordsOptIn",
			})
			return render().Rows(h, rows, items)
		},
	}
	list.Flags().StringVar(&campaignID, "campaign", "", "filter by campaign id")
	list.Flags().IntVar(&limit, "limit", 0, "max results (0 = all)")

	get := &cobra.Command{
		Use:   "get <id>",
		Short: "Ad group details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var ag json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/adgroups/"+args[0], &ag); err != nil {
				return err
			}
			return render().JSON(ag)
		},
	}

	var (
		cCampaign, cName, cCurrency string
		cBid                        float64
		cSearchMatch                bool
	)
	create := &cobra.Command{
		Use:   "create",
		Short: "Create an ad group",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			body := map[string]any{
				"campaignId":             json.Number(cCampaign),
				"name":                   cName,
				"defaultBidAmount":       map[string]string{"amount": api.FmtUSD(cBid), "currency": cCurrency},
				"automatedKeywordsOptIn": cSearchMatch,
				"pricingModel":           "CPC",
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create ad group %q in campaign %s (default bid %.2f %s)", cName, cCampaign, cBid, cCurrency))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/adgroups", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	create.Flags().StringVar(&cCampaign, "campaign", "", "campaign id (required)")
	create.Flags().StringVar(&cName, "name", "", "ad group name (required)")
	create.Flags().Float64Var(&cBid, "default-bid", 1.00, "default CPT/CPC bid")
	create.Flags().StringVar(&cCurrency, "currency", "USD", "bid currency")
	create.Flags().BoolVar(&cSearchMatch, "search-match", false, "enable Search Match (auto keyword discovery)")
	_ = create.MarkFlagRequired("campaign")
	_ = create.MarkFlagRequired("name")
	addMutationFlags(create)

	var uBid float64
	var uName, uStatus string
	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update ad group name, default bid, or status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			body := map[string]any{}
			if uName != "" {
				body["name"] = uName
			}
			if uStatus != "" {
				body["status"] = uStatus
			}
			if cmd.Flags().Changed("default-bid") {
				var ag json.RawMessage
				if err := c.Get(cmd.Context(), "/v1/adgroups/"+args[0], &ag); err != nil {
					return err
				}
				cur := api.Field(ag, "defaultBidAmount.currency")
				if cur == "" {
					cur = "USD"
				}
				body["defaultBidAmount"] = map[string]string{"amount": api.FmtUSD(uBid), "currency": cur}
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update — pass --name, --status, and/or --default-bid")
			}
			ok, err := confirmOrAbort(cmd, "update ad group "+args[0])
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Put(cmd.Context(), "/v1/adgroups/"+args[0], body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	update.Flags().StringVar(&uName, "name", "", "new name")
	update.Flags().StringVar(&uStatus, "status", "", "ENABLED|PAUSED")
	update.Flags().Float64Var(&uBid, "default-bid", 0, "new default bid")
	addMutationFlags(update)

	adgroupsCmd.AddCommand(list, get, create, update)
	rootCmd.AddCommand(adgroupsCmd)
}

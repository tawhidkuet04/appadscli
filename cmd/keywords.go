package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

var keywordCols = []string{
	"Id=id", "Text=text", "Match=matchType", "Status=status",
	"Bid=$money:bidAmount", "AdGroupId=adGroupId", "CampaignId=campaignId",
}

func init() {
	keywordsCmd := &cobra.Command{Use: "keywords", Short: "Manage targeting keywords"}

	var adgroup, campaign string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List targeting keywords",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if adgroup != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "adGroupId", Operator: "EQUALS", Values: []string{adgroup}})
			}
			if campaign != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "campaignId", Operator: "EQUALS", Values: []string{campaign}})
			}
			if len(sel.Conditions) == 0 {
				return fmt.Errorf("pass --adgroup or --campaign (the API requires a scope filter)")
			}
			items, err := c.Query(cmd.Context(), "/v1/keywords/query", sel, limit)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, keywordCols)
			return render().Rows(h, rows, items)
		},
	}
	list.Flags().StringVar(&adgroup, "adgroup", "", "ad group id")
	list.Flags().StringVar(&campaign, "campaign", "", "campaign id")
	list.Flags().IntVar(&limit, "limit", 0, "max results (0 = all)")

	var (
		addAdgroup, addMatch, addTerms, addTermsFile, addCurrency string
		addBid                                                    float64
	)
	add := &cobra.Command{
		Use:   "add",
		Short: "Add keywords to an ad group (bulk-create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var terms []string
			if addTerms != "" {
				terms = strings.Split(addTerms, ",")
			}
			if addTermsFile != "" {
				fromFile, err := readLines(addTermsFile)
				if err != nil {
					return err
				}
				terms = append(terms, fromFile...)
			}
			if len(terms) == 0 {
				return fmt.Errorf("no keywords — pass --terms or --file")
			}
			var body []map[string]any
			for _, t := range terms {
				body = append(body, map[string]any{
					"adGroupId": json.Number(addAdgroup),
					"text":      strings.TrimSpace(t),
					"matchType": strings.ToUpper(addMatch),
					"bidAmount": map[string]string{"amount": api.FmtUSD(addBid), "currency": addCurrency},
					"status":    "ACTIVE",
				})
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("add %d %s-match keywords to ad group %s at %.2f %s",
				len(terms), strings.ToLower(addMatch), addAdgroup, addBid, addCurrency))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/keywords/bulk-create", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	add.Flags().StringVar(&addAdgroup, "adgroup", "", "ad group id (required)")
	add.Flags().StringVar(&addTerms, "terms", "", "comma-separated keywords")
	add.Flags().StringVar(&addTermsFile, "file", "", "newline-separated keyword file")
	add.Flags().StringVar(&addMatch, "match", "exact", "exact|broad")
	add.Flags().Float64Var(&addBid, "bid", 1.00, "CPT bid per keyword")
	add.Flags().StringVar(&addCurrency, "currency", "USD", "bid currency")
	_ = add.MarkFlagRequired("adgroup")
	addMutationFlags(add)

	var updBid float64
	var updStatus string
	update := &cobra.Command{
		Use:   "update <keywordId>...",
		Short: "Update bid or status on keywords (bulk-update)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var body []map[string]any
			for _, id := range args {
				u := map[string]any{"id": json.Number(id)}
				if cmd.Flags().Changed("bid") {
					u["bidAmount"] = map[string]string{"amount": api.FmtUSD(updBid), "currency": "USD"}
				}
				if updStatus != "" {
					u["status"] = strings.ToUpper(updStatus)
				}
				body = append(body, u)
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("update %d keyword(s)", len(args)))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/keywords/bulk-update", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	update.Flags().Float64Var(&updBid, "bid", 0, "new CPT bid (USD)")
	update.Flags().StringVar(&updStatus, "status", "", "active|paused")
	addMutationFlags(update)

	pause := &cobra.Command{
		Use:   "pause <keywordId>...",
		Short: "Pause keywords",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var body []map[string]any
			for _, id := range args {
				body = append(body, map[string]any{"id": json.Number(id), "status": "PAUSED"})
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("pause %d keyword(s)", len(args)))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/keywords/bulk-update", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	addMutationFlags(pause)

	var bulkFile string
	bulk := &cobra.Command{
		Use:   "bulk",
		Short: "Bulk operations from CSV",
	}
	upsert := &cobra.Command{
		Use:   "upsert",
		Short: "Create/update keywords from CSV (columns: adGroupId,text,matchType,bid[,currency][,id])",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			f, err := os.Open(bulkFile)
			if err != nil {
				return err
			}
			defer f.Close()
			recs, err := csv.NewReader(f).ReadAll()
			if err != nil {
				return err
			}
			if len(recs) < 2 {
				return fmt.Errorf("csv needs a header row and at least one data row")
			}
			col := map[string]int{}
			for i, h := range recs[0] {
				col[strings.ToLower(strings.TrimSpace(h))] = i
			}
			need := func(name string, rec []string) string {
				i, ok := col[name]
				if !ok || i >= len(rec) {
					return ""
				}
				return strings.TrimSpace(rec[i])
			}
			var creates, updates []map[string]any
			for _, rec := range recs[1:] {
				cur := need("currency", rec)
				if cur == "" {
					cur = "USD"
				}
				kw := map[string]any{
					"adGroupId": json.Number(need("adgroupid", rec)),
					"text":      need("text", rec),
					"matchType": strings.ToUpper(need("matchtype", rec)),
					"bidAmount": map[string]string{"amount": need("bid", rec), "currency": cur},
					"status":    "ACTIVE",
				}
				if id := need("id", rec); id != "" {
					kw["id"] = json.Number(id)
					updates = append(updates, kw)
				} else {
					creates = append(creates, kw)
				}
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("upsert keywords: %d creates, %d updates", len(creates), len(updates)))
			if err != nil || !ok {
				return err
			}
			results := map[string]any{}
			if len(creates) > 0 {
				var out json.RawMessage
				if err := c.Post(cmd.Context(), "/v1/keywords/bulk-create", creates, &out); err != nil {
					return err
				}
				results["created"] = out
			}
			if len(updates) > 0 {
				var out json.RawMessage
				if err := c.Post(cmd.Context(), "/v1/keywords/bulk-update", updates, &out); err != nil {
					return err
				}
				results["updated"] = out
			}
			return render().JSON(results)
		},
	}
	upsert.Flags().StringVar(&bulkFile, "file", "", "CSV file (required)")
	_ = upsert.MarkFlagRequired("file")
	addMutationFlags(upsert)
	bulk.AddCommand(upsert)

	keywordsCmd.AddCommand(list, add, update, pause, bulk)
	rootCmd.AddCommand(keywordsCmd)

	// negatives
	negativesCmd := &cobra.Command{Use: "negatives", Short: "Manage negative keywords"}
	var nAdgroup, nCampaign string
	nList := &cobra.Command{
		Use:   "list",
		Short: "List negative keywords",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if nAdgroup != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "adGroupId", Operator: "EQUALS", Values: []string{nAdgroup}})
			}
			if nCampaign != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "campaignId", Operator: "EQUALS", Values: []string{nCampaign}})
			}
			items, err := c.Query(cmd.Context(), "/v1/negative-keywords/query", sel, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Text=text", "Match=matchType", "Status=status",
				"AdGroupId=adGroupId", "CampaignId=campaignId",
			})
			return render().Rows(h, rows, items)
		},
	}
	nList.Flags().StringVar(&nAdgroup, "adgroup", "", "ad group id")
	nList.Flags().StringVar(&nCampaign, "campaign", "", "campaign id")

	var (
		negAdgroup, negCampaign, negMatch, negTerms, negFile string
	)
	nAdd := &cobra.Command{
		Use:   "add",
		Short: "Add negative keywords at campaign or ad group level (bulk-create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			if (negAdgroup == "") == (negCampaign == "") {
				return fmt.Errorf("pass exactly one of --adgroup or --campaign")
			}
			var terms []string
			if negTerms != "" {
				terms = strings.Split(negTerms, ",")
			}
			if negFile != "" {
				fromFile, err := readLines(negFile)
				if err != nil {
					return err
				}
				terms = append(terms, fromFile...)
			}
			if len(terms) == 0 {
				return fmt.Errorf("no keywords — pass --terms or --file")
			}
			var body []map[string]any
			for _, t := range terms {
				n := map[string]any{
					"text":      strings.TrimSpace(t),
					"matchType": strings.ToUpper(negMatch),
					"status":    "ACTIVE",
				}
				if negAdgroup != "" {
					n["adGroupId"] = json.Number(negAdgroup)
				} else {
					n["campaignId"] = json.Number(negCampaign)
				}
				body = append(body, n)
			}
			scope := "campaign " + negCampaign
			if negAdgroup != "" {
				scope = "ad group " + negAdgroup
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("add %d negative keyword(s) to %s", len(terms), scope))
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/negative-keywords/bulk-create", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	nAdd.Flags().StringVar(&negAdgroup, "adgroup", "", "ad group id")
	nAdd.Flags().StringVar(&negCampaign, "campaign", "", "campaign id")
	nAdd.Flags().StringVar(&negTerms, "terms", "", "comma-separated keywords")
	nAdd.Flags().StringVar(&negFile, "file", "", "newline-separated keyword file")
	nAdd.Flags().StringVar(&negMatch, "match", "exact", "exact|broad")
	addMutationFlags(nAdd)

	negativesCmd.AddCommand(nList, nAdd)
	rootCmd.AddCommand(negativesCmd)
}

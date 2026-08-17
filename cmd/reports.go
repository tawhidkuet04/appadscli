package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

var reportMetricCols = []string{
	"Impressions=impressions", "Taps=taps", "Installs=totalInstalls",
	"Spend=localSpend", "AvgCPT=cpt", "AvgCPA=totalAvgCPI", "TTR=ttr", "CR=totalInstallRate",
}

func init() {
	reportsCmd := &cobra.Command{Use: "reports", Short: "Performance reports (spend, taps, installs, CPA)"}

	// Reports name the reported entity `id`/`name` and its parents by id, so
	// the campaign filter is `id` on the campaign report and `campaignId`
	// everywhere else — likewise for the ad group filter.
	type reportDef struct {
		name, path              string
		campaignField, adgField string
		idCols                  []string
	}
	defs := []reportDef{
		{"campaigns", "/v1/reports/apps/campaigns/query", "id", "",
			[]string{"CampaignId=id", "Campaign=name"}},
		{"adgroups", "/v1/reports/apps/adgroups/query", "campaignId", "id",
			[]string{"AdGroupId=id", "AdGroup=name", "CampaignId=campaignId"}},
		{"keywords", "/v1/reports/apps/keywords/query", "campaignId", "adGroupId",
			[]string{"KeywordId=id", "Keyword=text", "Match=matchType", "Bid=bid", "AdGroupId=adGroupId"}},
		{"ads", "/v1/reports/apps/ads/query", "campaignId", "adGroupId",
			[]string{"AdId=id", "Ad=name", "AdGroupId=adGroupId"}},
		{"searchterms", "/v1/reports/apps/searchterms/query", "campaignId", "adGroupId",
			[]string{"SearchTerm=searchTermText", "Source=searchTermSource", "Keyword=keyword.text", "AdGroupId=adGroupId", "CampaignId=campaignId"}},
	}

	for _, d := range defs {
		d := d
		var since, granularity, campaign, adgroup string
		sub := &cobra.Command{
			Use:   d.name,
			Short: fmt.Sprintf("%s report", d.name),
			RunE: func(cmd *cobra.Command, args []string) error {
				c := client()
				if err := c.RequireAccount(); err != nil {
					return err
				}
				req, err := api.NewReportRequest(since, granularity)
				if err != nil {
					return err
				}
				if campaign != "" {
					req.Filters = append(req.Filters,
						api.Filter{Field: d.campaignField, Operator: "EQUALS", Value: campaign})
				}
				if adgroup != "" && d.adgField != "" {
					req.Filters = append(req.Filters,
						api.Filter{Field: d.adgField, Operator: "EQUALS", Value: adgroup})
				}
				rows, err := c.RunReport(cmd.Context(), d.path, req)
				if err != nil {
					return err
				}
				cols := append([]string{}, d.idCols...)
				if granularity != "" {
					cols = append(cols, "Date=date")
				}
				cols = append(cols, reportMetricCols...)
				h, tbl := api.Table(rows, cols)
				return render().Rows(h, tbl, rows)
			},
		}
		sub.Flags().StringVar(&since, "since", "30d", "window start: 30d, 4w, or YYYY-MM-DD")
		sub.Flags().StringVar(&granularity, "granularity", "", "hourly|daily|weekly|monthly (default: totals)")
		sub.Flags().StringVar(&campaign, "campaign", "", "filter by campaign id")
		if d.name != "campaigns" {
			sub.Flags().StringVar(&adgroup, "adgroup", "", "filter by ad group id")
		}
		reportsCmd.AddCommand(sub)
	}

	rootCmd.AddCommand(reportsCmd)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/api"
)

var reportMetricCols = []string{
	"Impressions=impressions", "Taps=taps", "Installs=totalInstalls",
	"Spend=localSpend", "AvgCPT=avgCPT", "AvgCPA=totalAvgCPI", "TTR=ttr", "CR=totalInstallRate",
}

func init() {
	reportsCmd := &cobra.Command{Use: "reports", Short: "Performance reports (spend, taps, installs, CPA)"}

	type reportDef struct {
		name, path string
		idCols     []string
	}
	defs := []reportDef{
		{"campaigns", "/v1/reports/apps/campaigns/query", []string{"CampaignId=campaignId", "Campaign=campaignName"}},
		{"adgroups", "/v1/reports/apps/adgroups/query", []string{"AdGroupId=adGroupId", "AdGroup=adGroupName", "CampaignId=campaignId"}},
		{"keywords", "/v1/reports/apps/keywords/query", []string{"KeywordId=keywordId", "Keyword=keyword", "Match=matchType", "Bid=bidAmount", "AdGroupId=adGroupId"}},
		{"ads", "/v1/reports/apps/ads/query", []string{"AdId=adId", "Ad=adName", "AdGroupId=adGroupId"}},
		{"searchterms", "/v1/reports/apps/searchterms/query", []string{"SearchTerm=searchTermText", "Source=searchTermSource", "Keyword=keyword", "AdGroupId=adGroupId", "CampaignId=campaignId"}},
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
					req.Selector.Conditions = append(req.Selector.Conditions,
						api.Condition{Field: "campaignId", Operator: "EQUALS", Values: []string{campaign}})
				}
				if adgroup != "" {
					req.Selector.Conditions = append(req.Selector.Conditions,
						api.Condition{Field: "adGroupId", Operator: "EQUALS", Values: []string{adgroup}})
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

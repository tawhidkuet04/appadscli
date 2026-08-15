package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/appadscli/internal/api"
)

func init() {
	var since string
	dash := &cobra.Command{
		Use:   "dashboard",
		Short: "One-screen summary: spend, taps, installs, CPA by campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			req, err := api.NewReportRequest(since, "")
			if err != nil {
				return err
			}
			rows, err := c.RunReport(cmd.Context(), "/v1/reports/apps/campaigns/query", req)
			if err != nil {
				return err
			}
			type line struct {
				name                     string
				spend, taps, installs, i float64
			}
			var lines []line
			var tSpend, tTaps, tInstalls, tImp float64
			for _, r := range rows {
				l := line{
					name:     api.Field(r, "campaignName"),
					spend:    api.FloatField(r, "localSpend"),
					taps:     api.FloatField(r, "taps"),
					installs: api.FloatField(r, "totalInstalls"),
					i:        api.FloatField(r, "impressions"),
				}
				if l.name == "" {
					l.name = api.Field(r, "campaignId")
				}
				lines = append(lines, l)
				tSpend += l.spend
				tTaps += l.taps
				tInstalls += l.installs
				tImp += l.i
			}
			sort.Slice(lines, func(a, b int) bool { return lines[a].spend > lines[b].spend })

			if render().Format == "json" {
				return render().JSON(rows)
			}
			cpa := func(spend, installs float64) string {
				if installs == 0 {
					return "—"
				}
				return fmt.Sprintf("%.2f", spend/installs)
			}
			fmt.Printf("Apple Ads — last %s\n\n", since)
			fmt.Printf("  Spend      %10.2f\n  Taps       %10.0f\n  Installs   %10.0f\n  Avg CPA    %10s\n  Impressions%10.0f\n\n",
				tSpend, tTaps, tInstalls, cpa(tSpend, tInstalls), tImp)
			headers := []string{"Campaign", "Spend", "Taps", "Installs", "CPA"}
			var tbl [][]string
			for _, l := range lines {
				tbl = append(tbl, []string{l.name, fmt.Sprintf("%.2f", l.spend),
					fmt.Sprintf("%.0f", l.taps), fmt.Sprintf("%.0f", l.installs), cpa(l.spend, l.installs)})
			}
			return render().Rows(headers, tbl, nil)
		},
	}
	dash.Flags().StringVar(&since, "since", "7d", "window start: 7d, 4w, or YYYY-MM-DD")
	rootCmd.AddCommand(dash)
}

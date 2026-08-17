package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
	"github.com/appadscli/appadscli/internal/rc"
	"github.com/appadscli/appadscli/internal/store"
)

func init() {
	rcCmd := &cobra.Command{
		Use:   "rc",
		Short: "RevenueCat connector — keyword-level revenue attribution",
		Long: `Connect RevenueCat to join Apple Ads spend with real revenue.

Setup (one-time):
  1. In your app: Purchases.shared.attribution.enableAdServicesAttributionTokenCollection()
     (2 lines with the RC SDK — no ATT prompt needed)
  2. In RevenueCat: connect the Apple Search Ads integration
     (Sign in with Apple, Read Only role is sufficient)
  3. appadscli rc connect --api-key <v2-key> --project <id>
  4. Export data (Scheduled Data Exports recommended) and:
     appadscli rc ingest ./export.csv

Then: appadscli roas report --by keyword`,
	}

	var apiKey, project string
	connect := &cobra.Command{
		Use:   "connect",
		Short: "Store and validate RevenueCat API v2 credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds := &rc.Credentials{APIKey: apiKey, ProjectID: project}
			if err := rc.Validate(cmd.Context(), creds); err != nil {
				return err
			}
			if err := rc.SaveCredentials(creds); err != nil {
				return err
			}
			fmt.Println("✓ RevenueCat connected (project", project+")")
			return nil
		},
	}
	connect.Flags().StringVar(&apiKey, "api-key", "", "RevenueCat REST API v2 secret key (required)")
	connect.Flags().StringVar(&project, "project", "", "RevenueCat project id (required)")
	_ = connect.MarkFlagRequired("api-key")
	_ = connect.MarkFlagRequired("project")

	ingest := &cobra.Command{
		Use:   "ingest <export.csv|export.jsonl>",
		Short: "Load an RC data export into the local store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			rows, attributed, err := rc.IngestFile(args[0], st)
			if err != nil {
				return err
			}
			fmt.Printf("✓ ingested %d transaction(s), %d with ASA attribution (%.0f%%)\n",
				rows, attributed, pctOf(attributed, rows))
			if attributed == 0 && rows > 0 {
				fmt.Println("⚠ no ASA-attributed rows — check that AdServices token collection and the")
				fmt.Println("  Apple Search Ads integration are enabled in RevenueCat (see `appadscli rc --help`)")
			}
			return nil
		},
	}

	rcCmd.AddCommand(connect, ingest)
	rootCmd.AddCommand(rcCmd)

	// roas
	roasCmd := &cobra.Command{Use: "roas", Short: "Return on ad spend, down to keyword level"}
	var by, since string
	report := &cobra.Command{
		Use:   "report",
		Short: "Spend (Ads API) ⋈ revenue (RevenueCat) → ROAS by campaign/adgroup/keyword",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			start, err := api.ParseSince(since, time.Now())
			if err != nil {
				return err
			}
			revenue, err := rc.RevenueBy(st, by, start)
			if err != nil {
				return err
			}
			if len(revenue) == 0 {
				return fmt.Errorf("no RevenueCat data in window — run `appadscli rc ingest` first")
			}
			path := map[string]string{
				"campaign": "/v1/reports/apps/campaigns/query",
				"adgroup":  "/v1/reports/apps/adgroups/query",
				"keyword":  "/v1/reports/apps/keywords/query",
			}[by]
			// Reports key the reported entity as id/name (keywords carry text).
			idField := "id"
			nameField := map[string]string{
				"campaign": "name", "adgroup": "name", "keyword": "text",
			}[by]
			req, err := api.NewReportRequest(since, "")
			if err != nil {
				return err
			}
			spendRows, err := c.RunReport(cmd.Context(), path, req)
			if err != nil {
				return err
			}
			spendByID := map[string]float64{}
			nameByID := map[string]string{}
			for _, r := range spendRows {
				id := api.Field(r, idField)
				spendByID[id] += api.FloatField(r, "localSpend")
				nameByID[id] = api.Field(r, nameField)
			}
			type roasRow struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				Spend    float64 `json:"spend"`
				Proceeds float64 `json:"proceeds"`
				ROAS     float64 `json:"roas"`
				Users    int     `json:"attributedUsers"`
			}
			var out []roasRow
			for _, rev := range revenue {
				if rev.Key == "" {
					out = append(out, roasRow{ID: "(unattributed)", Name: "organic + limit-ad-tracking",
						Proceeds: rev.Proceeds, Users: rev.Users})
					continue
				}
				row := roasRow{ID: rev.Key, Name: nameByID[rev.Key],
					Spend: spendByID[rev.Key], Proceeds: rev.Proceeds, Users: rev.Users}
				if row.Spend > 0 {
					row.ROAS = row.Proceeds / row.Spend
				}
				out = append(out, row)
			}
			if render().Format == "json" {
				return render().JSON(out)
			}
			var rows [][]string
			for _, r := range out {
				roas := "—"
				if r.ROAS > 0 {
					roas = fmt.Sprintf("%.0f%%", r.ROAS*100)
				}
				rows = append(rows, []string{r.ID, r.Name, fmt.Sprintf("%.2f", r.Spend),
					fmt.Sprintf("%.2f", r.Proceeds), roas, fmt.Sprint(r.Users)})
			}
			fmt.Println("note: measured ROAS is a floor — the organic bucket includes limit-ad-tracking users")
			return render().Rows([]string{"Id", "Name", "Spend", "Proceeds", "ROAS", "Users"}, rows, out)
		},
	}
	report.Flags().StringVar(&by, "by", "campaign", "campaign|adgroup|keyword")
	report.Flags().StringVar(&since, "since", "30d", "window start")
	roasCmd.AddCommand(report)
	rootCmd.AddCommand(roasCmd)

	// ltv
	ltvCmd := &cobra.Command{Use: "ltv", Short: "Lifetime value by acquisition source"}
	var lBy, lHorizon string
	lReport := &cobra.Command{
		Use:   "report",
		Short: "Per-user revenue by campaign/keyword over a horizon",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			horizon := map[string]time.Duration{
				"30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour, "1y": 365 * 24 * time.Hour,
			}[lHorizon]
			if horizon == 0 {
				return fmt.Errorf("--horizon must be 30d, 90d, or 1y")
			}
			revenue, err := rc.RevenueBy(st, lBy, time.Now().Add(-horizon))
			if err != nil {
				return err
			}
			type ltvRow struct {
				ID       string  `json:"id"`
				Proceeds float64 `json:"proceeds"`
				Users    int     `json:"users"`
				LTV      float64 `json:"ltvPerUser"`
			}
			var out []ltvRow
			for _, r := range revenue {
				id := r.Key
				if id == "" {
					id = "(unattributed)"
				}
				row := ltvRow{ID: id, Proceeds: r.Proceeds, Users: r.Users}
				if r.Users > 0 {
					row.LTV = r.Proceeds / float64(r.Users)
				}
				out = append(out, row)
			}
			return render().JSON(map[string]any{
				"horizon": lHorizon, "by": lBy, "rows": out,
				"note": "separate mature vs. active trials before judging profitability; ROAS/LTV here is a floor",
			})
		},
	}
	lReport.Flags().StringVar(&lBy, "by", "campaign", "campaign|adgroup|keyword")
	lReport.Flags().StringVar(&lHorizon, "horizon", "90d", "30d|90d|1y")
	ltvCmd.AddCommand(lReport)
	rootCmd.AddCommand(ltvCmd)
}

func pctOf(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

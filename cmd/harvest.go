package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/appadscli/internal/engine"
	"github.com/tawhidkuet04/appadscli/internal/store"
)

func init() {
	harvestCmd := &cobra.Command{
		Use:   "harvest",
		Short: "The harvest loop: promote converting search terms, negate waste",
	}

	var (
		discovery, target, since, rankBy, currency string
		minInstalls, maxCPA, wasteTaps, bidFactor  float64
		autoNegate                                 bool
	)
	run := &cobra.Command{
		Use:   "run",
		Short: "Mine discovery search terms → exact-match winners in target (+ negatives)",
		Long: `The core ASA workflow. Reads the discovery campaign's search terms report,
then:

  1. promotes terms with ≥ --min-installs (and CPA ≤ --max-cpa) to exact
     match in the target campaign's ad group, bid = observed CPT × --bid-factor
  2. negates promoted terms in discovery so they don't compete
  3. with --auto-negate, negates terms wasting spend (taps, no installs)

Terms promoted in earlier runs are remembered locally and never promoted
twice. Always try --dry-run first.`,
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
			opts := engine.HarvestOpts{
				DiscoveryCampaign: discovery, TargetCampaign: target, Since: since,
				MinInstalls: minInstalls, MaxCPA: maxCPA, AutoNegate: autoNegate,
				WasteTaps: wasteTaps, BidFactor: bidFactor, RankBy: rankBy,
			}
			actions, err := engine.HarvestPlan(cmd.Context(), c, st, opts)
			if err != nil {
				return err
			}
			if len(actions) == 0 {
				fmt.Println("nothing to harvest — no search terms met the thresholds")
				return nil
			}
			dry, _ := cmd.Flags().GetBool("dry-run")
			if dry {
				return render().JSON(actions)
			}
			promotes, negates := 0, 0
			for _, a := range actions {
				if a.Action == "promote" {
					promotes++
				} else {
					negates++
				}
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("promote %d term(s) to campaign %s and add %d negative(s)",
				promotes, target, promotes+negates))
			if err != nil || !ok {
				return err
			}
			result, err := engine.HarvestApply(cmd.Context(), c, st, opts, actions, currency)
			if err != nil {
				return err
			}
			return render().JSON(result)
		},
	}
	run.Flags().StringVar(&discovery, "discovery", "", "discovery campaign id (required)")
	run.Flags().StringVar(&target, "target", "", "target campaign id for promoted keywords (required)")
	run.Flags().StringVar(&since, "since", "30d", "search terms window")
	run.Flags().Float64Var(&minInstalls, "min-installs", 2, "promote terms with at least this many installs")
	run.Flags().Float64Var(&maxCPA, "max-cpa", 0, "…and CPA at or under this (0 = ignore)")
	run.Flags().BoolVar(&autoNegate, "auto-negate", false, "negate wasteful terms (taps, zero installs)")
	run.Flags().Float64Var(&wasteTaps, "waste-taps", 20, "taps with zero installs to call a term wasteful")
	run.Flags().Float64Var(&bidFactor, "bid-factor", 1.1, "promoted bid = observed CPT × this")
	run.Flags().StringVar(&rankBy, "rank-by", "installs", "installs|roas (roas needs `appadscli rc ingest`)")
	run.Flags().StringVar(&currency, "currency", "USD", "bid currency for promoted keywords")
	_ = run.MarkFlagRequired("discovery")
	_ = run.MarkFlagRequired("target")
	addMutationFlags(run)

	var repSince string
	report := &cobra.Command{
		Use:   "report",
		Short: "What past harvest runs promoted and negated",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			start := time.Now().AddDate(0, 0, -7)
			if repSince != "" {
				if t, err := time.Parse("2006-01-02", repSince); err == nil {
					start = t
				} else if d, err := time.ParseDuration(repSince); err == nil {
					start = time.Now().Add(-d)
				} else {
					var n int
					if _, err := fmt.Sscanf(repSince, "%dd", &n); err == nil {
						start = time.Now().AddDate(0, 0, -n)
					}
				}
			}
			entries, err := st.HarvestLog(start)
			if err != nil {
				return err
			}
			if render().Format == "json" {
				return render().JSON(entries)
			}
			var rows [][]string
			for _, e := range entries {
				rows = append(rows, []string{e.At.Format("2006-01-02 15:04"), e.Action, e.SearchTerm, e.Detail})
			}
			return render().Rows([]string{"At", "Action", "SearchTerm", "Detail"}, rows, entries)
		},
	}
	report.Flags().StringVar(&repSince, "since", "7d", "window start")

	harvestCmd.AddCommand(run, report)
	rootCmd.AddCommand(harvestCmd)
}

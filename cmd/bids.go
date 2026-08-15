package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/engine"
	"github.com/tawhidkuet04/adastra/internal/store"
)

func init() {
	bidsCmd := &cobra.Command{Use: "bids", Short: "Bid optimization: CPA/ROAS targets and declarative rules"}

	var (
		adgroup, since, maxChange, currency string
		targetCPA, targetROAS, minTaps      float64
	)
	adjust := &cobra.Command{
		Use:   "adjust",
		Short: "Move keyword bids toward --target-cpa or --target-roas (capped by --max-change)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			if (targetCPA == 0) == (targetROAS == 0) {
				return fmt.Errorf("pass exactly one of --target-cpa or --target-roas")
			}
			maxPct := 20.0
			if maxChange != "" {
				fmt.Sscanf(strings.TrimSuffix(maxChange, "%"), "%f", &maxPct)
			}
			roas := targetROAS
			if roas > 5 { // allow "150%" style input
				roas = roas / 100
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			changes, err := engine.BidPlan(cmd.Context(), c, st, engine.BidAdjustOpts{
				AdGroupID: adgroup, TargetCPA: targetCPA, TargetROAS: roas,
				MaxChangePct: maxPct, Since: since, MinTaps: minTaps,
			})
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Println("no bid changes needed")
				return nil
			}
			dry, _ := cmd.Flags().GetBool("dry-run")
			if dry {
				return render().JSON(changes)
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("adjust %d keyword bid(s) in ad group %s", len(changes), adgroup))
			if err != nil || !ok {
				return err
			}
			out, err := engine.BidApply(cmd.Context(), c, st, changes, currency)
			if err != nil {
				return err
			}
			return render().JSON(map[string]any{"changed": len(changes), "result": out})
		},
	}
	adjust.Flags().StringVar(&adgroup, "adgroup", "", "ad group id (required)")
	adjust.Flags().Float64Var(&targetCPA, "target-cpa", 0, "target cost per install")
	adjust.Flags().Float64Var(&targetROAS, "target-roas", 0, "target ROAS, e.g. 1.5 or 150 (needs `adastra rc ingest`)")
	adjust.Flags().StringVar(&maxChange, "max-change", "20%", "max per-run bid change")
	adjust.Flags().StringVar(&since, "since", "14d", "performance window")
	adjust.Flags().Float64Var(&minTaps, "min-taps", 10, "ignore keywords with fewer taps")
	adjust.Flags().StringVar(&currency, "currency", "USD", "bid currency")
	_ = adjust.MarkFlagRequired("adgroup")
	addMutationFlags(adjust)

	rulesCmd := &cobra.Command{Use: "rules", Short: "Declarative bid rules from bid-rules.json"}
	var rulesFile string
	apply := &cobra.Command{
		Use:   "apply",
		Short: "Evaluate and apply bid rules from a config file",
		Long: `bid-rules.json format:

  {
    "rules": [
      { "adgroup": "123", "targetCpa": 2.50, "maxChangePct": 15,
        "since": "14d", "minTaps": 10 },
      { "adgroup": "456", "targetRoas": 1.5, "maxChangePct": 20 }
    ]
  }

Each rule runs the same engine as ` + "`adastra bids adjust`" + ` scoped to its
ad group. Use --dry-run to preview all changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			b, err := os.ReadFile(rulesFile)
			if err != nil {
				return err
			}
			var conf struct {
				Rules []struct {
					AdGroup      string  `json:"adgroup"`
					TargetCPA    float64 `json:"targetCpa"`
					TargetROAS   float64 `json:"targetRoas"`
					MaxChangePct float64 `json:"maxChangePct"`
					Since        string  `json:"since"`
					MinTaps      float64 `json:"minTaps"`
					Currency     string  `json:"currency"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(b, &conf); err != nil {
				return fmt.Errorf("%s: %w", rulesFile, err)
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			dry, _ := cmd.Flags().GetBool("dry-run")
			all := map[string]any{}
			total := 0
			for _, r := range conf.Rules {
				if r.MaxChangePct == 0 {
					r.MaxChangePct = 20
				}
				if r.Since == "" {
					r.Since = "14d"
				}
				if r.MinTaps == 0 {
					r.MinTaps = 10
				}
				if r.Currency == "" {
					r.Currency = "USD"
				}
				changes, err := engine.BidPlan(cmd.Context(), c, st, engine.BidAdjustOpts{
					AdGroupID: r.AdGroup, TargetCPA: r.TargetCPA, TargetROAS: r.TargetROAS,
					MaxChangePct: r.MaxChangePct, Since: r.Since, MinTaps: r.MinTaps,
				})
				if err != nil {
					return fmt.Errorf("rule for ad group %s: %w", r.AdGroup, err)
				}
				all[r.AdGroup] = changes
				total += len(changes)
				if !dry && len(changes) > 0 {
					ok, err := confirmOrAbort(cmd, fmt.Sprintf("apply %d bid change(s) to ad group %s", len(changes), r.AdGroup))
					if err != nil {
						return err
					}
					if ok {
						if _, err := engine.BidApply(cmd.Context(), c, st, changes, r.Currency); err != nil {
							return err
						}
					}
				}
			}
			if dry {
				return render().JSON(all)
			}
			fmt.Printf("✓ %d bid change(s) across %d rule(s)\n", total, len(conf.Rules))
			return nil
		},
	}
	apply.Flags().StringVar(&rulesFile, "config", "./bid-rules.json", "rules file")
	addMutationFlags(apply)
	rulesCmd.AddCommand(apply)

	bidsCmd.AddCommand(adjust, rulesCmd)
	rootCmd.AddCommand(bidsCmd)
}

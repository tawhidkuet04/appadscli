package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/engine"
	"github.com/appadscli/appadscli/internal/store"
)

func init() {
	var configPath, planOut string
	var autoApply bool
	watchCmd := &cobra.Command{
		Use:   "watch",
		Short: "The agent tick: evaluate guardrails, alert / propose / auto-apply",
		Long: `Run from cron or GitHub Actions every few hours. Evaluates your
guardrails.json against live data:

  - CPA breaches per campaign (with per-campaign overrides)
  - account-level spend anomalies
  - organic rank drops (from ` + "`aso track`" + ` history)
  - harvest candidates in the discovery campaign

Autonomy ladder ("autonomy" in guardrails.json, --auto-apply overrides):
  alert    print findings only (default)
  propose  also write a plan file for ` + "`appadscli plan apply`" + `
  auto     apply changes within caps (never touches "neverPause" campaigns)

Exit code is non-zero when alerts fired — wire it to CI notifications.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			g, err := engine.LoadGuardrails(configPath)
			if err != nil {
				return err
			}
			if autoApply {
				g.Autonomy = "auto"
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			res, err := engine.WatchTick(cmd.Context(), c, st, g)
			if err != nil {
				return err
			}
			if res.Proposals != nil && len(res.Proposals.Changes) > 0 {
				path := planOut
				if path == "" {
					path = fmt.Sprintf("appadscli-plan-%s.json", time.Now().Format("20060102-150405"))
				}
				if err := engine.WritePlan(res.Proposals, path); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "→ plan written to %s (review with `appadscli plan show %s`)\n", path, path)
			}
			if g.Alerts != nil && g.Alerts.Webhook != "" {
				notifyWebhook(g.Alerts.Webhook, res)
			}
			if err := render().JSON(res); err != nil {
				return err
			}
			for _, f := range res.Findings {
				if f.Severity == "alert" {
					return fmt.Errorf("%d finding(s) include alerts", len(res.Findings))
				}
			}
			return nil
		},
	}
	watchCmd.Flags().StringVar(&configPath, "config", "./guardrails.json", "guardrails file")
	watchCmd.Flags().BoolVar(&autoApply, "auto-apply", false, "force autonomy=auto for this tick")
	watchCmd.Flags().StringVar(&planOut, "plan-out", "", "where to write the proposal plan (propose mode)")
	rootCmd.AddCommand(watchCmd)

	// plan
	planCmd := &cobra.Command{Use: "plan", Short: "Review and apply proposed change plans (PR-style)"}
	show := &cobra.Command{
		Use:   "show <plan.json>",
		Short: "Human-readable diff of proposed changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := engine.ReadPlan(args[0])
			if err != nil {
				return err
			}
			if render().Format == "json" {
				return render().JSON(p)
			}
			fmt.Printf("plan from %s (source: %s, account: %s)\n%d change(s):\n\n",
				p.CreatedAt.Format(time.RFC822), p.Source, p.Account, len(p.Changes))
			for i, ch := range p.Changes {
				fmt.Printf("%2d. %s\n    %s %s\n", i+1, ch.Description, ch.Method, ch.Path)
			}
			fmt.Println("\napply with: appadscli plan apply", args[0], "--confirm")
			return nil
		},
	}
	apply := &cobra.Command{
		Use:   "apply <plan.json>",
		Short: "Execute every change in a plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			p, err := engine.ReadPlan(args[0])
			if err != nil {
				return err
			}
			if len(p.Changes) == 0 {
				fmt.Println("plan has no changes")
				return nil
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("apply %d change(s) from %s", len(p.Changes), args[0]))
			if err != nil || !ok {
				return err
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			results, err := engine.ApplyPlan(cmd.Context(), c, st, p)
			if jerr := render().JSON(results); jerr != nil {
				return jerr
			}
			return err
		},
	}
	addMutationFlags(apply)
	planCmd.AddCommand(show, apply)
	rootCmd.AddCommand(planCmd)
}

func notifyWebhook(url string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		fmt.Fprintln(os.Stderr, "⚠ webhook notify failed:", err)
		return
	}
	resp.Body.Close()
}

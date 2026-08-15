package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/api"
)

// recoPath maps --type to the v1 recommendation family.
func recoPath(t string) (string, error) {
	switch strings.ToLower(t) {
	case "budget", "daily-budget", "daily-budgets":
		return "/v1/recommendations/daily-budgets", nil
	case "target-cpa", "target-cpas", "cpa":
		return "/v1/recommendations/target-cpas", nil
	}
	return "", fmt.Errorf("unknown --type %q (use budget or target-cpa)", t)
}

func init() {
	recoCmd := &cobra.Command{Use: "reco", Short: "Apple's own optimization recommendations"}

	var listType string
	list := &cobra.Command{
		Use:   "list",
		Short: "List open recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			types := []string{"budget", "target-cpa"}
			if listType != "" {
				types = []string{listType}
			}
			var all []json.RawMessage
			for _, t := range types {
				base, err := recoPath(t)
				if err != nil {
					return err
				}
				items, err := c.Query(cmd.Context(), base+"/query", &api.Selector{}, 0)
				if err != nil {
					return err
				}
				all = append(all, items...)
			}
			h, rows := api.Table(all, []string{
				"Id=id", "Type=recommendationType", "CampaignId=campaignId",
				"State=state", "Current=$money:currentValue", "Recommended=$money:recommendedValue",
				"UpliftTaps=estimatedTapsUplift", "UpliftInstalls=estimatedInstallsUplift",
			})
			return render().Rows(h, rows, all)
		},
	}
	list.Flags().StringVar(&listType, "type", "", "budget|target-cpa (default: both)")

	applyOrDismiss := func(verb string) *cobra.Command {
		var t string
		cmd := &cobra.Command{
			Use:   verb + " <recommendationId>",
			Short: strings.ToUpper(verb[:1]) + verb[1:] + " a recommendation",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c := client()
				if err := c.RequireAccount(); err != nil {
					return err
				}
				base, err := recoPath(t)
				if err != nil {
					return err
				}
				ok, err := confirmOrAbort(cmd, verb+" recommendation "+args[0]+" ("+t+")")
				if err != nil || !ok {
					return err
				}
				body := []map[string]any{{"id": args[0]}}
				var out json.RawMessage
				if err := c.Post(cmd.Context(), base+"/"+verb, body, &out); err != nil {
					return err
				}
				if out == nil {
					fmt.Println(verb, "OK:", args[0])
					return nil
				}
				return render().JSON(out)
			},
		}
		cmd.Flags().StringVar(&t, "type", "budget", "budget|target-cpa (required to route the id)")
		addMutationFlags(cmd)
		return cmd
	}

	recoCmd.AddCommand(list, applyOrDismiss("apply"), applyOrDismiss("dismiss"))
	rootCmd.AddCommand(recoCmd)
}

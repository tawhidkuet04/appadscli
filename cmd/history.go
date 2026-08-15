package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/appadscli/internal/api"
	"github.com/tawhidkuet04/appadscli/internal/store"
)

func init() {
	var since, entity string
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Change history — the audit trail of every mutation in the account",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			start, err := api.ParseSince(since, time.Now())
			if err != nil {
				return err
			}
			sel := &api.Selector{Conditions: []api.Condition{{
				Field: "timestamp", Operator: "GREATER_THAN", Values: []string{start.Format("2006-01-02")},
			}}}
			if entity != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{
					Field: "entityType", Operator: "EQUALS", Values: []string{strings.ToUpper(entity)},
				})
			}
			items, err := c.Query(cmd.Context(), "/v1/change-history/query", sel, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Time=timestamp", "Entity=entityType", "EntityId=entityId",
				"Action=action", "Field=field", "From=oldValue", "To=newValue", "User=userName", "Source=source",
			})
			return render().Rows(h, rows, items)
		},
	}
	historyCmd.Flags().StringVar(&since, "since", "7d", "window start")
	historyCmd.Flags().StringVar(&entity, "entity", "", "campaign|adgroup|keyword|...")

	verify := &cobra.Command{
		Use:   "verify",
		Short: "Cross-check Apple's change history against appadscli's local mutation log",
		Long: `Compares mutations recorded locally by appadscli against Apple's change
history for the same window. Changes present in Apple's history but absent
from the local log were made outside this CLI (web UI, another tool, or
another machine).`,
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
			localSince := time.Now().AddDate(0, 0, -7)
			local, err := st.MutationsSince(localSince)
			if err != nil {
				return err
			}
			sel := &api.Selector{Conditions: []api.Condition{{
				Field: "timestamp", Operator: "GREATER_THAN", Values: []string{localSince.Format("2006-01-02")},
			}}}
			remote, err := c.Query(cmd.Context(), "/v1/change-history/query", sel, 0)
			if err != nil {
				return err
			}
			localKeys := map[string]bool{}
			for _, m := range local {
				localKeys[m.EntityType+":"+m.EntityID] = true
			}
			var external [][]string
			for _, r := range remote {
				key := strings.ToLower(api.Field(r, "entityType")) + ":" + api.Field(r, "entityId")
				if !localKeys[key] {
					external = append(external, []string{
						api.Field(r, "timestamp"), api.Field(r, "entityType"), api.Field(r, "entityId"),
						api.Field(r, "action"), api.Field(r, "userName"),
					})
				}
			}
			fmt.Printf("local mutations (7d): %d   apple-recorded changes: %d   external: %d\n\n",
				len(local), len(remote), len(external))
			if len(external) == 0 {
				fmt.Println("✓ no changes outside appadscli detected")
				return nil
			}
			return render().Rows([]string{"Time", "Entity", "EntityId", "Action", "User"}, external, nil)
		},
	}

	historyCmd.AddCommand(verify)
	rootCmd.AddCommand(historyCmd)
}

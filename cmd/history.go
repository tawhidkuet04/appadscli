package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
	"github.com/appadscli/appadscli/internal/store"
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
			items, err := changeHistory(cmd, c, start, time.Now(), entity)
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
	historyCmd.Flags().StringVar(&since, "since", "7d", "window start (Apple keeps 30 days)")
	historyCmd.Flags().StringVar(&entity, "entity", "", "campaign|adgroup|keyword|ad|org (default: all)")

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
			remote, err := changeHistory(cmd, c, localSince, time.Now(), "")
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

// changeHistoryEntities are the entity types v1 change history serves. The
// endpoint takes exactly one per query, so "everything" means one call each.
var changeHistoryEntities = []string{"CAMPAIGN", "ADGROUP", "KEYWORD", "AD", "ORG"}

// changeHistory queries Apple's change history for a window. entity == ""
// walks every entity type; Apple caps the window at 30 days.
func changeHistory(cmd *cobra.Command, c *api.Client, start, end time.Time, entity string) ([]json.RawMessage, error) {
	entities := changeHistoryEntities
	if entity != "" {
		entities = []string{strings.ToUpper(entity)}
	}
	var all []json.RawMessage
	for _, e := range entities {
		sel := &api.Selector{Filters: []api.Filter{
			{Field: "eventTime", Operator: "BETWEEN", Value: []string{
				start.Format("2006-01-02"), end.Format("2006-01-02"),
			}},
			{Field: "entityType", Operator: "EQUALS", Value: e},
		}}
		items, err := c.Query(cmd.Context(), "/v1/change-history/query", sel, 0)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

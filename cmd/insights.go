package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/api"
)

func init() {
	insightsCmd := &cobra.Command{Use: "insights", Short: "Apple's demand data: impression share & search term popularity"}

	var (
		isApp, isSince, isKeywordsFile, isCountries string
	)
	impressionShare := &cobra.Command{
		Use:   "impression-share",
		Short: "Impression share — who's squeezing you, especially on brand terms",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			start, err := api.ParseSince(isSince, time.Now())
			if err != nil {
				return err
			}
			sel := &api.Selector{}
			if isApp != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "adamId", Operator: "EQUALS", Values: []string{isApp}})
			}
			if isCountries != "" {
				sel.Conditions = append(sel.Conditions, api.Condition{
					Field: "countryOrRegion", Operator: "IN", Values: strings.Split(strings.ToUpper(isCountries), ","),
				})
			}
			if isKeywordsFile != "" {
				terms, err := readLines(isKeywordsFile)
				if err != nil {
					return err
				}
				sel.Conditions = append(sel.Conditions, api.Condition{Field: "searchTerm", Operator: "IN", Values: terms})
			}
			body := map[string]any{
				"startTime": start.Format("2006-01-02"),
				"endTime":   time.Now().Format("2006-01-02"),
				"selector":  sel,
			}
			env, err := c.DoRaw(cmd.Context(), "POST", "/v1/insights/apps/impression-share/query", body)
			if err != nil {
				return err
			}
			items := envItems(env)
			h, rows := api.Table(items, []string{
				"SearchTerm=searchTerm", "Country=countryOrRegion",
				"LowIS=lowImpressionShare", "HighIS=highImpressionShare",
				"Rank=rank", "Popularity=searchPopularity",
			})
			return render().Rows(h, rows, items)
		},
	}
	impressionShare.Flags().StringVar(&isApp, "app", "", "adamId")
	impressionShare.Flags().StringVar(&isSince, "since", "7d", "window start")
	impressionShare.Flags().StringVar(&isKeywordsFile, "keywords", "", "newline-separated search terms file (e.g. your brand terms)")
	impressionShare.Flags().StringVar(&isCountries, "countries", "", "comma-separated storefronts")

	var popTermsFile, popTerms, popCountries string
	popularity := &cobra.Command{
		Use:   "popularity",
		Short: "Search term popularity (1–5) — Apple's own demand scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var terms []string
			if popTerms != "" {
				terms = strings.Split(popTerms, ",")
			}
			if popTermsFile != "" {
				fromFile, err := readLines(popTermsFile)
				if err != nil {
					return err
				}
				terms = append(terms, fromFile...)
			}
			if len(terms) == 0 {
				return fmt.Errorf("no terms — pass --terms or --file")
			}
			items, err := searchTermPopularity(cmd, terms, popCountries)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"SearchTerm=searchTerm", "Country=countryOrRegion", "Popularity=searchPopularity",
			})
			return render().Rows(h, rows, items)
		},
	}
	popularity.Flags().StringVar(&popTerms, "terms", "", "comma-separated search terms")
	popularity.Flags().StringVar(&popTermsFile, "file", "", "newline-separated terms file")
	popularity.Flags().StringVar(&popCountries, "countries", "us", "comma-separated storefronts")

	insightsCmd.AddCommand(impressionShare, popularity)
	rootCmd.AddCommand(insightsCmd)
}

// searchTermPopularity queries Apple's search-term-popularity insight.
func searchTermPopularity(cmd *cobra.Command, terms []string, countries string) ([]json.RawMessage, error) {
	c := client()
	if err := c.RequireAccount(); err != nil {
		return nil, err
	}
	sel := &api.Selector{Conditions: []api.Condition{
		{Field: "searchTerm", Operator: "IN", Values: terms},
	}}
	if countries != "" {
		sel.Conditions = append(sel.Conditions, api.Condition{
			Field: "countryOrRegion", Operator: "IN", Values: strings.Split(strings.ToUpper(countries), ","),
		})
	}
	env, err := c.DoRaw(cmd.Context(), "POST", "/v1/insights/apps/search-term-popularity/query", map[string]any{"selector": sel})
	if err != nil {
		return nil, err
	}
	return envItems(env), nil
}

// envItems decodes an envelope's data as an item slice, tolerating single objects.
func envItems(env *api.Envelope) []json.RawMessage {
	if env == nil || len(env.Data) == 0 {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(env.Data, &items); err == nil {
		return items
	}
	return []json.RawMessage{env.Data}
}

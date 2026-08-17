package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
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
			var filters []api.Filter
			if isApp != "" {
				filters = append(filters, api.Filter{Field: "adamId", Operator: "EQUALS", Value: isApp})
			}
			if isCountries != "" {
				filters = append(filters, api.Filter{
					Field: "countryOrRegion", Operator: "IN", Value: strings.Split(strings.ToUpper(isCountries), ","),
				})
			}
			if isKeywordsFile != "" {
				terms, err := readLines(isKeywordsFile)
				if err != nil {
					return err
				}
				filters = append(filters, api.Filter{Field: "searchTerm", Operator: "IN", Value: terms})
			}
			body := map[string]any{
				"timeRange": insightsTimeRange(start, time.Now(), "DAILY"),
				"filters":   filters,
			}
			env, err := c.DoRaw(cmd.Context(), "POST", "/v1/insights/apps/impression-share/query", body)
			if err != nil {
				return err
			}
			items := envItems(env)
			h, rows := api.Table(items, []string{
				"SearchTerm=searchTerm", "Country=countryOrRegion",
				"LowIS=lowImpressionShare", "HighIS=highImpressionShare",
				"Rank=rank", "Popularity=searchPopularity1to5",
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
				"SearchTerm=searchTerm", "Country=countryOrRegion", "Week=week",
				"Genre=genre", "Popularity=searchPopularity1to5", "RankInGenre=rankInGenre",
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

// searchTermPopularity queries Apple's search-term-popularity insight. The
// endpoint reports whole Sun–Sat weeks, so the window has to be wide enough to
// contain one — a bare 7 days can miss every week boundary and come back empty.
func searchTermPopularity(cmd *cobra.Command, terms []string, countries string) ([]json.RawMessage, error) {
	c := client()
	if err := c.RequireAccount(); err != nil {
		return nil, err
	}
	filters := []api.Filter{{Field: "searchTerm", Operator: "IN", Value: terms}}
	if countries != "" {
		filters = append(filters, api.Filter{
			Field: "countryOrRegion", Operator: "IN", Value: strings.Split(strings.ToUpper(countries), ","),
		})
	}
	body := map[string]any{
		"timeRange": insightsTimeRange(time.Now().AddDate(0, 0, -28), time.Now(), "WEEKLY_SUN_SAT"),
		"filters":   filters,
	}
	env, err := c.DoRaw(cmd.Context(), "POST", "/v1/insights/apps/search-term-popularity/query", body)
	if err != nil {
		return nil, err
	}
	return envItems(env), nil
}

// insightsTimeRange builds the window insights endpoints require. Both reject a
// missing granularity, and each accepts its own set: impression share takes
// DAILY or WEEKLY_SUN_SAT, search term popularity WEEKLY_SUN_SAT or MONTHLY.
func insightsTimeRange(start, end time.Time, granularity string) map[string]string {
	return map[string]string{
		"start":       start.Format("2006-01-02"),
		"end":         end.Format("2006-01-02"),
		"granularity": granularity,
	}
}

// envItems decodes an envelope's result as an item slice. Insights wrap theirs
// in {"rows": [...]}; a bare list or single object is tolerated too.
func envItems(env *api.Envelope) []json.RawMessage {
	if env == nil || len(env.Result) == 0 {
		return nil
	}
	var wrapper struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(env.Result, &wrapper); err == nil && wrapper.Rows != nil {
		return wrapper.Rows
	}
	var items []json.RawMessage
	if err := json.Unmarshal(env.Result, &items); err == nil {
		return items
	}
	return []json.RawMessage{env.Result}
}

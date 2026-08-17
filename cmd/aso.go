package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
	"github.com/appadscli/appadscli/internal/aso"
	"github.com/appadscli/appadscli/internal/itunes"
)

func asoCountry(c string) string {
	if c != "" {
		return c
	}
	if def := cfg().DefaultCountry; def != "" {
		return def
	}
	return "us"
}

func init() {
	asoCmd := &cobra.Command{Use: "aso", Short: "Organic App Store optimization: research, ranks, metadata, competitors"}

	// --- research ---
	var rCountry string
	var rExpand bool
	research := &cobra.Command{
		Use:   "research <seed-term>",
		Short: "Popularity + difficulty + top apps for a term (with --expand fan-out)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := args[0]
			country := asoCountry(rCountry)
			apps, err := itunes.Search(cmd.Context(), term, country, 10)
			if err != nil {
				return err
			}
			diff := aso.ComputeDifficulty(term, country, apps)
			result := map[string]any{
				"term": term, "country": country,
				"difficulty":          diff.Score,
				"top10AvgRatingCount": diff.Top10AvgRatings,
				"top10AvgStars":       diff.Top10AvgStars,
				"titleMatches":        diff.TitleMatches,
			}
			// Apple's own popularity, when credentials + account are set.
			if pop, err := searchTermPopularity(cmd, []string{term}, country); err == nil && len(pop) > 0 {
				result["searchPopularity"] = api.Field(pop[0], "searchPopularity1to5")
			} else if err != nil {
				result["searchPopularity"] = "n/a (login + account required)"
			}
			var top []map[string]any
			for i, a := range diff.Apps {
				top = append(top, map[string]any{
					"rank": i + 1, "adamId": a.AdamID, "name": a.Name,
					"ratingCount": a.RatingCount, "stars": a.Rating,
				})
			}
			result["top10"] = top
			if rExpand {
				candidates := aso.CandidateTerms(apps, term)
				result["expandedCandidates"] = candidates
				if len(candidates) > 0 {
					if pop, err := searchTermPopularity(cmd, candidates, country); err == nil {
						result["expandedPopularity"] = pop
					}
				}
			}
			return render().JSON(result)
		},
	}
	research.Flags().StringVar(&rCountry, "country", "", "storefront (default us)")
	research.Flags().BoolVar(&rExpand, "expand", false, "fan out to candidate terms mined from the top apps")

	// --- popularity (bulk) ---
	var popFile, popCountries string
	popularity := &cobra.Command{
		Use:   "popularity",
		Short: "Bulk search-term popularity from Apple's own data",
		RunE: func(cmd *cobra.Command, args []string) error {
			terms, err := readLines(popFile)
			if err != nil {
				return err
			}
			items, err := searchTermPopularity(cmd, terms, popCountries)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"SearchTerm=searchTerm", "Country=countryOrRegion", "Week=week",
				"Popularity=searchPopularity1to5", "RankInGenre=rankInGenre",
			})
			return render().Rows(h, rows, items)
		},
	}
	popularity.Flags().StringVar(&popFile, "terms", "", "newline-separated terms file (required)")
	popularity.Flags().StringVar(&popCountries, "countries", "us", "comma-separated storefronts")
	_ = popularity.MarkFlagRequired("terms")

	// --- suggest ---
	var sApp, sType string
	suggest := &cobra.Command{
		Use:   "suggest",
		Short: "Apple's keyword/phrase/category suggestions for an app",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			path := map[string]string{
				"keyword":  "/v1/suggestions/keywords/query",
				"phrase":   "/v1/suggestions/phrases/query",
				"category": "/v1/suggestions/categories/query",
			}[strings.ToLower(sType)]
			if path == "" {
				return fmt.Errorf("--type must be keyword, phrase, or category")
			}
			sel := &api.Selector{Filters: promotedAppFilters(sApp)}
			items, err := c.Query(cmd.Context(), path, sel, 0)
			if err != nil {
				return err
			}
			return render().JSON(items)
		},
	}
	suggest.Flags().StringVar(&sApp, "app", "", "adamId (required)")
	suggest.Flags().StringVar(&sType, "type", "keyword", "keyword|phrase|category")
	_ = suggest.MarkFlagRequired("app")

	// --- difficulty ---
	var dCountry string
	difficulty := &cobra.Command{
		Use:   "difficulty <term>",
		Short: "Keyword difficulty computed from the live top-10",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			country := asoCountry(dCountry)
			apps, err := itunes.Search(cmd.Context(), args[0], country, 10)
			if err != nil {
				return err
			}
			d := aso.ComputeDifficulty(args[0], country, apps)
			return render().JSON(d)
		},
	}
	difficulty.Flags().StringVar(&dCountry, "country", "", "storefront (default us)")

	asoCmd.AddCommand(research, popularity, suggest, difficulty,
		newTrackCmd(), newMetadataCmd(), newCompetitorsCmd(), newReviewsCmd())
	rootCmd.AddCommand(asoCmd)
}

func newReviewsCmd() *cobra.Command {
	reviewsCmd := &cobra.Command{Use: "reviews", Short: "App Store customer reviews"}
	var app, country, stars string
	var pages int
	list := &cobra.Command{
		Use:   "list",
		Short: "Recent reviews (RSS feed), optionally filtered by star rating",
		RunE: func(cmd *cobra.Command, args []string) error {
			revs, err := itunes.Reviews(cmd.Context(), app, asoCountry(country), pages)
			if err != nil && len(revs) == 0 {
				return err
			}
			if len(revs) == 0 {
				fmt.Fprintln(os.Stderr, "no reviews returned — Apple's public RSS reviews feed has been returning")
				fmt.Fprintln(os.Stderr, "empty results for many storefronts since 2025. For your own apps, the App")
				fmt.Fprintln(os.Stderr, "Store Connect API exposes reviews reliably (planned: `appadscli asc reviews`).")
			}
			var keep []itunes.Review
			starSet := map[string]bool{}
			for _, s := range strings.Split(stars, ",") {
				if s = strings.TrimSpace(s); s != "" {
					starSet[s] = true
				}
			}
			for _, r := range revs {
				if len(starSet) == 0 || starSet[r.Rating] {
					keep = append(keep, r)
				}
			}
			if render().Format == "json" {
				return render().JSON(keep)
			}
			var rows [][]string
			for _, r := range keep {
				title := r.Title
				if len(title) > 48 {
					title = title[:45] + "..."
				}
				rows = append(rows, []string{r.Rating + "★", title, r.Author, r.Version})
			}
			return render().Rows([]string{"Stars", "Title", "Author", "Version"}, rows, keep)
		},
	}
	list.Flags().StringVar(&app, "app", "", "adamId (required)")
	list.Flags().StringVar(&country, "country", "", "storefront (default us)")
	list.Flags().StringVar(&stars, "stars", "", "filter, e.g. 1,2")
	list.Flags().IntVar(&pages, "pages", 2, "RSS pages to fetch (50 reviews/page)")
	_ = list.MarkFlagRequired("app")
	reviewsCmd.AddCommand(list)
	return reviewsCmd
}

// jsonNumberString tolerates numeric adamIds in JSON output.
func jsonNumberString(v json.RawMessage) string { return strings.Trim(string(v), `"`) }

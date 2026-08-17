package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

func init() {
	appsCmd := &cobra.Command{Use: "apps", Short: "Search the App Store and check ads eligibility"}

	var country string
	var limit int
	search := &cobra.Command{
		Use:   "search <query>",
		Short: "Search apps by name/keyword (Apple Ads app search)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			q := url.Values{"query": {args[0]}, "returnOwnedApps": {"false"}}
			var apps []json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/search/apps?"+q.Encode(), &apps); err != nil {
				return err
			}
			if limit > 0 && len(apps) > limit {
				apps = apps[:limit]
			}
			h, rows := api.Table(apps, []string{
				"AdamId=adamId", "Name=appName", "Developer=developerName", "Countries=countryOrRegionCodes",
			})
			return render().Rows(h, rows, apps)
		},
	}
	search.Flags().StringVar(&country, "country", "", "filter results to a storefront (e.g. us)")
	search.Flags().IntVar(&limit, "limit", 25, "max results")

	get := &cobra.Command{
		Use:   "get <adamId>",
		Short: "App details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var app json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/apps/"+args[0], &app); err != nil {
				return err
			}
			return render().JSON(app)
		},
	}

	eligibility := &cobra.Command{
		Use:   "eligibility <adamId>",
		Short: "Can this app run ads? Why not?",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := api.EqFilter("adamId", args[0])
			items, err := c.Query(cmd.Context(), "/v1/eligibilities/apps/query", sel, 0)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return fmt.Errorf("no eligibility record for adamId %s", args[0])
			}
			h, rows := api.Table(items, []string{
				"AdamId=adamId", "State=state", "Reasons=reasons",
				"SupplySource=supplySource", "MinAge=minimumAge",
			})
			return render().Rows(h, rows, items)
		},
	}

	rejections := &cobra.Command{
		Use:   "rejections [rejectionReasonId]",
		Short: "Creative rejection reasons (all, or one by id)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			if len(args) == 1 {
				var r json.RawMessage
				if err := c.Get(cmd.Context(), "/v1/rejection-reasons/apps/"+args[0], &r); err != nil {
					return err
				}
				return render().JSON(r)
			}
			items, err := c.Query(cmd.Context(), "/v1/rejection-reasons/apps/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "AdamId=adamId", "Reason=reasonType", "Comment=comment", "Level=reasonLevel",
			})
			return render().Rows(h, rows, items)
		},
	}

	// The endpoint answers per storefront, not per app: each row is a country
	// with the languages ads can run in there.
	var langCountry string
	languages := &cobra.Command{
		Use:   "languages",
		Short: "Ad languages/locales supported per storefront",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if langCountry != "" {
				sel = api.EqFilter("countryCode", strings.ToUpper(langCountry))
			}
			items, err := c.Query(cmd.Context(), "/v1/metadata/apps/supported-languages/query", sel, 0)
			if err != nil {
				return err
			}
			return render().JSON(items)
		},
	}
	languages.Flags().StringVar(&langCountry, "country", "", "storefront code, e.g. US (default: all)")

	appsCmd.AddCommand(search, get, eligibility, rejections, languages)
	rootCmd.AddCommand(appsCmd)

	// geo lives at top level per the spec: `appadscli geo search`
	geoCmd := &cobra.Command{Use: "geo", Short: "Geo targeting metadata"}
	var entity string
	geoSearch := &cobra.Command{
		Use:   "search <query>",
		Short: "Search geo targeting locations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			// supplySource is mandatory even though it doesn't narrow the results.
			q := url.Values{"query": {args[0]}, "supplySource": {"APPSTORE_SEARCH_RESULTS"}}
			if entity != "" {
				q.Set("entity", entity)
			}
			var items []json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/search/geo?"+q.Encode(), &items); err != nil {
				return err
			}
			h, rows := api.Table(items, []string{"Id=id", "Entity=entity", "DisplayName=displayName"})
			return render().Rows(h, rows, items)
		},
	}
	geoSearch.Flags().StringVar(&entity, "entity", "", "Country|AdminArea|Locality")
	geoCmd.AddCommand(geoSearch)
	rootCmd.AddCommand(geoCmd)
}

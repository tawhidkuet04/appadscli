package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tawhidjoarder/adastra/internal/api"
)

func init() {
	mapsCmd := &cobra.Command{
		Use:   "maps",
		Short: "Apple Maps ads: brands, locations, location groups, reports",
	}

	// brands
	brandsCmd := &cobra.Command{Use: "brands", Short: "Business brands for Maps campaigns"}
	bList := &cobra.Command{
		Use:   "list",
		Short: "List business brands",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			items, err := c.Query(cmd.Context(), "/v1/business-brands/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{"Id=id", "Name=name", "Status=status", "Categories=businessCategoryIds"})
			return render().Rows(h, rows, items)
		},
	}
	bGet := &cobra.Command{
		Use:   "get <id>",
		Short: "Brand details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var b json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/business-brands/"+args[0], &b); err != nil {
				return err
			}
			return render().JSON(b)
		},
	}
	brandsCmd.AddCommand(bList, bGet)

	// locations
	locationsCmd := &cobra.Command{Use: "locations", Short: "Business locations"}
	var brand string
	lList := &cobra.Command{
		Use:   "list",
		Short: "List locations (optionally for one brand)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if brand != "" {
				sel = api.EqCond("businessBrandId", brand)
			}
			items, err := c.Query(cmd.Context(), "/v1/locations/query", sel, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Name=name", "Address=address", "Brand=businessBrandId",
			})
			return render().Rows(h, rows, items)
		},
	}
	lList.Flags().StringVar(&brand, "brand", "", "business brand id")
	locationsCmd.AddCommand(lList)

	// location groups
	groupsCmd := &cobra.Command{Use: "groups", Short: "Location groups (targeting sets for Maps campaigns)"}
	gList := &cobra.Command{
		Use:   "list",
		Short: "List location groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			items, err := c.Query(cmd.Context(), "/v1/location-groups/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{"Id=id", "Name=name", "Locations=locationIds"})
			return render().Rows(h, rows, items)
		},
	}
	var gName, gLocations string
	gCreate := &cobra.Command{
		Use:   "create",
		Short: "Create a location group",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			locs := strings.Split(gLocations, ",")
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create location group %q with %d location(s)", gName, len(locs)))
			if err != nil || !ok {
				return err
			}
			body := map[string]any{"name": gName, "locationIds": locs}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/location-groups", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	gCreate.Flags().StringVar(&gName, "name", "", "group name (required)")
	gCreate.Flags().StringVar(&gLocations, "locations", "", "comma-separated location ids (required)")
	_ = gCreate.MarkFlagRequired("name")
	_ = gCreate.MarkFlagRequired("locations")
	addMutationFlags(gCreate)
	groupsCmd.AddCommand(gList, gCreate)

	// categories
	categoriesCmd := &cobra.Command{Use: "categories", Short: "Business categories"}
	cList := &cobra.Command{
		Use:   "list",
		Short: "List business categories",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			items, err := c.Query(cmd.Context(), "/v1/business-categories/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{"Id=id", "Name=name"})
			return render().Rows(h, rows, items)
		},
	}
	categoriesCmd.AddCommand(cList)

	// reports (brand campaigns)
	reportsCmd := &cobra.Command{Use: "reports", Short: "Maps (business brand) performance reports"}
	for _, kind := range []struct{ name, path string }{
		{"campaigns", "/v1/reports/business-brands/campaigns/query"},
		{"adgroups", "/v1/reports/business-brands/adgroups/query"},
		{"keywords", "/v1/reports/business-brands/keywords/query"},
		{"searchterms", "/v1/reports/business-brands/searchterms/query"},
	} {
		kind := kind
		var since, granularity string
		sub := &cobra.Command{
			Use:   kind.name,
			Short: "Maps " + kind.name + " report",
			RunE: func(cmd *cobra.Command, args []string) error {
				c := client()
				if err := c.RequireAccount(); err != nil {
					return err
				}
				req, err := api.NewReportRequest(since, granularity)
				if err != nil {
					return err
				}
				rows, err := c.RunReport(cmd.Context(), kind.path, req)
				if err != nil {
					return err
				}
				return render().JSON(rows)
			},
		}
		sub.Flags().StringVar(&since, "since", "30d", "window start")
		sub.Flags().StringVar(&granularity, "granularity", "", "hourly|daily|weekly|monthly")
		reportsCmd.AddCommand(sub)
	}

	mapsCmd.AddCommand(brandsCmd, locationsCmd, groupsCmd, categoriesCmd, reportsCmd)
	rootCmd.AddCommand(mapsCmd)
}

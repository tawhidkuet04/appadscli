package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/appadscli/appadscli/internal/api"
)

func init() {
	// creatives
	creativesCmd := &cobra.Command{Use: "creatives", Short: "Manage creatives"}
	cList := &cobra.Command{
		Use:   "list",
		Short: "List creatives",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			items, err := c.Query(cmd.Context(), "/v1/creatives/query", &api.Selector{}, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Name=name", "Type=creativeType", "Status=systemStatus",
				"AdamId=destination.parameters.adamId", "ProductPageId=destination.parameters.productPageId",
			})
			return render().Rows(h, rows, items)
		},
	}
	cGet := &cobra.Command{
		Use:   "get <id>",
		Short: "Creative details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var out json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/creatives/"+args[0], &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	var crName, crAdam, crCPP string
	cCreate := &cobra.Command{
		Use:   "create",
		Short: "Create a custom-product-page creative",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create creative %q for app %s (CPP %s)", crName, crAdam, crCPP))
			if err != nil || !ok {
				return err
			}
			body := map[string]any{
				"name": crName, "adamId": json.Number(crAdam),
				"type": "CUSTOM_PRODUCT_PAGE", "productPageId": crCPP,
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/creatives", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	cCreate.Flags().StringVar(&crName, "name", "", "creative name (required)")
	cCreate.Flags().StringVar(&crAdam, "app", "", "adamId (required)")
	cCreate.Flags().StringVar(&crCPP, "cpp", "", "custom product page id (required)")
	_ = cCreate.MarkFlagRequired("name")
	_ = cCreate.MarkFlagRequired("app")
	_ = cCreate.MarkFlagRequired("cpp")
	addMutationFlags(cCreate)

	cDelete := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a creative",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, "DELETE creative "+args[0])
			if err != nil || !ok {
				return err
			}
			if err := c.Delete(cmd.Context(), "/v1/creatives/"+args[0]); err != nil {
				return err
			}
			fmt.Println("deleted", args[0])
			return nil
		},
	}
	addMutationFlags(cDelete)
	creativesCmd.AddCommand(cList, cGet, cCreate, cDelete)
	rootCmd.AddCommand(creativesCmd)

	// ads
	adsCmd := &cobra.Command{Use: "ads", Short: "Manage ads (creative ↔ ad group links)"}
	var adGroup string
	aList := &cobra.Command{
		Use:   "list",
		Short: "List ads",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if adGroup != "" {
				sel = api.EqFilter("adGroupId", adGroup)
			}
			items, err := c.Query(cmd.Context(), "/v1/ads/query", sel, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Name=name", "Status=status", "DisplayStatus=displayStatus",
				"CreativeId=creativeId", "AdGroupId=adGroupId",
			})
			return render().Rows(h, rows, items)
		},
	}
	aList.Flags().StringVar(&adGroup, "adgroup", "", "ad group id")

	var adName, adCreative, adAdGroup string
	aCreate := &cobra.Command{
		Use:   "create",
		Short: "Create an ad from a creative in an ad group",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("create ad %q in ad group %s", adName, adAdGroup))
			if err != nil || !ok {
				return err
			}
			body := map[string]any{
				"name": adName, "creativeId": json.Number(adCreative), "adGroupId": json.Number(adAdGroup),
				"status": "ENABLED",
			}
			var out json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/ads", body, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	aCreate.Flags().StringVar(&adName, "name", "", "ad name (required)")
	aCreate.Flags().StringVar(&adCreative, "creative", "", "creative id (required)")
	aCreate.Flags().StringVar(&adAdGroup, "adgroup", "", "ad group id (required)")
	_ = aCreate.MarkFlagRequired("name")
	_ = aCreate.MarkFlagRequired("creative")
	_ = aCreate.MarkFlagRequired("adgroup")
	addMutationFlags(aCreate)

	aPause := &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause an ad",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			ok, err := confirmOrAbort(cmd, "pause ad "+args[0])
			if err != nil || !ok {
				return err
			}
			var out json.RawMessage
			if err := c.Put(cmd.Context(), "/v1/ads/"+args[0], map[string]string{"status": "PAUSED"}, &out); err != nil {
				return err
			}
			return render().JSON(out)
		},
	}
	addMutationFlags(aPause)
	adsCmd.AddCommand(aList, aCreate, aPause)
	rootCmd.AddCommand(adsCmd)

	// cpp
	cppCmd := &cobra.Command{Use: "cpp", Short: "Custom product pages and CPP A/B tests"}
	var cppApp string
	cppList := &cobra.Command{
		Use:   "list",
		Short: "Custom product pages available for an app",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			sel := &api.Selector{}
			if cppApp != "" {
				sel = api.EqFilter("adamId", cppApp)
			}
			items, err := c.Query(cmd.Context(), "/v1/product-pages/query", sel, 0)
			if err != nil {
				return err
			}
			h, rows := api.Table(items, []string{
				"Id=id", "Name=name", "State=state", "AdamId=adamId",
			})
			return render().Rows(h, rows, items)
		},
	}
	cppList.Flags().StringVar(&cppApp, "app", "", "adamId")

	testCmd := &cobra.Command{Use: "test", Short: "CPP A/B testing via parallel ads"}
	var tAdGroup, tCPP, tName string
	tStart := &cobra.Command{
		Use:   "start",
		Short: "Run a CPP against the default page in one ad group (two ads, rotate evenly)",
		Long: `Creates a creative for the CPP and an ad alongside the ad group's default
ad. Apple rotates serving; compare with ` + "`appadscli cpp test report`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			var ag json.RawMessage
			if err := c.Get(cmd.Context(), "/v1/adgroups/"+tAdGroup, &ag); err != nil {
				return err
			}
			adam := api.Field(ag, "orgSharedVoc.adamId")
			if adam == "" {
				adam = api.Field(ag, "adamId")
			}
			ok, err := confirmOrAbort(cmd, fmt.Sprintf("start CPP test in ad group %s with product page %s", tAdGroup, tCPP))
			if err != nil || !ok {
				return err
			}
			crBody := map[string]any{
				"name": tName + " (creative)", "adamId": json.Number(adam),
				"type": "CUSTOM_PRODUCT_PAGE", "productPageId": tCPP,
			}
			var creative json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/creatives", crBody, &creative); err != nil {
				return err
			}
			adBody := map[string]any{
				"name": tName, "creativeId": json.Number(api.Field(creative, "id")),
				"adGroupId": json.Number(tAdGroup), "status": "ENABLED",
			}
			var ad json.RawMessage
			if err := c.Post(cmd.Context(), "/v1/ads", adBody, &ad); err != nil {
				return err
			}
			return render().JSON(map[string]any{"creative": creative, "ad": ad,
				"next": "compare with `appadscli cpp test report --adgroup " + tAdGroup + "`"})
		},
	}
	tStart.Flags().StringVar(&tAdGroup, "adgroup", "", "ad group id (required)")
	tStart.Flags().StringVar(&tCPP, "cpp", "", "custom product page id (required)")
	tStart.Flags().StringVar(&tName, "name", "cpp-test", "test ad name")
	_ = tStart.MarkFlagRequired("adgroup")
	_ = tStart.MarkFlagRequired("cpp")
	addMutationFlags(tStart)

	var rAdGroup, rSince string
	tReport := &cobra.Command{
		Use:   "report",
		Short: "Per-ad performance in an ad group (CPP vs default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			req, err := api.NewReportRequest(rSince, "")
			if err != nil {
				return err
			}
			if rAdGroup != "" {
				req.Filters = []api.Filter{{Field: "adGroupId", Operator: "EQUALS", Value: rAdGroup}}
			}
			rows, err := c.RunReport(cmd.Context(), "/v1/reports/apps/ads/query", req)
			if err != nil {
				return err
			}
			h, tbl := api.Table(rows, append([]string{"AdId=id", "Ad=name"}, reportMetricCols...))
			return render().Rows(h, tbl, rows)
		},
	}
	tReport.Flags().StringVar(&rAdGroup, "adgroup", "", "ad group id")
	tReport.Flags().StringVar(&rSince, "since", "14d", "window start")
	testCmd.AddCommand(tStart, tReport)

	cppCmd.AddCommand(cppList, testCmd)
	rootCmd.AddCommand(cppCmd)
}

package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/api"
	"github.com/tawhidkuet04/adastra/internal/aso"
	"github.com/tawhidkuet04/adastra/internal/itunes"
	"github.com/tawhidkuet04/adastra/internal/store"
)

func newMetadataCmd() *cobra.Command {
	metaCmd := &cobra.Command{Use: "metadata", Short: "Metadata audits and keyword generation"}

	var aApp, aCountry string
	audit := &cobra.Command{
		Use:   "audit",
		Short: "Char budgets, duplication, title/subtitle overlap",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := itunes.Lookup(cmd.Context(), aApp, asoCountry(aCountry))
			if err != nil {
				return err
			}
			issues := aso.AuditMetadata(app)
			out := map[string]any{
				"adamId":        app.AdamID,
				"name":          app.Name,
				"subtitle":      app.Subtitle,
				"titleChars":    len([]rune(app.Name)),
				"subtitleChars": len([]rune(app.Subtitle)),
				"issues":        issues,
			}
			if render().Format == "json" {
				return render().JSON(out)
			}
			fmt.Printf("%s — metadata audit (%s)\n", app.Name, asoCountry(aCountry))
			fmt.Printf("title (%d/30):    %s\nsubtitle (%d/30): %s\n\n",
				len([]rune(app.Name)), app.Name, len([]rune(app.Subtitle)), app.Subtitle)
			var rows [][]string
			for _, i := range issues {
				rows = append(rows, []string{i.Severity, i.Field, i.Message})
			}
			return render().Rows([]string{"Severity", "Field", "Message"}, rows, issues)
		},
	}
	audit.Flags().StringVar(&aApp, "app", "", "adamId (required)")
	audit.Flags().StringVar(&aCountry, "country", "", "storefront (default us)")
	_ = audit.MarkFlagRequired("app")

	var gApp, gSince string
	var gMinInstalls int
	generate := &cobra.Command{
		Use:   "generate",
		Short: "Build a keyword-field draft from your converting paid search terms",
		Long: `The paid↔organic bridge: pulls your search terms report, keeps terms that
actually converted (≥ --min-installs), drops words already in your visible
title/subtitle, and packs the rest into a 100-character keyword field draft,
best converters first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client()
			if err := c.RequireAccount(); err != nil {
				return err
			}
			req, err := api.NewReportRequest(gSince, "")
			if err != nil {
				return err
			}
			rows, err := c.RunReport(cmd.Context(), "/v1/reports/apps/searchterms/query", req)
			if err != nil {
				return err
			}
			type termStat struct {
				term     string
				installs float64
			}
			var stats []termStat
			for _, r := range rows {
				installs := api.FloatField(r, "totalInstalls")
				term := api.Field(r, "searchTermText")
				if term == "" || installs < float64(gMinInstalls) {
					continue
				}
				stats = append(stats, termStat{term, installs})
			}
			sort.Slice(stats, func(i, j int) bool { return stats[i].installs > stats[j].installs })

			// Words already indexed by visible metadata are wasted in the field.
			used := map[string]bool{}
			if gApp != "" {
				if app, err := itunes.Lookup(cmd.Context(), gApp, "us"); err == nil {
					for _, t := range aso.Tokens(app.Name + " " + app.Subtitle) {
						used[t] = true
					}
				}
			}
			seen := map[string]bool{}
			var words []string
			total := 0
			for _, s := range stats {
				for _, w := range aso.Tokens(s.term) {
					if used[w] || seen[w] {
						continue
					}
					cost := len(w)
					if len(words) > 0 {
						cost++ // comma
					}
					if total+cost > 100 {
						continue
					}
					seen[w] = true
					words = append(words, w)
					total += cost
				}
			}
			out := map[string]any{
				"keywordField": strings.Join(words, ","),
				"chars":        total,
				"sourceTerms":  len(stats),
				"minInstalls":  gMinInstalls,
				"note":         "review before pasting into App Store Connect — the field is not validated for trademark/relevance",
			}
			return render().JSON(out)
		},
	}
	generate.Flags().StringVar(&gApp, "app", "", "adamId (used to exclude words already in your title/subtitle)")
	generate.Flags().StringVar(&gSince, "since", "90d", "search terms window")
	generate.Flags().IntVar(&gMinInstalls, "min-installs", 2, "only use terms with at least this many installs")

	metaCmd.AddCommand(audit, generate)
	return metaCmd
}

func newCompetitorsCmd() *cobra.Command {
	compCmd := &cobra.Command{Use: "competitors", Short: "Competitor metadata intel: gap analysis and change watch"}

	var gapApp, gapCountry string
	gap := &cobra.Command{
		Use:   "gap --app <adamId> --vs <adamId,adamId,...>",
		Short: "Terms competitors use in visible metadata that you don't",
		RunE: func(cmd *cobra.Command, args []string) error {
			vs, _ := cmd.Flags().GetString("vs")
			if vs == "" {
				return fmt.Errorf("pass --vs with at least one competitor adamId")
			}
			country := asoCountry(gapCountry)
			mine, err := itunes.Lookup(cmd.Context(), gapApp, country)
			if err != nil {
				return err
			}
			var comps []*itunes.App
			for _, id := range strings.Split(vs, ",") {
				a, err := itunes.Lookup(cmd.Context(), strings.TrimSpace(id), country)
				if err != nil {
					fmt.Printf("⚠ %s: %v\n", id, err)
					continue
				}
				comps = append(comps, a)
			}
			gaps := aso.CompetitorGap(mine, comps)
			if render().Format == "json" {
				return render().JSON(gaps)
			}
			var rows [][]string
			for _, g := range gaps {
				rows = append(rows, []string{g.Term, fmt.Sprint(g.UsedCount), strings.Join(g.UsedBy, ", ")})
			}
			return render().Rows([]string{"Term", "Count", "UsedBy"}, rows, gaps)
		},
	}
	gap.Flags().StringVar(&gapApp, "app", "", "your adamId (required)")
	gap.Flags().String("vs", "", "comma-separated competitor adamIds (required)")
	gap.Flags().StringVar(&gapCountry, "country", "", "storefront (default us)")
	_ = gap.MarkFlagRequired("app")

	watchCmd := &cobra.Command{Use: "watch", Short: "Snapshot competitor metadata; diff on change"}
	var wCountry string
	wAdd := &cobra.Command{
		Use:   "add <adamId>",
		Short: "Watch a competitor app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.WatchCompetitor(args[0], asoCountry(wCountry)); err != nil {
				return err
			}
			fmt.Println("✓ watching", args[0], "— run `adastra aso competitors watch run` to snapshot & diff")
			return nil
		},
	}
	wAdd.Flags().StringVar(&wCountry, "country", "", "storefront (default us)")

	wRun := &cobra.Command{
		Use:   "run",
		Short: "Snapshot all watched competitors and report changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			watched, err := st.WatchedCompetitors()
			if err != nil {
				return err
			}
			if len(watched) == 0 {
				return fmt.Errorf("no competitors watched — `adastra aso competitors watch add <adamId>` first")
			}
			type change struct {
				AdamID, Field, Old, New string
			}
			var changes []change
			for _, w := range watched {
				app, err := itunes.Lookup(cmd.Context(), w.AdamID, w.Country)
				if err != nil {
					fmt.Printf("⚠ %s: %v\n", w.AdamID, err)
					continue
				}
				snap := map[string]string{
					"name": app.Name, "subtitle": app.Subtitle, "version": app.Version,
					"releaseNotes": app.ReleaseNotes,
				}
				prev, err := st.LastCompetitorSnapshot(w.AdamID, w.Country)
				if err != nil {
					return err
				}
				if prev != "" {
					var old map[string]string
					if json.Unmarshal([]byte(prev), &old) == nil {
						for k, newVal := range snap {
							if oldVal := old[k]; oldVal != newVal {
								changes = append(changes, change{w.AdamID, k, truncate(oldVal, 40), truncate(newVal, 40)})
							}
						}
					}
				}
				if err := st.SnapshotCompetitor(w.AdamID, w.Country, snap); err != nil {
					return err
				}
			}
			if len(changes) == 0 {
				fmt.Printf("✓ %d competitor(s) snapshotted — no changes since last run at %s\n",
					len(watched), time.Now().Format("15:04"))
				return nil
			}
			var rows [][]string
			for _, ch := range changes {
				rows = append(rows, []string{ch.AdamID, ch.Field, ch.Old, ch.New})
			}
			return render().Rows([]string{"App", "Field", "Old", "New"}, rows, changes)
		},
	}
	watchCmd.AddCommand(wAdd, wRun)

	compCmd.AddCommand(gap, watchCmd)
	return compCmd
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

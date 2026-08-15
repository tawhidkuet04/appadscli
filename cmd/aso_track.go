package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/asacli/internal/api"
	"github.com/tawhidkuet04/asacli/internal/itunes"
	"github.com/tawhidkuet04/asacli/internal/store"
)

func newTrackCmd() *cobra.Command {
	trackCmd := &cobra.Command{Use: "track", Short: "Organic rank tracking with local history (cron/CI-friendly)"}

	var app, kwFile, countries, terms string
	add := &cobra.Command{
		Use:   "add",
		Short: "Register keywords to track for an app",
		RunE: func(cmd *cobra.Command, args []string) error {
			var kws []string
			if terms != "" {
				kws = strings.Split(terms, ",")
			}
			if kwFile != "" {
				fromFile, err := readLines(kwFile)
				if err != nil {
					return err
				}
				kws = append(kws, fromFile...)
			}
			if len(kws) == 0 {
				return fmt.Errorf("no keywords — pass --keywords <file> or --terms a,b,c")
			}
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			n := 0
			for _, country := range strings.Split(countries, ",") {
				for _, kw := range kws {
					if err := st.TrackKeyword(app, strings.TrimSpace(kw), strings.TrimSpace(country)); err != nil {
						return err
					}
					n++
				}
			}
			fmt.Printf("✓ tracking %d keyword/country pairs for app %s\n", n, app)
			fmt.Println("run `asacli aso track run` (e.g. from cron) to snapshot ranks")
			return nil
		},
	}
	add.Flags().StringVar(&app, "app", "", "adamId (required)")
	add.Flags().StringVar(&kwFile, "keywords", "", "newline-separated keywords file")
	add.Flags().StringVar(&terms, "terms", "", "comma-separated keywords")
	add.Flags().StringVar(&countries, "countries", "us", "comma-separated storefronts")
	_ = add.MarkFlagRequired("app")

	var depth int
	run := &cobra.Command{
		Use:   "run",
		Short: "Snapshot current ranks for everything tracked",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			tracked, err := st.TrackedKeywords()
			if err != nil {
				return err
			}
			if len(tracked) == 0 {
				return fmt.Errorf("nothing tracked — run `asacli aso track add` first")
			}
			now := time.Now()
			var rows [][]string
			for _, t := range tracked {
				rank, err := itunes.Rank(cmd.Context(), t.AdamID, t.Keyword, t.Country, depth)
				if err != nil {
					fmt.Printf("⚠ %s [%s]: %v\n", t.Keyword, t.Country, err)
					continue
				}
				if err := st.RecordRank(t.AdamID, t.Keyword, t.Country, rank, now); err != nil {
					return err
				}
				disp := fmt.Sprint(rank)
				if rank == 0 {
					disp = fmt.Sprintf(">%d", depth)
				}
				rows = append(rows, []string{t.AdamID, t.Keyword, t.Country, disp})
			}
			return render().Rows([]string{"App", "Keyword", "Country", "Rank"}, rows, nil)
		},
	}
	run.Flags().IntVar(&depth, "depth", 100, "how deep to search for your app")

	var since string
	report := &cobra.Command{
		Use:   "report",
		Short: "Rank history: current, best, delta over the window",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			tracked, err := st.TrackedKeywords()
			if err != nil {
				return err
			}
			start, err := api.ParseSince(since, time.Now())
			if err != nil {
				return err
			}
			type entry struct {
				App, Keyword, Country       string
				Current, Best, First, Delta int
				Checks                      int
			}
			var out []entry
			for _, t := range tracked {
				hist, err := st.RankHistory(t.AdamID, t.Keyword, t.Country, start)
				if err != nil {
					return err
				}
				if len(hist) == 0 {
					continue
				}
				e := entry{App: t.AdamID, Keyword: t.Keyword, Country: t.Country, Checks: len(hist)}
				e.First = hist[0].Rank
				e.Current = hist[len(hist)-1].Rank
				e.Best = 9999
				for _, p := range hist {
					if p.Rank > 0 && p.Rank < e.Best {
						e.Best = p.Rank
					}
				}
				if e.Best == 9999 {
					e.Best = 0
				}
				if e.First > 0 && e.Current > 0 {
					e.Delta = e.First - e.Current // positive = improved
				}
				out = append(out, e)
			}
			if render().Format == "json" {
				return render().JSON(out)
			}
			var rows [][]string
			for _, e := range out {
				cur := fmt.Sprint(e.Current)
				if e.Current == 0 {
					cur = "unranked"
				}
				delta := fmt.Sprintf("%+d", e.Delta)
				if e.Delta == 0 {
					delta = "="
				}
				rows = append(rows, []string{e.Keyword, e.Country, cur, fmt.Sprint(e.Best), delta, fmt.Sprint(e.Checks)})
			}
			return render().Rows([]string{"Keyword", "Country", "Current", "Best", "Δ", "Checks"}, rows, out)
		},
	}
	report.Flags().StringVar(&since, "since", "30d", "window start")

	var drop int
	alerts := &cobra.Command{
		Use:   "alerts",
		Short: "Exit non-zero if any keyword dropped ≥ --drop positions (CI-friendly)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open()
			if err != nil {
				return err
			}
			defer st.Close()
			tracked, err := st.TrackedKeywords()
			if err != nil {
				return err
			}
			start := time.Now().AddDate(0, 0, -30)
			bad := 0
			for _, t := range tracked {
				hist, err := st.RankHistory(t.AdamID, t.Keyword, t.Country, start)
				if err != nil {
					return err
				}
				if len(hist) < 2 {
					continue
				}
				prev, cur := hist[len(hist)-2].Rank, hist[len(hist)-1].Rank
				dropped := 0
				switch {
				case prev > 0 && cur == 0:
					dropped = drop // fell out entirely counts as a full breach
				case prev > 0 && cur > prev:
					dropped = cur - prev
				}
				if dropped >= drop {
					bad++
					curDisp := fmt.Sprint(cur)
					if cur == 0 {
						curDisp = "unranked"
					}
					fmt.Printf("DROP %s [%s]: %d → %s\n", t.Keyword, t.Country, prev, curDisp)
				}
			}
			if bad > 0 {
				return fmt.Errorf("%d keyword(s) dropped ≥%d positions", bad, drop)
			}
			fmt.Println("✓ no rank drops beyond threshold")
			return nil
		},
	}
	alerts.Flags().IntVar(&drop, "drop", 5, "alert threshold in positions")

	trackCmd.AddCommand(add, run, report, alerts)
	return trackCmd
}

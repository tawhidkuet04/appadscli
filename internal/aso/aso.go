// Package aso computes ASO intelligence from organic data: keyword
// difficulty, metadata audits, and competitor gap analysis.
package aso

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/tawhidkuet04/adastra/internal/itunes"
)

// Difficulty scores how hard it is to rank organically for a term, from the
// live top-10: install-base proxy (rating counts), rating quality, and how
// many of the top apps use the term in their visible metadata.
// Scale 1 (easy) – 10 (brutal).
type Difficulty struct {
	Term            string       `json:"term"`
	Country         string       `json:"country"`
	Score           float64      `json:"score"` // 1..10
	Top10AvgRatings float64      `json:"top10AvgRatingCount"`
	Top10AvgStars   float64      `json:"top10AvgStars"`
	TitleMatches    int          `json:"titleMatches"` // top-10 apps with term in title/subtitle
	Top             []AppBrief   `json:"top10"`
	Apps            []itunes.App `json:"-"` // full objects for internal reuse
}

// AppBrief is the compact projection of a ranked app.
type AppBrief struct {
	Rank        int     `json:"rank"`
	AdamID      int64   `json:"adamId"`
	Name        string  `json:"name"`
	RatingCount int64   `json:"ratingCount"`
	Stars       float64 `json:"stars"`
}

// ComputeDifficulty scores a term from its top-10 organic results.
func ComputeDifficulty(term, country string, apps []itunes.App) Difficulty {
	d := Difficulty{Term: term, Country: country}
	n := len(apps)
	if n > 10 {
		apps = apps[:10]
		n = 10
	}
	d.Apps = apps
	for i, a := range apps {
		d.Top = append(d.Top, AppBrief{Rank: i + 1, AdamID: a.AdamID, Name: a.Name,
			RatingCount: a.RatingCount, Stars: a.Rating})
	}
	if n == 0 {
		d.Score = 1
		return d
	}
	var ratings, stars float64
	lower := strings.ToLower(term)
	for _, a := range apps {
		ratings += float64(a.RatingCount)
		stars += a.Rating
		hay := strings.ToLower(a.Name + " " + a.Subtitle)
		if strings.Contains(hay, lower) {
			d.TitleMatches++
		}
	}
	d.Top10AvgRatings = ratings / float64(n)
	d.Top10AvgStars = stars / float64(n)

	// log-scaled install-base proxy: 100 ratings ≈ 2, 10k ≈ 5.3, 1M ≈ 8.6
	base := math.Log10(d.Top10AvgRatings+1) * 10 / 7
	metaPressure := float64(d.TitleMatches) / float64(n) * 2 // up to +2 when all use the term
	score := base + metaPressure
	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}
	d.Score = math.Round(score*10) / 10
	return d
}

// AuditIssue is one metadata audit finding.
type AuditIssue struct {
	Severity string `json:"severity"` // error|warn|info
	Field    string `json:"field"`
	Message  string `json:"message"`
}

// AuditMetadata checks character budgets and duplication for the metadata
// visible via the iTunes API (title, subtitle, description).
func AuditMetadata(app *itunes.App) []AuditIssue {
	var issues []AuditIssue
	add := func(sev, field, msg string) { issues = append(issues, AuditIssue{sev, field, msg}) }

	title, subtitle := app.Name, app.Subtitle
	if l := len([]rune(title)); l > 30 {
		add("error", "title", "title exceeds the 30-character limit — App Store truncates it")
	} else if l < 15 {
		add("info", "title", "title uses under half the 30-character budget — consider a keyword-bearing suffix")
	}
	if subtitle == "" {
		add("warn", "subtitle", "no subtitle — 30 free characters of indexed keywords unused")
	} else if l := len([]rune(subtitle)); l > 30 {
		add("error", "subtitle", "subtitle exceeds the 30-character limit")
	}

	tTok := Tokens(title)
	sTok := Tokens(subtitle)
	dup := intersect(tTok, sTok)
	if len(dup) > 0 {
		add("warn", "subtitle", "duplicates title words (wasted indexed characters): "+strings.Join(dup, ", "))
	}
	if len(app.Description) < 400 {
		add("info", "description", "description is short; it doesn't index for search but drives conversion")
	}
	if app.RatingCount > 0 && app.Rating < 4.0 {
		add("warn", "rating", "average rating below 4.0 suppresses conversion and featuring")
	}
	if len(issues) == 0 {
		add("info", "all", "no issues found in publicly visible metadata")
	}
	return issues
}

var tokenRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// stopwords excluded from token comparisons.
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true,
	"for": true, "to": true, "in": true, "on": true, "with": true, "your": true,
	"by": true, "app": true, "apps": true, "my": true, "is": true, "at": true,
}

// Tokens lower-cases and tokenizes visible metadata, dropping stopwords.
func Tokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tokenRe.FindAllString(strings.ToLower(s), -1) {
		if stopwords[t] || len(t) < 2 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	var out []string
	for _, y := range b {
		if set[y] {
			out = append(out, y)
		}
	}
	return out
}

// GapTerm is a term competitors use in visible metadata that you don't.
type GapTerm struct {
	Term      string   `json:"term"`
	UsedBy    []string `json:"usedBy"` // competitor app names
	UsedCount int      `json:"usedCount"`
}

// CompetitorGap returns terms present in competitors' title/subtitle but
// absent from yours, ranked by how many competitors use them.
func CompetitorGap(mine *itunes.App, competitors []*itunes.App) []GapTerm {
	mineSet := map[string]bool{}
	for _, t := range Tokens(mine.Name + " " + mine.Subtitle) {
		mineSet[t] = true
	}
	counts := map[string][]string{}
	for _, c := range competitors {
		for _, t := range Tokens(c.Name + " " + c.Subtitle) {
			if !mineSet[t] {
				counts[t] = append(counts[t], c.Name)
			}
		}
	}
	var out []GapTerm
	for term, users := range counts {
		out = append(out, GapTerm{Term: term, UsedBy: users, UsedCount: len(users)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UsedCount != out[j].UsedCount {
			return out[i].UsedCount > out[j].UsedCount
		}
		return out[i].Term < out[j].Term
	})
	return out
}

// CandidateTerms extracts unique candidate keywords from a result set's
// visible metadata (for research --expand).
func CandidateTerms(apps []itunes.App, exclude string) []string {
	ex := map[string]bool{}
	for _, t := range Tokens(exclude) {
		ex[t] = true
	}
	counts := map[string]int{}
	for _, a := range apps {
		for _, t := range Tokens(a.Name + " " + a.Subtitle) {
			if !ex[t] {
				counts[t]++
			}
		}
	}
	type tc struct {
		t string
		c int
	}
	var all []tc
	for t, c := range counts {
		if c >= 2 { // used by at least 2 apps → likely a category term
			all = append(all, tc{t, c})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].c > all[j].c })
	var out []string
	for _, x := range all {
		out = append(out, x.t)
	}
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

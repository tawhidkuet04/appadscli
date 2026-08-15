// Package itunes wraps the public iTunes Search API and App Store RSS feeds
// for organic data: rankings, app metadata, and reviews. Unofficial for this
// purpose, so: gentle pacing (1 req/sec), aggressive on-disk caching (1h TTL),
// and a descriptive User-Agent.
package itunes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tawhidjoarder/adastra/internal/config"
)

const userAgent = "adastra-cli (+https://github.com/tawhidjoarder/adastra)"

var (
	paceMu   sync.Mutex
	lastCall time.Time
)

// App is the subset of iTunes Search API fields adastra uses.
type App struct {
	AdamID       int64    `json:"trackId"`
	Name         string   `json:"trackName"`
	BundleID     string   `json:"bundleId"`
	Developer    string   `json:"artistName"`
	Subtitle     string   `json:"subtitle,omitempty"`
	Description  string   `json:"description"`
	Genres       []string `json:"genres"`
	Rating       float64  `json:"averageUserRating"`
	RatingCount  int64    `json:"userRatingCount"`
	Price        float64  `json:"price"`
	ReleaseNotes string   `json:"releaseNotes"`
	Version      string   `json:"version"`
	URL          string   `json:"trackViewUrl"`
}

func fetch(ctx context.Context, u string, ttl time.Duration) ([]byte, error) {
	// cache
	dir, err := config.Dir()
	if err == nil {
		cacheDir := filepath.Join(dir, "cache")
		_ = os.MkdirAll(cacheDir, 0o700)
		sum := sha256.Sum256([]byte(u))
		cf := filepath.Join(cacheDir, hex.EncodeToString(sum[:16])+".json")
		if fi, err := os.Stat(cf); err == nil && time.Since(fi.ModTime()) < ttl {
			if b, err := os.ReadFile(cf); err == nil {
				return b, nil
			}
		}
		defer func() {
			// best-effort write-through happens below via closure variable
		}()
		b, err := fetchLive(ctx, u)
		if err != nil {
			return nil, err
		}
		_ = os.WriteFile(cf, b, 0o600)
		return b, nil
	}
	return fetchLive(ctx, u)
}

func fetchLive(ctx context.Context, u string) ([]byte, error) {
	// pace: ≥1s between live calls
	paceMu.Lock()
	if d := time.Second - time.Since(lastCall); d > 0 {
		time.Sleep(d)
	}
	lastCall = time.Now()
	paceMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("itunes api: HTTP %d for %s", resp.StatusCode, u)
	}
	return b, nil
}

// Search returns App Store search results for a term — i.e. the organic
// ranking order for that keyword on that storefront.
func Search(ctx context.Context, term, country string, limit int) ([]App, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := url.Values{
		"term":    {term},
		"country": {strings.ToLower(country)},
		"entity":  {"software"},
		"limit":   {fmt.Sprint(limit)},
	}
	b, err := fetch(ctx, "https://itunes.apple.com/search?"+q.Encode(), time.Hour)
	if err != nil {
		return nil, err
	}
	var out struct {
		Results []App `json:"results"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// Lookup fetches one app's metadata by adamId.
func Lookup(ctx context.Context, adamID, country string) (*App, error) {
	q := url.Values{"id": {adamID}, "country": {strings.ToLower(country)}, "entity": {"software"}}
	b, err := fetch(ctx, "https://itunes.apple.com/lookup?"+q.Encode(), 30*time.Minute)
	if err != nil {
		return nil, err
	}
	var out struct {
		Results []App `json:"results"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if len(out.Results) == 0 {
		return nil, fmt.Errorf("no app found for adamId %s in %s", adamID, country)
	}
	return &out.Results[0], nil
}

// Rank returns the 1-based position of adamID in results for term, 0 if absent.
func Rank(ctx context.Context, adamID, term, country string, depth int) (int, error) {
	apps, err := Search(ctx, term, country, depth)
	if err != nil {
		return 0, err
	}
	for i, a := range apps {
		if fmt.Sprint(a.AdamID) == adamID {
			return i + 1, nil
		}
	}
	return 0, nil
}

// Review is one customer review from the RSS feed.
type Review struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Rating  string `json:"rating"`
	Author  string `json:"author"`
	Version string `json:"version"`
}

// Reviews fetches recent customer reviews (RSS feed, newest first).
func Reviews(ctx context.Context, adamID, country string, pages int) ([]Review, error) {
	if pages <= 0 {
		pages = 1
	}
	var out []Review
	for page := 1; page <= pages; page++ {
		u := fmt.Sprintf("https://itunes.apple.com/%s/rss/customerreviews/page=%d/id=%s/sortby=mostrecent/json",
			strings.ToLower(country), page, adamID)
		b, err := fetch(ctx, u, time.Hour)
		if err != nil {
			return out, err
		}
		var feed struct {
			Feed struct {
				Entry []struct {
					ID struct {
						Label string `json:"label"`
					} `json:"id"`
					Title struct {
						Label string `json:"label"`
					} `json:"title"`
					Content struct {
						Label string `json:"label"`
					} `json:"content"`
					Rating struct {
						Label string `json:"label"`
					} `json:"im:rating"`
					Author struct {
						Name struct {
							Label string `json:"label"`
						} `json:"name"`
					} `json:"author"`
					Version struct {
						Label string `json:"label"`
					} `json:"im:version"`
				} `json:"entry"`
			} `json:"feed"`
		}
		if err := json.Unmarshal(b, &feed); err != nil {
			return out, nil // feed ends / shape varies — return what we have
		}
		if len(feed.Feed.Entry) == 0 {
			break
		}
		for _, e := range feed.Feed.Entry {
			out = append(out, Review{
				ID: e.ID.Label, Title: e.Title.Label, Content: e.Content.Label,
				Rating: e.Rating.Label, Author: e.Author.Name.Label, Version: e.Version.Label,
			})
		}
	}
	return out, nil
}

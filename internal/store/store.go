// Package store is asacli's local state: rank history, tracked keywords,
// competitor snapshots, harvest memory, and the mutation audit log.
// SQLite (pure-Go driver, no CGO) in ~/.asacli/asacli.db.
package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tawhidkuet04/asacli/internal/config"
)

// Store wraps the SQLite database.
type Store struct{ DB *sql.DB }

// Open opens (and migrates) the database.
func Open() (*Store, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "asacli.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite: single writer
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS tracked_keywords (
  adam_id TEXT NOT NULL, keyword TEXT NOT NULL, country TEXT NOT NULL,
  added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (adam_id, keyword, country));
CREATE TABLE IF NOT EXISTS ranks (
  adam_id TEXT NOT NULL, keyword TEXT NOT NULL, country TEXT NOT NULL,
  rank INTEGER NOT NULL, checked_at TIMESTAMP NOT NULL);
CREATE INDEX IF NOT EXISTS idx_ranks ON ranks (adam_id, keyword, country, checked_at);
CREATE TABLE IF NOT EXISTS competitor_watch (
  adam_id TEXT NOT NULL, country TEXT NOT NULL,
  PRIMARY KEY (adam_id, country));
CREATE TABLE IF NOT EXISTS competitor_snapshots (
  adam_id TEXT NOT NULL, country TEXT NOT NULL,
  taken_at TIMESTAMP NOT NULL, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS harvest_log (
  at TIMESTAMP NOT NULL, search_term TEXT NOT NULL,
  from_campaign TEXT, to_campaign TEXT, action TEXT NOT NULL, detail TEXT);
CREATE TABLE IF NOT EXISTS mutations (
  at TIMESTAMP NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
  action TEXT NOT NULL, detail TEXT);
CREATE TABLE IF NOT EXISTS rc_transactions (
  txn_id TEXT PRIMARY KEY, app_user_id TEXT, product_id TEXT,
  price REAL, proceeds REAL, currency TEXT, purchased_at TIMESTAMP,
  is_trial INTEGER DEFAULT 0, is_renewal INTEGER DEFAULT 0,
  campaign_id TEXT, ad_group_id TEXT, keyword_id TEXT, ad_id TEXT,
  country TEXT, claim_type TEXT, raw TEXT);
CREATE INDEX IF NOT EXISTS idx_rc_campaign ON rc_transactions (campaign_id, ad_group_id, keyword_id);
`)
	return err
}

// --- rank tracking ---

// TrackKeyword registers a keyword+country to snapshot for an app.
func (s *Store) TrackKeyword(adamID, keyword, country string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO tracked_keywords (adam_id, keyword, country) VALUES (?,?,?)`,
		adamID, keyword, country)
	return err
}

// TrackedKeyword is one tracked (app, keyword, country) tuple.
type TrackedKeyword struct{ AdamID, Keyword, Country string }

// TrackedKeywords lists everything being tracked.
func (s *Store) TrackedKeywords() ([]TrackedKeyword, error) {
	rows, err := s.DB.Query(`SELECT adam_id, keyword, country FROM tracked_keywords ORDER BY adam_id, country, keyword`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackedKeyword
	for rows.Next() {
		var t TrackedKeyword
		if err := rows.Scan(&t.AdamID, &t.Keyword, &t.Country); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RecordRank stores one rank observation (rank 0 = not in results).
func (s *Store) RecordRank(adamID, keyword, country string, rank int, at time.Time) error {
	_, err := s.DB.Exec(`INSERT INTO ranks (adam_id, keyword, country, rank, checked_at) VALUES (?,?,?,?,?)`,
		adamID, keyword, country, rank, at.UTC())
	return err
}

// RankPoint is one historical observation.
type RankPoint struct {
	Rank      int       `json:"rank"`
	CheckedAt time.Time `json:"checkedAt"`
}

// RankHistory returns observations for a tracked tuple since a time, oldest first.
func (s *Store) RankHistory(adamID, keyword, country string, since time.Time) ([]RankPoint, error) {
	rows, err := s.DB.Query(`SELECT rank, checked_at FROM ranks
		WHERE adam_id=? AND keyword=? AND country=? AND checked_at>=? ORDER BY checked_at`,
		adamID, keyword, country, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RankPoint
	for rows.Next() {
		var p RankPoint
		if err := rows.Scan(&p.Rank, &p.CheckedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- competitor watch ---

// WatchCompetitor registers a competitor app for metadata snapshots.
func (s *Store) WatchCompetitor(adamID, country string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO competitor_watch (adam_id, country) VALUES (?,?)`, adamID, country)
	return err
}

// WatchedCompetitors lists watched competitor apps.
func (s *Store) WatchedCompetitors() ([]TrackedKeyword, error) { // Keyword unused
	rows, err := s.DB.Query(`SELECT adam_id, country FROM competitor_watch`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackedKeyword
	for rows.Next() {
		var t TrackedKeyword
		if err := rows.Scan(&t.AdamID, &t.Country); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SnapshotCompetitor stores a metadata snapshot payload.
func (s *Store) SnapshotCompetitor(adamID, country string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO competitor_snapshots (adam_id, country, taken_at, payload) VALUES (?,?,?,?)`,
		adamID, country, time.Now().UTC(), string(b))
	return err
}

// LastCompetitorSnapshot returns the latest snapshot payload, or "" if none.
func (s *Store) LastCompetitorSnapshot(adamID, country string) (string, error) {
	var payload string
	err := s.DB.QueryRow(`SELECT payload FROM competitor_snapshots
		WHERE adam_id=? AND country=? ORDER BY taken_at DESC LIMIT 1`, adamID, country).Scan(&payload)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return payload, err
}

// --- harvest memory ---

// LogHarvest records a harvest action.
func (s *Store) LogHarvest(term, fromCampaign, toCampaign, action, detail string) error {
	_, err := s.DB.Exec(`INSERT INTO harvest_log (at, search_term, from_campaign, to_campaign, action, detail)
		VALUES (?,?,?,?,?,?)`, time.Now().UTC(), term, fromCampaign, toCampaign, action, detail)
	return err
}

// HarvestedTerms returns terms already promoted (so we never promote twice).
func (s *Store) HarvestedTerms() (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT DISTINCT search_term FROM harvest_log WHERE action='promote'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out[t] = true
	}
	return out, rows.Err()
}

// HarvestEntry is one harvest log line.
type HarvestEntry struct {
	At           time.Time `json:"at"`
	SearchTerm   string    `json:"searchTerm"`
	FromCampaign string    `json:"fromCampaign"`
	ToCampaign   string    `json:"toCampaign"`
	Action       string    `json:"action"`
	Detail       string    `json:"detail"`
}

// HarvestLog returns harvest entries since a time, newest first.
func (s *Store) HarvestLog(since time.Time) ([]HarvestEntry, error) {
	rows, err := s.DB.Query(`SELECT at, search_term, COALESCE(from_campaign,''), COALESCE(to_campaign,''), action, COALESCE(detail,'')
		FROM harvest_log WHERE at>=? ORDER BY at DESC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HarvestEntry
	for rows.Next() {
		var e HarvestEntry
		if err := rows.Scan(&e.At, &e.SearchTerm, &e.FromCampaign, &e.ToCampaign, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- mutation audit log ---

// Mutation is one locally-recorded write against the Ads API.
type Mutation struct {
	At         time.Time `json:"at"`
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	Action     string    `json:"action"`
	Detail     string    `json:"detail"`
}

// LogMutation records a write performed by this CLI.
func (s *Store) LogMutation(entityType, entityID, action, detail string) error {
	_, err := s.DB.Exec(`INSERT INTO mutations (at, entity_type, entity_id, action, detail) VALUES (?,?,?,?,?)`,
		time.Now().UTC(), entityType, entityID, action, detail)
	return err
}

// MutationsSince lists local mutations after a time.
func (s *Store) MutationsSince(since time.Time) ([]Mutation, error) {
	rows, err := s.DB.Query(`SELECT at, entity_type, entity_id, action, COALESCE(detail,'')
		FROM mutations WHERE at>=? ORDER BY at DESC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mutation
	for rows.Next() {
		var m Mutation
		if err := rows.Scan(&m.At, &m.EntityType, &m.EntityID, &m.Action, &m.Detail); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

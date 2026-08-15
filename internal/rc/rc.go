// Package rc is the RevenueCat connector: credential storage, data ingest
// (Scheduled Data Exports / Customer Lists CSV), REST API v2 supplement, and
// the spend⋈revenue join powering `adastra roas` and `adastra ltv`.
//
// Attribution path: the RC SDK's enableAdServicesAttributionTokenCollection()
// fills reserved subscriber attributes ($campaignId, $adGroupId, $keywordId,
// $adId, $countryOrRegion, $claimType) for ALL attributed users — no ATT
// prompt required for keyword-level revenue.
package rc

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tawhidkuet04/adastra/internal/config"
	"github.com/tawhidkuet04/adastra/internal/store"
)

// Credentials for the RevenueCat REST API v2.
type Credentials struct {
	APIKey    string `json:"apiKey"`
	ProjectID string `json:"projectId"`
}

func credsPath() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "revenuecat.json"), nil
}

// SaveCredentials stores RC credentials (0600).
func SaveCredentials(c *Credentials) error {
	p, err := credsPath()
	if err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// LoadCredentials loads RC credentials.
func LoadCredentials() (*Credentials, error) {
	p, err := credsPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("RevenueCat not connected — run `adastra rc connect` first")
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks the key against the v2 API.
func Validate(ctx context.Context, c *Credentials) error {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.revenuecat.com/v2/projects/"+c.ProjectID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("revenuecat api: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Txn is one normalized transaction with ASA attribution.
type Txn struct {
	TxnID       string
	AppUserID   string
	ProductID   string
	Price       float64
	Proceeds    float64
	Currency    string
	PurchasedAt time.Time
	IsTrial     bool
	IsRenewal   bool
	CampaignID  string
	AdGroupID   string
	KeywordID   string
	AdID        string
	Country     string
	ClaimType   string
	Raw         string
}

// IngestFile loads an RC export (CSV, or JSON-lines) into the local store.
// Column names are matched case-insensitively against RC's export schema and
// the reserved $-attributes.
func IngestFile(path string, st *store.Store) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	if strings.HasSuffix(strings.ToLower(path), ".jsonl") || strings.HasSuffix(strings.ToLower(path), ".ndjson") {
		return ingestJSONL(f, st)
	}
	return ingestCSV(f, st)
}

func ingestCSV(r io.Reader, st *store.Store) (rows, attributed int, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		return 0, 0, err
	}
	if len(recs) < 2 {
		return 0, 0, fmt.Errorf("export has no data rows")
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[normalizeCol(h)] = i
	}
	pick := func(rec []string, names ...string) string {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}
	for _, rec := range recs[1:] {
		t := Txn{
			TxnID:      pick(rec, "transactionid", "storetransactionid", "id"),
			AppUserID:  pick(rec, "appuserid", "rcoriginalappuserid", "originalappuserid"),
			ProductID:  pick(rec, "productid", "productidentifier"),
			Currency:   pick(rec, "currency", "purchasecurrency"),
			CampaignID: pick(rec, "campaignid", "rccampaignid"),
			AdGroupID:  pick(rec, "adgroupid", "rcadgroupid"),
			KeywordID:  pick(rec, "keywordid", "rckeywordid"),
			AdID:       pick(rec, "adid", "rcadid"),
			Country:    pick(rec, "countryorregion", "country", "storefront"),
			ClaimType:  pick(rec, "claimtype", "rcclaimtype"),
		}
		t.Price, _ = strconv.ParseFloat(pick(rec, "price", "priceinusd", "purchaseprice"), 64)
		t.Proceeds, _ = strconv.ParseFloat(pick(rec, "proceeds", "takehomepercentageproceeds", "proceedsinusd"), 64)
		if t.Proceeds == 0 && t.Price > 0 {
			t.Proceeds = t.Price * 0.85 // small-business program default; caveat in docs
		}
		if ts := pick(rec, "purchasedate", "purchasedat", "starttime"); ts != "" {
			t.PurchasedAt = parseTime(ts)
		}
		t.IsTrial = strings.EqualFold(pick(rec, "istrialperiod", "istrial"), "true")
		t.IsRenewal = strings.EqualFold(pick(rec, "isrenewal", "renewalnumber"), "true") ||
			numGT(pick(rec, "renewalnumber"), 1)
		if t.TxnID == "" {
			continue
		}
		if err := upsertTxn(st, &t); err != nil {
			return rows, attributed, err
		}
		rows++
		if t.CampaignID != "" {
			attributed++
		}
	}
	return rows, attributed, nil
}

func ingestJSONL(r io.Reader, st *store.Store) (rows, attributed int, err error) {
	dec := json.NewDecoder(r)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return rows, attributed, err
		}
		get := func(names ...string) string {
			for _, n := range names {
				for k, v := range m {
					if normalizeCol(k) == n {
						return fmt.Sprint(v)
					}
				}
			}
			return ""
		}
		t := Txn{
			TxnID: get("transactionid", "storetransactionid", "id"), AppUserID: get("appuserid"),
			ProductID: get("productid"), Currency: get("currency"),
			CampaignID: get("campaignid"), AdGroupID: get("adgroupid"),
			KeywordID: get("keywordid"), AdID: get("adid"),
			Country: get("countryorregion", "country"), ClaimType: get("claimtype"),
		}
		t.Price, _ = strconv.ParseFloat(get("price"), 64)
		t.Proceeds, _ = strconv.ParseFloat(get("proceeds"), 64)
		if t.Proceeds == 0 && t.Price > 0 {
			t.Proceeds = t.Price * 0.85
		}
		t.PurchasedAt = parseTime(get("purchasedate", "purchasedat"))
		if t.TxnID == "" {
			continue
		}
		if err := upsertTxn(st, &t); err != nil {
			return rows, attributed, err
		}
		rows++
		if t.CampaignID != "" {
			attributed++
		}
	}
	return rows, attributed, nil
}

func normalizeCol(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("_", "", "-", "", " ", "", "$", "", ".", "").Replace(s)
	return s
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	// epoch millis
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 1e12 {
		return time.UnixMilli(n)
	}
	return time.Time{}
}

func numGT(s string, n int) bool {
	v, err := strconv.Atoi(s)
	return err == nil && v > n
}

func upsertTxn(st *store.Store, t *Txn) error {
	_, err := st.DB.Exec(`INSERT OR REPLACE INTO rc_transactions
		(txn_id, app_user_id, product_id, price, proceeds, currency, purchased_at,
		 is_trial, is_renewal, campaign_id, ad_group_id, keyword_id, ad_id, country, claim_type)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.TxnID, t.AppUserID, t.ProductID, t.Price, t.Proceeds, t.Currency, t.PurchasedAt.UTC(),
		boolInt(t.IsTrial), boolInt(t.IsRenewal), t.CampaignID, t.AdGroupID, t.KeywordID, t.AdID, t.Country, t.ClaimType)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RevenueRow is one aggregate from the local RC store.
type RevenueRow struct {
	Key       string  `json:"key"` // campaignId / adGroupId / keywordId value
	Proceeds  float64 `json:"proceeds"`
	Txns      int     `json:"transactions"`
	Users     int     `json:"users"`
	TrialTxns int     `json:"trialTransactions"`
}

// RevenueBy aggregates proceeds by campaign_id, ad_group_id, or keyword_id.
func RevenueBy(st *store.Store, dim string, since time.Time) ([]RevenueRow, error) {
	colName := map[string]string{
		"campaign": "campaign_id", "adgroup": "ad_group_id", "keyword": "keyword_id",
	}[dim]
	if colName == "" {
		return nil, fmt.Errorf("dimension must be campaign, adgroup, or keyword")
	}
	rows, err := st.DB.Query(`SELECT COALESCE(`+colName+`, ''),
			SUM(proceeds), COUNT(*), COUNT(DISTINCT app_user_id), SUM(is_trial)
		FROM rc_transactions WHERE purchased_at >= ?
		GROUP BY `+colName+` ORDER BY SUM(proceeds) DESC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RevenueRow
	for rows.Next() {
		var r RevenueRow
		if err := rows.Scan(&r.Key, &r.Proceeds, &r.Txns, &r.Users, &r.TrialTxns); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

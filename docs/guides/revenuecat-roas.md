# ROAS & LTV with RevenueCat

Apple's Ads API stops at installs. With the RevenueCat connector, asacli joins
**spend (Ads API) ⋈ revenue (RevenueCat)** down to the keyword — no ATT prompt
required.

## Setup (one-time, ~10 minutes)

1. **In your app** (RevenueCat SDK):
   ```swift
   Purchases.shared.attribution.enableAdServicesAttributionTokenCollection()
   ```
2. **In RevenueCat**: connect the *Apple Search Ads* integration
   (Sign in with Apple; the Read Only role is sufficient). The Standard
   payload already includes `campaignId`, `adGroupId`, `keywordId`, `adId`,
   `countryOrRegion`, and `claimType` for **all** attributed users.
3. **Connect asacli**:
   ```
   asacli rc connect --api-key <RC_v2_secret_key> --project <projectId>
   ```
4. **Feed it data** — RevenueCat *Scheduled Data Exports* are the recommended
   route (they carry the reserved `$` attributes with the ASA ids):
   ```
   asacli rc ingest ./export.csv     # also accepts .jsonl
   ```

## Reports

```
asacli roas report --by keyword --since 30d
asacli ltv report --by campaign --horizon 90d
```

## New optimization modes

```
asacli bids adjust --adgroup <id> --target-roas 150% --dry-run   # bid to profitability
asacli harvest run ... --rank-by roas                            # promote by revenue, not installs
```

## Honest caveats (also printed in output)

- "Unspecified" attribution rows come from Search Ads **Basic** accounts or
  TestFlight/debug placeholder tokens — Advanced accounts are clean.
- The "Organic" bucket includes limit-ad-tracking users, so measured ROAS is a
  **floor**, not the truth.
- RC fetches attribution within ~24h of the token; new integrations can take up
  to ~7 days to populate.
- Separate mature trials from active ones before judging keyword profitability.

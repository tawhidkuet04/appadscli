---
name: launch-playbook
description: Launch Apple Ads for a new app with asacli — research keywords, scaffold the best-practice campaign structure, seed keywords, and set up rank tracking. Use when asked to start advertising an iOS app, set up Apple Search Ads, or plan an App Store launch.
---

# App Launch Playbook (ASO + Apple Ads)

## 1. Research before spending

```
asacli aso research "<main category term>" --expand      # popularity + difficulty + candidates
asacli aso difficulty "<term>" --country us              # per-term depth
asacli insights popularity --terms "<t1>,<t2>,<t3>"      # Apple's own 1-5 demand scores
```

Pick 10–30 terms: mostly popularity ≥ 3 with difficulty ≤ 6, plus your brand
name and 2–3 competitor names. Save them to `keywords.txt` (one per line).

## 2. Verify the app can run ads

```
asacli apps search "<app name>"          # find the adamId
asacli apps eligibility <adamId>
```

## 3. Scaffold the campaign structure

```
asacli campaigns scaffold --app <adamId> \
  --structure brand,category,competitor,discovery \
  --daily-budget 10 --country us --dry-run
```

Review, then re-run with `--confirm`. This creates 4 campaigns, each with one
ad group; discovery gets Search Match ON.

## 4. Seed keywords

```
asacli keywords add --adgroup <brandAdGroupId>      --terms "<your app name>" --match exact --bid 0.50 --confirm
asacli keywords add --adgroup <categoryAdGroupId>   --file ./keywords.txt --match exact --bid 1.20 --confirm
asacli keywords add --adgroup <competitorAdGroupId> --terms "<comp1>,<comp2>" --match exact --bid 1.50 --confirm
```

Discovery needs no keywords — Search Match mines them.

## 5. Track organic outcomes from day one

```
asacli aso track add --app <adamId> --keywords ./keywords.txt --countries us
asacli aso track run          # snapshot now; then schedule daily via cron/CI
```

## 6. After 1–2 weeks

- `asacli dashboard --since 7d` — spend, installs, CPA at a glance
- `asacli harvest run ... --dry-run` — first harvest pass (see harvest-playbook)
- `asacli reco list` — Apple's own budget/CPA suggestions

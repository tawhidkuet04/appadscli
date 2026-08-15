---
name: launch-playbook
description: Launch Apple Ads for a new app with adastra — research keywords, scaffold the best-practice campaign structure, seed keywords, and set up rank tracking. Use when asked to start advertising an iOS app, set up Apple Search Ads, or plan an App Store launch.
---

# App Launch Playbook (ASO + Apple Ads)

## 1. Research before spending

```
adastra aso research "<main category term>" --expand      # popularity + difficulty + candidates
adastra aso difficulty "<term>" --country us              # per-term depth
adastra insights popularity --terms "<t1>,<t2>,<t3>"      # Apple's own 1-5 demand scores
```

Pick 10–30 terms: mostly popularity ≥ 3 with difficulty ≤ 6, plus your brand
name and 2–3 competitor names. Save them to `keywords.txt` (one per line).

## 2. Verify the app can run ads

```
adastra apps search "<app name>"          # find the adamId
adastra apps eligibility <adamId>
```

## 3. Scaffold the campaign structure

```
adastra campaigns scaffold --app <adamId> \
  --structure brand,category,competitor,discovery \
  --daily-budget 10 --country us --dry-run
```

Review, then re-run with `--confirm`. This creates 4 campaigns, each with one
ad group; discovery gets Search Match ON.

## 4. Seed keywords

```
adastra keywords add --adgroup <brandAdGroupId>      --terms "<your app name>" --match exact --bid 0.50 --confirm
adastra keywords add --adgroup <categoryAdGroupId>   --file ./keywords.txt --match exact --bid 1.20 --confirm
adastra keywords add --adgroup <competitorAdGroupId> --terms "<comp1>,<comp2>" --match exact --bid 1.50 --confirm
```

Discovery needs no keywords — Search Match mines them.

## 5. Track organic outcomes from day one

```
adastra aso track add --app <adamId> --keywords ./keywords.txt --countries us
adastra aso track run          # snapshot now; then schedule daily via cron/CI
```

## 6. After 1–2 weeks

- `adastra dashboard --since 7d` — spend, installs, CPA at a glance
- `adastra harvest run ... --dry-run` — first harvest pass (see harvest-playbook)
- `adastra reco list` — Apple's own budget/CPA suggestions

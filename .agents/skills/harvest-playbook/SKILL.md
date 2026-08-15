---
name: harvest-playbook
description: Run the Apple Ads harvest loop with adastra — promote converting search terms from discovery campaigns to exact match, negate waste, and keep CPA under control. Use when asked to optimize Apple Ads keywords, harvest search terms, or reduce wasted ad spend.
---

# Apple Ads Harvest Playbook

The harvest loop is the highest-leverage recurring workflow in Apple Search Ads.
Discovery campaigns (Search Match + broad match) mine what users actually type;
harvest promotes the winners to controlled exact-match keywords and negates them
in discovery so the two never compete.

## Prerequisites

- `adastra auth doctor` passes and an account is selected (`adastra accounts use`)
- A discovery campaign (Search Match ON) and a target campaign exist
  (create both with `adastra campaigns scaffold` if missing)

## The loop (run weekly, or from cron)

1. **Inspect candidates first** — always dry-run:
   ```
   adastra harvest run --discovery <discoveryCampaignId> --target <targetCampaignId> \
     --min-installs 2 --max-cpa 3.00 --auto-negate --dry-run
   ```
2. **Review the plan.** Each action shows installs, taps, spend, CPA, and the
   proposed exact-match bid (observed CPT × 1.1). Question anything promoting
   a term with high CPA or negating a term with meaningful installs.
3. **Apply**:
   ```
   adastra harvest run --discovery <id> --target <id> \
     --min-installs 2 --max-cpa 3.00 --auto-negate --confirm
   ```
4. **Verify**: `adastra harvest report --since 7d` shows what was promoted and
   negated. Promoted terms are remembered locally and never promoted twice.

## Revenue-aware harvesting (optional)

With RevenueCat connected (`adastra rc connect` + `adastra rc ingest`), rank
candidates by actual revenue instead of installs:

```
adastra harvest run ... --rank-by roas
```

## Guardrails

Prefer running harvest through `adastra watch` with a guardrails.json in
`propose` mode — it writes a plan file a human approves with
`adastra plan apply <file> --confirm`.

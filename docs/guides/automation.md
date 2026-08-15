# Automation: watch, guardrails, plan/apply

`asacli watch` is the agent tick: run it from cron or GitHub Actions every few
hours and it evaluates your guardrails against live data.

## guardrails.json

```json
{
  "maxDailySpend": 25.00,
  "maxCpa": { "default": 3.00, "campaigns": { "brand-us": 1.50 } },
  "maxBidChangePct": 20,
  "neverPause": ["brand-us"],
  "harvest": { "minInstalls": 2, "autoNegate": true,
               "discovery": "<discoveryCampaignId>", "promoteTo": "<targetCampaignId>" },
  "alerts": { "rankDrop": 5, "webhook": "https://hooks.slack.com/..." },
  "autonomy": "propose"
}
```

## The autonomy ladder

| Mode | Behavior |
|---|---|
| `alert` | print findings, exit non-zero on alerts (default) |
| `propose` | also write a plan file for human review |
| `auto` | apply changes within caps; `neverPause` campaigns are untouchable |

Trust is earned: start on `alert`, move to `propose` when the findings look
right, and only then consider `auto`.

## PR-style approval

```
asacli watch --config ./guardrails.json          # writes asacli-plan-<ts>.json in propose mode
asacli plan show ./asacli-plan-<ts>.json        # human-readable diff
asacli plan apply ./asacli-plan-<ts>.json --confirm
```

## GitHub Actions example

```yaml
name: ads-watch
on:
  schedule: [{ cron: "0 */6 * * *" }]
jobs:
  watch:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: curl -fsSL https://raw.githubusercontent.com/tawhidkuet04/asacli/main/install.sh | sh
      - run: |
          asacli auth login --client-id "$ADS_CLIENT_ID" --team-id "$ADS_TEAM_ID" \
            --key-id "$ADS_KEY_ID" --private-key <(echo "$ADS_PRIVATE_KEY") --bypass-keychain
          asacli accounts use "$ADS_ACCOUNT_ID"
          asacli watch --config ./guardrails.json
        env:
          ADS_CLIENT_ID: ${{ secrets.ADS_CLIENT_ID }}
          ADS_TEAM_ID: ${{ secrets.ADS_TEAM_ID }}
          ADS_KEY_ID: ${{ secrets.ADS_KEY_ID }}
          ADS_PRIVATE_KEY: ${{ secrets.ADS_PRIVATE_KEY }}
          ADS_ACCOUNT_ID: ${{ secrets.ADS_ACCOUNT_ID }}
```

Every mutation asacli makes is logged locally; `asacli history verify`
cross-checks Apple's change history to detect changes made outside the CLI.

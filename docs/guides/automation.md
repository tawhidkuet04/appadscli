# Automation: watch, guardrails, plan/apply

`appadscli watch` is the agent tick: run it from cron or GitHub Actions every few
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
appadscli watch --config ./guardrails.json          # writes appadscli-plan-<ts>.json in propose mode
appadscli plan show ./appadscli-plan-<ts>.json        # human-readable diff
appadscli plan apply ./appadscli-plan-<ts>.json --confirm
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
      - run: curl -fsSL https://raw.githubusercontent.com/appadscli/appadscli/main/install.sh | sh
      - run: |
          appadscli auth login --client-id "$ADS_CLIENT_ID" --team-id "$ADS_TEAM_ID" \
            --key-id "$ADS_KEY_ID" --private-key <(echo "$ADS_PRIVATE_KEY") --bypass-keychain
          appadscli accounts use "$ADS_ACCOUNT_ID"
          appadscli watch --config ./guardrails.json
        env:
          ADS_CLIENT_ID: ${{ secrets.ADS_CLIENT_ID }}
          ADS_TEAM_ID: ${{ secrets.ADS_TEAM_ID }}
          ADS_KEY_ID: ${{ secrets.ADS_KEY_ID }}
          ADS_PRIVATE_KEY: ${{ secrets.ADS_PRIVATE_KEY }}
          ADS_ACCOUNT_ID: ${{ secrets.ADS_ACCOUNT_ID }}
```

Every mutation appadscli makes is logged locally; `appadscli history verify`
cross-checks Apple's change history to detect changes made outside the CLI.

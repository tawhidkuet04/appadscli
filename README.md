<div align="center">

# ✦ appadscli

**Your App Store growth stack in one binary.**

The complete ASO + Apple Ads CLI for indie developers — Apple Ads Platform API v1
management fused with organic rank tracking, keyword intelligence, and
keyword-level revenue attribution. JSON-first, agent-native, `--dry-run` everywhere.

[![CI](https://github.com/appadscli/appadscli/actions/workflows/ci.yml/badge.svg)](https://github.com/appadscli/appadscli/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/appadscli/appadscli)](https://goreportcard.com/report/github.com/appadscli/appadscli)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/appadscli/appadscli)](https://github.com/appadscli/appadscli/releases)

*the Apple Ads + ASO command line — from research to ROAS*

</div>

---

```console
$ appadscli aso research "meditation" --expand
{
  "term": "meditation",
  "searchPopularity": 5,        ← Apple's own demand score
  "difficulty": 9.6,            ← computed from the live top-10
  "top10": [ { "rank": 1, "name": "Insight Timer", ... } ],
  "expandedCandidates": [ "sleep", "mindfulness", "breathing", ... ]
}

$ appadscli harvest run --discovery 74130 --target 74131 --min-installs 2 --dry-run
[ { "action": "promote", "searchTerm": "habit streak app",
    "installs": 6, "cpa": 1.82, "bid": 1.21,
    "reason": "6 installs at 1.82 CPA — promote to exact in target, negate in discovery" } ]
```

## Why appadscli

Every existing tool has half the picture. ASO tools have no ads management.
ASA tools have no organic data. Both charge SaaS prices for data Apple gives
you for free through your own API credentials.

| | ASO tools | ASA tools | **appadscli** |
|---|:---:|:---:|:---:|
| Apple's search popularity (1–5) | 💰 resold | ✗ | ✅ free, from your API |
| Organic rank tracking | ✅ | ✗ | ✅ local SQLite history |
| Campaign / keyword / bid management | ✗ | ✅ | ✅ full v1 surface |
| Search-term harvesting loop | ✗ | some | ✅ `harvest run` |
| Impression share insights | ✗ | some | ✅ `insights impression-share` |
| Keyword-level ROAS (RevenueCat) | ✗ | 💰 SaaS | ✅ `roas report --by keyword` |
| Guided campaign setup | ✗ | ✗ | ✅ `campaigns scaffold` |
| Guardrail automation with approval | ✗ | black box | ✅ `watch` → `plan apply` |
| MCP server + agent skills | ✗ | ✗ | ✅ `mcp serve`, `install-skills` |
| Price | $29–299/mo | % of spend | **free, open source** |

Built v1-native for the **Apple Ads Platform API** (the v5 Campaign Management
API sunsets January 26, 2027).

## Install

**npm** (macOS / Linux / Windows)

```sh
npm install -g appadscli
```

> The package downloads the prebuilt binary for your platform and verifies
> its SHA-256 against the release checksums.

**Homebrew**

```sh
brew install appadscli/tap/appadscli
```

> The `appadscli/tap/` prefix names the tap the formula lives in. A bare
> `brew install appadscli` requires acceptance into homebrew-core, which gates on
> notability (30 forks / 30 watchers / 75 stars) — tracked in
> [#1](https://github.com/appadscli/appadscli/issues/1).

**Install script** (macOS / Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/appadscli/appadscli/main/install.sh | sh
```

**Go**

```sh
go install github.com/appadscli/appadscli@latest
```

**From source**

```sh
git clone https://github.com/appadscli/appadscli && cd appadscli && make install
```

## Quick start

**1. Create API credentials** — Apple Ads → Account Settings → API:

```sh
openssl ecparam -genkey -name prime256v1 -noout -out private-key.pem
openssl ec -in private-key.pem -pubout -out public-key.pem   # upload this one
```

**2. Log in** (credentials go to the macOS keychain, or a 0600 file elsewhere):

```sh
appadscli auth login \
  --client-id SEARCHADS.xxxx --team-id SEARCHADS.xxxx \
  --key-id xxxx --private-key ./private-key.pem
appadscli auth doctor          # full diagnostic: key, token, org access
appadscli accounts list
appadscli accounts use <adAccountId>
```

**3. Go**:

```sh
appadscli dashboard --since 7d
appadscli aso research "your category" --expand
appadscli campaigns scaffold --app <adamId> --daily-budget 10 --dry-run
```

No credentials yet? The ASO half works without any login:
`appadscli aso difficulty`, `aso track`, `aso metadata audit`,
`aso competitors gap` all run on public data.

## The command tree

```
appadscli
├── auth          login · status · doctor · logout
├── accounts      list · use
├── me
├── apps          search · get · eligibility · rejections · languages
├── geo           search
│
├── aso           ← organic growth (no ads credentials needed for most)
│   ├── research      popularity + difficulty + top-10 + candidate fan-out
│   ├── popularity    bulk Apple demand scores (1–5) across countries
│   ├── suggest       Apple's keyword/phrase/category suggestions
│   ├── difficulty    scored 1–10 from the live top-10
│   ├── track         add · run · report · alerts   (cron/CI-friendly)
│   ├── metadata      audit · generate  (keyword field from converting terms)
│   ├── competitors   gap · watch       (snapshot + diff competitor metadata)
│   └── reviews       list
│
├── campaigns     list · get · create · update · pause · resume · delete
│   └── scaffold  ← one-shot brand/category/competitor/discovery structure
├── adgroups      list · get · create · update
├── keywords      list · add · update · pause · bulk upsert
├── negatives     list · add
├── harvest       run · report   ← THE core loop: promote winners, negate waste
├── bids          adjust (CPA/ROAS targets) · rules apply (declarative)
├── budget        pacing · orders (shared budgets)
├── reco          list · apply · dismiss   (Apple's own recommendations)
│
├── reports       campaigns · adgroups · keywords · ads · searchterms
├── insights      impression-share · popularity
├── dashboard     one-screen spend/CPA/installs summary
│
├── creatives     list · get · create · delete
├── ads           list · create · pause
├── cpp           list · test start · test report   (CPP A/B testing)
├── maps          brands · locations · groups · categories · reports
│
├── watch         guardrails tick: alert → propose → auto
├── plan          show · apply --confirm   (PR-style approval)
├── history       Apple's change history · verify (external-change detection)
│
├── rc            connect · ingest         (RevenueCat)
├── roas          report --by campaign|adgroup|keyword
├── ltv           report --horizon 30d|90d|1y
│
├── mcp serve     built-in MCP server for AI agents
├── install-skills  harvest/launch/audit playbooks for agents
├── docs          list · show <topic>
└── completion    bash · zsh · fish · powershell
```

## The workflows that matter

### 🌱 Scaffold — from zero to best-practice structure

```sh
appadscli campaigns scaffold --app 1459969523 \
  --structure brand,category,competitor,discovery \
  --daily-budget 10 --country us --dry-run
```

Four campaigns, each with a configured ad group — discovery gets Search Match
ON to mine terms. Drop `--dry-run`, add `--confirm` to create.

### ♻️ Harvest — the loop that actually grows accounts

```sh
appadscli harvest run \
  --discovery <discoveryCampaignId> --target <categoryCampaignId> \
  --min-installs 2 --max-cpa 3.00 --auto-negate --dry-run
```

Promotes converting search terms to exact match (bid = observed CPT × 1.1),
negates them in discovery, negates wasteful terms. Local memory ensures a term
is never promoted twice. Run weekly, or wire it into `watch`.

### 📈 Fuse paid and organic

```sh
appadscli aso track add --app <adamId> --keywords ./kw.txt --countries us,gb
appadscli aso track run                        # cron this
appadscli aso track alerts --drop 5            # CI: exit 1 on rank drops
appadscli aso metadata generate --app <adamId> # keyword field from converting paid terms
```

### 💰 ROAS down to the keyword (RevenueCat)

```sh
appadscli rc connect --api-key <v2-key> --project <id>
appadscli rc ingest ./export.csv
appadscli roas report --by keyword --since 30d
appadscli bids adjust --adgroup <id> --target-roas 150% --dry-run
```

Two lines of RC SDK setup, no ATT prompt required — see
[`docs/guides/revenuecat-roas.md`](docs/guides/revenuecat-roas.md).

### 🤖 Autonomy with a leash

```sh
appadscli watch --config ./guardrails.json     # cron every few hours
appadscli plan show ./appadscli-plan-*.json      # review what it wants to do
appadscli plan apply ./appadscli-plan-*.json --confirm
```

`guardrails.json` ([example](examples/guardrails.example.json)) sets CPA
ceilings, spend caps, never-pause lists, and the autonomy level:
`alert` → `propose` → `auto`. Every mutation is logged locally;
`appadscli history verify` cross-checks Apple's change history for changes made
outside the CLI.

## For AI agents

```sh
claude mcp add appadscli -- appadscli mcp serve   # 32 tools, 1:1 with commands
appadscli install-skills                        # harvest/launch/audit playbooks
```

Mutating MCP tools require `confirm: true` — otherwise they run `--dry-run`
and return the plan. An agent must show you changes before it can spend a cent.

## Safety invariants

- every mutating command supports `--dry-run`
- anything that spends, pauses, or deletes requires `--confirm`
  (or an interactive "y")
- all mutations are logged to `~/.appadscli/appadscli.db` and verifiable against
  Apple's change history (`appadscli history verify`)
- credentials live in the macOS keychain (or 0600 files), never in configs
- no telemetry — the API traffic goes to Apple and nowhere else

## Output

TTY → table. Pipe/CI → JSON. `--output json|table|csv|markdown` always wins;
`APPADSCLI_OUTPUT` sets the default.

```sh
appadscli reports keywords --since 7d | jq '.[] | select(.totalInstalls == "0")'
appadscli reports campaigns --since 30d --output csv > spend.csv
appadscli aso track report --output markdown >> weekly-report.md
```

## Honest constraints

- **Rank tracking** uses the public iTunes Search API (unofficial for this
  purpose) — appadscli paces requests at 1 rps and caches for an hour.
- **Reviews**: Apple's public RSS feed has been returning empty results for
  many storefronts since 2025; the command degrades gracefully. ASC API
  support for own-app reviews is planned.
- **ROAS is a floor** — limit-ad-tracking users land in the organic bucket,
  and Basic Search Ads accounts produce "Unspecified" attribution rows.
- **No daemon** — 24/7 autonomy is cron/CI running `appadscli watch`.
- Each user runs their own credentials. Don't aggregate or resell Apple's data.

## Development

```sh
make build     # build ./appadscli
make test      # go test ./...
make lint      # go vet
make install   # install to GOPATH/bin
```

Repo layout: `cmd/` (command tree) · `internal/api` (v1 client) ·
`internal/aso`, `internal/engine` (intelligence + workflows) ·
`internal/store` (SQLite state) · `.agents/skills` (agent playbooks) ·
`docs/guides` (embedded docs).

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE) © Tawhid Joarder

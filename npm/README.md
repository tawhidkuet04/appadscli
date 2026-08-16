# appadscli

**Your App Store growth stack in one binary** — the complete Apple Search Ads +
ASO CLI for indie developers.

```sh
npm install -g appadscli
appadscli --help
```

This package downloads the prebuilt `appadscli` binary for your platform
(macOS, Linux, Windows — x64/arm64) from the project's GitHub release and
verifies its SHA-256 checksum. Node is only the delivery vehicle; the tool
itself is a single static Go binary.

- Campaigns, ad groups, keywords, bids, reports, insights — full Apple Ads Platform API v1
- ASO: keyword research, difficulty, organic rank tracking, competitor intel
- The harvest loop, guardrail automation with plan/apply approval
- Keyword-level ROAS via RevenueCat
- Built-in MCP server for AI agents (`appadscli mcp serve`)

Docs, guides, and source: **https://github.com/appadscli/appadscli**

License: MIT

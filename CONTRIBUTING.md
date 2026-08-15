# Contributing to adastra

Thanks for helping build the App Store growth stack.

## Setup

```sh
git clone https://github.com/tawhidkuet04/adastra && cd adastra
make build && make test
```

Go 1.24+. No CGO (SQLite is the pure-Go modernc driver) — keep it that way so
single-binary cross-compilation stays trivial.

## Ground rules

- **Safety first.** Every new mutating command must support `--dry-run`, go
  through `confirmOrAbort`, and log to the local mutation store. No exceptions.
- **JSON-first.** Table output is a projection; the JSON payload carries the
  full API objects. New commands should render both via `render().Rows(...)`.
- **Honest data.** If a metric is an estimate or a floor (ROAS, difficulty),
  say so in the output. Don't model what we can't measure.
- **v1 endpoints.** Validate new endpoints against Apple's Platform API v1
  docs (developer.apple.com/documentation/apple-ads-platform-api) and note the
  source URL in the PR.
- Match the existing code style; `make lint` must pass.

## Testing

Pure logic (difficulty scoring, report flattening, harvest planning) gets unit
tests. API-touching code is exercised against recorded fixtures — don't write
tests that hit Apple's live API.

## Releases

Tag `v*` → GitHub Actions runs goreleaser → binaries + Homebrew tap.

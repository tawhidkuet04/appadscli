# Getting Started

## 1. Create Apple Ads API credentials

1. Sign in to [Apple Ads](https://ads.apple.com) → **Account Settings → API**.
2. Generate a key pair (Apple Ads requires EC P-256):
   ```
   openssl ecparam -genkey -name prime256v1 -noout -out private-key.pem
   openssl ec -in private-key.pem -pubout -out public-key.pem
   ```
3. Upload `public-key.pem`, and note the **clientId**, **teamId**, and **keyId**
   Apple shows you.

Roles: read-only commands need **API Read Only**; mutations need
**API Account Manager** (or Admin).

## 2. Log in

```
appadscli auth login \
  --client-id SEARCHADS.xxxxxxxx \
  --team-id   SEARCHADS.xxxxxxxx \
  --key-id    xxxxxxxx-xxxx-xxxx \
  --private-key ./private-key.pem
```

Credentials go to the macOS keychain (or a 0600 file with `--bypass-keychain`
/ on Linux). Verify everything with:

```
appadscli auth doctor
```

## 3. Pick your ad account

```
appadscli accounts list
appadscli accounts use <adAccountId>
```

## 4. First commands

```
appadscli dashboard --since 7d
appadscli campaigns list
appadscli aso research "your category" --expand
```

Every command supports `--output json|table|csv|markdown`. In pipes and CI the
default is JSON; on a TTY it's a table. `APPADSCLI_OUTPUT` overrides the default,
`--output` always wins.

## Environment variables

| Variable | Effect |
|---|---|
| `APPADSCLI_DIR` | state directory (default `~/.appadscli`) |
| `APPADSCLI_OUTPUT` | default output format |
| `APPADSCLI_API_BASE` | override the Apple Ads API base URL |

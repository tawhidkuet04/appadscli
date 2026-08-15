---
name: audit-playbook
description: Audit an Apple Ads account and App Store presence with appadscli — find wasted spend, CPA breaches, impression-share losses, metadata problems, and competitor gaps. Use when asked to audit, review, or diagnose Apple Ads performance or ASO health.
---

# Account & ASO Audit Playbook

Run these read-only checks in order; none of them mutate anything.

## 1. Account health

```
appadscli auth doctor
appadscli dashboard --since 30d
appadscli reports campaigns --since 30d
```

Flag: campaigns with spend but zero installs; CPA above the app's LTV;
budget concentrated in one campaign.

## 2. Wasted spend (the usual jackpot)

```
appadscli reports searchterms --since 30d
```

Flag: search terms with ≥ 20 taps and 0 installs (negate them — see
harvest-playbook), and irrelevant terms leaking through broad match.

## 3. Impression share — who's squeezing you

```
appadscli insights impression-share --app <adamId> --since 7d
appadscli insights impression-share --keywords ./brand-terms.txt
```

Flag: single-digit impression share on your own brand terms (competitors are
buying your name — raise brand bids or file a dispute).

## 4. Bids vs targets

```
appadscli bids adjust --adgroup <id> --target-cpa <goal> --dry-run
appadscli reco list
```

## 5. Metadata & organic

```
appadscli aso metadata audit --app <adamId>
appadscli aso competitors gap --app <adamId> --vs <comp1>,<comp2>
appadscli aso track report --since 30d
appadscli aso reviews list --app <adamId> --stars 1,2
```

Flag: unused subtitle characters, title/subtitle duplication, terms every
competitor uses that you don't, rank drops, recurring 1–2★ complaints.

## 6. Deliverable

Summarize findings by impact: wasted spend first, then CPA breaches,
impression-share losses on brand, metadata fixes, and a prioritized action
list (each action as the exact appadscli command, dry-run first).

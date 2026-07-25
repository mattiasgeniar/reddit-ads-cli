---
name: reddit-ads
description: >-
  Manage Reddit ad campaigns from the terminal with the reddit-ads CLI. Use when
  the user wants to list, inspect, pause, resume, or steer Reddit campaigns, ad
  groups, or ads; change budgets, bids, or click URLs; edit subreddit (community)
  targeting; resolve subreddit names to targeting ids; or pull Reddit Ads
  performance reports (impressions, clicks, spend, CTR, CPC) from the command line.
license: MIT
metadata:
  author: mattiasgeniar
  version: "0.1.0"
---

# reddit-ads CLI

The `reddit-ads` CLI drives the [Reddit Ads API v3](https://ads-api.reddit.com/docs/v3/).
It covers reads and safe writes: reporting, listing, pause/resume, budget and bid
changes, click-URL edits, and ad group targeting. It does not create campaigns or
creatives.

## Prerequisites

```bash
reddit-ads --version
```

If missing, build and install from the repo with `make install` (drops the binary in
`~/bin`).

## Authentication

Authorization is a one-time OAuth flow that needs a browser exactly once. It is a human
step; do not attempt it unattended.

```bash
reddit-ads auth status     # who the token belongs to, when the access token expires
reddit-ads auth refresh    # force a new access token from the stored refresh token
```

If `auth status` errors with "not authorized", the user runs `reddit-ads auth login`
themselves. It is interactive: it prompts for the developer app's client id and secret,
prints an authorize URL to open in a browser, and takes the pasted code back. You cannot
do this step for them. Scopes are `adsread,adsedit`.

## Output contract for agents

- **Always pass `--json`** for machine consumption. It prints the raw API response
  verbatim. The default (no `--json`) is a human table.
- **Money in `--json` is microcurrency** (a dollar is 1,000,000). The human table prints
  dollars. When you set a budget or bid you give **dollars**; the CLI converts.
- Set a default account in `~/.config/reddit-ads/config.json` (`default_ad_account_id`)
  or pass `--account <id>` on each command.

## Safety on writes

Every write prompts for confirmation. For non-interactive use:

- `--dry-run` prints the exact request and sends nothing. **Preview every write with
  `--dry-run` first.**
- `--yes` skips the confirmation prompt to actually send.

There is no sandbox: every write hits a live ad account with real spend.

## Reading

```bash
reddit-ads accounts list --json
reddit-ads campaigns list --json
reddit-ads campaigns get cmp_abc123 --json
reddit-ads adgroups list --json
reddit-ads adgroups get ag_abc123 --json
reddit-ads ads list --json
reddit-ads ads get ad_abc123 --json
```

## Reporting

`POST`-backed report. Dates are `YYYY-MM-DD` (`--to` is exclusive) or full
`YYYY-MM-DDTHH:00:00Z` timestamps.

```bash
# default fields: IMPRESSIONS,CLICKS,CTR,SPEND,CPC
reddit-ads report --from 2026-07-01 --to 2026-07-25 --json

# breakdown shorthand: community,date,hour,ad,adgroup,campaign,keyword,interest,placement,country,gender
reddit-ads report --from 2026-07-01 --to 2026-07-25 --by community --json

# explicit fields and breakdowns (max 3 breakdowns)
reddit-ads report --from 2026-07-01 --to 2026-07-25 \
  --fields IMPRESSIONS,CLICKS,SPEND,CTR,CPC --breakdowns COMMUNITY,DATE --json
```

Metrics can take up to 6 hours to stabilize; delivery data goes back 24 months.

## Subreddit (community) targeting

Targeting lives on the ad group and is replaced wholesale on write, so add/remove are
read-modify-write: sibling targeting fields are preserved automatically.

```bash
reddit-ads communities search technology --json     # resolve a name to a targeting id (t5_...)
reddit-ads communities suggest programming --json   # expand from a seed name
reddit-ads adgroups targeting show ag_abc123 --json
reddit-ads adgroups targeting add-community ag_abc123 t5_2qh1i --dry-run
reddit-ads adgroups targeting remove-community ag_abc123 t5_2qh1i --yes
reddit-ads adgroups targeting exclude-community ag_abc123 t5_2qh0u --yes
```

## Budgets, bids, status, click URLs

```bash
# Campaign lifetime spend cap and/or CBO goal, in dollars
reddit-ads campaigns budget cmp_abc123 --spend-cap 250 --dry-run
reddit-ads campaigns budget cmp_abc123 --goal 100 --yes

# Ad group bid and budget, in dollars
reddit-ads adgroups bid ag_abc123 --bid 0.75 --dry-run
reddit-ads adgroups budget ag_abc123 --goal 50 --goal-type DAILY_SPEND --yes

# Pause / resume (campaigns, adgroups, ads share the same verbs)
reddit-ads campaigns pause cmp_abc123 --yes
reddit-ads adgroups resume ag_abc123 --yes
reddit-ads ads pause ad_abc123 --yes

# Click URL / UTM change (no new creative needed)
reddit-ads ads click-url ad_abc123 --url https://example.com --query-params "utm_source=reddit&utm_medium=cpc" --dry-run
```

## Typical flow

1. `reddit-ads auth status --json` to confirm authorization.
2. `reddit-ads campaigns list --json` (or `adgroups`/`ads`) to find ids.
3. Preview any change with `--dry-run`.
4. Re-run with `--yes` to apply.
5. `reddit-ads report ... --json` to check the effect (allow up to 6h for metrics).

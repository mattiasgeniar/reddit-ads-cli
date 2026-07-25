# reddit-ads

A single-binary Go CLI for the [Reddit Ads API v3](https://ads-api.reddit.com/docs/v3/).
It manages campaigns, ad groups, ads, subreddit targeting, budgets and bids, and pulls
performance reports, all from a terminal or an agent. After a one-time OAuth
authorization there is no browser in the loop.

v1 is deliberately scoped to reads and safe writes: reporting and listing, pause and
resume, budget and bid changes, click-URL edits, and ad group targeting. It does not
create campaigns or creatives.

## Install

```
make install    # builds and copies reddit-ads into ~/bin
```

Or just `make build` for a local `./reddit-ads`.

## One-time authorization

You need a developer app first. A business admin with a verified account creates it in
Ads Manager under Business > Developer Portal, and notes the client id, client secret,
and a redirect URI. The redirect URI never has to serve anything: you copy the `code`
out of the address bar by hand. It only has to match what you registered.

Then:

```
reddit-ads auth login \
  --client-id YOUR_ID \
  --client-secret YOUR_SECRET \
  --redirect-uri https://your-registered-redirect
```

It prints an authorize URL. Open it, click Allow, and you land on your redirect URI
with `?code=...` in the address bar. Copy that code value (strip a trailing `#_` if
present, it is valid for ten minutes and single use) and paste it back. The CLI
exchanges it for a permanent refresh token and stores everything.

Check it worked:

```
reddit-ads auth status
```

The v1 scopes are `adsread,adsedit`. These are Reddit's real ads scopes, not the
`ads:read` / `ads:manage` that third-party blog posts repeat.

## Configuration

Credentials and the volatile token live in separate files so a token refresh can never
rewrite your credentials.

- `~/.config/reddit-ads/config.json` (mode 600): `client_id`, `client_secret`,
  `refresh_token`, `default_ad_account_id`, `default_profile_id`, optional `user_agent`.
- `~/.config/reddit-ads/token.json` (mode 600): cached access token and its expiry.

Environment variables override the files, so cron and CI need no home directory:
`REDDIT_ADS_CLIENT_ID`, `REDDIT_ADS_CLIENT_SECRET`, `REDDIT_ADS_REFRESH_TOKEN`,
`REDDIT_ADS_ACCOUNT_ID`, `REDDIT_ADS_PROFILE_ID`, `REDDIT_ADS_USER_AGENT`.

Set a default account so you can drop `--account` from every command:

```
reddit-ads accounts list
# put the id you want in default_ad_account_id in config.json
```

## Commands

```
reddit-ads auth        login|status|refresh
reddit-ads accounts    list
reddit-ads campaigns   list|get|pause|resume|budget
reddit-ads adgroups    list|get|pause|resume|bid|budget
reddit-ads adgroups    targeting show|add-community|remove-community|exclude-community
reddit-ads ads         list|get|pause|resume|click-url
reddit-ads communities search|suggest
reddit-ads report      --from --to --fields --breakdowns --by
```

Global flags: `--json`, `--account`, `--profile`, `--yes`, `--dry-run`, `--config`.

### Reading

```
reddit-ads campaigns list
reddit-ads campaigns get cmp_abc123
reddit-ads adgroups get ag_abc123 --json
```

Table output is the default. `--json` prints the raw API response verbatim, which is
what an agent or a script should consume.

### Reporting

```
reddit-ads report --from 2026-07-01 --to 2026-07-25
reddit-ads report --from 2026-07-01 --to 2026-07-25 --by community
reddit-ads report --from 2026-07-01 --to 2026-07-25 \
  --fields IMPRESSIONS,CLICKS,SPEND,CTR,CPC --breakdowns COMMUNITY,DATE
```

Dates are `YYYY-MM-DD` (expanded to hour granularity; `--to` is exclusive) or a full
`YYYY-MM-DDTHH:00:00Z` timestamp. Default fields are `IMPRESSIONS,CLICKS,CTR,SPEND,CPC`.
Money fields print as dollars in the table; `--json` gives you raw microcurrency
exactly as Reddit returns it. Metrics can take up to six hours to stabilize.

### Targeting

Subreddit targeting lives on the ad group, and sending targeting replaces it wholesale,
so add and remove are read-modify-write under the hood: every other targeting field
round-trips untouched.

```
reddit-ads communities search technology          # find the targeting id
reddit-ads adgroups targeting show ag_abc123
reddit-ads adgroups targeting add-community ag_abc123 t5_2qh1i
reddit-ads adgroups targeting exclude-community ag_abc123 t5_2qh0u
```

### Safe writes

Every write prompts for confirmation unless you pass `--yes`, and every write supports
`--dry-run`, which prints the exact request that would be sent and sends nothing:

```
reddit-ads campaigns budget cmp_abc123 --spend-cap 250 --dry-run
reddit-ads adgroups bid ag_abc123 --bid 0.75
reddit-ads campaigns pause cmp_abc123 --yes
```

Budgets and bids are given and printed in dollars. Internally Reddit uses
microcurrency (a dollar is 1,000,000), and the conversion lives in one tested helper,
because getting it wrong is a 1,000,000x budget error.

## A word on testing

There is no sandbox and no test mode for the Reddit Ads API. Every write touches a live
ad account with real spend. The client is unit-tested against fixtures, the money
conversion, targeting merge, and pagination have real tests, and any live smoke test
should run against a paused campaign with `--dry-run` first.

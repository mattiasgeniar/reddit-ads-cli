# reddit-ads-cli

A Go CLI for the Reddit Ads API v3. The point is to manage, steer and review Reddit ad
campaigns from a terminal or from an agent, with no browser in the loop after the
one-time OAuth authorization.

Read [HANDOVER.md](HANDOVER.md) first if you are starting work here. It has the build
order, the v1 command surface and the open questions.

## Decisions already made

- **Go**, single static binary named `reddit-ads`, installed to `~/bin`.
- **Module path is `github.com/mattiasgeniar/reddit-ads-cli`**, matching the repo fetch
  path exactly. `go get` resolves modules over the network by their path, so a guessed
  name cannot be fetched at all.
- **v1 scope is read + safe writes.** Full reporting and listing, plus pause/resume,
  budget and bid changes, and ad group targeting edits. No creative creation in v1.
- Same shape as the `ohdear` and `flare` CLIs: a CLI plus a Claude skill that drives it,
  not an MCP server. If an MCP wrapper is ever wanted, it can reuse the same client
  package.

## Conventions

- Cobra for commands, stdlib `net/http` for the client. No SDK generator: the API is
  ~76 boring REST paths and v1 touches maybe 15 of them, so hand-write only the structs
  we need.
- Every command supports `--json` for raw output. Human-readable table output is the
  default, JSON is what the agent and scripts consume.
- Write commands prompt for confirmation unless `--yes`, and support `--dry-run` which
  prints the exact request that would be sent.
- No em-dashes in output or docs.

## Auth

Reddit's OAuth code flow, which needs a browser exactly once:

1. `reddit-ads auth login` prints `https://www.reddit.com/api/v1/authorize?...` with
   `duration=permanent` and the scopes below. Open it, click Allow, copy the `code`
   query parameter out of the redirect URL (strip a trailing `#_` if present, it is
   valid for ten minutes and single-use).
2. The CLI exchanges it at `https://www.reddit.com/api/v1/access_token` (HTTP Basic with
   client id and secret) and stores the refresh token.

Scopes are Reddit-specific and are NOT the `ads:read` / `ads:manage` you will find in
third-party blog posts. The real ones: `adsread`, `adsedit`, `adsconversions`,
`adsdatadeletion`, plus `adsmeasurement:read` and `adsmeasurement:write` which are gated
behind your Reddit account team. v1 needs `adsread,adsedit`.

The developer app itself is created in Ads Manager under **Business > Developer Portal**
by a business admin with a verified account, not at reddit.com/prefs/apps. There is no
allowlist, approval queue or spend minimum for the Ads API.

### Config layout

Static credentials and the volatile token live in separate files so a token refresh can
never rewrite the credentials.

- `~/.config/reddit-ads/config.json`, mode 600, never in git:
  `client_id`, `client_secret`, `refresh_token`, `default_ad_account_id`,
  `default_profile_id`, optional `user_agent`.
- `~/.config/reddit-ads/token.json`, mode 600: cached `access_token` and its expiry.
- Env vars (`REDDIT_ADS_CLIENT_ID`, `REDDIT_ADS_CLIENT_SECRET`,
  `REDDIT_ADS_REFRESH_TOKEN`, ...) override the file, so cron and CI do not need a home
  directory.

## API facts worth not re-discovering

Base URL `https://ads-api.reddit.com/api/v3`, docs at https://ads-api.reddit.com/docs/v3/

- **Money is microcurrency.** `bid_value`, `goal_value`, `spend_cap`, and the `SPEND`,
  `CPC`, `CPV`, `ECPM` report fields are all multiplied by 1e6. `bid_value: 500000` is
  $0.50. The CLI takes and prints dollars and converts at the boundary. Getting this
  wrong is a 1,000,000x budget error, so it belongs in a single helper with tests.
  `CONVERSION*TOTAL_VALUE` report fields are the odd ones out: divide by 100, not 1e6.
- **Subreddit targeting lives on the ad group**, not the campaign or the ad:
  `PATCH /ad_groups/{ad_group_id}` with `data.targeting.communities[]` and
  `excluded_communities[]`. The same object holds `interests`, `keywords`,
  `excluded_keywords`, `geolocations`, `languages`, `devices`, `platforms`,
  `custom_audience_ids`, `expand_targeting`. Sending `targeting` replaces it wholesale,
  so add/remove commands must read-modify-write.
- Resolve community names to targeting IDs with `GET /targeting/communities/search`,
  and expand from a seed with `GET /targeting/communities/suggestions`.
- **Creatives are effectively immutable.** `PATCH /structured_posts/{post_id}` only
  accepts `allow_comments`. Changing a headline or an image means creating a new
  creative with `POST /profiles/{profile_id}/structured_posts/jobs` (async, poll
  `GET /structured_posts/jobs/{id}`) and repointing the ad via `PATCH /ads/{ad_id}`
  with a new `post_id`. New creatives go through Reddit ad review. Out of scope for v1.
- `click_url` and `click_url_query_parameters` on the ad ARE patchable, so UTM changes
  do not need a new creative.
- **Media is supplied as a public URL**, not a multipart upload. Reddit downloads
  `creative.image.media = {type: "URL", url: "https://..."}` and rehosts it. The asset
  library endpoints exist but need the Max campaign type, still in beta.
- `configured_status` is what you set (`ACTIVE`, `PAUSED`, `ARCHIVED`, `DELETED`);
  `effective_status` is computed and read-only. Pause/resume writes `configured_status`.
- Budgets: campaign has `spend_cap` (lifetime) and, for CBO, `goal_value`. Ad group has
  `goal_type` (`DAILY_SPEND` / `LIFETIME_SPEND`) and `goal_value`.
- **Reports are a POST**, not a GET: `POST /ad_accounts/{id}/reports` with `starts_at`,
  `ends_at`, `fields[]` (447 of them, including `CLICKS`, `IMPRESSIONS`, `SPEND`, `CTR`,
  `CPC`, `ECPM`, `REACH`, `FREQUENCY`, the `VIDEO_WATCHED_*` ladder) and `breakdowns[]`,
  max 3 (4 when using `COUNTRY` plus `REGION`). Breakdowns include `COMMUNITY`, `DATE`,
  `HOUR`, `AD_ID`, `CAMPAIGN_ID`, `KEYWORD`, `INTEREST`, `PLACEMENT`. Metrics take up to
  6 hours to stabilize; delivery data goes back 24 months.
- **Pagination is URL-driven.** Responses carry `pagination.next_url` / `previous_url`
  and the docs are explicit that you follow those URLs directly rather than
  reconstructing query parameters.
- **Rate limits are per authorizing user**, reported in IETF `RateLimit-Policy` and
  `RateLimit` headers. Campaign reads 400/60s, campaign writes 200/60s, creative
  management 200/60s, reporting 60/60s. Handle 429 with exponential backoff and honour
  the `t` (seconds until reset) value from the header rather than guessing.
- Reddit requires a descriptive `User-Agent` of the form
  `<platform>:<app id>:<version> (by /u/<username>)` and bans clients that spoof
  browsers. Make it configurable, default to something honest.
- Two dated changes in the upstream docs: `conversion_pixel_id` became required for ad
  groups and CBO campaigns on 2026-07-13, and campaign objective enums change on
  2026-09-30.

### The OpenAPI spec

There is no official machine-readable spec published by Reddit. A third-party,
fully-inlined OpenAPI 3.1 document covering all 76 paths exists here:

https://raw.githubusercontent.com/pooriaarab/clis/main/reddit-ads/spec.json

It matched the official HTML docs everywhere I checked it, and it is the fastest way to
look up a field. It is not authoritative. Verify anything load-bearing against
https://ads-api.reddit.com/docs/v3/ before relying on it.

## Verifying work

**There is no sandbox and no test mode for the Reddit Ads API.** Every write touches a
live ad account with real spend. So:

- Unit-test the client against recorded fixtures, never against the live API.
- Money conversion, targeting read-modify-write, and pagination get real tests.
- Live smoke tests run against a paused campaign only, and every destructive-ish command
  gets exercised with `--dry-run` first.
- Do not claim a command works because the code looks right. Run it, paste the output.

## Git workflow

Normal app-repo flow: branch first with a descriptive kebab-case name, one commit per
iteration, PRs via `gh pr create`. Pushing and opening PRs only when asked.

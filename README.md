# reddit-ads

[![CI](https://github.com/mattiasgeniar/reddit-ads-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/mattiasgeniar/reddit-ads-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mattiasgeniar/reddit-ads-cli?sort=semver)](https://github.com/mattiasgeniar/reddit-ads-cli/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/mattiasgeniar/reddit-ads-cli)](https://goreportcard.com/report/github.com/mattiasgeniar/reddit-ads-cli)
[![Go version](https://img.shields.io/github/go-mod/go-version/mattiasgeniar/reddit-ads-cli)](go.mod)
[![License: MIT](https://img.shields.io/github/license/mattiasgeniar/reddit-ads-cli)](LICENSE)

I want to manage my Reddit ads from a terminal: change subreddit targeting, nudge
budgets and bids, pause and resume, and pull click and impression numbers, all without
clicking through Ads Manager. The [Reddit Ads API v3](https://ads-api.reddit.com/docs/v3/)
does every bit of that, but there's no official CLI. The third-party MCP servers that
exist broker your OAuth token through someone else's infrastructure, and this is ad
spend, so I built my own.

`reddit-ads` is a single static Go binary. After a one-time OAuth authorization, there's
no browser in the loop.

v1 sticks to reads and safe writes: reporting and listing, pause and resume, budget and
bid changes, click-URL edits, and ad group targeting. It doesn't create campaigns or
creatives yet.

## Install

Download a prebuilt binary for your platform from the
[releases page](https://github.com/mattiasgeniar/reddit-ads-cli/releases), or install
with Go:

```
go install github.com/mattiasgeniar/reddit-ads-cli/cmd/reddit-ads@latest
```

Or build from a clone:

```
make install    # builds and copies reddit-ads into ~/bin
make build      # just a local ./reddit-ads
```

Requires Go 1.26+ to build from source.

## One-time authorization

You need a developer app first. A business admin with a verified account creates it in
Ads Manager under Business > Developer Portal, which gives you two values Reddit labels
**App ID** and **Secret**. Those are the names this CLI uses too, in the prompts, the
flags (`--app-id`/`--app-secret`), and the config (`app_id`/`app_secret`). (Reddit's
OAuth calls the App ID the `client_id` on the wire, but you never have to.) For the
redirect URI, register this exact value and you're done thinking about it:

```
http://localhost:8080/reddit-ads/callback
```

Then just run:

```
reddit-ads auth login
```

It asks for the App ID, the Secret (hidden while you type), and the redirect URI (hit
enter to accept the default above), then prints an authorize URL. Open it and click
Allow.

If the CLI is running on the same machine as your browser, it catches the redirect on a
local server, shows an "Authentication successful" page in the tab, and you're done. No
copy-paste. If the browser is on another machine (you're SSH'd into a box, say), the
redirect can't reach the CLI, so paste the code or the whole redirect URL back into the
terminal instead. The CLI waits for either, whichever comes first.

Two flags for the loopback capture:

- `--port N` if you registered a redirect on a port other than 8080, or 8080 is taken.
  The port has to match a redirect registered on your app, Reddit checks it exactly.
- `--manual` to skip the local server entirely and just paste (handy on headless boxes).

Either way the CLI exchanges the code for a permanent refresh token and stores
everything, so you only do this once.

Login also picks your default ad account: with exactly one it sets it automatically,
and with several it lists them and asks which to use. That becomes
`default_ad_account_id`, so `campaigns list` and the rest work without `--account`.

Prefer flags for CI? They still work, and skip every prompt:

```
reddit-ads auth login --app-id ID --app-secret SECRET --redirect-uri URL
```

Check it worked:

```
reddit-ads auth status
```

The v1 scopes are `adsread,adsedit`. These are Reddit's real ads scopes, not the
`ads:read` / `ads:manage` that third-party blog posts repeat.

## Configuration

Credentials and the volatile token live in separate files, so a token refresh can never
rewrite your credentials.

- `~/.config/reddit-ads/config.json` (mode 600): `app_id`, `app_secret`,
  `refresh_token`, `redirect_uri`, `default_ad_account_id`, `default_profile_id`,
  optional `user_agent`.
- `~/.config/reddit-ads/token.json` (mode 600): cached access token and its expiry.

Both files are written 600 (owner read/write only) and the `reddit-ads` directory is
created 700, so other users on the box can't read your secret or even list the
directory. That holds regardless of how `~/.config` itself is set: a 600 file inside a
700 directory is protected by its own permissions, not the parent's. On load, if either
file was created or hand-edited with looser permissions, the CLI tightens it back to 600
rather than trusting it as-is.

Environment variables override the files, so cron and CI need no home directory:
`REDDIT_ADS_APP_ID`, `REDDIT_ADS_APP_SECRET`, `REDDIT_ADS_REFRESH_TOKEN`,
`REDDIT_ADS_REDIRECT_URI`, `REDDIT_ADS_ACCOUNT_ID`, `REDDIT_ADS_PROFILE_ID`,
`REDDIT_ADS_USER_AGENT`.

Set a default account so you can drop `--account` off every command:

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
reddit-ads adgroups    targeting show|add-community|remove-community|exclude-community|unexclude-community
reddit-ads ads         list|get|pause|resume|click-url
reddit-ads communities search|suggest
reddit-ads report      --from --to --fields --breakdowns --by
```

Global flags: `--json`, `--account`, `--yes`, `--dry-run`, `--config`.

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

Subreddit targeting lives on the ad group, and the API replaces the whole targeting
object on every write. So add and remove read it first, change the one list you asked
for, and send it back whole. Every other targeting field you had set (gender, interests,
geolocations, and the rest) survives untouched.

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

## Testing

There is no sandbox and no test mode for the Reddit Ads API. Every write hits a live ad
account with real spend. So the client is unit-tested against `httptest` fixtures, the
money conversion, targeting merge, pagination, OAuth exchange, and the write commands
(budget microcurrency, targeting read-modify-write, confirmation) all have real tests,
and any live smoke test should run against a paused campaign with `--dry-run` first.

Run the same checks CI runs:

```
gofmt -l internal/ cmd/     # prints nothing when clean
go vet ./...
go test -race ./...
golangci-lint run ./...     # config in .golangci.yml
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The short version: no live-API tests, keep the
money/targeting/pagination tests green, and every write keeps its `--dry-run` and
confirmation behavior.

## License

[MIT](LICENSE) &copy; Mattias Geniar.

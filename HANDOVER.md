# Handover: build `reddit-ads`, a CLI for the Reddit Ads API v3

Written 2026-07-25 from a research session on code.ma.ttias.be. The repo was empty
before this commit. Everything here is verified against the live Reddit Ads API docs and
their OpenAPI document, not recalled from memory. Read [CLAUDE.md](CLAUDE.md) alongside
this, it holds the API facts and conventions you would otherwise have to rediscover.

## Why

I want to manage Reddit ads programmatically: change subreddit targeting, adjust budgets
and bids, pause and resume, and pull click/impression numbers, without clicking through
Ads Manager. The Reddit Ads API v3 covers all of it. There is no official CLI and no
official MCP server. Third-party MCPs exist but they broker your OAuth token through
someone else's infrastructure, and this is ad spend, so we build our own.

## What is already decided

- **Go**, binary `reddit-ads`, module path `github.com/mattiasgeniar/reddit-ads-cli`.
- **v1 = read + safe writes.** Reporting and listing, pause/resume, budget and bid,
  ad group targeting. No creative creation, no campaign creation.
- Credentials in `~/.config/reddit-ads/config.json` (600), token cache in a separate
  `token.json`, env vars override both.
- CLI plus a Claude skill, not an MCP server.

Toolchain is already installed on this box: `go1.26.0` at `/usr/bin/go`, system-wide
from the Ubuntu repo, so a systemd-spawned T3 session sees it without a shell profile.

## Build order

1. **Scaffold.** `go mod init github.com/mattiasgeniar/reddit-ads-cli`, cobra, a
   `cmd/reddit-ads/main.go`, `internal/config`, `internal/api`, `internal/cmd`. A
   Makefile with `build`, `test`, `install` (into `~/bin`).
2. **Config + token store.** Load/merge config file and env, 600 permissions enforced on
   write, separate token cache with expiry.
3. **`auth login` / `auth status`.** Print the authorize URL, read the pasted code from
   stdin, exchange it, persist the refresh token. `auth status` shows which account the
   token belongs to via `GET /me` and when the access token expires.
4. **HTTP client.** Bearer transport that refreshes on 401 and on a stale expiry,
   `next_url` pagination, 429 handling with exponential backoff that reads the
   `RateLimit` header's reset value, honest `User-Agent`, `--dry-run` short-circuit that
   prints method, URL and body instead of sending.
5. **Read commands.** `accounts list`, `campaigns list|get`, `adgroups list|get`,
   `ads list|get`, `communities search|suggest`. Table output by default, `--json` raw.
6. **`report`.** The one command that earns its keep on day one. Wraps
   `POST /ad_accounts/{id}/reports`: `--from`, `--to`, `--fields`, `--breakdowns`,
   `--tz`. Sensible default fields (`IMPRESSIONS,CLICKS,CTR,SPEND,CPC`) and a
   `--by community` style shorthand for the common breakdowns. Converts microcurrency
   to dollars on the way out.
7. **Safe writes.** `campaigns pause|resume|budget`, `adgroups pause|resume|bid|budget`,
   `ads pause|resume|click-url`, and `adgroups targeting show|add-community|
   remove-community|exclude-community`. Confirmation prompt unless `--yes`, `--dry-run`
   on every one of them. Targeting edits are read-modify-write because `targeting`
   replaces wholesale.
8. **Skill.** `~/.claude/skills/reddit-ads/SKILL.md` in the same shape as the existing
   `ohdear` and `flare` skills, so an agent can drive the CLI without guessing flags.
9. **README** with the one-time auth walkthrough, and tests for the money conversion,
   targeting merge and pagination.

Commit per iteration on a branch, as usual. Do not push or open the PR until asked.

## v1 command surface

```
reddit-ads auth login|status|refresh
reddit-ads accounts list
reddit-ads campaigns list|get|pause|resume|budget
reddit-ads adgroups  list|get|pause|resume|bid|budget
reddit-ads adgroups  targeting show|add-community|remove-community|exclude-community
reddit-ads ads       list|get|pause|resume|click-url
reddit-ads communities search|suggest
reddit-ads report --from --to --fields --breakdowns
```

Global flags: `--json`, `--account`, `--profile`, `--yes`, `--dry-run`, `--config`.

## Open questions to settle before or during step 3

- **The developer app does not exist yet.** Someone with business admin rights and a
  verified account has to create it in Ads Manager under Business > Developer Portal and
  note the app id, secret and redirect URL. The redirect URL never has to serve anything,
  the code is copied out of the address bar by hand, but it must match exactly what is
  registered. Nothing past step 3 can be verified live until this exists.
- Which ad account and profile to use as defaults, and whether there is a paused campaign
  safe to smoke-test writes against.

## Where the research came from

- Official docs: https://ads-api.reddit.com/docs/v3/
- Auth guide: https://ads-api.reddit.com/docs/v3/guides/quick-start/authenticate
- Dev app guide: https://ads-api.reddit.com/docs/v3/guides/quick-start/create-dev-app
- Rate limits: https://ads-api.reddit.com/docs/v3/guides/quick-start/rate-limiting
- Unofficial OpenAPI 3.1 document, useful and accurate everywhere I checked it but not
  authoritative: https://raw.githubusercontent.com/pooriaarab/clis/main/reddit-ads/spec.json

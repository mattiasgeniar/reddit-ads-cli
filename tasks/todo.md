# Build reddit-ads CLI v1

- [x] 1. Scaffold: go.mod, cobra, cmd/reddit-ads/main.go, internal/{config,api,cmd}, Makefile
- [x] 2. Config + token store (600 perms, env override, separate token cache)
- [x] 3. auth login/status/refresh
- [x] 4. HTTP client (bearer refresh, pagination, 429 backoff, User-Agent, --dry-run)
- [x] 5. Read commands: accounts, campaigns, adgroups, ads, communities
- [x] 6. report (POST reports, microcurrency conversion, breakdown shorthands)
- [x] 7. Safe writes: pause/resume/budget/bid/click-url + targeting RMW
- [x] 8. Skill: skill/reddit-ads/SKILL.md (installed to ~/.claude/skills/reddit-ads/)
- [x] 9. README + tests (money conversion, targeting merge, pagination, report)

## Verified so far
- `go build ./...`, `go vet ./...`, `go test ./...` all clean.
- Binary help renders; dry-run writes print correct PATCH bodies with correct
  microcurrency ($50 -> 50000000, $0.50 -> 500000).
- Authorize URL carries scope=adsread,adsedit and duration=permanent.

## Blocked on live verification
- Dev app does not exist yet. Nothing that hits the live API (auth login exchange,
  reads, report, real writes) can be smoke-tested until a business admin creates the
  developer app and we have client id/secret + a default ad account.

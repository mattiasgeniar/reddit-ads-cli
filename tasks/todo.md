# Build reddit-ads CLI v1

- [ ] 1. Scaffold: go.mod, cobra, cmd/reddit-ads/main.go, internal/{config,api,cmd}, Makefile
- [ ] 2. Config + token store (600 perms, env override, separate token cache)
- [ ] 3. auth login/status/refresh
- [ ] 4. HTTP client (bearer refresh, pagination, 429 backoff, User-Agent, --dry-run)
- [ ] 5. Read commands: accounts, campaigns, adgroups, ads, communities
- [ ] 6. report (POST reports, microcurrency conversion, breakdown shorthands)
- [ ] 7. Safe writes: pause/resume/budget/bid/click-url + targeting RMW
- [ ] 8. Skill: ~/.claude/skills/reddit-ads/SKILL.md
- [ ] 9. README + tests (money conversion, targeting merge, pagination)

## Notes
- No sandbox: unit-test against fixtures only. Live writes only vs paused campaign.
- Money: 1e6 microcurrency, except CONVERSION*TOTAL_VALUE which is /100.
- Dev app does not exist yet -> steps 4+ not live-verifiable until it does.

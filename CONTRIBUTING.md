# Contributing to reddit-ads

Thanks for taking the time. This is a small, focused CLI, so the bar is simple: keep it
correct, keep it tested, and do not surprise anyone who is spending real money through it.

## The one rule that matters most

There is no sandbox and no test mode for the Reddit Ads API. Every write touches a live
ad account with real spend, and money is microcurrency, so a factor-of-1e6 slip is a real
budget bug. Because of that:

- **Never test against the live API in this repo.** Unit tests run against `httptest`
  servers and recorded shapes, never against `ads-api.reddit.com`.
- Money conversion, targeting read-modify-write, and pagination are the load-bearing
  paths. If you touch them, the tests around them must stay green and, ideally, grow.
- Any live smoke testing you do yourself runs against a paused campaign, with `--dry-run`
  first.

## Development

```bash
make build      # ./reddit-ads
make test       # go test ./...
make fmt        # gofmt -w
make install    # into ~/bin
```

Before opening a pull request, run the same gate CI runs:

```bash
gofmt -l internal/ cmd/          # must print nothing
go vet ./...
go test -race ./...
golangci-lint run ./...          # config in .golangci.yml
```

`golangci-lint` is not required to be installed for `make test`, but CI runs it and PRs
are expected to pass it. Install it from https://golangci-lint.run.

## Style

- Standard Go: `gofmt`, `go vet`, and the linters in `.golangci.yml` are the style guide.
- Prefer early returns over nested conditionals.
- Comments explain the non-obvious *why*, not the *what*. Names carry the what.
- No em-dashes in code, output, or docs.
- New commands support `--json`, and every write supports `--dry-run` and confirms unless
  `--yes` is passed. Keep that contract.

## Pull requests

- One focused change per PR. Keep the commit history readable.
- Add or update tests for behavior you change. A PR that changes money, targeting, or
  pagination without a test will be asked for one.
- Describe what changed and why in a sentence or two. Link an issue if there is one.

## Reporting bugs

Open an issue with the command you ran (redact ids and tokens), what you expected, and
what happened. If it involves a write, include the `--dry-run` output, never real
credentials.

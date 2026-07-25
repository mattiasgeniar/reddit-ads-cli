package config

// Version is the CLI version, stamped into the User-Agent and shown by `--version`.
// It is a var so a release build can inject the git tag via
// -ldflags "-X .../internal/config.Version=v1.2.3".
var Version = "0.1.0"

// DefaultUserAgent is Reddit's required descriptive form:
// <platform>:<app id>:<version> (by /u/<username>). Reddit bans clients that spoof
// browsers. Users can override with user_agent in config or REDDIT_ADS_USER_AGENT,
// ideally adding their own /u/<username>. It is computed from Version at init, so an
// injected release version flows through to the User-Agent too.
var DefaultUserAgent = "reddit-ads-cli:github.com/mattiasgeniar/reddit-ads-cli:" + Version + " (by /u/unknown)"

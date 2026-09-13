# Changelog

## Windows 0.1.0 — 2026-09-13

- Native Windows CLI launches and npm executable discovery.
- Windows account directory ACLs and case-insensitive project identity.
- PowerShell installer, isolated integration check, and Windows CI.
- Separate source-only repository; the Linux desktop launcher is unsupported.

The entries below describe the original Easy-G project before this Windows port.

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and
this project uses [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-08-17

First release.

### Added

- Terminal account switcher: sign in, switch, launch, add, rename, enable,
  disable, and sign out, one keypress each. Opens with a bare `codexr`.
- Device-code sign-in rendered in the UI, with the account you are signing into
  named on screen so a slot cannot be filled by the wrong account by accident.
- Quota-aware routing that spends the allowance closest to expiring first, with
  a bounded bonus for banked usage resets.
- Per-directory account assignment, so a project keeps its account across
  launches and only moves when that account runs dry.
- Full command-line equivalents for every UI action, plus `pick`, `serve`, and
  `doctor`.
- `scripts/install.sh`, a `--dry-run`-first installer that stays inside `$HOME`,
  needs no sudo, and leaves the `codex` on `PATH` alone.
- Codex Desktop integration: switch the running app to any connected account
  (`d` in the UI, `codexr desktop`). Because the app reads `CODEX_HOME` only at
  launch, switching restarts it — gracefully, with a twenty-second grace period
  before anything harder than `SIGTERM`. `D` / `--new-window` opens an
  additional instance instead, and `--stop` closes every window. Uses the
  launcher's own `CODEX_HOME` and `--new-instance` support, so nothing is
  patched and nothing is written outside `$HOME`.
- An `APP` column showing which accounts already have a desktop window open,
  detected from the running processes rather than a state file that can go
  stale.
- Isolated account homes: credentials, history, and databases never mix.

### Fixed

- The switcher no longer gets painted over by its own subprocesses. Child
  app-servers log on their own schedule; that output went to the terminal and
  drew straight through the full-screen UI, leaving a broken half-black screen.
  It now goes to `~/.codex-mux/logs/codexr.log`, which `codexr doctor` points
  at. `codexr serve` still logs to stderr, where a foreground service belongs.

### Derived from

- The routing engine comes from
  [b-nnett/codex-subscription-router](https://github.com/b-nnett/codex-subscription-router)
  (MIT), with its macOS app-patching and code-signing layer removed. See
  [NOTICE.md](NOTICE.md).

[0.1.0]: https://github.com/mika2go/easy-g-account-switch/releases/tag/v0.1.0

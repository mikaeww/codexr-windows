# Contributing

Thanks for taking a look. Easy-G is small on purpose — a routing engine, a
terminal UI, and a thin CLI over both.

## Getting set up

```sh
go build ./...
go test ./...
```

You need the Codex CLI installed to run anything against a real account, but
the tests do not touch the network or your accounts.

## Before you open a pull request

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
```

On Windows, build `dist/codexr.exe` and run `scripts/test-windows.ps1 -BinaryPath dist/codexr.exe` against a separately installed Codex CLI.

CI runs exactly these.

## House style

- No comments that restate the code. A comment should explain *why* a
  non-obvious constraint exists — a protocol quirk, a footgun someone already
  stepped in. Look at the existing ones for the bar.
- Match the surrounding file rather than introducing a new pattern.
- Keep the routing algorithm and the UI separate. `internal/mux` must not know
  a terminal exists; `internal/tui` must not talk to an app-server directly.

## Adding a new account backend

This is the most useful contribution right now — GitHub is first on the
roadmap. The shape to aim for:

1. The backend owns discovering, listing, and switching accounts for one
   service.
2. It reports a snapshot the UI can render without knowing what the service is.
3. Switching is a single call that either succeeds or returns a real error.
   Never report success for a no-op.

Open an issue describing the service before writing much code, so we can agree
on the seam first.

## Safety rules

These are not negotiable, because getting them wrong costs someone their data
or their credentials:

- Never replace a user's durable state with empty state after a parse error.
  Snapshot first, fail loudly second.
- Never log, print, or return OAuth tokens, device codes, or the control token.
- Never report success for a fallback that did nothing. An unsupported path
  must fail clearly.
- Credentials belong to exactly one account home and never move between them.

## Reporting a security issue

Please don't open a public issue for a suspected credential leak, control-API
authentication flaw, or arbitrary code execution path. See
[SECURITY.md](SECURITY.md).

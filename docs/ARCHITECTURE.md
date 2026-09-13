# Architecture

Two entry points share one routing core.

```
                    ┌──────────────────────────────┐
                    │ internal/mux                 │
                    │ urgency scoring · ownership  │
                    │ failover · reset credits     │
                    └───────┬──────────────┬───────┘
      app-server clients    │              │   terminal use
            ┌───────────────┴──┐        ┌──┴──────────────────┐
            │ cmd/codex-mux    │        │ cmd/codexr          │
            │ stdio JSON-RPC   │        │ accounts · login    │
            │ + control API    │        │ pick · run · serve  │
            └──────────────────┘        └─────────────────────┘
                     │                            │
              N × `codex app-server`      1 × `codex` (real CLI)
              one per CODEX_HOME          on the chosen CODEX_HOME
```

## Request routing

An app-server client opens one JSON-RPC connection to the multiplexer. The
multiplexer starts one real app-server child for every enabled account, each
with its own `CODEX_HOME` and `CODEX_SQLITE_HOME`.

New threads are assigned using a quota-urgency score: weekly percentage
remaining divided by the hours until that account resets. Banked usage resets
add a capped bonus, while short-window usage, existing pinned-thread count, and
stable account order break close results. Reset-credit metadata is fetched in
parallel, cached for five minutes, and treated as neutral when unavailable.
Once a thread ID is known, `state.json` persists its owner. Requests, responses,
approvals, and notifications are rewritten only as needed to preserve one
coherent session.

If the owner is depleted, the multiplexer resumes the rollout on an account
with capacity and updates ownership. Threads do not migrate for ordinary load
balancing.

## Command-line routing

`cmd/codexr` cannot sit in front of the interactive CLI, which does not speak
the app-server protocol. It instead runs the multiplexer headlessly — children
started, handshake performed, no stdio ownership — asks it which account would
win, shuts the children down, and replaces its own process image with the real
`codex` under the chosen `CODEX_HOME`.

Because that decision happens before any thread exists, the launcher pins on the
working directory rather than a thread ID. `state.json` carries both maps:
`threadOwner` for the app-server path and `projectOwner` for the launcher path.
The file format is unchanged from upstream — `projectOwner` is optional, so an
upstream state file still loads.

Depletion is handled at both ends: the launcher re-routes when a pinned
directory's account has no capacity left, and the app-server path fails a turn
over mid-flight.

## Account isolation

The Primary account uses `~/.codex`. Added accounts use
`~/.codex-mux/accounts/<id>/codex-home`. Managed configuration is copied from
the Primary account, excluding credential-store settings and project trust.
Each isolated account forces file-backed CLI and MCP OAuth credentials.

`codexr` deliberately resolves the Primary account from
`CODEX_MUX_PRIMARY_CODEX_HOME` or `~/.codex`, never from `CODEX_HOME` — a
`codexr` invoked inside a routed session inherits that session's `CODEX_HOME`,
and following it would register a secondary account as the primary one.

## Control API

`cmd/codex-mux` and `codexr serve` both expose a loopback-only HTTP service on
port 48123. All private routes require a random 256-bit token read from
`~/.codex-mux/control-token`. The service exposes account metadata, aggregated
usage and profile data, thread ownership, login and logout actions, and an
authenticated SSE event stream; it never returns OAuth tokens.

## Removed with the macOS delivery layer

The ASAR patcher, injected renderer UI, native launcher, Computer Use helper
re-signing, bundle identifiers, and Chromium profile separation are all gone.
Nothing in the Go tree referenced them — the port removed delivery code, not
routing code.

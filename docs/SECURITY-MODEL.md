# Security model

## Trust boundaries

- The Codex CLI is trusted input and is never modified; the router only spawns
  it with a different `CODEX_HOME`.
- Each real Codex child is trusted with only its assigned account home.
- Whoever can read `~/.codex-mux/control-token` can drive the control API.
- Other local users and remote origins are outside the control API boundary.
- Processes running as the same operating-system user are not considered isolated from one
  another; they can already read that user's files.

## Credentials

OAuth material stays in `auth.json` under each account's Codex home. The
multiplexer reads an account token only to call the same authenticated ChatGPT
profile and rate-limit-reset endpoints the official client uses. It does not log
or return tokens. State persisted by the router contains account paths, labels,
enabled state, thread ownership, and project ownership only.

On Windows, the state root and isolated account directories use a protected,
inheritable ACL granting full access to the current user and SYSTEM. On Linux,
directories use mode `0700` and files use `0600`. Existing control tokens are
validated as 256-bit hexadecimal values.

Plugin and MCP configuration is deliberately synchronized from the Primary
account so installed definitions stay consistent. Inline environment values
inside those definitions are therefore copied into every isolated account home
with mode `0600`; account isolation is not a separate secret boundary for
shared plugin configuration.

## Network

The control server binds to `127.0.0.1`. Private endpoints require the token in
`~/.codex-mux/control-token`. Profile images must use HTTPS. Response sizes and
JSON request bodies are bounded.

The project itself does not provide a telemetry or update endpoint. Network
traffic beyond loopback is performed by the Codex children or by the documented
ChatGPT profile and rate-limit APIs.

## Process handling

`codexr run` replaces its own process image with the real Codex CLI. It stops
every app-server child and waits for exit first, so a routed launch leaves no
orphaned subscription process behind.

## Diagnostics

`CODEX_MUX_UI_TESTS=1` enables deterministic preview endpoints on `cmd/codex-mux`.
They are unavailable during a normal launch, bind only to loopback, and require
the same control token.

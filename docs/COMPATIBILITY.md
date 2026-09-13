# Compatibility

Easy-G depends on the Codex CLI app-server protocol, not on any desktop
application bundle. There is no ASAR hash, no bundle version, and no signing
identity to verify — the only moving target is the `codex` binary itself.

## Verified

| Component | Tested value |
| --- | --- |
| `codex-cli` | `0.154.0`, official `x86_64-pc-windows-msvc` release |
| Platform | Windows 11, build 26200, `windows/amd64` in a local QEMU VM |
| Checks | Go test executables, non-admin installation and reinstall, doctor, account add/rename/enable/disable, routed CLI launch |

The checks used empty temporary account homes. Interactive terminal rendering,
live account login, quota refresh, and authenticated conversations require a
user account and were not exercised in the automated VM check.

## Required app-server methods

The multiplexer breaks if any of these disappear from the protocol. Check with
`codex app-server generate-json-schema --out <dir>` after a CLI upgrade.

| Method | Used for |
| --- | --- |
| `initialize` | child handshake, one per subscription |
| `account/read` | email, plan type, connection state |
| `account/rateLimits/read` | weekly and short-window usage for scoring |
| `account/login/start` | device-code sign-in (`chatgptDeviceCode`) |
| `account/login/completed` | notification that a sign-in landed |
| `account/logout` | disconnecting a subscription |
| `account/rateLimitResetCredit/consume` | redeeming a banked reset |
| `thread/start`, `thread/read`, `thread/resume` | routing and failover |
| `thread/fork`, `thread/unarchive`, `thread/list` | ownership learning, aggregation |
| `turn/start` | the request that triggers depletion failover |

## Required environment isolation

Each subscription runs a child with its own `CODEX_HOME` and `CODEX_SQLITE_HOME`.
`initialize` echoes the effective home back in its result, which is the cheapest
way to confirm isolation still works after an upgrade:

```sh
codexr accounts
```

reports a per-account error instead of a quota row when a child fails to start.

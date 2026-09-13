# Security policy

## Reporting

Do not open a public issue for a suspected credential leak, control-API
authentication flaw, or arbitrary code execution path. Use GitHub's private
vulnerability reporting on this repository instead.

Never include real access tokens, device codes, `auth.json` contents, control
tokens, or private conversation content in a report. An account ID and a
description of the path are enough to reproduce.

## Scope

In scope: the multiplexer, the terminal UI, the CLI, the control API, isolated
account state, and the installer.

Out of scope: the Codex CLI itself and OpenAI's services. Report those to
OpenAI directly.

## Model

See [docs/SECURITY-MODEL.md](docs/SECURITY-MODEL.md) for trust boundaries,
credential handling, and network exposure.

# codexr for Windows

A native Windows terminal account switcher and subscription router for the Codex CLI. Keep accounts in separate Codex homes, see five-hour and weekly usage, and launch Codex with the account assigned to your project.

This is a separate Windows port of Easy-G. No WSL is required. It does not include accounts, credentials, desktop settings, or the Codex CLI itself.

## Install

Requires Windows 11 x64 and a separately installed Codex CLI. Install Node.js LTS if you do not already have npm, then run in PowerShell:

```powershell
npm.cmd install -g @openai/codex
codex.cmd --version
```

Download `codexr-windows-x64.zip` from [Releases](https://github.com/mikaeww/codexr-windows/releases/latest), extract it, and open PowerShell in that folder:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

The installer copies `codexr.exe` to `%LOCALAPPDATA%\Programs\codexr` and adds that directory to your user PATH. Administrator rights are not needed. Open a new terminal after installation:

```powershell
codexr doctor
codexr
```

You can also run `codexr.exe` directly from the extracted folder. The executable is unsigned; Windows may show a reputation warning.

## Use

Your existing `%USERPROFILE%\.codex` account appears as `primary`. To add another account:

```powershell
codexr add Work
codexr login Work
codexr accounts
cd C:\Projects\my-project
codexr run
```

Complete the device-code sign-in using your own account. In the terminal UI, use arrow keys to select an account, Enter to assign it to the current project, `s` to start Codex, `a` to add an account, `l` to sign in, and `q` to quit.

```powershell
codexr run --account Work
codexr run --new
codexr run -- --help
codexr pick
codexr disable Work
codexr enable Work
codexr help
```

Arguments after `--` pass through to Codex unchanged. Codex keeps its normal approval and sandbox settings. This port does not force full access.

## Data and troubleshooting

- Routing state and isolated accounts: `%USERPROFILE%\.codex-mux`.
- Existing primary account: `%USERPROFILE%\.codex`.
- Diagnostic log: `%USERPROFILE%\.codex-mux\logs\codexr.log`.
- Isolated account directories receive a Windows ACL allowing the current user and SYSTEM. Credentials are not encrypted by this application.
- Settings from the primary `config.toml` are shared with isolated accounts; authentication and project trust remain separate. Do not publish your state directory or logs.

Standard global npm installations are detected automatically. For other installations, point to the actual native executable:

```powershell
$env:CODEX_MUX_REAL_CODEX = 'C:\Tools\Codex\codex.exe'
codexr doctor
```

`CODEX_MUX_HOME` overrides the routing directory; `CODEX_MUX_PRIMARY_CODEX_HOME` overrides the primary account directory. Use private local directories for both. Keep only one codexr process per state directory: the inherited state store does not coordinate concurrent writers.

The original Linux desktop launcher and its `d`/`D` shortcuts are **not supported on Windows**. Use `s` or `codexr run` for the native CLI. Account switching and routing do not increase an account's quota. Live sign-in requires your own account and network access.

To update, extract a newer release and run its installer again after closing codexr. To uninstall, remove `%LOCALAPPDATA%\Programs\codexr` and its entry from your user PATH. Account data remains in `.codex-mux`; keep or back it up as needed.

## Build and test

With Go 1.26 or newer installed, from this source directory:

```powershell
go test ./...
go vet ./...
go build -trimpath -buildvcs=false -o dist/codexr.exe ./cmd/codexr
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -BinaryPath .\dist\codexr.exe
```

For an isolated integration check against your installed Codex CLI:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-windows.ps1 -BinaryPath .\dist\codexr.exe
```

The check uses temporary empty account directories and does not log into your accounts. CI runs the Go suite on Windows and Linux. The inherited `codex-mux` app-server wrapper is available from source for advanced integrations; set `CODEX_MUX_REAL_CODEX` to a native executable when using it.

## License

MIT; see [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md). Based on Easy-G and [b-nnett/codex-subscription-router](https://github.com/b-nnett/codex-subscription-router). Not affiliated with OpenAI.

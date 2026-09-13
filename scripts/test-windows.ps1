param([Parameter(Mandatory=$true)][string]$BinaryPath)
$ErrorActionPreference = 'Stop'
$BinaryPath = (Resolve-Path -LiteralPath $BinaryPath).Path
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('codexr-test-' + [guid]::NewGuid())
$previousRoot = $env:CODEX_MUX_HOME
$previousPrimary = $env:CODEX_MUX_PRIMARY_CODEX_HOME
try {
    $env:CODEX_MUX_HOME = Join-Path $testRoot 'router'
    $env:CODEX_MUX_PRIMARY_CODEX_HOME = Join-Path $testRoot 'primary'
    New-Item -ItemType Directory -Path $env:CODEX_MUX_PRIMARY_CODEX_HOME -Force | Out-Null
    foreach ($arguments in @(@('version'), @('doctor'), @('add', 'Test Account'), @('accounts'), @('rename', 'Test Account', 'Renamed'), @('disable', 'Renamed'), @('enable', 'Renamed'), @('run', '--account', 'Renamed', '--', '--version'))) {
        & $BinaryPath @arguments
        if ($LASTEXITCODE -ne 0) { throw "Failed: codexr $arguments (exit $LASTEXITCODE)" }
    }
    $state = Get-Content -LiteralPath (Join-Path $env:CODEX_MUX_HOME 'state.json') -Raw | ConvertFrom-Json
    if (@($state.accounts).Count -ne 2) { throw 'Expected two isolated accounts' }
    if (-not ($state.accounts | Where-Object { $_.label -eq 'Renamed' -and $_.enabled })) { throw 'Account changes did not persist' }
    Write-Output 'Windows integration checks passed (no account login).'
} finally {
    $env:CODEX_MUX_HOME = $previousRoot
    $env:CODEX_MUX_PRIMARY_CODEX_HOME = $previousPrimary
    if (Test-Path -LiteralPath $testRoot) { Remove-Item -LiteralPath $testRoot -Recurse -Force }
}

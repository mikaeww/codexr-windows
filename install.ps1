param(
    [string]$BinaryPath = (Join-Path $PSScriptRoot 'codexr.exe'),
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\codexr')
)
$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Missing executable: $BinaryPath. Extract a release or build dist/codexr.exe and pass -BinaryPath."
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -LiteralPath $BinaryPath -Destination (Join-Path $InstallDir 'codexr.exe') -Force
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | Where-Object { $_ })
if ($entries -notcontains $InstallDir) {
    [Environment]::SetEnvironmentVariable('Path', (($entries + $InstallDir) -join ';'), 'User')
}
Write-Output "Installed to $InstallDir. Open a new terminal and run: codexr doctor"

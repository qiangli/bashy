# Platform-aware whisper.cpp v1.9.5 source build + cache recipe for Windows runners
# Supports: windows-2025 (windows/amd64), windows-11-arm (windows/arm64)
#
# Sprint #503 / Story #2742 / Story-ID 74ed924ba244

[CmdletBinding()]
param(
    [string]$WorkDir,
    [string]$CacheDir,
    [string]$DestCli,
    [string]$SourceArchive,
    [switch]$SkipDownload,
    [int]$Parallel = 8,
    [switch]$Json,
    [string]$ReportFile
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$pyScript = Join-Path $scriptDir 'build_whisper.py'

if (-not (Test-Path -LiteralPath $pyScript)) {
    throw "Build script missing: $pyScript"
}

# Resolve python
$pyExe = $null
if (Get-Command python.exe -ErrorAction SilentlyContinue) {
    $pyExe = 'python.exe'
} elseif (Get-Command py.exe -ErrorAction SilentlyContinue) {
    $pyExe = 'py.exe'
} elseif (Get-Command python3.exe -ErrorAction SilentlyContinue) {
    $pyExe = 'python3.exe'
} else {
    throw "Python runtime required on Windows runner"
}

$argsList = @($pyScript)
if ($WorkDir) { $argsList += @('--work-dir', $WorkDir) }
if ($CacheDir) { $argsList += @('--cache-dir', $CacheDir) }
if ($DestCli) { $argsList += @('--dest-cli', $DestCli) }
if ($SourceArchive) { $argsList += @('--source-archive', $SourceArchive) }
if ($SkipDownload) { $argsList += @('--skip-download') }
if ($Parallel) { $argsList += @('--parallel', $Parallel.ToString()) }
if ($Json) { $argsList += @('--json') }
if ($ReportFile) { $argsList += @('--report-file', $ReportFile) }

& $pyExe $argsList
if ($LASTEXITCODE -ne 0) {
    throw "whisper build failed with exit code $LASTEXITCODE"
}

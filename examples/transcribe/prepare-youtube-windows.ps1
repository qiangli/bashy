# Prepare the reviewed S503 YouTube dependencies on a claimed Windows/amd64 host.
# This script does not run transcription or mark the fixture gate passed.
param([string]$WorkRoot = (Join-Path $env:LOCALAPPDATA 'Temp\s503-youtube'))

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$denoVersion = '2.9.8'
$denoArchiveSHA256 = 'cba84b26e0df5ae7f74fc66b86a95b866872a86292ebd84fa1e959927edd93bf'
$denoExecutableSHA256 = '924397800533d3eb0ed0c889b20ad70e18b970d93acd8135bc043f425e528266'
$ytDlpVersion = '2026.10.21'
$ytDlpSHA256 = 'b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14'
$ejsVersion = '0.8.0'

if (-not [Environment]::Is64BitOperatingSystem -or
    [Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne [Runtime.InteropServices.Architecture]::X64) {
    throw 'This pinned preparation script targets native Windows amd64 only.'
}

# A managed CPython archive would bring app-local Microsoft VC DLLs. Require
# an explicit, already installed interpreter with reviewed bytes instead.
if (-not $env:BASHY_TRANSCRIBE_PYTHON) {
    throw 'Set BASHY_TRANSCRIBE_PYTHON to an externally installed, reviewed python.exe.'
}
$runtimePolicy = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'dependencies/windows-external-runtime.json') -Raw | ConvertFrom-Json
$python = (Resolve-Path -LiteralPath $env:BASHY_TRANSCRIBE_PYTHON -ErrorAction Stop).Path
if (-not (Test-Path -LiteralPath $python -PathType Leaf) -or
    [IO.Path]::GetFileName($python).ToLowerInvariant() -ne 'python.exe') {
    throw 'External Python must be a python.exe file.'
}
$pythonSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $python).Hash.ToLowerInvariant()
if ($pythonSHA256 -ne $runtimePolicy.targets.AMD64.python_sha256) {
    throw 'External Python executable SHA-256 does not match the reviewed Windows/amd64 pin.'
}
$runtimeCheck = Join-Path $PSScriptRoot 'dependencies/verify_windows_external_runtime.py'
$externalRuntime = (& $python -I $runtimeCheck | ConvertFrom-Json)
if ($LASTEXITCODE -ne 0 -or -not $externalRuntime.python.sha256) {
    throw 'External Python and System32 VC runtime prerequisite verification failed.'
}

New-Item -ItemType Directory -Force -Path $WorkRoot | Out-Null
$denoArchive = Join-Path $WorkRoot 'deno-x86_64-pc-windows-msvc.zip'
$denoDir = Join-Path $WorkRoot 'deno'
$deno = Join-Path $denoDir 'deno.exe'
$ytDlp = Join-Path $WorkRoot 'yt-dlp'

function Assert-SHA256([string]$Path, [string]$Expected) {
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
    if ($actual -ne $Expected) { throw "SHA-256 mismatch for $Path`: $actual" }
}

if (-not (Test-Path -LiteralPath $denoArchive)) {
    & curl.exe -fL --retry 2 --max-time 180 -o $denoArchive "https://github.com/denoland/deno/releases/download/v$denoVersion/deno-x86_64-pc-windows-msvc.zip"
    if ($LASTEXITCODE -ne 0) { throw "Deno download failed: $LASTEXITCODE" }
}
Assert-SHA256 $denoArchive $denoArchiveSHA256
if (-not (Test-Path -LiteralPath $deno)) {
    New-Item -ItemType Directory -Force -Path $denoDir | Out-Null
    Expand-Archive -LiteralPath $denoArchive -DestinationPath $denoDir -Force
}
Assert-SHA256 $deno $denoExecutableSHA256
$denoVersionOutput = & $deno --version
if ($LASTEXITCODE -ne 0 -or $denoVersionOutput[0] -ne "deno $denoVersion (stable, release, x86_64-pc-windows-msvc)") {
    throw "Unexpected Deno version: $($denoVersionOutput[0])"
}

if (-not (Test-Path -LiteralPath $ytDlp)) {
    & curl.exe -fL --retry 2 --max-time 90 -o $ytDlp "https://github.com/yt-dlp/yt-dlp/releases/download/$ytDlpVersion/yt-dlp"
    if ($LASTEXITCODE -ne 0) { throw "yt-dlp download failed: $LASTEXITCODE" }
}
Assert-SHA256 $ytDlp $ytDlpSHA256

# Python's zipfile accepts the official zipapp's shell preamble.
$readEjsVersion = @'
import re, sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as archive:
    data = archive.read('yt_dlp_ejs/_version.py').decode('utf-8')
match = re.search(r"^__version__ = version = '([^']+)'$", data, re.M)
if not match:
    raise SystemExit('yt-dlp-ejs version is missing')
print(match.group(1))
'@
$ejsProbe = Join-Path $WorkRoot 'read-ejs-version.py'
Set-Content -LiteralPath $ejsProbe -Value $readEjsVersion -Encoding UTF8
$embeddedEjsVersion = (& $python -I $ejsProbe $ytDlp).Trim()
if ($LASTEXITCODE -ne 0 -or $embeddedEjsVersion -ne $ejsVersion) {
    throw "Unexpected bundled yt-dlp-ejs version: $embeddedEjsVersion"
}
$ytDlpVersionOutput = (& $python -I $ytDlp --version).Trim()
if ($LASTEXITCODE -ne 0 -or $ytDlpVersionOutput -ne $ytDlpVersion) {
    throw "Unexpected yt-dlp version: $ytDlpVersionOutput"
}

[pscustomobject]@{
    platform = 'windows/amd64'
    deno_version = $denoVersion
    deno_archive_sha256 = $denoArchiveSHA256
    deno_executable_sha256 = $denoExecutableSHA256
    deno_path = $deno
    yt_dlp_version = $ytDlpVersionOutput
    yt_dlp_sha256 = $ytDlpSHA256
    yt_dlp_ejs_version = $ejsVersion
    yt_dlp_path = $ytDlp
    python_path = $python
    external_runtime = $externalRuntime
} | ConvertTo-Json -Compress

# Real S503.6 whisper.cpp standalone verification on a claimed Windows/amd64 host.
# Validates native execution under clean system PATH, speech inference accuracy,
# and a repeat using cached files. It does not enforce network denial.
param(
    [string]$WorkRoot = (Join-Path $env:LOCALAPPDATA 'Temp\s503-whisper')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$prepare = Join-Path $PSScriptRoot 'prepare-whisper-windows.ps1'
if (-not (Test-Path -LiteralPath $prepare)) { throw "Preparation script missing: $prepare" }
$prepared = (& $prepare -WorkRoot $WorkRoot | ConvertFrom-Json)

$cli = $prepared.cli_path
$model = $prepared.model_path
$wav = $prepared.sample_wav_path

if (-not (Test-Path -LiteralPath $cli)) { throw "Cached CLI missing: $cli" }
if (-not (Test-Path -LiteralPath $model)) { throw "Cached model missing: $model" }
if (-not (Test-Path -LiteralPath $wav)) { throw "Sample WAV missing: $wav" }

$wavSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $wav).Hash.ToLowerInvariant()
$wavBytes = (Get-Item -LiteralPath $wav).Length

# Clean system PATH execution
$cleanPath = 'C:\Windows\system32;C:\Windows'
$origPath = $env:PATH
$env:PATH = $cleanPath

# Initial inference
$inferSw = [System.Diagnostics.Stopwatch]::StartNew()
$inferRaw = & $cli -m $model -f $wav -nt --no-prints
$inferExit = $LASTEXITCODE
$inferDurationMs = $inferSw.ElapsedMilliseconds

if ($inferExit -ne 0) {
    $env:PATH = $origPath
    throw "Initial inference failed with exit code $inferExit"
}

$transcript = ($inferRaw -join ' ').Trim()
if ($transcript -notmatch 'ask not what your country can do for you') {
    $env:PATH = $origPath
    throw "Inference output did not match JFK reference speech: $transcript"
}

# Cached repeat test; OS-enforced network denial is a separate native gate.
$repeatSw = [System.Diagnostics.Stopwatch]::StartNew()
$repeatRaw = & $cli -m $model -f $wav -nt --no-prints
$repeatExit = $LASTEXITCODE
$repeatDurationMs = $repeatSw.ElapsedMilliseconds
$env:PATH = $origPath

if ($repeatExit -ne 0) { throw "Cached repeat inference failed with exit code $repeatExit" }
$repeatTranscript = ($repeatRaw -join ' ').Trim()
if ($repeatTranscript -ne $transcript) { throw "Cached repeat transcript diverged!" }

[pscustomobject]@{
    platform = $prepared.platform
    host = $prepared.host
    os_caption = $prepared.os_caption
    os_version = $prepared.os_version
    architecture = $prepared.architecture
    whisper_version = $prepared.whisper_version
    source_archive_sha256 = $prepared.source_archive_sha256
    cli_path = $cli
    cli_sha256 = $prepared.cli_sha256
    cli_bytes = $prepared.cli_bytes
    imported_dlls = $prepared.imported_dlls
    model_name = $prepared.model_name
    model_sha256 = $prepared.model_sha256
    model_bytes = $prepared.model_bytes
    sample_wav = 'samples/jfk.wav'
    sample_wav_sha256 = $wavSHA256
    sample_wav_bytes = $wavBytes
    transcript = $transcript
    inference_duration_ms = $inferDurationMs
    cached_repeat_duration_ms = $repeatDurationMs
    clean_path_verified = $true
    cached_repeat_match = $true
} | ConvertTo-Json -Compress

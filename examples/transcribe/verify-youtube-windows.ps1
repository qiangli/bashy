# Real S503.5 acquisition gate on a claimed Windows/amd64 host.
# Keep result JSON and downloaded media under WorkRoot for later inference.
param(
    [string]$WorkRoot = (Join-Path $env:LOCALAPPDATA 'Temp\s503-youtube'),
    [string]$SourcePath = (Join-Path $PSScriptRoot 's503-source_youtube.py')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not (Test-Path -LiteralPath $SourcePath)) { throw "Adapter missing: $SourcePath" }
$prepare = Join-Path $PSScriptRoot 's503-prepare.ps1'
if (-not (Test-Path -LiteralPath $prepare)) { throw "Preparation script missing: $prepare" }
$prepared = (& $prepare | ConvertFrom-Json)
$env:BASHY_TRANSCRIBE_JS_RUNTIME = $prepared.deno_path
$env:BASHY_TRANSCRIBE_YTDLP = $prepared.yt_dlp_path
$env:BASHY_TRANSCRIBE_PYTHON = $prepared.python_path

$cases = @(
    @{ name = 'canonical'; url = 'https://www.youtube.com/watch?v=_j_PixI3Muo' },
    @{ name = 'shortened'; url = 'https://youtu.be/_j_PixI3Muo?si=if_lqvXvU5GaM1nb' }
)
$results = @()
foreach ($case in $cases) {
    $work = Join-Path $WorkRoot ('acquire-' + $case.name)
    New-Item -ItemType Directory -Force -Path $work | Out-Null
    $requestPath = Join-Path $work 'request.json'
    $resultPath = Join-Path $work 'result.json'
    $stderrPath = Join-Path $work 'stderr.log'
    $request = @{
        version = 'bashy-transcribe-stage-v1'
        stage = 'acquire'
        input = $case.url
        work_dir = $work
    } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($requestPath, $request, [Text.UTF8Encoding]::new($false))
    # Windows PowerShell 5 turns native stderr into an ErrorRecord under Stop.
    $ErrorActionPreference = 'Continue'
    $raw = & $prepared.python_path -I $SourcePath $requestPath 2> $stderrPath
    $status = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    [IO.File]::WriteAllText($resultPath, (($raw -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
    if ($status -ne 0) { throw "$($case.name) acquisition failed (exit $status); see $stderrPath and $resultPath" }
    $result = $raw | ConvertFrom-Json
    if ($result.PSObject.Properties['error'] -or $result.media.video_id -ne '_j_PixI3Muo' -or
        $result.media.duration_ms -le 0 -or -not $result.media.format_id) {
        throw "$($case.name) acquisition returned incomplete provenance: $resultPath"
    }
    $media = $result.artifacts[0]
    if (-not (Test-Path -LiteralPath $media.path)) { throw "Downloaded media missing: $($media.path)" }
    $actualSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $media.path).Hash.ToLowerInvariant()
    if ($actualSHA256 -ne $media.sha256) { throw "Downloaded media SHA-256 mismatch: $($media.path)" }
    $results += [pscustomobject]@{
        variant = $case.name
        url = $case.url
        video_id = $result.media.video_id
        format_id = $result.media.format_id
        duration_ms = $result.media.duration_ms
        language = $result.media.language
        media_path = $media.path
        media_sha256 = $actualSHA256
        media_bytes = (Get-Item -LiteralPath $media.path).Length
        result_path = $resultPath
    }
}
$results | ConvertTo-Json -Compress

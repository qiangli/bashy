# Hosted Windows only. Prima no-egress gate on the built-in Windows Defender
# Firewall: disposable outbound BLOCK rules on the exact exercise programs,
# no custom native code.
#
# Enforcement model (per protected executable, by exact file path):
#   - Outbound BLOCK for the exercise interpreter, the CLI, the ring test
#     (and the server executable when present);
#   - loopback is not filtered by Windows Firewall, so the ring/exercise
#     loopback traffic works unimpeded and needs no allowlist.
#
# PASS requires all three at once:
#   1. the negative control (outbound TCP 443 from the blocked interpreter
#      to unroutable TEST-NET-1) fails to connect AND appears as DROP in
#      the firewall dropped-packet log (dropped-packet logging is enabled
#      here and restored afterwards);
#   2. the same-interpreter exercise (single inference, ring test, two-rank
#      engine inference) exits 0 with all three PASS stages and no denial
#      signature in its own output;
#   3. a Get-NetTCPConnection sampler running for the whole gate observes
#      no Established/SynSent connection from an exercise program to a
#      non-loopback peer (the control's own TEST-NET-1 attempt is proven
#      via the firewall log instead, so the sampler ignores it).
# Anything else fails. Rules and logging are removed/restored in a finally
# block, so a failure cannot leave policy behind.
#
# CI run 38473601546 (tag 0.1.2-9): the previous custom WFP-API harness
# segfaulted; three CI rounds had gone to that custom native code. This
# script replaces it with the platform firewall.
param(
    [Parameter(Mandatory = $true)][string]$PythonExe,
    [Parameter(Mandatory = $true)][string]$CliExe,
    [Parameter(Mandatory = $true)][string]$RingExe,
    [string]$ServerExe = '',
    [Parameter(Mandatory = $true)][string]$ExercisePy,
    [Parameter(Mandatory = $true)][string]$ModelFile,
    [Parameter(Mandatory = $true)][string]$OutFile,
    [Parameter(Mandatory = $true)][string]$WorkDir
)

$ErrorActionPreference = 'Stop'

$ControlHost = '192.0.2.1'  # TEST-NET-1, never answers
$ControlPort = 443
$Profiles = @('Domain', 'Public', 'Private')
$RuleName = "Bashy prima no-egress $([guid]::NewGuid())"
$Evidence = New-Object System.Collections.ArrayList

function Note([string]$line) {
    Write-Output $line
    [void]$Evidence.Add($line)
}

function Fail([string]$message) {
    # -ErrorAction Continue: under the script's Stop preference a bare
    # Write-Error throws, which would skip the evidence append and the
    # failed flag below. Reporting must never throw.
    Write-Error "FAIL: $message" -ErrorAction Continue
    [void]$Evidence.Add("FAIL: $message")
    throw "FAIL: $message"
}

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { Fail 'Defender gate requires an elevated (Administrator) session' }

foreach ($file in @($PythonExe, $CliExe, $RingExe, $ExercisePy, $ModelFile)) {
    if (-not (Test-Path $file)) { Fail "missing gate input: $file" }
}
if (($ServerExe -ne '') -and (-not (Test-Path $ServerExe))) { Fail "missing gate input: $ServerExe" }
if (-not (Test-Path $WorkDir)) { Fail "missing work dir: $WorkDir" }

$programs = @($PythonExe, $CliExe, $RingExe)
if (($ServerExe -ne '') -and (Test-Path $ServerExe)) { $programs += $ServerExe }
$procNames = @()
foreach ($program in $programs) {
    $base = [System.IO.Path]::GetFileName($program)
    $procNames += $base
    $procNames += [System.IO.Path]::GetFileNameWithoutExtension($program)
}
$procNames += @('python', 'python3')

$samplesFile = Join-Path $WorkDir 'tcp-samples.txt'
if (Test-Path $samplesFile) { Remove-Item $samplesFile -Force }
$controlPy = Join-Path $WorkDir 'defender-control.py'
$engineLog = Join-Path $WorkDir 'engine.log'

$savedLogging = @{}
$logPath = ''
$sampler = $null
$failed = $false

try {
    # Profiles on and dropped-packet logging on, saving prior state for
    # restore. Rules cannot enforce anything under a disabled profile
    # (live noviwin1 proof: both networks are Public with that profile's
    # firewall off, so a block rule matched nothing and the control
    # timed out unlogged), so the gate enables every profile it relies on.
    foreach ($profile in $Profiles) {
        $current = Get-NetFirewallProfile -Profile $profile
        $savedLogging[$profile] = @{
            Enabled     = $current.Enabled
            LogBlocked  = $current.LogBlocked
            LogFileName = $current.LogFileName
        }
    }
    Set-NetFirewallProfile -Profile $Profiles -Enabled True -LogBlocked True
    Note 'Defender profiles enabled with dropped-packet logging for the gate'
    $logPath = [System.Environment]::ExpandEnvironmentVariables(
        (Get-NetFirewallProfile -Profile Public).LogFileName)
    if ([string]::IsNullOrWhiteSpace($logPath)) {
        $logPath = 'C:\Windows\System32\LogFiles\Firewall\pfirewall.log'
    }
    Note "Defender dropped-packet log: $logPath"

    # Disposable outbound BLOCK rules, one per exercise program.
    foreach ($program in $programs) {
        New-NetFirewallRule -DisplayName $RuleName `
            -Description 'Bashy prima no-egress gate (disposable)' `
            -Direction Outbound -Action Block -Program $program `
            -Profile Any -Enabled True | Out-Null
    }
    $installed = @(Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue)
    if ($installed.Count -lt $programs.Count) {
        Fail "Defender block rules missing: $($installed.Count) of $($programs.Count) installed"
    }
    Note "Defender block rules installed count=$($installed.Count) programs=$($programs.Count)"

    # Connection sampler for the whole gate: any Established/SynSent from
    # an exercise program to a non-loopback peer fails the gate. The
    # control's own TEST-NET-1 attempt is proven via the firewall log, so
    # the sampler ignores that destination.
    $sampler = Start-Job -ScriptBlock {
        param($names, $out, $ignoreHost)
        while ($true) {
            try {
                $conns = Get-NetTCPConnection -ErrorAction SilentlyContinue |
                    Where-Object {
                        ($_.State -eq 'Established' -or $_.State -eq 'SynSent') -and
                        ($_.RemoteAddress -ne '127.0.0.1') -and
                        ($_.RemoteAddress -ne '::1') -and
                        ($_.RemoteAddress -ne $ignoreHost)
                    }
                foreach ($conn in $conns) {
                    try {
                        $proc = Get-Process -Id $conn.OwningProcess -ErrorAction Stop
                        if ($names -contains $proc.ProcessName) {
                            "TCP local=$($conn.LocalAddress):$($conn.LocalPort) " +
                            "remote=$($conn.RemoteAddress):$($conn.RemotePort) " +
                            "state=$($conn.State) proc=$($proc.ProcessName) pid=$($conn.OwningProcess)" |
                                Add-Content $out
                        }
                    } catch { }
                }
            } catch { }
            Start-Sleep -Seconds 2
        }
    } -ArgumentList (,@($procNames)), $samplesFile, $ControlHost

    # Negative control, run under the SAME blocked interpreter as the
    # exercise: the connect must fail, and the failure must appear as DROP
    # in the firewall log. A connect that succeeds means no enforcement.
    Set-Content -Encoding ascii $controlPy (
        "import socket`n" +
        "s = socket.create_connection(('$ControlHost', $ControlPort), 10)`n" +
        "s.close()`n" +
        "print('CONTROL-CONNECTED')`n")
    $logBaseline = 0
    if (Test-Path $logPath) {
        $logBaseline = @(Get-Content $logPath -ErrorAction SilentlyContinue).Count
    }
    # File redirects, not 2>&1: under the script's Stop preference a
    # native command's merged stderr throws with the first stderr line
    # as the message, which would mask the control's real outcome.
    $controlStdout = Join-Path $WorkDir 'control.out'
    $controlStderr = Join-Path $WorkDir 'control.err'
    $controlProc = Start-Process -FilePath $PythonExe `
        -ArgumentList @($controlPy) -NoNewWindow -Wait -PassThru `
        -RedirectStandardOutput $controlStdout -RedirectStandardError $controlStderr
    $controlRc = $controlProc.ExitCode
    $controlText = ''
    if (Test-Path $controlStdout) {
        $controlText += Get-Content $controlStdout -Raw -ErrorAction SilentlyContinue
    }
    if (Test-Path $controlStderr) {
        $controlText += Get-Content $controlStderr -Raw -ErrorAction SilentlyContinue
    }
    if ($controlRc -eq 0 -and $controlText -match 'CONTROL-CONNECTED') {
        Fail 'Defender control connected to 192.0.2.1:443 despite the block rule (no enforcement)'
    }
    $controlErr = ($controlText.Trim() -split "`r?`n" | Where-Object { $_ -ne '' } | Select-Object -Last 1)
    Note "Defender control connect failed as required (rc=$controlRc): $controlErr"
    $dropPattern = "DROP TCP \S+ $([regex]::Escape($ControlHost)) \d+ $ControlPort(\s|$)"
    $dropLine = $null
    for ($i = 0; $i -lt 30 -and (-not $dropLine); $i++) {
        Start-Sleep -Seconds 1
        if (Test-Path $logPath) {
            $fresh = @()
            $n = 0
            foreach ($line in (Get-Content $logPath -ErrorAction SilentlyContinue)) {
                $n++
                if ($n -gt $logBaseline) { $fresh += $line }
            }
            $dropLine = @($fresh | Where-Object { $_ -match $dropPattern } | Select-Object -First 1)
        }
    }
    if (-not $dropLine -or $dropLine.Count -eq 0) {
        Fail "Defender control failure is not in the firewall log ($logPath): no DROP for ${ControlHost}:${ControlPort}"
    }
    Note "Defender control blocked ${ControlHost}:${ControlPort} and logged: $($dropLine[0].Trim())"

    # The exercise, run under the blocked interpreter. Internal per-stage
    # deadlines bound it; the 600s Wait-Job below is the backstop. The job
    # reports the exit code as a marker line: Start-Process -PassThru
    # with a manual wait leaves the process exit-code property empty on
    # some hosts (live noviwin1 proof), so the gate never reads it.
    # File redirects,
    # not pipes or 2>&1, so a chatty rank can never deadlock the gate or
    # trip the Stop preference. PYTHONUTF8 keeps model output (non-ASCII
    # tokenizer bytes) printable whatever the host codepage is.
    $engineOut = Join-Path $WorkDir 'engine.out'
    $engineErr = Join-Path $WorkDir 'engine.err'
    $engineJob = Start-Job -ScriptBlock {
        param($py, $ex, $cli, $model, $ring, $outFile, $errFile)
        $env:PYTHONUTF8 = '1'
        $env:PYTHONIOENCODING = 'utf-8'
        & $py $ex $cli $model $ring > $outFile 2> $errFile
        "EXERCISE-RC=$LASTEXITCODE"
    } -ArgumentList @($PythonExe, $ExercisePy, $CliExe, $ModelFile, $RingExe, $engineOut, $engineErr)
    $completedJob = Wait-Job $engineJob -Timeout 600
    if (-not $completedJob -or $engineJob.State -ne 'Completed') {
        if ($engineJob.State -eq 'Failed') {
            $reason = $engineJob.ChildJobs[0].JobStateInfo.Reason.Message
            try { Remove-Job $engineJob -Force -ErrorAction SilentlyContinue } catch { }
            Fail "Defender exercise job failed to run: $reason"
        }
        try { Stop-Job $engineJob -ErrorAction SilentlyContinue } catch { }
        try { Remove-Job $engineJob -Force -ErrorAction SilentlyContinue } catch { }
        Fail 'Defender exercise exceeded the 600s outer deadline'
    }
    $jobLines = @(Receive-Job $engineJob)
    Remove-Job $engineJob -ErrorAction SilentlyContinue
    $rcLine = @($jobLines | Where-Object { $_ -match '^EXERCISE-RC=\d+$' } | Select-Object -Last 1)
    if ($rcLine.Count -eq 0) {
        Fail 'Defender exercise produced no exit-code marker'
    }
    $engineRc = [int]([regex]::Match($rcLine[0], '^EXERCISE-RC=(\d+)$').Groups[1].Value)
    $engineStdout = ''
    $engineStderr = ''
    if (Test-Path $engineOut) { $engineStdout = Get-Content $engineOut -Raw -ErrorAction SilentlyContinue }
    if (Test-Path $engineErr) { $engineStderr = Get-Content $engineErr -Raw -ErrorAction SilentlyContinue }
    Set-Content -Encoding utf8 $engineLog ($engineStdout + $engineStderr)
    if ($engineRc -ne 0) {
        Fail "Defender exercise failed under the block policy (rc=$engineRc)"
    }
    $passes = ([regex]::Matches($engineStdout, '(?m)^PASS:')).Count
    if ($passes -ne 3) {
        Fail "Defender exercise stages did not all PASS (found $passes of 3)"
    }
    Note 'Defender exercise succeeded: 3 PASS stages'
    if (($engineStdout + $engineStderr) -match 'WSAEACCES|Firewall gate failure|operation not permitted|EPERM') {
        Fail 'Defender exercise output shows a denied network attempt'
    }
}
catch {
    $message = $_.Exception.Message
    if ($message -notmatch '^FAIL:') {
        Write-Error $message -ErrorAction Continue
        [void]$Evidence.Add("FAIL: $message")
    }
    $failed = $true
}
finally {
    # Sampler verdict first, while the evidence is fresh.
    if ($sampler -ne $null) {
        Stop-Job $sampler -ErrorAction SilentlyContinue | Out-Null
        Remove-Job $sampler -Force -ErrorAction SilentlyContinue
    }
    $samples = @()
    if (Test-Path $samplesFile) {
        $samples = @(Get-Content $samplesFile -ErrorAction SilentlyContinue |
            Where-Object { $_ -match '\S' })
    }
    if ($samples.Count -gt 0) {
        $detail = ($samples | Select-Object -First 5) -join '; '
        [void]$Evidence.Add("FAIL: allowed non-loopback connection(s) observed: $detail")
        Write-Error "Defender gate: allowed non-loopback connection(s) observed: $detail" -ErrorAction Continue
        $global:LASTEXITCODE = 1
        $failed = $true
    }
    else {
        Note 'Defender sampler: no non-loopback connections observed from exercise programs'
    }
    Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue |
        Remove-NetFirewallRule -ErrorAction SilentlyContinue
    foreach ($profile in $Profiles) {
        if ($savedLogging.ContainsKey($profile)) {
            Set-NetFirewallProfile -Profile $profile `
                -Enabled $savedLogging[$profile].Enabled `
                -LogBlocked $savedLogging[$profile].LogBlocked `
                -LogFileName $savedLogging[$profile].LogFileName `
                -ErrorAction SilentlyContinue
        }
    }
    $remaining = @(Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue)
    if ($remaining.Count -eq 0) {
        Note 'Defender block rules removed; firewall logging restored'
    }
    else {
        Write-Error "Defender gate: $($remaining.Count) block rule(s) left behind" -ErrorAction Continue
        $global:LASTEXITCODE = 1
        $failed = $true
    }
    [void]$Evidence.Add('---- engine output ----')
    if (Test-Path $engineLog) {
        [void]$Evidence.Add((Get-Content $engineLog -Raw -ErrorAction SilentlyContinue))
    }
    Set-Content -Encoding utf8 $OutFile ($Evidence -join "`n")
    if ($failed) { exit 1 }
}

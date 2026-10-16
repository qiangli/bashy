# Hosted Windows only. Runner no-egress gate on the built-in Windows Defender
# Firewall: disposable outbound BLOCK rules on the exact exercise programs,
# no custom native code.
#
# Ported from the prima.cpp-proven gate (tag bashy-bashy-prima-0.1.2-13, run
# 38486571631), which replaced a segfaulting custom WFP-API harness
# (windows-net-guard.c, deleted here) with the platform firewall. CI runs
# 38448516095, 38450982320, 38452584047 failed in the custom-harness path.
#
# Enforcement model (per protected executable, by exact file path):
#   - Outbound BLOCK for the exercise interpreter and the runner executable;
#   - loopback is not filtered by Windows Firewall, so the runner's
#     127.0.0.1 exercise traffic works unimpeded and needs no allowlist.
#
# PASS requires all three at once:
#   1. the negative control (outbound TCP 443 from the blocked interpreter
#      to unroutable TEST-NET-1) fails to connect AND proves the block rule
#      matched it: an immediate access-denied (WinError 10013) OR a DROP
#      for that destination in the firewall dropped-packet log (dropped
#      and allowed logging are enabled here and restored afterwards);
#   2. the same-interpreter exercise (native-gate.py: bind-owner check,
#      model load, streaming completion, embedding, cancellation slot reuse,
#      shutdown) exits 0 and records PROTOCOL_PASS in the evidence file;
#   3. a Get-NetTCPConnection sampler running for the whole gate observes
#      no Established/SynSent connection from an exercise program to a
#      non-loopback peer (the control's own TEST-NET-1 attempt is proven
#      via the firewall log instead, so the sampler ignores it).
# Anything else fails. Rules and logging are removed/restored in a finally
# block, so a failure cannot leave policy behind.
param(
    [Parameter(Mandatory = $true)][string]$PythonExe,
    [Parameter(Mandatory = $true)][string]$RunnerExe,
    [Parameter(Mandatory = $true)][string]$GatePy,
    [Parameter(Mandatory = $true)][string]$ModelFile,
    [Parameter(Mandatory = $true)][string]$EvidenceFile,
    [Parameter(Mandatory = $true)][int]$Port,
    [Parameter(Mandatory = $true)][string]$OutFile,
    [Parameter(Mandatory = $true)][string]$WorkDir
)

$ErrorActionPreference = 'Stop'

$ControlHost = '192.0.2.1'  # TEST-NET-1, never answers
$ControlPort = 443
$Profiles = @('Domain', 'Public', 'Private')
$RuleName = "Bashy runner no-egress $([guid]::NewGuid())"
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

function Resolve-ExePath([string]$path) {
    # Canonicalize a rule program to the on-disk image path the firewall
    # matches on: expand variables, then take the filesystem's own
    # FullName (long form, on-disk case). A shim, launcher, 8.3 short
    # name, or case variant that differs from the running image makes a
    # -Program rule match nothing: the control then times out instead of
    # failing fast with access denied.
    $expanded = [System.Environment]::ExpandEnvironmentVariables($path)
    try {
        $item = Get-Item -LiteralPath $expanded -ErrorAction Stop
        return $item.FullName
    } catch {
        return $expanded
    }
}

function Read-FirewallLog([string]$path, [int]$skipLines) {
    # pfirewall.log is held open by the firewall service and its writes
    # are buffered: a plain Get-Content can hit a sharing violation or a
    # stale view, so read it through a share-read handle and return the
    # lines after $skipLines.
    $lines = @()
    try {
        $stream = [System.IO.File]::Open(
            $path,
            [System.IO.FileMode]::Open,
            [System.IO.FileAccess]::Read,
            [System.IO.FileShare]::ReadWrite)
        try {
            $reader = New-Object System.IO.StreamReader($stream)
            $n = 0
            while (-not $reader.EndOfStream) {
                $line = $reader.ReadLine()
                $n++
                if ($n -gt $skipLines) { $lines += $line }
            }
            $reader.Close()
        } finally {
            $stream.Close()
        }
    } catch { }
    return $lines
}

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { Fail 'Defender gate requires an elevated (Administrator) session' }

foreach ($file in @($PythonExe, $RunnerExe, $GatePy, $ModelFile)) {
    if (-not (Test-Path $file)) { Fail "missing gate input: $file" }
}
if (-not (Test-Path $WorkDir)) { Fail "missing work dir: $WorkDir" }
$evidenceDir = Split-Path -Parent $EvidenceFile
if (-not (Test-Path $evidenceDir)) { Fail "missing evidence dir: $evidenceDir" }
if ($Port -lt 1024 -or $Port -gt 65534) { Fail "refusing privileged or out-of-range gate port: $Port" }

$programs = @($PythonExe, $RunnerExe)
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
    # (live Windows proof on the prima gate: both networks are Public with
    # that profile's firewall off, so a block rule matched nothing and the
    # control timed out unlogged), so the gate enables every profile it
    # relies on.
    foreach ($profile in $Profiles) {
        $current = Get-NetFirewallProfile -Profile $profile
        $savedLogging[$profile] = @{
            Enabled     = $current.Enabled
            LogBlocked  = $current.LogBlocked
            LogAllowed  = $current.LogAllowed
            LogFileName = $current.LogFileName
        }
    }
    Set-NetFirewallProfile -Profile $Profiles -Enabled True -LogBlocked True -LogAllowed True
    # Belt and braces: the dropped/allowed logging switches for every
    # profile, so a per-profile cmdlet gap cannot leave the log silent.
    # File redirects, not 2>&1, per the Stop-preference rule below.
    $netshOut = Join-Path $WorkDir 'netsh-logging.out'
    $netshErr = Join-Path $WorkDir 'netsh-logging.err'
    try {
        & netsh advfirewall set allprofiles logging droppedconnections enable > $netshOut 2> $netshErr
        & netsh advfirewall set allprofiles logging allowedconnections enable >> $netshOut 2>> $netshErr
        Note 'Defender dropped+allowed connection logging enabled via netsh'
    } catch {
        Note "Defender netsh logging switch best-effort only: $($_.Exception.Message)"
    }
    Note 'Defender profiles enabled with dropped-packet logging for the gate'
    # The ActiveStore is what actually enforces: a GPO/managed policy can
    # override the persistent store the gate just wrote, leaving a profile
    # disabled (and every rule on it unmatched) while everything above
    # looks enabled. Record it and fail closed on any managed disable.
    $activeProfiles = Get-NetFirewallProfile -PolicyStore ActiveStore
    foreach ($entry in $activeProfiles) {
        Note "Defender ActiveStore profile $($entry.Name): Enabled=$($entry.Enabled)"
    }
    $managedOff = @($activeProfiles | Where-Object {
        ($Profiles -contains $_.Name) -and (-not $_.Enabled) })
    if ($managedOff.Count -gt 0) {
        $names = (($managedOff | ForEach-Object { $_.Name }) -join ', ')
        Fail "Defender ActiveStore profile(s) disabled by policy (GPO/managed override): $names"
    }
    $logPath = [System.Environment]::ExpandEnvironmentVariables(
        (Get-NetFirewallProfile -Profile Public).LogFileName)
    if ([string]::IsNullOrWhiteSpace($logPath)) {
        $logPath = 'C:\Windows\System32\LogFiles\Firewall\pfirewall.log'
    }
    Note "Defender dropped-packet log: $logPath"

    # Disposable outbound BLOCK rules, one per exercise program, keyed on
    # the canonical on-disk image path: the firewall matches -Program
    # against the running image, so a shim, launcher, 8.3 short name, or
    # case variant (hostedtoolcache python vs a Store shim on the runner)
    # would match nothing and the control would time out unlogged.
    $resolvedPrograms = @()
    foreach ($program in $programs) {
        $resolved = Resolve-ExePath $program
        Note "Defender rule program raw=$program resolved=$resolved"
        $resolvedPrograms += $resolved
    }
    # Ask the control interpreter for its own real image: when the
    # invoking python is a shim that re-execs another python.exe, the
    # control runs under the target, not under the passed path. Block both.
    $selfPathPy = Join-Path $WorkDir 'defender-selfpath.py'
    $selfPathOut = Join-Path $WorkDir 'selfpath.out'
    $selfPathErr = Join-Path $WorkDir 'selfpath.err'
    Set-Content -Encoding ascii $selfPathPy (
        "import os,sys`n" +
        "print(os.path.realpath(sys.executable))`n")
    Start-Process -FilePath $PythonExe -ArgumentList @($selfPathPy) `
        -NoNewWindow -Wait `
        -RedirectStandardOutput $selfPathOut -RedirectStandardError $selfPathErr | Out-Null
    $controlSelfPath = ''
    if (Test-Path $selfPathOut) {
        $controlSelfPath = (Get-Content $selfPathOut -Raw -ErrorAction SilentlyContinue).Trim()
    }
    if ([string]::IsNullOrWhiteSpace($controlSelfPath)) {
        Fail 'Defender gate could not read the control interpreter executable path'
    }
    $controlSelfResolved = Resolve-ExePath $controlSelfPath
    Note "Defender control interpreter self-reported=$controlSelfPath resolved=$controlSelfResolved"
    $already = @($resolvedPrograms | Where-Object {
        $_.ToLowerInvariant() -eq $controlSelfResolved.ToLowerInvariant() })
    if ($already.Count -eq 0) {
        Note 'Defender control interpreter differs from the invoking python; blocking both paths'
        $resolvedPrograms += $controlSelfResolved
    }
    foreach ($program in $resolvedPrograms) {
        New-NetFirewallRule -DisplayName $RuleName `
            -Description 'Bashy runner no-egress gate (disposable)' `
            -Direction Outbound -Action Block -Program $program `
            -Profile Any -Enabled True | Out-Null
    }
    $installed = @(Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue)
    if ($installed.Count -lt $resolvedPrograms.Count) {
        Fail "Defender block rules missing: $($installed.Count) of $($resolvedPrograms.Count) installed"
    }
    Note "Defender block rules installed count=$($installed.Count) programs=$($resolvedPrograms.Count)"
    # Read the rule programs back and compare case-insensitively against
    # the resolved paths: a rule the API normalized or rejected proves
    # itself here, before the control runs, instead of as a timeout later.
    $rulePrograms = @($installed | Get-NetFirewallApplicationFilter |
        ForEach-Object { $_.Program })
    Note "Defender installed rule programs: $($rulePrograms -join '; ')"
    foreach ($need in $resolvedPrograms) {
        $hit = @($rulePrograms | Where-Object {
            $_ -and ($_.ToLowerInvariant() -eq $need.ToLowerInvariant()) })
        if ($hit.Count -eq 0) {
            Fail "Defender block rule program mismatch: no rule carries $need"
        }
    }

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
        $logBaseline = @(Read-FirewallLog $logPath 0).Count
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
    # The control proves the rule matched it in one of two ways: an
    # immediate access-denied from the blocked interpreter (WinError 10013
    # fails fast at connect time), or a DROP for that destination in the
    # firewall log. A TimeoutError with no DROP means the rule missed the
    # control process (wrong -Program path or a managed profile override)
    # and fails the gate with the compared paths attached.
    $fastDeny = ($controlText -match '10013|WSAEACCES|PermissionError|access denied|operation not permitted')
    if ($fastDeny) {
        Note 'Defender control failed fast with access-denied (rule matched the control process)'
    }
    $dropPattern = "DROP TCP \S+ $([regex]::Escape($ControlHost)) \d+ $ControlPort(\s|$)"
    $dropLine = $null
    for ($i = 0; $i -lt 30 -and (-not $dropLine); $i++) {
        Start-Sleep -Seconds 1
        if (Test-Path $logPath) {
            $fresh = @(Read-FirewallLog $logPath $logBaseline)
            $dropLine = @($fresh | Where-Object { $_ -match $dropPattern } | Select-Object -First 1)
        }
    }
    if (-not $dropLine -or $dropLine.Count -eq 0) {
        if ($fastDeny) {
            Note "Defender control blocked ${ControlHost}:${ControlPort} via immediate access-denied (rule matched; no DROP line in $logPath)"
        } else {
            Fail "Defender control failure is not in the firewall log ($logPath): no DROP for ${ControlHost}:${ControlPort} (control: $controlErr; rule programs: $($rulePrograms -join '; '); control image: $controlSelfResolved)"
        }
    } else {
        Note "Defender control blocked ${ControlHost}:${ControlPort} and logged: $($dropLine[0].Trim())"
    }

    # The exercise: native-gate.py runs the runner protocol gate (bind-owner
    # check, model load, streaming completion, embedding, cancellation slot
    # reuse, shutdown) and records PROTOCOL_PASS in the evidence file.
    # Internal per-stage deadlines bound it; the 600s Wait-Job below is the
    # backstop. The job reports the exit code as a marker line:
    # Start-Process -PassThru with a manual wait leaves the process
    # exit-code property empty on some hosts (live Windows proof on the
    # prima gate), so the gate never reads it. File redirects, not pipes
    # or 2>&1, so a chatty stage can never deadlock the gate or trip the
    # Stop preference. PYTHONUTF8 keeps model output (non-ASCII tokenizer
    # bytes) printable whatever the host codepage is.
    $engineOut = Join-Path $WorkDir 'engine.out'
    $engineErr = Join-Path $WorkDir 'engine.err'
    $engineJob = Start-Job -ScriptBlock {
        param($py, $gate, $runner, $model, $evidence, $port, $outFile, $errFile)
        $env:PYTHONUTF8 = '1'
        $env:PYTHONIOENCODING = 'utf-8'
        & $py $gate $runner $model $evidence $port > $outFile 2> $errFile
        "EXERCISE-RC=$LASTEXITCODE"
    } -ArgumentList @($PythonExe, $GatePy, $RunnerExe, $ModelFile, $EvidenceFile, $Port, $engineOut, $engineErr)
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
    # The evidence file decides: only PROTOCOL_PASS (all eight protocol
    # checks, loopback-only bind owned by the gated runner) counts. The
    # no-egress-test.sh --finalize step re-verifies every check.
    try {
        $gateEvidence = Get-Content $EvidenceFile -Raw -ErrorAction Stop | ConvertFrom-Json
    } catch {
        Fail "Defender exercise left no readable protocol evidence: $($_.Exception.Message)"
    }
    if ($gateEvidence.status -ne 'PROTOCOL_PASS') {
        Fail "Defender exercise evidence is not PROTOCOL_PASS (status=$($gateEvidence.status))"
    }
    Note 'Defender exercise succeeded: protocol gate PROTOCOL_PASS'
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
                -LogAllowed $savedLogging[$profile].LogAllowed `
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

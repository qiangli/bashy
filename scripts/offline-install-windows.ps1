param([Parameter(Mandatory=$true)][string]$Source,
      [Parameter(Mandatory=$true)][string]$Destination,
      [Parameter(Mandatory=$true)][string]$Work,
      [Parameter(Mandatory=$true)][string]$Repo)
$ErrorActionPreference = 'Stop'
$prefix = 'bashy-offline-' + [guid]::NewGuid().ToString('N')
$rules = @()
try {
    # Dedicated candidate paths only. Do not disable a host interface/firewall.
    foreach ($dir in @($Source, $Destination)) {
        foreach ($name in @('bashy.exe', 'outpost.exe', 'bash.exe', 'sh.exe')) {
            $program = [System.IO.Path]::GetFullPath((Join-Path $dir $name))
            $rule = "$prefix-$($rules.Count)"
            New-NetFirewallRule -Name $rule -DisplayName $rule -Direction Outbound -Action Block -Program $program -Profile Any | Out-Null
            $rules += $rule
        }
    }
    python (Join-Path $Repo 'scripts/offline-install-smoke.py') $Source $Destination $Work $Repo
    if ($LASTEXITCODE -ne 0) { throw "Offline candidate check failed: $LASTEXITCODE" }
} finally {
    foreach ($rule in $rules) { Remove-NetFirewallRule -Name $rule }
}

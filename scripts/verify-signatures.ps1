param(
    [Parameter(Mandatory=$true)][string]$Dist,
    [Parameter(Mandatory=$true)][ValidateSet('true','false')][string]$Signed
)
$ErrorActionPreference = 'Stop'
if ($Signed -eq 'false') { Write-Host 'UNSIGNED: Windows release manifest declares signed=false'; exit 0 }
$work = Join-Path $env:RUNNER_TEMP ('bashy-signature-qa-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    foreach ($arch in @('amd64','arm64')) {
        $zip = Join-Path $Dist "bashy-windows-$arch.zip"
        $target = Join-Path $work $arch
        Expand-Archive -LiteralPath $zip -DestinationPath $target
        foreach ($name in @('bashy.exe','outpost.exe','bash.exe','sh.exe')) {
            $file = Get-ChildItem -LiteralPath $target -Filter $name -Recurse -File
            if (@($file).Count -ne 1) { throw "expected one $name in $zip" }
            $signature = Get-AuthenticodeSignature -LiteralPath $file.FullName
            if ($signature.Status -ne 'Valid') { throw "$($file.FullName): $($signature.Status)" }
        }
        $small = Join-Path $work "$arch-small"
        Expand-Archive -LiteralPath (Join-Path $Dist "bash-windows-$arch.zip") -DestinationPath $small
        foreach ($name in @('bash.exe','sh.exe')) {
            $copy = @(Get-ChildItem -LiteralPath $small -Filter $name -Recurse -File)
            $original = @(Get-ChildItem -LiteralPath $target -Filter $name -Recurse -File)
            if ($copy.Count -ne 1 -or $original.Count -ne 1 -or
                (Get-FileHash $copy[0].FullName -Algorithm SHA256).Hash -ne (Get-FileHash $original[0].FullName -Algorithm SHA256).Hash) {
                throw "bash archive $name differs from full archive for $arch"
            }
        }
        $raw = @(Get-ChildItem -LiteralPath $Dist -Filter "outpost-v*-windows-$arch.exe" -File)
        if ($raw.Count -ne 1) { throw "expected one raw outpost windows/$arch asset" }
        $archivedOutpost = @(Get-ChildItem -LiteralPath $target -Filter 'outpost.exe' -Recurse -File)
        if ((Get-FileHash $raw[0].FullName -Algorithm SHA256).Hash -ne (Get-FileHash $archivedOutpost[0].FullName -Algorithm SHA256).Hash) {
            throw "raw outpost differs from full archive for $arch"
        }
        $signature = Get-AuthenticodeSignature -LiteralPath $raw[0].FullName
        if ($signature.Status -ne 'Valid') { throw "$($raw[0].FullName): $($signature.Status)" }
    }
    Write-Host 'Windows Authenticode signatures: PASS'
} finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}

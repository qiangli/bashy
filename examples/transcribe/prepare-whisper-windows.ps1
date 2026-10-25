# Prepare and compile the reviewed S503 whisper.cpp v1.9.5 CLI on a claimed Windows/amd64 host.
# This script compiles a standalone static executable from the SHA-256 verified upstream source archive.
param(
    [string]$WorkRoot = (Join-Path $env:LOCALAPPDATA 'Temp\s503-whisper')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not [Environment]::Is64BitOperatingSystem -or
    [Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne [Runtime.InteropServices.Architecture]::X64) {
    throw 'This pinned build script targets native Windows amd64 only.'
}

$srcVersion = '1.9.5'
$srcExpectedSHA = 'ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720'
$srcExpectedBytes = 9626464

$modelName = 'ggml-tiny.bin'
$modelExpectedSHA = 'be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21'
$modelExpectedBytes = 77691713

New-Item -ItemType Directory -Force -Path $WorkRoot | Out-Null
$srcArchive = Join-Path $WorkRoot "whisper.cpp-$srcVersion.tar.gz"
$modelFile = Join-Path $WorkRoot $modelName

function Assert-SHA256([string]$Path, [string]$Expected) {
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
    if ($actual -ne $Expected) { throw "SHA-256 mismatch for $Path`: expected $Expected, got $actual" }
}

if (-not (Test-Path -LiteralPath $srcArchive)) {
    & curl.exe -fL --retry 3 --max-time 180 -o $srcArchive "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v$srcVersion.tar.gz"
    if ($LASTEXITCODE -ne 0) { throw "Source download failed: $LASTEXITCODE" }
}
Assert-SHA256 $srcArchive $srcExpectedSHA
$srcBytes = (Get-Item -LiteralPath $srcArchive).Length
if ($srcBytes -ne $srcExpectedBytes) { throw "Source byte length mismatch: expected $srcExpectedBytes, got $srcBytes" }

if (-not (Test-Path -LiteralPath $modelFile)) {
    & curl.exe -fL --retry 3 --max-time 300 -o $modelFile "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/$modelName"
    if ($LASTEXITCODE -ne 0) { throw "Model download failed: $LASTEXITCODE" }
}
Assert-SHA256 $modelFile $modelExpectedSHA
$modelBytes = (Get-Item -LiteralPath $modelFile).Length
if ($modelBytes -ne $modelExpectedBytes) { throw "Model byte length mismatch: expected $modelExpectedBytes, got $modelBytes" }

# Extract source
$srcDir = Join-Path $WorkRoot "whisper.cpp-$srcVersion"
if (-not (Test-Path -LiteralPath $srcDir)) {
    Push-Location $WorkRoot
    & tar.exe -xf $srcArchive
    if ($LASTEXITCODE -ne 0) { Pop-Location; throw "Source extraction failed: $LASTEXITCODE" }
    Pop-Location
}

# Resolve toolchain provisioned by bashy
$cmakeBin = 'C:\Users\liqia\AppData\Local\bashy\bin\cmake\4.3.3\cmake-4.3.3-windows-x86_64\bin'
$clangBin = 'C:\Users\liqia\AppData\Local\bashy\bin\clang\20260616\llvm-mingw-20260616-ucrt-x86_64\bin'

if (-not (Test-Path -LiteralPath $cmakeBin)) {
    # Fallback to PATH search
    $cmakeCmd = Get-Command cmake.exe -ErrorAction SilentlyContinue
    if ($cmakeCmd) { $cmakeBin = Split-Path $cmakeCmd.Source }
}
if (-not (Test-Path -LiteralPath $clangBin)) {
    $clangCmd = Get-Command clang.exe -ErrorAction SilentlyContinue
    if ($clangCmd) { $clangBin = Split-Path $clangCmd.Source }
}

$cmakeExe = Join-Path $cmakeBin 'cmake.exe'
$clangExe = Join-Path $clangBin 'clang.exe'
$clangxxExe = Join-Path $clangBin 'clang++.exe'
$makeExe = Join-Path $clangBin 'mingw32-make.exe'

if (-not (Test-Path -LiteralPath $cmakeExe)) { throw "CMake missing: $cmakeExe" }
if (-not (Test-Path -LiteralPath $clangExe)) { throw "Clang missing: $clangExe" }
if (-not (Test-Path -LiteralPath $clangxxExe)) { throw "Clang++ missing: $clangxxExe" }
if (-not (Test-Path -LiteralPath $makeExe)) { throw "Make missing: $makeExe" }

$env:PATH = "$clangBin;$cmakeBin;" + $env:PATH

$buildDir = Join-Path $srcDir 'build-static'
$cComp = $clangExe.Replace('\', '/')
$cxxComp = $clangxxExe.Replace('\', '/')
$makeProg = $makeExe.Replace('\', '/')

$cliExe = Join-Path $buildDir 'bin\whisper-cli.exe'
if (-not (Test-Path -LiteralPath $cliExe)) {
    if (Test-Path -LiteralPath $buildDir) { Remove-Item -Recurse -Force -LiteralPath $buildDir }
    & $cmakeExe -S $srcDir -B $buildDir -G 'MinGW Makefiles' `
        -DCMAKE_BUILD_TYPE=Release `
        -DBUILD_SHARED_LIBS=OFF `
        -DWHISPER_BUILD_EXAMPLES=ON `
        -DWHISPER_BUILD_TESTS=OFF `
        -DGGML_OPENMP=OFF `
        "-DCMAKE_MAKE_PROGRAM=$makeProg" `
        "-DCMAKE_C_COMPILER=$cComp" `
        "-DCMAKE_CXX_COMPILER=$cxxComp" `
        -DCMAKE_EXE_LINKER_FLAGS='-static'
    if ($LASTEXITCODE -ne 0) { throw "CMake configure failed: $LASTEXITCODE" }

    & $cmakeExe --build $buildDir --config Release --target whisper-cli --parallel 8
    if ($LASTEXITCODE -ne 0) { throw "Build failed: $LASTEXITCODE" }
}

if (-not (Test-Path -LiteralPath $cliExe)) { throw "whisper-cli.exe missing after build: $cliExe" }

$cliSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $cliExe).Hash.ToLowerInvariant()
$cliBytes = (Get-Item -LiteralPath $cliExe).Length

# Inspect imported DLLs
$llvmObjdump = Join-Path $clangBin 'llvm-objdump.exe'
$importedDlls = @()
if (Test-Path -LiteralPath $llvmObjdump) {
    $dumpLines = & $llvmObjdump -p $cliExe
    foreach ($line in $dumpLines) {
        if ($line -match 'DLL Name:\s*(\S+)') {
            $importedDlls += $Matches[1]
        }
    }
}

# Coordination cache: store in standard cache directory
$cacheToolsDir = Join-Path $env:LOCALAPPDATA 'bashy\tools\whisper-cpp\v1.9.5'
$cacheModelsDir = Join-Path $env:LOCALAPPDATA 'bashy\models\whisper'
New-Item -ItemType Directory -Force -Path $cacheToolsDir | Out-Null
New-Item -ItemType Directory -Force -Path $cacheModelsDir | Out-Null
Remove-Item -Force (Join-Path $cacheToolsDir 'libomp.dll') -ErrorAction SilentlyContinue

$cachedCli = Join-Path $cacheToolsDir 'whisper-cli.exe'
$cachedModel = Join-Path $cacheModelsDir $modelName
Copy-Item -LiteralPath $cliExe -Destination $cachedCli -Force
Copy-Item -LiteralPath $modelFile -Destination $cachedModel -Force

Assert-SHA256 $cachedCli $cliSHA256
Assert-SHA256 $cachedModel $modelExpectedSHA

$jfkWav = Join-Path $srcDir 'samples\jfk.wav'

[pscustomobject]@{
    platform = 'windows/amd64'
    host = $env:COMPUTERNAME
    os_caption = (Get-CimInstance Win32_OperatingSystem).Caption
    os_version = (Get-CimInstance Win32_OperatingSystem).Version
    architecture = 'X64'
    whisper_version = $srcVersion
    source_archive_sha256 = $srcExpectedSHA
    source_archive_bytes = $srcBytes
    cmake_version = (& $cmakeExe --version | Select-Object -First 1)
    clang_version = (& $clangExe --version | Select-Object -First 1)
    cli_path = $cachedCli
    cli_sha256 = $cliSHA256
    cli_bytes = $cliBytes
    imported_dlls = $importedDlls
    model_name = $modelName
    model_sha256 = $modelExpectedSHA
    model_bytes = $modelBytes
    model_path = $cachedModel
    sample_wav_path = $jfkWav
} | ConvertTo-Json -Compress

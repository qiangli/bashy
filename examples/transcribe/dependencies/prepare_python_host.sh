#!/usr/bin/env bashy
# Operator-invoked Python.org artifact and host prerequisite gate.
# No CI tool-cache archive is an install input. Nothing downloads implicitly.
set -euo pipefail

fail() { printf 'Python host preparation rejected: %s\n' "$1" >&2; exit 1; }
usage() {
    printf '%s\n' 'usage: bashy prepare_python_host.sh --plan | --verify-artifact FILE | --install FILE | --verify-existing PYTHON' >&2
    exit 2
}

mode= artifact= python=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --plan) [ -z "$mode" ] || usage; mode=plan ;;
        --verify-artifact|--install|--verify-existing)
            [ -z "$mode" ] && [ "$#" -ge 2 ] || usage
            mode=$1
            case "$mode" in
                --verify-existing) python=$2 ;;
                *) artifact=$2 ;;
            esac
            shift ;;
        *) usage ;;
    esac
    shift
done
[ -n "$mode" ] || usage

script_source=${BASH_SOURCE[0]}
case "$(uname -s)" in
    Windows*|MINGW*|MSYS*)
        # Native Bashy supplies a drive path with backslashes on Windows.
        # POSIX dirname would treat that whole path as one filename.
        script_source=${script_source//\\//}
        script_dir=$(dirname -- "$script_source") ;;
    *) script_dir=$(cd -- "$(dirname -- "$script_source")" && pwd -P) ;;
esac
policy=$script_dir/external-python-runtime.json
windows_policy=$script_dir/windows-external-runtime.json
admission=$script_dir/operator-python-admission.json

hash_file() {
    [ -f "$1" ] || fail "regular file is absent: $1"
    local line digest
    line=$(sha256sum -- "$1") || fail "cannot hash file: $1"
    digest=${line%% *}
    # GNU checksum output prefixes the line with '\' for Windows paths.
    case "$digest" in \\*) digest=${digest#?} ;; esac
    case "$digest" in *[!0123456789abcdef]*|'') fail 'unreadable SHA-256 output' ;; esac
    [ "${#digest}" -eq 64 ] || fail 'unreadable SHA-256 output'
    printf '%s\n' "$digest"
}

[ "$(hash_file "$policy")" = bcd13fd6844b079f99f400b54bbe1c844d52ebb66579a6baeb35df5d42044a75 ] ||
    fail 'six-target Python policy bytes changed'
[ "$(hash_file "$admission")" = bfedfa9d97509788c7532ad6675a334c2c7bbf84222b3c6a077cd0e0bc371d3f ] ||
    fail 'operator-host admission policy bytes changed'
jq -e '.schema == "s503-external-python-runtime-v1" and
    (.targets | keys == ["darwin/amd64", "darwin/arm64", "linux/amd64",
                        "linux/arm64", "windows/amd64", "windows/arm64"])' \
    "$policy" >/dev/null || fail 'six-target Python policy is incomplete'
jq -e '.schema == "s503-operator-python-admission-v1" and
    (.targets | keys == ["darwin/amd64", "darwin/arm64", "linux/amd64",
                        "linux/arm64", "windows/amd64", "windows/arm64"])' \
    "$admission" >/dev/null || fail 'operator-host admission policy is incomplete'

case "$(uname -s)/$(uname -m)" in
    Linux/x86_64|Linux/amd64) target=linux/amd64 ;;
    Linux/aarch64|Linux/arm64) target=linux/arm64 ;;
    Darwin/x86_64|Darwin/amd64) target=darwin/amd64 ;;
    Darwin/arm64|Darwin/aarch64) target=darwin/arm64 ;;
    Windows*/x86_64|Windows*/amd64|MINGW*/x86_64|MSYS*/x86_64) target=windows/amd64 ;;
    Windows*/aarch64|Windows*/arm64|MINGW*/aarch64|MSYS*/aarch64) target=windows/arm64 ;;
    *) fail 'unsupported host OS or process architecture' ;;
esac
version=$(jq -er ".targets[\"$target\"].python_version" "$policy") || fail 'Python version pin missing'
ci_python_sha=$(jq -er ".targets[\"$target\"].python_sha256" "$policy") || fail 'CI Python executable pin missing'
case "$ci_python_sha" in *[!0123456789abcdef]*|'') fail 'invalid CI Python executable pin' ;; esac
[ "${#ci_python_sha}" -eq 64 ] || fail 'invalid CI Python executable pin'
admission_status=$(jq -er ".targets[\"$target\"].status" "$admission") || fail 'operator-host admission status missing'
installer_review=$(jq -er ".targets[\"$target\"].installer_review" "$admission") || fail 'installer review status missing'

require_operator_admission() {
    [ "$admission_status" = admitted ] && [ "$installer_review" = approved ] ||
        fail 'operator-host installer and installed interpreter pins are unadmitted'
    installed_python_sha=$(jq -er ".targets[\"$target\"].installed_python_sha256" "$admission") ||
        fail 'approved installed interpreter SHA-256 is absent'
    case "$installed_python_sha" in *[!0123456789abcdef]*|'') fail 'invalid installed interpreter SHA-256 pin' ;; esac
    [ "${#installed_python_sha}" -eq 64 ] || fail 'invalid installed interpreter SHA-256 pin'
}

case "$target" in
    linux/*)
        [ "$mode" = plan ] || fail 'Linux installation uses setup_python_source.py with operator-supplied signed source inputs'
        printf 'target=%s\nversion=%s\nsource_url=https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz\nsource_sha256=c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a\nsignature_sha256=e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6\nrelease_key_fingerprint=7169605F62C751356D054A26A821E680E5FA6305\nroute=setup_python_source.py\nstatus=local_source_build_unadmitted\n' "$target" "$version"
        exit 0 ;;
    darwin/*)
        if [ "$target" = darwin/amd64 ]; then
            translated=$(/usr/sbin/sysctl -n sysctl.proc_translated 2>/dev/null || true)
            [ "$translated" != 1 ] || fail 'translated macOS process is not a native target'
        fi
        installer_url=$(jq -er ".targets[\"$target\"].archive_url" "$policy") || fail 'macOS package URL missing'
        installer_sha=$(jq -er ".targets[\"$target\"].archive_sha256" "$policy") || fail 'macOS package pin missing'
        default_python=/Library/Frameworks/Python.framework/Versions/3.12/bin/python3.12 ;;
    windows/*)
        [ "$(hash_file "$windows_policy")" = 22fdf176901b22b7cbc32be79f57feffcd566767dddd8c2523d58eee493771e0 ] ||
            fail 'Windows VC/runtime policy bytes changed'
        if [ "$target" = windows/amd64 ]; then arch_key=AMD64; else arch_key=ARM64; fi
        jq -e '.schema == "s503-windows-external-runtime-v1" and
            (.targets | keys == ["AMD64", "ARM64"])' "$windows_policy" >/dev/null ||
            fail 'Windows VC/runtime policy is incomplete'
        [ "$(jq -er ".targets.$arch_key.python_sha256" "$windows_policy")" = "$ci_python_sha" ] ||
            fail 'Windows CI policies disagree on interpreter hash'
        [ "$(jq -er ".targets.$arch_key.python_version" "$windows_policy")" = "$version" ] ||
            fail 'Windows policies disagree on interpreter version'
        [ "$(jq -er ".targets.$arch_key.python_official_package.url" "$windows_policy")" = \
          "$(jq -er ".targets[\"$target\"].archive_url" "$policy")" ] ||
            fail 'Windows policies disagree on official package URL'
        [ "$(jq -er ".targets.$arch_key.python_official_package.sha256" "$windows_policy")" = \
          "$(jq -er ".targets[\"$target\"].archive_sha256" "$policy")" ] ||
            fail 'Windows policies disagree on official package hash'
        installer_url=$(jq -er ".targets.$arch_key.python_official_installer.url" "$windows_policy") ||
            fail 'Windows installer URL missing'
        installer_sha=$(jq -er ".targets.$arch_key.python_official_installer.sha256" "$windows_policy") ||
            fail 'Windows installer pin missing' ;;
esac

case "$installer_url" in https://www.python.org/ftp/python/*) ;; *) fail 'installer URL is not Python.org' ;; esac
case "$installer_sha" in *[!0123456789abcdef]*|'') fail 'invalid installer SHA-256 pin' ;; esac
[ "${#installer_sha}" -eq 64 ] || fail 'invalid installer SHA-256 pin'

if [ "$mode" = plan ]; then
    printf 'target=%s\nversion=%s\ninstaller_url=%s\ninstaller_sha256=%s\nci_python_sha256=%s\ninstaller_review=%s\nstatus=%s\n' \
        "$target" "$version" "$installer_url" "$installer_sha" "$ci_python_sha" "$installer_review" "$admission_status"
    exit 0
fi

windows_vc_check() {
    export BASHY_EXPECT_ARCH=$arch_key
    export BASHY_EXPECT_MSVCP_SHA BASHY_EXPECT_MSVCP_VERSION BASHY_EXPECT_VCRUNTIME_SHA BASHY_EXPECT_VCRUNTIME_VERSION
    BASHY_EXPECT_MSVCP_SHA=$(jq -er ".targets.$arch_key.system32[\"MSVCP140.dll\"].sha256" "$windows_policy")
    BASHY_EXPECT_MSVCP_VERSION=$(jq -er ".targets.$arch_key.system32[\"MSVCP140.dll\"].version" "$windows_policy")
    BASHY_EXPECT_VCRUNTIME_SHA=$(jq -er ".targets.$arch_key.system32[\"VCRUNTIME140.dll\"].sha256" "$windows_policy")
    BASHY_EXPECT_VCRUNTIME_VERSION=$(jq -er ".targets.$arch_key.system32[\"VCRUNTIME140.dll\"].version" "$windows_policy")
    powershell.exe -NoProfile -NonInteractive -Command '
      $ErrorActionPreference="Stop"
      try {
        $native=[Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToUpperInvariant()
        $process=[Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString().ToUpperInvariant()
        if ($native -eq "X64") { $native="AMD64" }
        if ($process -eq "X64") { $process="AMD64" }
        if ($native -ne $env:BASHY_EXPECT_ARCH -or $process -ne $native) { throw "Windows PowerShell and Bashy must run natively" }
        foreach ($name in @("MSVCP140.dll", "VCRUNTIME140.dll")) {
          $path=Join-Path $env:SystemRoot ("System32\" + $name)
          if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "$name absent from native System32" }
          $sha=(Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
          $v=(Get-Item -LiteralPath $path).VersionInfo
          $version="{0}.{1}.{2}.{3}" -f $v.FileMajorPart,$v.FileMinorPart,$v.FileBuildPart,$v.FilePrivatePart
          if ($name -eq "MSVCP140.dll") { $wantSha=$env:BASHY_EXPECT_MSVCP_SHA; $wantVersion=$env:BASHY_EXPECT_MSVCP_VERSION }
          else { $wantSha=$env:BASHY_EXPECT_VCRUNTIME_SHA; $wantVersion=$env:BASHY_EXPECT_VCRUNTIME_VERSION }
          if ($sha -ne $wantSha -or $version -ne $wantVersion) { throw "$name differs from reviewed System32 VC pin" }
        }
      } catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
    ' || fail 'Windows System32 VC prerequisite rejected before Python execution'
}

verify_existing() {
    local selected observed
    selected=$(realpath -- "$1") || fail 'installed interpreter path is absent'
    [ -f "$selected" ] || fail 'installed interpreter is not a file'
    observed=$(hash_file "$selected")
    [ "$observed" = "$installed_python_sha" ] || fail 'installed interpreter differs from reviewed operator-host Python SHA-256'
    case "$target" in windows/*) windows_vc_check ;; esac
    [ "$("$selected" -I -S -c 'import sys; print(sys.version.split()[0])')" = "$version" ] ||
        fail 'installed interpreter version differs from reviewed patch'
    printf 'target=%s\npython=%s\npython_sha256=%s\nstatus=operator_host_verified\n' \
        "$target" "$selected" "$observed"
}

if [ "$mode" = --verify-existing ]; then
    require_operator_admission
    verify_existing "$python"
    exit 0
fi

[ "$mode" = --install ] && require_operator_admission

artifact=$(realpath -- "$artifact") || fail 'installer artifact path is absent'
[ "$(hash_file "$artifact")" = "$installer_sha" ] || fail 'Python.org installer artifact differs from reviewed SHA-256'

# The downloaded pathname may be writable or replaceable. Check and execute
# the same private copy, including when the operator supplied a symlink.
stage_dir= staged_artifact=
cleanup_stage() {
    local status=$? cleanup_failed=0
    trap - EXIT
    if [ -n "$stage_dir" ]; then
        chmod 700 "$stage_dir" >/dev/null 2>&1 || cleanup_failed=1
        if [ -n "$staged_artifact" ] && [ -e "$staged_artifact" ]; then
            chmod 600 "$staged_artifact" >/dev/null 2>&1 || cleanup_failed=1
            rm -f "$staged_artifact" || cleanup_failed=1
        fi
        rmdir "$stage_dir" || cleanup_failed=1
    fi
    if [ "$cleanup_failed" -ne 0 ]; then
        printf 'Python host preparation rejected: private installer cleanup failed\n' >&2
        status=1
    fi
    exit "$status"
}
trap cleanup_stage EXIT
umask 077
case "$target" in
    darwin/*)
        if [ "$mode" = --install ]; then
            [ "$(id -u)" = 0 ] || fail 'run explicit macOS install as root after reviewing the package plan'
        fi
        # A root install must never stage beneath an inherited, user-owned TMPDIR.
        if [ "$(id -u)" = 0 ]; then stage_parent=/var/tmp
        else stage_parent=${TMPDIR:-/var/tmp}; fi
        stage_suffix=.pkg ;;
    windows/*)
        [ -n "${LOCALAPPDATA:-}" ] || fail 'LOCALAPPDATA is required for private Windows staging'
        stage_parent=$(realpath -- "$LOCALAPPDATA") || fail 'Windows staging parent is absent'
        case "$stage_parent" in [A-Za-z]:/*|[A-Za-z]:\\*) ;; *) fail 'Windows staging parent must be a local absolute path' ;; esac
        stage_suffix=.exe ;;
esac
[ -d "$stage_parent" ] || fail 'private installer staging parent is absent'
stage_dir=$(mktemp -d -- "$stage_parent/bashy-python-installer.XXXXXXXX") ||
    fail 'cannot create private installer staging directory'
chmod 700 "$stage_dir" || fail 'cannot protect installer staging directory'
staged_artifact=$stage_dir/python-installer$stage_suffix
cp "$artifact" "$staged_artifact" || fail 'cannot stage installer artifact'
chmod 500 "$staged_artifact" || fail 'cannot protect staged installer artifact'
chmod 500 "$stage_dir" || fail 'cannot protect staged installer directory'
[ "$(hash_file "$staged_artifact")" = "$installer_sha" ] ||
    fail 'staged installer differs from reviewed SHA-256'
case "$target" in
    darwin/*)
        signature=$(pkgutil --check-signature "$staged_artifact") || fail 'macOS installer signature is invalid'
        case "$signature" in *'Developer ID Installer: Python Software Foundation (BMM5U3QVKW)'*) ;; *) fail 'macOS installer signer differs from Python Software Foundation' ;; esac
        /usr/sbin/spctl --assess --type install "$staged_artifact" || fail 'macOS installer assessment rejected the package' ;;
    windows/*)
        BASHY_PYTHON_INSTALLER_ARTIFACT=$(cygpath -m "$staged_artifact") ||
            fail 'cannot pass staged installer path to native Windows signature check'
        export BASHY_PYTHON_INSTALLER_ARTIFACT
        powershell.exe -NoProfile -NonInteractive -Command '
          $ErrorActionPreference="Stop"
          try {
            $s=Get-AuthenticodeSignature -LiteralPath $env:BASHY_PYTHON_INSTALLER_ARTIFACT
            if ($s.Status -ne "Valid" -or $s.SignerCertificate.Subject -notmatch "(^|, )CN=Python Software Foundation(,|$)") {
              throw "Python.org installer Authenticode signer is invalid"
            }
          } catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
        ' || fail 'Windows installer signature is invalid'
        windows_vc_check ;;
esac
[ "$(hash_file "$staged_artifact")" = "$installer_sha" ] || fail 'staged installer changed during signature verification'
printf 'target=%s\ninstaller=%s\ninstaller_sha256=%s\nstatus=verified_artifact_not_installed\n' \
    "$target" "$artifact" "$installer_sha"
[ "$mode" = --install ] || exit 0

case "$target" in
    darwin/*)
        [ ! -e /Library/Frameworks/Python.framework/Versions/3.12 ] ||
            fail 'existing macOS Python 3.12 framework must not be overwritten'
        /usr/sbin/installer -pkg "$staged_artifact" -target / || fail 'macOS Python.org installer failed'
        verify_existing "$default_python" ;;
    windows/*)
        [ -n "${LOCALAPPDATA:-}" ] || fail 'LOCALAPPDATA is required for a per-user Windows install'
        prefix=$(realpath -m -- "$LOCALAPPDATA/Bashy/Python-$version-$arch_key") ||
            fail 'Windows install prefix is invalid'
        [ ! -e "$prefix" ] || fail 'Windows install prefix already exists'
        case "$prefix" in [A-Za-z]:/*|[A-Za-z]:\\*) ;; *) fail 'Windows install prefix must be absolute' ;; esac
        "$staged_artifact" /quiet InstallAllUsers=0 Include_pip=0 Include_test=0 \
            Include_tcltk=0 Include_launcher=0 PrependPath=0 "TargetDir=$prefix" ||
            fail 'Windows Python.org installer failed; inspect its native log before retrying'
        verify_existing "$prefix/python.exe" ;;
esac

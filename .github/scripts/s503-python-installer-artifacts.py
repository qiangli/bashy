"""Bounded native research gate for the four official Python.org installers.

The only executable under test is Bashy's --verify-artifact gate. This script
never invokes the installer or changes operator admission policy.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import urllib.request


DEPENDENCIES = Path("examples/transcribe/dependencies")
MAX_INSTALLER_BYTES = 100 * 1024 * 1024
SHA256 = re.compile(r"[0-9a-f]{64}\Z")


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def native_target():
    system = platform.system().lower()
    machine = platform.machine().lower()
    os_name = {"darwin": "darwin", "windows": "windows"}.get(system)
    arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64",
            "aarch64": "arm64"}.get(machine)
    require(os_name is not None and arch is not None, "unsupported runner identity")
    if os_name == "darwin" and arch == "amd64":
        translated = subprocess.run(["/usr/sbin/sysctl", "-n", "sysctl.proc_translated"],
                                    capture_output=True, text=True, timeout=10)
        require(translated.stdout.strip() != "1", "translated macOS process")
    if os_name == "windows":
        native = subprocess.run([
            "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
            "[Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()+'/' + "
            "[Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString()"],
            capture_output=True, text=True, timeout=20, check=True).stdout.strip().upper()
        expected = "X64/X64" if arch == "amd64" else "ARM64/ARM64"
        require(native == expected, "Windows PowerShell is not native: " + native)
    return os_name + "/" + arch


def invoke(bashy, script, args, evidence, label):
    result = subprocess.run([str(bashy), str(script), *args],
                            capture_output=True, text=True, timeout=180,
                            env=dict(os.environ, BASHY_HINTS="off"))
    (evidence / (label + ".stdout.txt")).write_text(result.stdout, encoding="utf-8")
    (evidence / (label + ".stderr.txt")).write_text(result.stderr, encoding="utf-8")
    return result


def parse_plan(output):
    lines = [line.split("=", 1) for line in output.splitlines()]
    require(all(len(line) == 2 for line in lines), "malformed host-prep plan")
    plan = dict(lines)
    require(len(plan) == len(lines), "duplicate host-prep plan key")
    return plan


def download(url, path):
    require(url.startswith("https://www.python.org/ftp/python/3.12.10/"),
            "unapproved Python.org installer URL")
    request = urllib.request.Request(url, headers={"User-Agent": "s503-python-installer-research"})
    with urllib.request.urlopen(request, timeout=120) as source:
        require(source.url.startswith("https://"), "installer redirected away from HTTPS")
        length = source.headers.get("Content-Length")
        if length is not None:
            require(int(length) <= MAX_INSTALLER_BYTES, "installer exceeds size limit")
        with path.open("xb") as sink:
            total = 0
            while block := source.read(1024 * 1024):
                total += len(block)
                require(total <= MAX_INSTALLER_BYTES, "installer exceeds size limit")
                sink.write(block)
    return total


def native_probe(target, artifact, evidence):
    """Retain native signer and prerequisite detail independently of the gate."""
    if target.startswith("darwin/"):
        commands = {
            "macos-package-signature": ["pkgutil", "--check-signature", str(artifact)],
            "macos-gatekeeper": ["/usr/sbin/spctl", "--assess", "--type", "install", "-vv", str(artifact)],
        }
    else:
        probe = r'''
$ErrorActionPreference = "Stop"
$package = Get-AuthenticodeSignature -LiteralPath $env:S503_INSTALLER_PATH
$files = foreach ($name in @("MSVCP140.dll", "VCRUNTIME140.dll")) {
  $path = Join-Path $env:SystemRoot ("System32\" + $name)
  if (Test-Path -LiteralPath $path -PathType Leaf) {
    $file = Get-Item -LiteralPath $path
    [pscustomobject]@{ name=$name; path=$path;
      sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant();
      version=$file.VersionInfo.FileVersion }
  } else { [pscustomobject]@{ name=$name; path=$path; missing=$true } }
}
[pscustomobject]@{ os_arch=[Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString();
  process_arch=[Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString();
  signature_status=$package.Status.ToString(); signer_subject=$package.SignerCertificate.Subject;
  signer_thumbprint=$package.SignerCertificate.Thumbprint; vc_system32=@($files) } |
  ConvertTo-Json -Depth 5 -Compress
'''
        commands = {"windows-signature-and-vc": [
            "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", probe]}
    receipts = {}
    for name, command in commands.items():
        env = dict(os.environ, S503_INSTALLER_PATH=str(artifact))
        result = subprocess.run(command, capture_output=True, text=True, timeout=90, env=env)
        stdout = evidence / (name + ".stdout.txt")
        stderr = evidence / (name + ".stderr.txt")
        stdout.write_text(result.stdout, encoding="utf-8")
        stderr.write_text(result.stderr, encoding="utf-8")
        receipts[name] = {"exit_code": result.returncode,
                          "stdout_sha256": digest(stdout), "stderr_sha256": digest(stderr)}
    return receipts


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--bashy", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    report = {"schema": "s503-python-installer-artifact-research-v1",
              "candidate_commit": os.environ["CANDIDATE_COMMIT"],
              "runner_label": os.environ["RUNNER_LABEL"],
              "expected_target": os.environ["EXPECTED_TARGET"],
              "scope": "artifact SHA, signature, staged-copy and Windows VC checks; no install or admission",
              "result": "BLOCKED"}
    artifact = None
    try:
        target = native_target()
        report["native_target"] = target
        require(target == report["expected_target"], "runner OS/architecture mismatch")
        report["bashy_sha256"] = digest(args.bashy)
        policy_paths = {name: DEPENDENCIES / name for name in (
            "prepare_python_host.sh", "external-python-runtime.json",
            "windows-external-runtime.json", "operator-python-admission.json")}
        report["policy_sha256"] = {name: digest(path) for name, path in policy_paths.items()}
        policy = json.loads(policy_paths["external-python-runtime.json"].read_text())
        windows = json.loads(policy_paths["windows-external-runtime.json"].read_text())
        admission = json.loads(policy_paths["operator-python-admission.json"].read_text())
        require(all(row["status"] == "unadmitted" for row in admission["targets"].values()),
                "operator admission policy changed")
        require(admission["targets"][target]["installer_review"] == "pending_native_review",
                "native installer review already changed")
        script = policy_paths["prepare_python_host.sh"].resolve()
        plan_result = invoke(args.bashy, script,
                             ["--plan"], args.evidence, "plan")
        require(plan_result.returncode == 0, "host-prep plan failed")
        plan = parse_plan(plan_result.stdout)
        official = (policy["targets"][target] if target.startswith("darwin/")
                    else windows["targets"][target.split("/")[1].upper()]["python_official_installer"])
        url = official.get("archive_url", official.get("url"))
        sha = official.get("archive_sha256", official.get("sha256"))
        require(SHA256.fullmatch(sha) is not None, "invalid installer policy digest")
        require(plan["target"] == target and plan["status"] == "unadmitted" and
                plan["installer_review"] == "pending_native_review", "plan admission mismatch")
        require(plan["installer_url"] == url and plan["installer_sha256"] == sha,
                "plan and policy disagree on installer")
        report["installer_url"] = url
        report["expected_installer_sha256"] = sha
        artifact = Path(os.environ["RUNNER_TEMP"]) / ("s503-python-installer-" + target.split("/")[1] + Path(url).suffix)
        report["download_bytes"] = download(url, artifact)
        report["download_sha256"] = digest(artifact)
        require(report["download_sha256"] == sha, "download differs from exact policy SHA-256")
        report["native_probe"] = native_probe(target, artifact, args.evidence)
        verified = invoke(args.bashy, script,
                          ["--verify-artifact", str(artifact.resolve())],
                          args.evidence, "verify-artifact")
        report["verify_exit_code"] = verified.returncode
        report["verify_stdout_sha256"] = digest(args.evidence / "verify-artifact.stdout.txt")
        report["verify_stderr_sha256"] = digest(args.evidence / "verify-artifact.stderr.txt")
        require(verified.returncode == 0, "native signature, staging, or VC check failed")
        require("status=verified_artifact_not_installed" in verified.stdout and
                "installer_sha256=" + sha in verified.stdout and
                "staged_installer_sha256=" + sha in verified.stdout and
                "signature_status=verified" in verified.stdout, "incomplete verifier receipt")
        report["result"] = "VERIFIED_ARTIFACT_NOT_INSTALLED"
    except Exception as error:
        report["error"] = str(error)
        raise
    finally:
        if artifact is not None:
            artifact.unlink(missing_ok=True)
        (args.evidence / "result.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n",
                                                     encoding="utf-8")
        print(json.dumps(report, sort_keys=True))


if __name__ == "__main__":
    main()

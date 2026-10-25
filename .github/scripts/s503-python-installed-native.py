"""CI-only installed-Python research on four native, disposable GitHub runners.

This intentionally bypasses the product's unadmitted --install mode. It reuses
the product's --verify-artifact gate, then invokes the reviewed Python.org
installer command directly. No result here changes operator admission policy.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys


BASE = Path("examples/transcribe/dependencies")
REVIEW = Path("docs/sprint-503/evidence/python-installer-40216443981/review.json")
ARTIFACT_GATE = Path(".github/scripts/s503-python-installer-artifacts.py")
NOTICE_NAMES = ("license", "licence", "copying", "copyright", "notice", "third_party")
NOTICE_SUFFIXES = ("", ".txt", ".md", ".rst", ".rtf", ".html", ".htm")
MAX_NOTICES = 256
MAX_NOTICE_COPY_BYTES = 2 * 1024 * 1024

spec = importlib.util.spec_from_file_location("s503_artifact_gate", ARTIFACT_GATE)
artifact_gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(artifact_gate)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(path):
    return artifact_gate.digest(Path(path))


def record_command(evidence, label, command, timeout, env=None):
    """Retain the literal argv, native exit code and bounded stdout/stderr."""
    receipt = {"argv": [str(item) for item in command], "exit_code": None}
    try:
        result = subprocess.run(command, capture_output=True, text=True,
                                errors="replace", timeout=timeout, env=env)
        receipt["exit_code"] = result.returncode
        for stream in ("stdout", "stderr"):
            data = getattr(result, stream)
            path = evidence / (label + "." + stream + ".txt")
            path.write_text(data, encoding="utf-8")
            receipt[stream + "_sha256"] = sha256(path)
            receipt[stream + "_bytes"] = path.stat().st_size
        return receipt, result.stdout
    except subprocess.TimeoutExpired as error:
        receipt["timeout_seconds"] = timeout
        receipt["error"] = str(error)
        return receipt, ""


def path_state(path):
    path = Path(path)
    state = {"path": str(path), "exists": path.exists(), "symlink": path.is_symlink()}
    if path.is_symlink():
        state["link_target"] = os.readlink(path)
    if path.is_file():
        state["size"] = path.stat().st_size
        state["sha256"] = sha256(path)
    return state


def host_state(target, prefix, evidence, phase):
    if target.startswith("darwin/"):
        watched = [
            "/Library/Frameworks/Python.framework/Versions/3.12",
            "/Library/Frameworks/Python.framework/Versions/3.12/bin/python3.12",
            "/Applications/Python 3.12",
            "/usr/local/bin/python3.12",
            "/usr/local/bin/pip3.12",
        ]
        package_receipt, package_stdout = record_command(
            evidence, phase + "-pkgutil", ["/usr/sbin/pkgutil", "--pkgs"], 30)
        require(package_receipt["exit_code"] == 0, "macOS package receipt listing failed")
        package_ids = sorted(line for line in package_stdout.splitlines()
                             if line.startswith("org.python.Python."))
        version_312_ids = [item for item in package_ids if item.endswith("-3.12")]
        package_info = {}
        for index, package_id in enumerate(version_312_ids):
            receipt, output = record_command(
                evidence, f"{phase}-pkg-info-{index:02d}",
                ["/usr/sbin/pkgutil", "--pkg-info", package_id], 30)
            require(receipt["exit_code"] == 0, "macOS Python 3.12 package info failed")
            fields = dict(line.split(": ", 1) for line in output.splitlines() if ": " in line)
            require(fields.get("package-id") == package_id and fields.get("version") and
                    fields.get("install-time"), "incomplete macOS Python 3.12 package info")
            package_info[package_id] = {"fields": fields, "command": receipt}
        return {"watched_paths": [path_state(path) for path in watched],
                "python_package_ids": package_ids, "python_312_package_info": package_info,
                "pkgutil": package_receipt}

    watched = [prefix, prefix / "python.exe",
               Path(os.environ["SystemRoot"]) / "py.exe",
               Path(os.environ["LOCALAPPDATA"]) / "Programs/Python/Launcher/py.exe"]
    launcher_receipt, launcher_stdout = record_command(
        evidence, phase + "-where-py", ["where.exe", "py"], 30)
    registry = r'''
$ErrorActionPreference = 'Stop'
$paths = @('HKCU:\Software\Python\PythonCore\3.12\InstallPath',
           'HKLM:\Software\Python\PythonCore\3.12\InstallPath')
$installed = foreach ($path in $paths) {
  if (Test-Path -LiteralPath $path) {
    Get-ItemProperty -LiteralPath $path | Select-Object PSPath, '(default)', ExecutablePath
  }
}
@($installed) | ConvertTo-Json -Depth 4 -Compress
'''
    registry_receipt, registry_stdout = record_command(
        evidence, phase + "-python-registry",
        ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", registry], 30)
    return {"watched_paths": [path_state(path) for path in watched],
            "launcher_path_search": launcher_stdout.splitlines(),
            "launcher_path_search_command": launcher_receipt,
            "python_registry": registry_stdout.strip(),
            "python_registry_command": registry_receipt}


def third_party_notice_candidate(path):
    parts = {part.lower() for part in Path(path).parts}
    return (bool(parts & {"vendor", "_vendor", "third_party", "third-party",
                          "site-packages"}) or "notice" in Path(path).name.lower())


def notice_inventory(roots, evidence, *, phase="after"):
    require(phase in ("before", "after"), "unknown notice inventory phase")
    matches = []
    for root in roots:
        if not root.exists():
            continue
        for parent, dirs, files in os.walk(root, followlinks=False):
            dirs[:] = sorted(d for d in dirs if not (Path(parent) / d).is_symlink())
            for name in sorted(files):
                if (any(token in name.lower() for token in NOTICE_NAMES) and
                        Path(name).suffix.lower() in NOTICE_SUFFIXES):
                    path = Path(parent) / name
                    if path.is_file() and not path.is_symlink():
                        matches.append(path)
    if phase == "after":
        require(matches, "installed Python license or notice files not found")
    require(len(matches) <= MAX_NOTICES, "notice inventory exceeds bounded limit")
    notice_dir = evidence / ("notices-before" if phase == "before" else "notices")
    notice_dir.mkdir(exist_ok=True)
    copied = 0
    inventory = []
    for index, path in enumerate(matches):
        size = path.stat().st_size
        row = {"path": str(path), "bytes": size, "sha256": sha256(path),
               "retained_copy": None}
        if size <= 256 * 1024 and copied + size <= MAX_NOTICE_COPY_BYTES:
            destination = notice_dir / f"{index:03d}-{path.name}"
            shutil.copyfile(path, destination)
            require(sha256(destination) == row["sha256"], "notice copy changed")
            row["retained_copy"] = str(destination)
            copied += size
        inventory.append(row)
    return {"files": inventory, "copied_bytes": copied,
            "python_license_found": any("license" in Path(row["path"]).name.lower() or
                                        "licence" in Path(row["path"]).name.lower()
                                        for row in inventory),
            "third_party_notice_candidates": sum(
                third_party_notice_candidate(row["path"]) for row in inventory),
            "all_files_retained": all(row["retained_copy"] for row in inventory)}


def classify_notice_delta(before, after):
    """Record observed file changes without attributing them to the installer."""
    previous = {row["path"]: row for row in before["files"]}
    current = {row["path"]: row for row in after["files"]}
    require(len(previous) == len(before["files"]) and
            len(current) == len(after["files"]), "duplicate notice path in inventory")
    counts = {"unchanged_from_preinstall": 0, "changed_from_preinstall": 0,
              "new_path_after_install": 0, "removed_after_install": 0}
    for row in after["files"]:
        prior = previous.get(row["path"])
        if prior is None:
            status = "new_path_after_install"
        elif prior["sha256"] == row["sha256"]:
            status = "unchanged_from_preinstall"
        else:
            status = "changed_from_preinstall"
            row["preinstall_sha256"] = prior["sha256"]
        row["change_from_preinstall"] = status
        counts[status] += 1
    removed = [previous[path] for path in sorted(previous.keys() - current.keys())]
    counts["removed_after_install"] = len(removed)
    return {"counts": counts, "removed": removed,
            "attribution": "file delta only; new or changed paths are not proven installer payload"}


PROBE = r'''
import bz2, hashlib, json, lzma, os, platform, ssl, struct, sys, sysconfig, zlib

def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()

modules = {}
for name in ('zlib', 'bz2', '_bz2', 'lzma', '_lzma', 'ssl', '_ssl'):
    module = sys.modules.get(name) or __import__(name)
    origin = getattr(module, '__file__', None)
    modules[name] = {'path': origin, 'sha256': digest(origin) if origin else None}
payload = b'S503 installed Python stdlib codec probe' * 8
roundtrip = {
    'zlib': zlib.decompress(zlib.compress(payload)) == payload,
    'bz2': bz2.decompress(bz2.compress(payload)) == payload,
    'lzma': lzma.decompress(lzma.compress(payload)) == payload,
}
vc_loaded = {}
if os.name == 'nt':
    import ctypes
    for name in ('MSVCP140.dll', 'VCRUNTIME140.dll'):
        library = ctypes.WinDLL(name)
        buffer = ctypes.create_unicode_buffer(32768)
        length = ctypes.windll.kernel32.GetModuleFileNameW(
            ctypes.c_void_p(library._handle), buffer, len(buffer))
        path = buffer.value if length else None
        vc_loaded[name] = {'path': path, 'sha256': digest(path) if path else None}
print(json.dumps({
    'executable': sys.executable,
    'executable_sha256': digest(sys.executable),
    'version': platform.python_version(),
    'version_info': list(sys.version_info[:3]),
    'machine': platform.machine(),
    'platform': sysconfig.get_platform(),
    'pointer_bits': struct.calcsize('P') * 8,
    'openssl_version': ssl.OPENSSL_VERSION,
    'modules': modules,
    'stdlib_roundtrip': roundtrip,
    'vc_loaded': vc_loaded,
}, sort_keys=True))
'''


def windows_vc_inventory(prefix, policy):
    arch = "AMD64" if platform.machine().lower() in ("amd64", "x86_64") else "ARM64"
    expected = policy["targets"][arch]["system32"]
    system32 = Path(os.environ["SystemRoot"]) / "System32"
    rows = {}
    for name, pin in expected.items():
        path = system32 / name
        rows[name] = {**path_state(path), "expected_sha256": pin["sha256"],
                      "expected_version": pin["version"]}
    app_local = []
    for pattern in ("*vcruntime*.dll", "*msvcp*.dll"):
        app_local.extend(path_state(path) for path in prefix.rglob(pattern) if path.is_file())
    return {"system32": rows, "install_local": app_local}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--bashy", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    target = os.environ["EXPECTED_TARGET"]
    report = {"schema": "s503-python-installed-native-research-v1",
              "candidate_commit": os.environ["CANDIDATE_COMMIT"],
              "runner_label": os.environ["RUNNER_LABEL"],
              "expected_target": target,
              "source_artifact_run": 40216443981,
              "product_admitted": False,
              "operator_admitted": False,
              "result": "BLOCKED",
              "cleanup_scope": "downloaded installer deleted; installed files retained only on disposable hosted runner until runner disposal"}
    installer = None
    try:
        observed_target = artifact_gate.native_target()
        report["native_target"] = observed_target
        report["runner_environment"] = os.environ.get("RUNNER_ENVIRONMENT")
        report["runner_identity"] = {"system": platform.system(),
                                     "machine": platform.machine(),
                                     "release": platform.release(),
                                     "python_bootstrap": sys.executable,
                                     "python_bootstrap_sha256": sha256(sys.executable)}
        require(observed_target == target, "runner is not the requested native target")
        require(os.environ.get("GITHUB_ACTIONS") == "true" and
                report["runner_environment"] == "github-hosted",
                "native installation research requires a GitHub-hosted disposable runner")
        admission = json.loads((BASE / "operator-python-admission.json").read_text())
        require(all(row["status"] == "unadmitted" for row in admission["targets"].values()),
                "operator admission policy changed")
        require(admission["targets"][target]["installer_review"] == "pending_native_review",
                "installer review changed")
        report["policy_sha256"] = {name: sha256(BASE / name) for name in (
            "prepare_python_host.sh", "external-python-runtime.json",
            "windows-external-runtime.json", "operator-python-admission.json")}
        review = json.loads(REVIEW.read_text())
        require(review["run_id"] == 40216443981 and review["run_conclusion"] == "success"
                and review["product_admitted"] is False, "artifact review identity changed")
        reviewed = next(row for row in review["targets"] if row["target"] == target)
        require(reviewed["staged_copy_verified"] is True and reviewed["installed"] is False,
                "artifact review scope changed")
        require(report["policy_sha256"] == reviewed["policy_sha256"],
                "policy bytes differ from reviewed artifact run")
        script = (BASE / "prepare_python_host.sh").resolve()
        plan = artifact_gate.invoke(args.bashy, script, ["--plan"], args.evidence, "plan")
        require(plan.returncode == 0, "host preparation plan rejected")
        parsed = artifact_gate.parse_plan(plan.stdout)
        require(parsed["target"] == target and parsed["version"] == "3.12.10"
                and parsed["status"] == "unadmitted", "host plan identity changed")
        url, expected_sha = parsed["installer_url"], parsed["installer_sha256"]
        require(url == reviewed["installer_url"] and expected_sha == reviewed["installer_sha256"],
                "installer differs from reviewed artifact run")
        report["installer"] = {"url": url, "expected_sha256": expected_sha,
                               "review_sha256": sha256(REVIEW)}
        installer = Path(os.environ["RUNNER_TEMP"]) / ("s503-python-installed-" +
                     target.replace("/", "-") + Path(url).suffix)
        report["installer"]["bytes"] = artifact_gate.download(url, installer)
        report["installer"]["sha256"] = sha256(installer)
        require(report["installer"]["sha256"] == expected_sha, "installer SHA-256 mismatch")
        report["native_signature"] = artifact_gate.native_probe(target, installer, args.evidence)
        verified = artifact_gate.invoke(args.bashy, script,
                                        ["--verify-artifact", str(installer.resolve())],
                                        args.evidence, "verify-artifact")
        report["verify_artifact_exit_code"] = verified.returncode
        require(verified.returncode == 0 and
                "status=verified_artifact_not_installed" in verified.stdout and
                "staged_installer_sha256=" + expected_sha in verified.stdout and
                "signature_status=verified" in verified.stdout,
                "native product artifact verifier rejected installer")

        if target.startswith("darwin/"):
            prefix = Path("/Library/Frameworks/Python.framework/Versions/3.12")
            executable = prefix / "bin/python3.12"
            notice_roots = [prefix, Path("/Applications/Python 3.12")]
            command = ["sudo", "-n", "/usr/sbin/installer", "-pkg", str(installer),
                       "-target", "/"]
        else:
            arch = target.split("/")[1].upper()
            prefix = Path(os.environ["LOCALAPPDATA"]) / "Bashy" / f"Python-3.12.10-{arch}"
            executable = prefix / "python.exe"
            notice_roots = [prefix]
            command = [str(installer), "/quiet", "InstallAllUsers=0", "Include_pip=0",
                       "Include_test=0", "Include_tcltk=0", "Include_launcher=0",
                       "PrependPath=0", "TargetDir=" + str(prefix)]
        report["install_prefix"] = str(prefix)
        report["before"] = host_state(target, prefix, args.evidence, "before")
        report["notices_before"] = notice_inventory(notice_roots, args.evidence,
                                                     phase="before")
        if prefix.exists():
            require(target.startswith("darwin/"), "preexisting Windows install path on runner")
            prior = report["before"]["watched_paths"][1]
            framework_id = "org.python.Python.PythonFramework-3.12"
            require(prior.get("sha256") and
                    framework_id in report["before"]["python_312_package_info"],
                    "preexisting macOS Python is not an identified framework install")
            require(report["notices_before"]["python_license_found"],
                    "preexisting macOS framework license inventory is missing")
            report["installation_kind"] = "reinstall_over_existing_ci_framework"
            report["prior_installed_python"] = prior
        else:
            report["installation_kind"] = "fresh_install"
        require(sha256(installer) == expected_sha, "installer changed before execution")
        install_receipt, _ = record_command(args.evidence, "install", command, 600)
        report["install_command"] = install_receipt
        report["after"] = host_state(target, prefix, args.evidence, "after")
        if target.startswith("darwin/"):
            report["macos_added_package_ids"] = sorted(
                set(report["after"]["python_package_ids"]) -
                set(report["before"]["python_package_ids"]))
            report["macos_312_package_file_receipts"] = {}
            for index, package_id in enumerate(report["after"]["python_312_package_info"]):
                receipt, _ = record_command(args.evidence, f"macos-pkg-files-{index:02d}",
                                            ["/usr/sbin/pkgutil", "--files", package_id], 60)
                require(receipt["exit_code"] == 0,
                        "macOS Python 3.12 package file listing failed")
                report["macos_312_package_file_receipts"][package_id] = receipt
            require("org.python.Python.PythonFramework-3.12" in
                    report["after"]["python_312_package_info"],
                    "macOS Python 3.12 framework receipt missing after installer")
            framework_after = report["after"]["python_312_package_info"][
                "org.python.Python.PythonFramework-3.12"]["fields"]
            # Python.org's macOS pkgutil receipt reports version 0 on both
            # hosted architectures. The exact installer/signature, changed
            # receipt time and the installed interpreter probe below establish
            # the package and executable version independently.
            require(framework_after["version"],
                    "macOS Python framework receipt has no version field")
            if report["installation_kind"] == "reinstall_over_existing_ci_framework":
                framework_before = report["before"]["python_312_package_info"][
                    "org.python.Python.PythonFramework-3.12"]["fields"]
                require(framework_after["install-time"] != framework_before["install-time"],
                        "macOS package receipt did not change after overwrite command")
        report["installer"]["post_install_sha256"] = sha256(installer)
        require(install_receipt["exit_code"] == 0, "official installer failed")
        require(report["installer"]["post_install_sha256"] == expected_sha,
                "installer changed during installation")
        require(executable.is_file(), "installed interpreter absent from expected path")
        report["installed_python"] = path_state(executable)
        require(report["installed_python"]["sha256"] == parsed["ci_python_sha256"],
                "installed interpreter differs from the reviewed executable pin")
        probe_receipt, probe_stdout = record_command(
            args.evidence, "installed-probe", [str(executable), "-I", "-S", "-c", PROBE], 60)
        report["probe_command"] = probe_receipt
        require(probe_receipt["exit_code"] == 0, "installed Python stdlib probe failed")
        probe = json.loads(probe_stdout)
        report["interpreter"] = probe
        require(probe["executable_sha256"] == report["installed_python"]["sha256"],
                "interpreter changed between path and process measurement")
        require(probe["version"] == "3.12.10" and probe["pointer_bits"] == 64,
                "installed Python version or bitness mismatch")
        expected_machine = ("amd64", "x86_64") if target.endswith("amd64") else ("arm64", "aarch64")
        require(probe["machine"].lower() in expected_machine,
                "installed Python process is not the native architecture")
        require(all(probe["stdlib_roundtrip"].values()) and probe["openssl_version"],
                "installed compression or SSL probe failed")
        report["notices"] = notice_inventory(notice_roots, args.evidence)
        report["notice_delta"] = classify_notice_delta(report["notices_before"],
                                                        report["notices"])
        require(report["notices"]["python_license_found"],
                "installed Python license notice not found")
        if target.startswith("windows/"):
            policy = json.loads((BASE / "windows-external-runtime.json").read_text())
            report["windows_vc"] = windows_vc_inventory(prefix, policy)
            post_probe_dir = args.evidence / "post-install"
            post_probe_dir.mkdir(exist_ok=True)
            report["post_install_native_signature_and_vc"] = artifact_gate.native_probe(
                target, installer, post_probe_dir)
            report["windows_launcher"] = {"before": report["before"]["launcher_path_search"],
                                          "after": report["after"]["launcher_path_search"],
                                          "installer_option": "Include_launcher=0"}
        report["result"] = ("REINSTALLED_EVIDENCE_CAPTURED_UNADMITTED" if
                            report["installation_kind"] == "reinstall_over_existing_ci_framework" else
                            "INSTALLED_EVIDENCE_CAPTURED_UNADMITTED")
    except Exception as error:
        report["error"] = str(error)
        raise
    finally:
        if installer is not None:
            installer.unlink(missing_ok=True)
            report["downloaded_installer_deleted"] = not installer.exists()
        (args.evidence / "result.json").write_text(
            json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"result": report["result"], "native_target": report.get("native_target"),
                          "product_admitted": False, "error": report.get("error")}, sort_keys=True))


if __name__ == "__main__":
    main()

"""Check a separately installed Python before the yt-dlp zipapp runs.

The reviewed bytes are external prerequisites. This module neither installs
Python nor redistributes Python or Microsoft runtime files.
"""

from __future__ import annotations

import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys

from dependencies import verify_windows_external_runtime as windows_runtime


POLICY_PATH = Path(__file__).with_name("external-python-runtime.json")
TARGETS = {
    "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64",
    "windows/amd64", "windows/arm64",
}
SHA256 = re.compile(r"[0-9a-f]{64}\Z")
VERSION = re.compile(r"\d+\.\d+\.\d+\Z")


def normalize_arch(machine: str) -> str:
    arch = machine.lower()
    if arch in ("amd64", "x86_64", "x64"):
        return "amd64"
    if arch in ("arm64", "aarch64"):
        return "arm64"
    raise RuntimeError(f"unsupported Python process or native architecture: {machine}")


def normalize_os(system: str) -> str:
    name = {"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}.get(system)
    if name is None:
        raise RuntimeError(f"unsupported external Python OS: {system}")
    return name


def verify_pins(
    policy: dict, configured: str, active_executable: str, version: str,
    system: str, process_arch: str, native_arch: str, translated: bool = False,
) -> tuple[Path, str]:
    """Verify one native target and the exact active interpreter bytes."""
    if not configured:
        raise RuntimeError("BASHY_TRANSCRIBE_PYTHON must explicitly name an external interpreter")
    if not isinstance(policy, dict):
        raise RuntimeError("external Python policy must be an object")
    targets = policy.get("targets")
    if policy.get("schema") != "s503-external-python-runtime-v1" or not isinstance(targets, dict) or set(targets) != TARGETS:
        raise RuntimeError("external Python policy is missing a reviewed six-target pin set")
    os_name = normalize_os(system)
    process = normalize_arch(process_arch)
    native = normalize_arch(native_arch)
    if process != native or translated:
        raise RuntimeError("external Python must run natively on the target OS and architecture")
    target_name = f"{os_name}/{native}"
    target = targets[target_name]
    if not isinstance(target, dict) or not isinstance(target.get("python_version"), str) or not isinstance(target.get("python_sha256"), str) or not VERSION.fullmatch(target["python_version"]) or not SHA256.fullmatch(target["python_sha256"]):
        raise RuntimeError(f"external Python pins are incomplete for {target_name}")
    if not isinstance(target.get("archive_url"), str) or not target["archive_url"].startswith("https://") or not isinstance(target.get("archive_sha256"), str) or not SHA256.fullmatch(target["archive_sha256"]):
        raise RuntimeError(f"external Python archive provenance is incomplete for {target_name}")
    selected = Path(configured).expanduser().resolve(strict=True)
    active = Path(active_executable).resolve(strict=True)
    if not selected.is_file() or selected != active:
        raise RuntimeError("BASHY_TRANSCRIBE_PYTHON must select the active interpreter")
    if version != target["python_version"]:
        raise RuntimeError(f"external Python version does not match the reviewed {target_name} pin")
    if windows_runtime.sha256_file(selected) != target["python_sha256"]:
        raise RuntimeError(f"external Python SHA-256 does not match the reviewed {target_name} pin")
    return selected, target_name


def _windows_native_arch() -> str:
    output = subprocess.check_output(
        ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
         "[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()"],
        text=True, timeout=30,
    ).strip()
    return normalize_arch(output)


def _darwin_translated() -> bool:
    result = subprocess.run(["sysctl", "-n", "sysctl.proc_translated"], capture_output=True, text=True, check=False, timeout=30)
    return result.returncode == 0 and result.stdout.strip() == "1"


def _verify_windows_vc(selected: Path, version: str, arch: str, python_pin: dict) -> None:
    policy = json.loads(windows_runtime.POLICY.read_text(encoding="utf-8"))
    target = policy.get("targets", {}).get(arch.upper())
    if not isinstance(target, dict) or any(
        target.get(key) != python_pin[key] for key in ("python_version", "python_sha256")
    ):
        raise RuntimeError("Windows VC policy and six-target Python pins disagree")
    system_root = os.environ.get("SystemRoot", "")
    if not system_root:
        raise RuntimeError("SystemRoot is absent for external Windows VC runtime check")
    windows_runtime.verify(
        policy, selected, version, arch.upper(), system_root,
        windows_runtime.windows_file_version,
    )


def verify_selected_python(configured: str) -> Path:
    """Resolve and verify the interpreter chosen by the direct source adapter."""
    if sys.maxsize <= 2**32:
        raise RuntimeError("a native 64-bit external Python is required")
    system = platform.system()
    process_arch = normalize_arch(platform.machine())
    native_arch = _windows_native_arch() if system == "Windows" else normalize_arch(platform.uname().machine)
    translated = _darwin_translated() if system == "Darwin" else False
    policy = json.loads(POLICY_PATH.read_text(encoding="utf-8"))
    selected, target_name = verify_pins(
        policy, configured, sys.executable, platform.python_version(),
        system, process_arch, native_arch, translated,
    )
    if system == "Windows":
        _verify_windows_vc(selected, platform.python_version(), native_arch, policy["targets"][target_name])
    return selected

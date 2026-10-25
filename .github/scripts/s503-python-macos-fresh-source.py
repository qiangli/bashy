#!/usr/bin/env python3
"""CI-only native macOS CPython source build into an absent private prefix.

This research lane never changes product/operator admission. It reuses the
Python.org 3.12.15 release pins and signature verifier from the Linux lane.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SETUP = ROOT / "examples/transcribe/dependencies/setup_python_source.py"
SOURCE_URL = "https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz"
PINS = {
    "Python-3.12.15.tar.xz": (20829824, "c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a"),
    "Python-3.12.15.tar.xz.asc": (4096, "e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6"),
    "thomas-wouters.gpg": (65536, "33c3af79f23a83c2a8a569bfd546601be400cb26dd7eb31f6d5e5b621667c391"),
}
KEY_FINGERPRINT = "7169605F62C751356D054A26A821E680E5FA6305"
MAX_FILES = 12000
MAX_NOTICES = 256
MAX_NOTICE_BYTES = 4 * 1024 * 1024
MAX_CONFIG_LOG_COPY_BYTES = 512 * 1024
MAX_CONFIG_LOG_TAIL_BYTES = 64 * 1024
NOTICE_WORDS = ("license", "licence", "copying", "copyright", "notice")
NOTICE_SUFFIXES = ("", ".txt", ".md", ".rst", ".rtf", ".html", ".htm")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def command(argv: list[str], *, timeout: int = 30, cwd: Path | None = None,
            env: dict[str, str] | None = None) -> str:
    result = subprocess.run(argv, capture_output=True, text=True, errors="replace",
                            cwd=cwd, env=env, timeout=timeout)
    require(result.returncode == 0, f"command failed: {argv[0]}: {result.stderr[-1000:]}")
    return result.stdout


def load_setup():
    spec = importlib.util.spec_from_file_location("s503_python_source_setup", SETUP)
    require(spec is not None and spec.loader is not None, "source verifier absent")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    require((module.VERSION, module.SOURCE_URL, module.SOURCE_SHA256,
             module.SOURCE_BYTES, module.SIGNATURE_SHA256, module.RELEASE_KEY) ==
            ("3.12.15", SOURCE_URL, PINS["Python-3.12.15.tar.xz"][1],
             PINS["Python-3.12.15.tar.xz"][0],
             PINS["Python-3.12.15.tar.xz.asc"][1], KEY_FINGERPRINT),
            "source setup pins differ from fixed macOS research pins")
    return module


def native_identity() -> dict:
    machine = platform.machine().lower()
    target = {"x86_64": "darwin/amd64", "arm64": "darwin/arm64"}.get(machine)
    require(platform.system() == "Darwin" and target == os.environ.get("EXPECTED_TARGET"),
            f"runner is not native {os.environ.get('EXPECTED_TARGET')}: {platform.system()}/{machine}")
    require(sys.maxsize > 2**32 and os.environ.get("GITHUB_ACTIONS") == "true" and
            os.environ.get("RUNNER_ENVIRONMENT") == "github-hosted",
            "requires 64-bit GitHub-hosted disposable runner")
    expected_label = "macos-15-intel" if target == "darwin/amd64" else "macos-15"
    require(os.environ.get("RUNNER_LABEL") == expected_label, "runner label differs")
    return {"system": platform.system(), "machine": machine, "target": target,
            "runner_label": expected_label, "macos": platform.mac_ver()[0],
            "bootstrap_python": sys.executable, "bootstrap_python_sha256": sha256(Path(sys.executable))}


def temp_root() -> Path:
    raw = os.environ.get("RUNNER_TEMP")
    require(bool(raw), "RUNNER_TEMP absent")
    path = Path(raw).resolve(strict=True)
    require(path.is_dir() and not Path(raw).is_symlink(), "RUNNER_TEMP is not a real directory")
    return path


def policy() -> dict:
    path = ROOT / "examples/transcribe/dependencies/operator-python-admission.json"
    data = json.loads(path.read_text(encoding="utf-8"))
    require(all(row["status"] == "unadmitted" for row in data["targets"].values()),
            "operator Python admission policy changed")
    return {"path": str(path.relative_to(ROOT)), "sha256": sha256(path),
            "product_admitted": False, "operator_admitted": False}


def framework_state() -> dict:
    executable = Path("/Library/Frameworks/Python.framework/Versions/3.12/bin/python3.12")
    packages = command(["/usr/sbin/pkgutil", "--pkgs"])
    ids = sorted(line for line in packages.splitlines()
                 if line.startswith("org.python.Python.") and line.endswith("-3.12"))
    return {"existing_executable": str(executable), "exists": executable.is_file(),
            "executable_sha256": sha256(executable) if executable.is_file() else None,
            "python_package_ids": ids,
            "python_package_info": {name: command(["/usr/sbin/pkgutil", "--pkg-info", name])
                                    for name in ids}}


def preflight(evidence: Path) -> dict:
    identity = native_identity()
    load_setup()
    temp_root()
    result = {"schema": "s503-python-macos-source-preflight-v1",
              "candidate_commit": os.environ["CANDIDATE_COMMIT"],
              "identity": identity, "source_verifier_sha256": sha256(SETUP),
              "policy": policy(), "system_framework_before": framework_state(),
              "result": "PASS_CANDIDATE_ONLY"}
    write_json(evidence / "preflight.json", result)
    return result


def inputs(input_dir: Path, evidence: Path) -> dict:
    require(input_dir.resolve(strict=True).is_relative_to(temp_root()) and
            not input_dir.is_symlink(), "input directory is outside RUNNER_TEMP")
    rows = {}
    for name, (limit, expected_hash) in PINS.items():
        path = input_dir / name
        require(path.is_file() and not path.is_symlink(), f"source input absent: {name}")
        size = path.stat().st_size
        require((size == limit if name.endswith(".tar.xz") else 0 < size <= limit) and
                sha256(path) == expected_hash, f"source input pin differs: {name}")
        rows[name] = {"bytes": size, "sha256": expected_hash}
    verified = load_setup().verify_source(input_dir / "Python-3.12.15.tar.xz",
                                          input_dir / "Python-3.12.15.tar.xz.asc",
                                          input_dir / "thomas-wouters.gpg")
    require(verified["release_key_fingerprint"] == KEY_FINGERPRINT,
            "signature signer fingerprint differs")
    result = {"schema": "s503-python-macos-source-inputs-v1", "source": verified,
              "inputs": rows, "detached_signature": "PASS", "result": "PASS_CANDIDATE_ONLY"}
    write_json(evidence / "inputs.json", result)
    return result


def brew_openssl(evidence: Path) -> dict:
    prefix = Path(command(["brew", "--prefix", "openssl@3"]).strip()).resolve(strict=True)
    require(prefix.is_dir(), "Homebrew OpenSSL 3 prefix absent")
    libraries = {}
    for name in ("libssl.3.dylib", "libcrypto.3.dylib"):
        path = (prefix / "lib" / name).resolve(strict=True)
        require(path.is_file() and path.is_relative_to(prefix), f"OpenSSL library absent: {name}")
        libraries[name] = {"path": str(path), "sha256": sha256(path), "bytes": path.stat().st_size}
    header = (prefix / "include/openssl/ssl.h").resolve(strict=True)
    require(header.is_file() and header.is_relative_to(prefix), "OpenSSL header absent")
    info = command(["brew", "info", "--json=v2", "openssl@3"], timeout=60)
    info_path = evidence / "openssl-brew-info.json"
    info_path.write_text(info, encoding="utf-8")
    metadata = json.loads(info)
    formulae = metadata.get("formulae", [])
    require(len(formulae) == 1 and formulae[0].get("name") == "openssl@3" and
            formulae[0].get("license") == "Apache-2.0", "OpenSSL formula/license changed")
    notices = notice_inventory(prefix, evidence, "openssl")
    return {"prefix": str(prefix), "formula_version": command(
                ["brew", "list", "--versions", "openssl@3"]).strip(),
            "brew_info_sha256": sha256(info_path), "license_declared": "Apache-2.0",
            "header": {"path": str(header), "sha256": sha256(header)},
            "libraries": libraries, "notices": notices}


def macos_sdk(env: dict[str, str]) -> dict:
    """Pass the native SDK to clang in the same restricted environment."""
    sdk = Path(command(["xcrun", "--sdk", "macosx", "--show-sdk-path"]).strip()).resolve(strict=True)
    stub = sdk / "usr/lib/libSystem.tbd"
    require(sdk.is_dir() and stub.is_file() and
            stub.resolve(strict=True).is_relative_to(sdk),
            "macOS SDK lacks the native libSystem linker stub")
    env["SDKROOT"] = str(sdk)
    return {"path": str(sdk), "libsystem_stub": str(stub),
            "libsystem_stub_sha256": sha256(stub)}


def notice_inventory(root: Path, evidence: Path, label: str) -> dict:
    paths = [path for path in sorted(root.rglob("*")) if path.is_file() and
             not path.is_symlink() and any(word in path.name.lower() for word in NOTICE_WORDS)
             and path.suffix.lower() in NOTICE_SUFFIXES]
    require(paths and len(paths) <= MAX_NOTICES, f"{label} notice inventory absent or oversized")
    target = evidence / (label + "-notices")
    target.mkdir(exist_ok=False)
    total = 0
    rows = []
    for index, path in enumerate(paths):
        size = path.stat().st_size
        total += size
        require(total <= MAX_NOTICE_BYTES, f"{label} notices exceed retained-byte bound")
        destination = target / f"{index:03d}-{path.name}"
        shutil.copyfile(path, destination)
        digest = sha256(path)
        require(sha256(destination) == digest, "retained notice copy changed")
        rows.append({"path": str(path), "relative_path": path.relative_to(root).as_posix(),
                     "bytes": size, "sha256": digest,
                     "retained_copy": str(destination.relative_to(evidence)),
                     "third_party_candidate": any(part.lower() in
                        {"vendor", "_vendor", "third_party", "third-party"}
                        for part in path.parts)})
    return {"files": rows, "total_bytes": total, "all_copies_verified": True,
            "third_party_candidates": sum(row["third_party_candidate"] for row in rows)}


def installed_files(prefix: Path) -> dict:
    rows = {}
    for path in sorted(prefix.rglob("*")):
        if path.is_symlink():
            resolved = path.resolve(strict=True)
            require(resolved.is_relative_to(prefix), "installed symlink escapes private prefix")
            continue
        if path.is_file():
            require(len(rows) < MAX_FILES, "installed file inventory exceeds bound")
            rows[path.relative_to(prefix).as_posix()] = {
                "bytes": path.stat().st_size, "sha256": sha256(path)}
    require("bin/python3.12" in rows, "private source executable absent")
    return rows


PROBE = r'''
import bz2,hashlib,json,lzma,os,platform,ssl,struct,sys,sysconfig,zlib,_hashlib,_ssl,_bz2,_lzma
def digest(path):
    h=hashlib.sha256()
    with open(path,'rb') as f:
        for block in iter(lambda:f.read(1048576),b''): h.update(block)
    return h.hexdigest()
modules={}
for name in ('zlib','bz2','_bz2','lzma','_lzma','ssl','_ssl','_hashlib'):
    mod=sys.modules.get(name) or __import__(name)
    path=getattr(mod,'__file__',None)
    modules[name]={'path':path,'sha256':digest(path) if path else None}
sample=b'S503 fresh-prefix codec probe'*8
print(json.dumps({'executable':sys.executable,'executable_sha256':digest(sys.executable),
 'prefix':sys.prefix,'base_prefix':sys.base_prefix,'version':platform.python_version(),
 'machine':platform.machine(),'platform':sysconfig.get_platform(),
 'config_args':sysconfig.get_config_var('CONFIG_ARGS'),'pointer_bits':struct.calcsize('P')*8,
 'openssl_version':ssl.OPENSSL_VERSION,'openssl_version_info':ssl.OPENSSL_VERSION_INFO,
 'modules':modules,'stdlib_roundtrip':{'zlib':zlib.decompress(zlib.compress(sample))==sample,
 'bz2':bz2.decompress(bz2.compress(sample))==sample,
 'lzma':lzma.decompress(lzma.compress(sample))==sample}},sort_keys=True))
'''


def runtime(prefix: Path, target: str, files: dict) -> dict:
    executable = prefix / "bin/python3.12"
    info = json.loads(command([str(executable), "-I", "-S", "-c", PROBE], timeout=60))
    require(info["version"] == "3.12.15" and info["pointer_bits"] == 64 and
            Path(info["prefix"]) == prefix and Path(info["base_prefix"]) == prefix and
            Path(info["executable"]).resolve(strict=True) == executable.resolve(strict=True) and
            info["executable_sha256"] == files["bin/python3.12"]["sha256"],
            "running interpreter is not the private source build")
    machine = "x86_64" if target == "darwin/amd64" else "arm64"
    require(info["machine"].lower() == machine and
            all(info["stdlib_roundtrip"].values()) and
            info["openssl_version"].startswith("OpenSSL 3."),
            "native architecture, codecs or OpenSSL probe failed")
    for name, module in info["modules"].items():
        path = Path(module["path"])
        require(path.resolve(strict=True).is_relative_to(prefix) and
                module["sha256"] == sha256(path),
                f"{name} was loaded from outside private prefix")
    require("/Library/Frameworks/Python.framework" not in json.dumps(info),
            "process reused system Python framework")
    return info


def macho_closure(prefix: Path, files: dict, openssl: dict, target: str) -> dict:
    machine = "x86_64" if target == "darwin/amd64" else "arm64"
    pending = [prefix / relative for relative in files if relative == "bin/python3.12"
               or relative.endswith((".so", ".dylib"))]
    require(pending, "native module inventory absent")
    rows = {}
    while pending:
        path = pending.pop()
        path = path.resolve(strict=True)
        if str(path) in rows:
            continue
        require(len(rows) < 512, "Mach-O dependency closure exceeds bound")
        kind = command(["file", "-b", str(path)]).strip()
        require("Mach-O" in kind, f"expected native Mach-O file: {path}")
        archs = command(["lipo", "-archs", str(path)]).split()
        require(machine in archs, f"native Mach-O architecture absent: {path}")
        links = []
        for line in command(["otool", "-L", str(path)]).splitlines()[1:]:
            raw = line.strip().split(" (", 1)[0]
            if not raw:
                continue
            if raw.startswith("/Library/Frameworks/Python.framework"):
                raise RuntimeError("Mach-O depends on preinstalled Python framework")
            resolved = None
            if raw.startswith("/"):
                resolved = Path(raw)
            elif raw.startswith("@loader_path/"):
                resolved = path.parent / raw[len("@loader_path/"):]
            elif raw.startswith("@executable_path/"):
                resolved = prefix / "bin" / raw[len("@executable_path/"):]
            elif raw.startswith("@rpath/"):
                name = raw[len("@rpath/"):]
                candidates = [Path(openssl["prefix"]) / "lib" / name,
                              prefix / "lib" / name, path.parent / name]
                resolved = next((item for item in candidates if item.is_file()), None)
            require(resolved is not None, f"unresolved Mach-O install name: {raw}")
            if resolved.is_file():
                resolved = resolved.resolve(strict=True)
                provenance = ("private_source_prefix" if resolved.is_relative_to(prefix) else
                              "homebrew_openssl_at_recorded_formula_version" if
                              resolved.is_relative_to(Path(openssl["prefix"])) else
                              "native_host_external_license_review_required")
                links.append({"install_name": raw, "path": str(resolved),
                              "sha256": sha256(resolved), "shared_cache": False,
                              "provenance": provenance})
                # Follow every on-disk non-system dependency, including a
                # Homebrew library linked by a native extension. System
                # libraries are represented by their direct install names,
                # hashes when present, and the explicit shared-cache limit.
                if (resolved.is_relative_to(prefix) or
                        not str(resolved).startswith(("/usr/lib/", "/System/Library/"))):
                    pending.append(resolved)
            else:
                require(str(resolved).startswith(("/usr/lib/", "/System/Library/")),
                        f"nonexistent non-system Mach-O dependency: {raw}")
                links.append({"install_name": raw, "path": str(resolved),
                              "sha256": None, "shared_cache": True,
                              "provenance": "macos_shared_cache_license_review_required"})
        rows[str(path)] = {"sha256": sha256(path), "archs": archs,
                           "file_kind": kind, "dependencies": links}
    linked = {link["path"] for row in rows.values() for link in row["dependencies"]}
    for item in openssl["libraries"].values():
        require(item["path"] in linked, "configured OpenSSL library is not in Mach-O closure")
    return {"files": rows, "shared_cache_license_status": "macOS system policy review required"}


def write_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def retain_config_log(path: Path, evidence: Path) -> dict:
    """Keep the actual configure diagnostic without uploading unbounded logs."""
    require(path.is_file() and not path.is_symlink(), "configure config.log absent")
    size = path.stat().st_size
    full = size <= MAX_CONFIG_LOG_COPY_BYTES
    destination = evidence / ("config.log" if full else "config-log-tail.txt")
    with path.open("rb") as source:
        if not full:
            source.seek(size - MAX_CONFIG_LOG_TAIL_BYTES)
        destination.write_bytes(source.read())
    return {"bytes": size, "sha256": sha256(path),
            "retained_copy": destination.name,
            "retained_copy_sha256": sha256(destination), "full_copy": full}


def build(input_dir: Path, evidence: Path) -> dict:
    prior = preflight(evidence)
    source = inputs(input_dir, evidence)
    target = prior["identity"]["target"]
    report = {"schema": "s503-python-macos-fresh-source-v1",
              "candidate_commit": os.environ["CANDIDATE_COMMIT"], "target": target,
              "runner_label": os.environ["RUNNER_LABEL"], "source": source,
              "candidate_scope": "alternate Python.org 3.12.15 source build; does not close the pinned 3.12.10 external installer policy",
              "policy": prior["policy"], "product_admitted": False,
              "operator_admitted": False, "result": "BLOCKED"}
    log = None
    config_log = None
    try:
        root = Path(tempfile.mkdtemp(prefix="s503-python-macos-", dir=temp_root()))
        os.chmod(root, 0o700)
        work, prefix = root / "work", root / "prefix"
        require(not work.exists() and not prefix.exists(), "source prefix was not fresh")
        report["fresh_prefix_absent_before"] = True
        report["private_root"] = str(root)
        work.mkdir(mode=0o700)
        setup = load_setup()
        archive = work / "Python-3.12.15.tar.xz"
        signature = work / "Python-3.12.15.tar.xz.asc"
        key = work / "thomas-wouters.gpg"
        for name, destination, limit in (("Python-3.12.15.tar.xz", archive, PINS["Python-3.12.15.tar.xz"][0]),
                                         ("Python-3.12.15.tar.xz.asc", signature, 4096),
                                         ("thomas-wouters.gpg", key, 65536)):
            setup.stage_input(input_dir / name, destination, limit)
        report["source_verified_again"] = setup.verify_source(archive, signature, key)
        extracted = setup.unpack(archive, work)
        config_log = extracted / "config.log"
        require(sha256(archive) == PINS["Python-3.12.15.tar.xz"][1],
                "source archive changed during extraction")
        report["source_notices"] = notice_inventory(extracted, evidence, "source")
        (extracted / "Modules/Setup.local").write_text(
            "*disabled*\nreadline\n_dbm\n_gdbm\n", encoding="ascii")
        openssl = brew_openssl(evidence)
        report["openssl"] = openssl
        env = setup.clean_env()
        env["CC"] = command(["xcrun", "--find", "clang"]).strip()
        env["PKG_CONFIG_PATH"] = str(Path(openssl["prefix"]) / "lib/pkgconfig")
        env["CPPFLAGS"] = "-I" + str(Path(openssl["prefix"]) / "include")
        env["LDFLAGS"] = "-L" + str(Path(openssl["prefix"]) / "lib")
        report["macos_sdk"] = macos_sdk(env)
        report["configure_env"] = {key: env[key] for key in
                                   ("CC", "CONFIG_SITE", "CPPFLAGS", "LDFLAGS",
                                    "PKG_CONFIG_PATH", "SDKROOT")}
        report["toolchain"] = {"clang": env["CC"], "clang_sha256": sha256(Path(env["CC"])),
                               "clang_version": command([env["CC"], "--version"]).splitlines()[0],
                               "xcode": command(["xcodebuild", "-version"]).splitlines()}
        smoke_source = root / "compiler-smoke.c"
        smoke_binary = root / "compiler-smoke"
        smoke_source.write_text("int main(void) { return 0; }\n", encoding="ascii")
        smoke_command = [env["CC"], env["CPPFLAGS"], str(smoke_source),
                         env["LDFLAGS"], "-o", str(smoke_binary)]
        smoke = subprocess.run(smoke_command, env=env, capture_output=True,
                               text=True, errors="replace", timeout=60)
        smoke_output = evidence / "compiler-smoke.txt"
        smoke_output.write_text(smoke.stdout + smoke.stderr, encoding="utf-8")
        report["compiler_smoke"] = {"argv": smoke_command,
                                     "exit_code": smoke.returncode,
                                     "output_sha256": sha256(smoke_output),
                                     "output_bytes": smoke_output.stat().st_size}
        require(smoke.returncode == 0 and smoke_binary.is_file(),
                "SDK-configured C compiler cannot create a native executable")
        smoke_run = subprocess.run([str(smoke_binary)], env=env, capture_output=True,
                                   text=True, errors="replace", timeout=30)
        report["compiler_smoke"]["executable_sha256"] = sha256(smoke_binary)
        report["compiler_smoke"]["run_exit_code"] = smoke_run.returncode
        require(smoke_run.returncode == 0,
                "SDK-configured C compiler executable cannot run natively")
        configure = [str(extracted / "configure"), "--prefix=" + str(prefix),
                     "--with-ensurepip=no", "--without-readline",
                     "--enable-option-checking=fatal", "--with-openssl=" + openssl["prefix"],
                     "--with-openssl-rpath=" + str(Path(openssl["prefix"]) / "lib")]
        report["build_commands"] = [configure, ["make", "-j", "2"], ["make", "altinstall"]]
        log = root / "build.log"
        with log.open("w", encoding="utf-8") as output:
            for argv, timeout in zip(report["build_commands"], (600, 2400, 600)):
                result = subprocess.run(argv, cwd=extracted, env=env, stdout=output,
                                        stderr=subprocess.STDOUT, timeout=timeout)
                require(result.returncode == 0, f"source build step failed: {argv[0]}")
        setup.remove_bundled_pip(prefix)
        shutil.copyfile(extracted / "LICENSE", prefix / "LICENSE.python-3.12.15")
        files = installed_files(prefix)
        report["installed_files"] = files
        report["runtime"] = runtime(prefix, target, files)
        report["installed_notices"] = notice_inventory(prefix, evidence, "installed")
        report["macho_closure"] = macho_closure(prefix, files, openssl, target)
        report["system_framework_after"] = framework_state()
        require(report["system_framework_after"] == prior["system_framework_before"],
                "system Python framework or package identity changed during source build")
        report["result"] = "FRESH_PREFIX_EVIDENCE_CAPTURED_UNADMITTED"
    except BaseException as error:
        report["error"] = str(error)
        raise
    finally:
        if config_log is not None and config_log.is_file():
            report["configure_log"] = retain_config_log(config_log, evidence)
        if log is not None and log.is_file():
            report["build_log_sha256"] = sha256(log)
            report["build_log_bytes"] = log.stat().st_size
            with log.open("rb") as source_log:
                source_log.seek(max(0, log.stat().st_size - 65536))
                tail = source_log.read()
            tail_path = evidence / "build-log-tail.txt"
            tail_path.write_bytes(tail)
            report["build_log_tail_sha256"] = sha256(tail_path)
        write_json(evidence / "result.json", report)
    return report


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("preflight", "inputs", "build"))
    parser.add_argument("--evidence", required=True, type=Path)
    parser.add_argument("--input-dir", type=Path)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    if args.action == "preflight":
        result = preflight(args.evidence)
    elif args.action == "inputs":
        require(args.input_dir is not None, "--input-dir required")
        result = inputs(args.input_dir, args.evidence)
    else:
        require(args.input_dir is not None, "--input-dir required")
        result = build(args.input_dir, args.evidence)
    print(json.dumps({"result": result["result"], "product_admitted": False,
                      "target": result.get("target", result.get("identity", {}).get("target"))},
                     sort_keys=True))


if __name__ == "__main__":
    main()

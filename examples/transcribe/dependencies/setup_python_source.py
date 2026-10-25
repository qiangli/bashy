#!/usr/bin/env python3
"""Operator-invoked CPython 3.12.15 Linux source setup; never called by adapters.

The receipt is local evidence, not an admitted release or a signed binary.
Inputs are supplied by the operator so verification can run without network.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile

VERSION = "3.12.15"
SOURCE_URL = "https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz"
SOURCE_SHA256 = "c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a"
SOURCE_BYTES = 20829824
SIGNATURE_SHA256 = "e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6"
RELEASE_KEY = "7169605F62C751356D054A26A821E680E5FA6305"
SCHEMA = "s503-linux-python-source-setup-v1"
FORBIDDEN_MODULES = {"readline", "_dbm", "_gdbm"}
FORBIDDEN_LIB = re.compile(r"(?:readline|gdbm|libdb(?:-|\.|$))", re.I)
MAX_FILES = 10000
MAX_UNPACK_BYTES = 200_000_000
MAX_SIGNATURE_BYTES = 4096
MAX_KEY_BYTES = 65536


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def target() -> str:
    if platform.system() != "Linux" or sys.maxsize <= 2**32:
        raise RuntimeError("source setup requires native 64-bit Linux")
    machine = platform.machine().lower()
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(machine)
    if not arch:
        raise RuntimeError(f"unsupported native Linux architecture: {machine}")
    return "linux/" + arch


def private_path(path: Path, *, existing: bool = False) -> Path:
    """Limit all build/write locations to the operator's real home tree."""
    home = Path.home().resolve(strict=True)
    resolved = path.expanduser().resolve(strict=existing)
    if resolved == home or not resolved.is_relative_to(home):
        raise RuntimeError(f"path must be beneath the operator home: {path}")
    if any(part.is_symlink() for part in [path.expanduser(), *path.expanduser().parents]):
        raise RuntimeError(f"path has a symlink component: {path}")
    return resolved


def verify_source(archive: Path, signature: Path, key: Path) -> dict:
    if archive.stat().st_size != SOURCE_BYTES or sha256(archive) != SOURCE_SHA256:
        raise RuntimeError("Python.org source archive size or SHA-256 mismatch")
    if not signature.is_file() or not key.is_file():
        raise RuntimeError("detached Python.org signature and release key are required")
    if sha256(signature) != SIGNATURE_SHA256:
        raise RuntimeError("Python.org detached source signature SHA-256 mismatch")
    with tempfile.TemporaryDirectory(prefix="s503-python-gpg-") as home:
        os.chmod(home, 0o700)
        imported = subprocess.run(["gpg", "--batch", "--no-tty", "--homedir", home,
                                   "--import", str(key)], capture_output=True, text=True, timeout=30)
        if imported.returncode:
            raise RuntimeError("could not import supplied Python release key")
        keys = subprocess.run(["gpg", "--batch", "--homedir", home, "--with-colons",
                               "--fingerprint", "--list-keys"], capture_output=True, text=True,
                              check=True, timeout=30)
        lines = keys.stdout.splitlines()
        primaries = [line for line in lines if line.startswith("pub:")]
        fingerprints = [line.split(":")[9].upper() for line in lines if line.startswith("fpr:")]
        if len(primaries) != 1 or not fingerprints or fingerprints[0] != RELEASE_KEY:
            raise RuntimeError("supplied release key fingerprint differs from Python.org pin")
        checked = subprocess.run(["gpg", "--batch", "--no-tty", "--homedir", home,
                                  "--status-fd=1", "--verify", str(signature), str(archive)],
                                 capture_output=True, text=True, timeout=30)
        valid = [line.split() for line in checked.stdout.splitlines() if line.startswith("[GNUPG:] VALIDSIG ")]
        if checked.returncode or len(valid) != 1 or RELEASE_KEY not in valid[0]:
            raise RuntimeError("Python.org detached source signature did not verify under pinned key")
    return {"url": SOURCE_URL, "sha256": SOURCE_SHA256, "bytes": SOURCE_BYTES,
            "signature_url": SOURCE_URL + ".asc", "release_key_fingerprint": RELEASE_KEY,
            "signature_sha256": SIGNATURE_SHA256}


def stage_input(source: Path, destination: Path, limit: int) -> None:
    """Copy one bounded regular input once; verification uses only this copy."""
    fd = os.open(source, os.O_RDONLY | os.O_NONBLOCK | getattr(os, "O_NOFOLLOW", 0))
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
            raise RuntimeError(f"source input is not a bounded regular file: {source}")
        with os.fdopen(fd, "rb", closefd=False) as incoming, destination.open("xb") as staged:
            copied = 0
            while block := incoming.read(min(1024 * 1024, limit - copied + 1)):
                copied += len(block)
                if copied > limit:
                    raise RuntimeError(f"source input exceeds byte bound: {source}")
                staged.write(block)
            staged.flush()
            os.fsync(staged.fileno())
        destination.chmod(0o400)
    finally:
        os.close(fd)


def unpack(archive: Path, work: Path) -> Path:
    source = work / ("Python-" + VERSION)
    if source.exists():
        raise RuntimeError("source directory already exists; use a fresh work directory")
    with tarfile.open(archive, "r:xz") as tar:
        members = tar.getmembers()
        if len(members) > MAX_FILES:
            raise RuntimeError("source archive has too many entries")
        if sum(member.size for member in members) > MAX_UNPACK_BYTES:
            raise RuntimeError("source archive exceeds unpacked byte bound")
        for member in members:
            path = Path(member.name)
            if path.is_absolute() or ".." in path.parts or not path.parts or path.parts[0] != source.name:
                raise RuntimeError("source archive contains an unsafe path")
            if not (member.isfile() or member.isdir()):
                raise RuntimeError("source archive contains a link or special file")
        tar.extractall(work)
    if not (source / "configure").is_file() or not (source / "LICENSE").is_file():
        raise RuntimeError("verified source archive lacks configure or LICENSE")
    return source


def clean_env() -> dict[str, str]:
    return {key: value for key in ("PATH", "HOME", "LANG", "LC_ALL", "TMPDIR")
            if (value := os.environ.get(key)) is not None} | {
                "CONFIG_SITE": "/dev/null", "PYTHONDONTWRITEBYTECODE": "1"}


def run(args: list[str], cwd: Path, env: dict[str, str]) -> None:
    subprocess.run(args, cwd=cwd, env=env, check=True)


def inventory(prefix: Path) -> dict[str, dict]:
    files: dict[str, dict] = {}
    for path in sorted(prefix.rglob("*")):
        if path.is_symlink():
            raise RuntimeError(f"installed tree contains a symlink: {path}")
        if path == prefix / "source-build-receipt.json":
            continue
        if path.is_file():
            if len(files) >= MAX_FILES:
                raise RuntimeError("installed tree exceeds file inventory bound")
            files[path.relative_to(prefix).as_posix()] = {"sha256": sha256(path), "bytes": path.stat().st_size}
    return files


def freeze_prefix(prefix: Path) -> None:
    """Avoid later import-generated pyc files invalidating the exact receipt."""
    for path in sorted(prefix.rglob("*"), key=lambda item: len(item.parts), reverse=True):
        if path.is_symlink():
            raise RuntimeError(f"installed tree contains a symlink: {path}")
        path.chmod(path.stat().st_mode & ~0o222)
    prefix.chmod(prefix.stat().st_mode & ~0o222)


def remove_bundled_pip(prefix: Path) -> None:
    """Omit bundled pip and test wheels; yt-dlp needs neither pip nor tests."""
    for relative in ("lib/python3.12/ensurepip/_bundled", "lib/python3.12/test"):
        directory = prefix / relative
        if directory.is_symlink():
            raise RuntimeError(f"installed wheel directory is a symlink: {relative}")
        if directory.exists():
            if not directory.is_dir():
                raise RuntimeError(f"installed wheel path is not a directory: {relative}")
            shutil.rmtree(directory)
    if any(prefix.rglob("*.whl")):
        raise RuntimeError("installed Python still contains a bundled wheel")


def dependencies(prefix: Path, files: dict[str, dict]) -> dict[str, dict]:
    result = {}
    for relative in files:
        path = prefix / relative
        if relative != "bin/python3.12" and not relative.endswith(".so"):
            continue
        checked = subprocess.run(["ldd", str(path)], capture_output=True, text=True, timeout=30)
        if checked.returncode or "not found" in checked.stdout:
            raise RuntimeError(f"unresolved dynamic dependency: {relative}")
        names = re.findall(r"(?:=>\s*)?(/[^\s()]+)", checked.stdout)
        if FORBIDDEN_LIB.search(checked.stdout):
            raise RuntimeError(f"forbidden or unreadable dynamic dependencies: {relative}")
        if not names:
            dynamic = subprocess.run(["readelf", "-d", str(path)], capture_output=True,
                                     text=True, timeout=30)
            if (dynamic.returncode or "Dynamic section at offset" not in dynamic.stdout
                    or re.search(r"\(NEEDED\)", dynamic.stdout)):
                raise RuntimeError(f"unreadable dynamic dependency closure: {relative}")
        result[relative] = {
            str(Path(name).resolve(strict=True)): {
                "sha256": sha256(Path(name).resolve(strict=True)),
                "license_status": "native_host_review_required",
            } for name in names}
    if "bin/python3.12" not in result:
        raise RuntimeError("installed executable dependency closure is absent")
    return result


def runtime(prefix: Path) -> dict:
    executable = prefix / "bin/python3.12"
    probe = ("import importlib,importlib.util,json,ssl,sys,sysconfig\n"
             "codecs = {}\n"
             "for name in ('zlib','bz2','lzma'):\n"
             "    try:\n"
             "        module = importlib.import_module(name)\n"
             "        sample = b'bashy-s503-python-source-codec-probe'\n"
             "        codecs[name] = module.decompress(module.compress(sample)) == sample\n"
             "    except (ImportError, OSError, ValueError):\n"
             "        codecs[name] = False\n"
             "print(json.dumps({'version':sys.version.split()[0], 'prefix':sys.prefix, "
             "'builtins':sorted(sys.builtin_module_names), 'stdlib':sorted(sys.stdlib_module_names), "
             "'openssl':ssl.OPENSSL_VERSION, 'modules':{name:importlib.util.find_spec(name) is not None "
             "for name in ('readline','_dbm','_gdbm','_ssl','_hashlib')}, "
             "'codec_roundtrip': codecs}))")
    output = subprocess.check_output([str(executable), "-I", "-S", "-c", probe],
                                     env=clean_env(), text=True, timeout=30)
    info = json.loads(output)
    if info["version"] != VERSION or Path(info["prefix"]).resolve() != prefix:
        raise RuntimeError("installed Python version or prefix differs from source setup")
    if FORBIDDEN_MODULES.intersection(info["builtins"]):
        raise RuntimeError("forbidden Python builtin module is linked")
    if any(info["modules"][name] for name in FORBIDDEN_MODULES):
        raise RuntimeError("forbidden Python extension module is installed")
    if not all(info["modules"][name] for name in ("_ssl", "_hashlib")):
        raise RuntimeError("required Python transport module is missing")
    if not all(info["codec_roundtrip"].get(name) is True for name in ("zlib", "bz2", "lzma")):
        raise RuntimeError("required Python archive codec import/round-trip failed")
    return info


def elf_machine(executable: Path, target_name: str) -> str:
    output = subprocess.check_output(["readelf", "-h", str(executable)], text=True, timeout=30)
    machines = [line.split(":", 1)[1].strip() for line in output.splitlines()
                if line.strip().startswith("Machine:")]
    expected = "Advanced Micro Devices X86-64" if target_name == "linux/amd64" else "AArch64"
    if machines != [expected]:
        raise RuntimeError(f"installed Python ELF machine differs from {target_name}: {machines}")
    return machines[0]


def audit(prefix: Path, source: dict, target_name: str, work: Path | None = None,
          toolchain: dict | None = None, expected_files: dict | None = None) -> dict:
    files = inventory(prefix)
    if expected_files is not None and files != expected_files:
        raise RuntimeError("installed Python differs from offline receipt: installed_files")
    if (not files or "sha256" not in files.get("bin/python3.12", {})
            or "LICENSE.python-3.12.15" not in files):
        raise RuntimeError("installed Python or source license is absent")
    info = runtime(prefix)
    machine = elf_machine(prefix / "bin/python3.12", target_name)
    libs = dependencies(prefix, files)
    for module, library in (("_ssl.", "libssl.so"), ("_hashlib.", "libcrypto.so")):
        extensions = [names for relative, names in libs.items()
                      if Path(relative).name.startswith(module)]
        if len(extensions) != 1 or not any(library in Path(path).name for path in extensions[0]):
            raise RuntimeError(f"{module} must expose an auditable dynamic {library} dependency")
    receipt = {"schema": SCHEMA, "status": "local_source_build_unadmitted",
               "target": target_name, "source": source, "prefix": str(prefix),
               "executable": str(prefix / "bin/python3.12"), "runtime": info,
               "elf_machine": machine,
               "installed_files": files, "dynamic_libraries": libs,
               "extension_modules": {name: value for name, value in files.items()
                                     if name.endswith(".so")},
               "license": {"python": "Python source LICENSE and bundled notices; see LICENSE.python-3.12.15",
                           "system_libraries": "native license review required per host",
                           "openssl": "native OpenSSL version and library license review required"},
               "toolchain": toolchain if toolchain is not None else {
                   "compiler": subprocess.check_output(["cc", "--version"], text=True,
                                                       timeout=30).splitlines()[0],
                   "libc": platform.libc_ver()},
               "work_dir": str(work) if work else None}
    return receipt


def setup(archive: Path, signature: Path, key: Path, work: Path, prefix: Path, jobs: int) -> dict:
    target_name = target()
    work, prefix = private_path(work), private_path(prefix)
    if work == prefix or work.is_relative_to(prefix) or prefix.is_relative_to(work):
        raise RuntimeError("work directory and install prefix must be separate")
    if prefix.exists() or work.exists():
        raise RuntimeError("work directory and prefix must be absent before setup")
    work.parent.mkdir(parents=True, exist_ok=True)
    if private_path(work) != work:
        raise RuntimeError("work directory parent changed before setup")
    work.mkdir(mode=0o700)
    try:
        if work.resolve(strict=True) != work or work.stat().st_mode & 0o077:
            raise RuntimeError("private work directory was replaced or is accessible to others")
        staged_archive = work / "Python-3.12.15.tar.xz"
        staged_signature = work / "Python-3.12.15.tar.xz.asc"
        staged_key = work / "release-key.gpg"
        stage_input(archive, staged_archive, SOURCE_BYTES)
        stage_input(signature, staged_signature, MAX_SIGNATURE_BYTES)
        stage_input(key, staged_key, MAX_KEY_BYTES)
        source_pin = verify_source(staged_archive, staged_signature, staged_key)
        source = unpack(staged_archive, work)
        if staged_archive.stat().st_size != SOURCE_BYTES or sha256(staged_archive) != SOURCE_SHA256:
            raise RuntimeError("private source archive changed during extraction")
        (source / "Modules/Setup.local").write_text("*disabled*\nreadline\n_dbm\n_gdbm\n", encoding="ascii")
        env = clean_env()
        run([str(source / "configure"), "--prefix=" + str(prefix), "--with-ensurepip=no",
             "--without-readline", "--enable-option-checking=fatal"], source, env)
        run(["make", "-j", str(jobs)], source, env)
        run(["make", "altinstall"], source, env)
        remove_bundled_pip(prefix)
        shutil.copyfile(source / "LICENSE", prefix / "LICENSE.python-3.12.15")
        receipt = audit(prefix, source_pin, target_name, work)
        destination = prefix / "source-build-receipt.json"
        with destination.open("x", encoding="utf-8") as out:
            json.dump(receipt, out, sort_keys=True, indent=2)
            out.write("\n")
        freeze_prefix(prefix)
        return {"receipt": str(destination), "executable": receipt["executable"],
                "executable_sha256": receipt["installed_files"]["bin/python3.12"]["sha256"],
                "status": receipt["status"]}
    except BaseException:
        shutil.rmtree(work)
        raise


def verify_existing(prefix: Path) -> dict:
    target_name = target()
    prefix = private_path(prefix, existing=True)
    path = prefix / "source-build-receipt.json"
    receipt = json.loads(path.read_text(encoding="utf-8"))
    if (receipt.get("schema") != SCHEMA or receipt.get("target") != target_name
            or receipt.get("status") != "local_source_build_unadmitted"
            or receipt.get("prefix") != str(prefix)
            or receipt.get("executable") != str(prefix / "bin/python3.12")
            or receipt.get("source", {}).get("url") != SOURCE_URL
            or receipt.get("source", {}).get("sha256") != SOURCE_SHA256
            or receipt.get("source", {}).get("bytes") != SOURCE_BYTES
            or receipt.get("source", {}).get("signature_url") != SOURCE_URL + ".asc"
            or receipt.get("source", {}).get("signature_sha256") != SIGNATURE_SHA256
            or receipt.get("source", {}).get("release_key_fingerprint") != RELEASE_KEY
            or not isinstance(receipt.get("installed_files"), dict)):
        raise RuntimeError("source receipt does not match pinned Linux Python setup")
    fresh = audit(prefix, receipt["source"], target_name,
                  toolchain=receipt.get("toolchain"), expected_files=receipt.get("installed_files"))
    for field in ("installed_files", "dynamic_libraries", "runtime", "license", "toolchain",
                  "elf_machine", "extension_modules"):
        if receipt.get(field) != fresh[field]:
            raise RuntimeError(f"installed Python differs from offline receipt: {field}")
    return {"receipt": str(path), "executable": receipt["executable"],
            "executable_sha256": receipt["installed_files"]["bin/python3.12"]["sha256"],
            "status": receipt["status"]}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-archive", type=Path)
    parser.add_argument("--signature", type=Path)
    parser.add_argument("--release-key", type=Path)
    parser.add_argument("--work-dir", type=Path)
    parser.add_argument("--prefix", type=Path, required=True)
    parser.add_argument("--jobs", type=int, default=2)
    parser.add_argument("--verify-existing", action="store_true")
    args = parser.parse_args()
    if args.jobs < 1 or args.jobs > 8:
        parser.error("--jobs must be between 1 and 8")
    if not args.verify_existing and not all((args.source_archive, args.signature, args.release_key, args.work_dir)):
        parser.error("source archive, signature, release key and work directory are required")
    try:
        result = (verify_existing(args.prefix) if args.verify_existing else
                  setup(args.source_archive, args.signature, args.release_key, args.work_dir,
                        args.prefix, args.jobs))
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, tarfile.TarError) as exc:
        print(f"Python source setup rejected: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

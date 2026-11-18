#!/usr/bin/env python3
"""Package completed, verified product bytes; never build or publish them."""
import gzip
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import zipfile

ROLES = ("bashy", "outpost", "bash", "sh")
MAIN = {"bashy": "github.com/qiangli/bashy/cmd/bashy",
        "outpost": "github.com/qiangli/outpost/cmd/outpost",
        "bash": "github.com/qiangli/bash/cmd/bash",
        "sh": "github.com/qiangli/bash/cmd/sh"}


def digest(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def build_info(path):
    return subprocess.check_output(["go", "version", "-m", str(path)], text=True)


def members(stage, goos, inspect=build_info, expected=None, arch=None):
    suffix = ".exe" if goos == "windows" else ""
    result = []
    for role in ROLES:
        name = role + suffix
        file = stage / name
        if not file.is_file() or file.is_symlink():
            raise ValueError(f"missing regular product: {name}")
        payload = stage / (name + ".real")
        if payload.exists():
            if goos == "windows" or role not in ("bash", "sh") or payload.is_symlink():
                raise ValueError(f"unexpected payload: {payload.name}")
            executable = payload
            result.append(payload.name)
        else:
            executable = file
        info = inspect(executable)
        if MAIN[role] not in [line.split()[1] for line in info.splitlines()
                              if len(line.split()) == 2 and line.split()[0] == "path"]:
            raise ValueError(f"wrong independent product role: {name}")
        if expected is not None:
            settings = dict(line.split()[1].split("=", 1) for line in info.splitlines()
                            if len(line.split()) == 2 and line.split()[0] == "build" and "=" in line.split()[1])
            if any(settings.get(key) != value for key, value in {"vcs.revision": expected[role],
                    "vcs.modified": "false", "GOOS": goos, "GOARCH": arch}.items()):
                raise ValueError(f"product source/platform differs: {name}")
        result.append(name)
    return sorted(result)


def archive(destination, files):
    if destination.suffix == ".zip":
        with zipfile.ZipFile(destination, "w", zipfile.ZIP_DEFLATED) as out:
            for name, file in sorted(files.items()):
                out.write(file, name)
    else:
        with tarfile.open(destination, "w:gz", compresslevel=6) as out:
            for name, file in sorted(files.items()):
                out.add(file, arcname=name, recursive=False)


def nfpm_config(stage, root, outpost_root, version, arch):
    files = [p for p in stage.iterdir() if p.name in ROLES or p.name in ("bash.real", "sh.real")]
    contents = [{"src": str(p), "dst": "/usr/lib/bashy/bin/" + p.name} for p in sorted(files)]
    contents += [{"src": "/usr/lib/bashy/bin/" + name, "dst": "/usr/bin/" + name, "type": "symlink"}
                 for name in ("bashy", "outpost")]
    contents += [{"src": str(source), "dst": "/usr/share/licenses/bashy/" + name}
                 for name, source in (("LICENSE", root / "LICENSE"), ("LICENSE.outpost", outpost_root / "LICENSE"))]
    return {"name": "bashy", "arch": arch, "platform": "linux", "version": version,
            "vendor": "dhnt", "homepage": "https://github.com/qiangli/bashy", "maintainer": "Qiang Li",
            "description": "Bashy userland, independent shell entry points, and outpost host agent",
            "license": "BSD-3-Clause AND MIT", "contents": contents,
            "scripts": {"postinstall": str(root / "scripts/linux-package-postinstall.sh")},
            "overrides": {"apk": {"depends": ["gcompat"]}}}


def package(root, outpost_root, staging, dist, tag, platforms, otel_platforms, nfpm, inspect=build_info, run=None, expected=None):
    assets = dist / "assets"
    if assets.exists():
        raise ValueError("refusing to replace an existing asset set")
    assets.mkdir(parents=True)
    metadata = dist / "metadata"
    metadata.mkdir(exist_ok=True)
    base = tag.removesuffix("-dev")
    layouts = {}
    for target in platforms:
        goos, arch = target.split("/")
        stage = staging / goos / arch
        names = members(stage, goos, inspect, expected=expected, arch=arch)
        layouts[target] = names
        docs = {"README.md": root / "README.md", "LICENSE": root / "LICENSE"}
        extension = "zip" if goos == "windows" else "tar.gz"
        archive(assets / f"bashy-{goos}-{arch}.{extension}",
                {**docs, "LICENSE.outpost": outpost_root / "LICENSE", **{name: stage / name for name in names}})
        archive(assets / f"bash-{goos}-{arch}.{extension}",
                {**docs, **{name: stage / name for name in names if name in ("bash", "sh", "bash.real", "sh.real", "bash.exe", "sh.exe")}})
        suffix = ".exe" if goos == "windows" else ""
        raw = assets / f"outpost-{base}-{goos}-{arch}{suffix}"
        shutil.copy2(stage / ("outpost" + suffix), raw)
        raw.with_name(raw.name + ".sha256").write_text(digest(raw) + "  " + raw.name + "\n")
        if goos == "linux":
            shutil.copy2(stage / "bashy-scratch", assets / f"bashy-scratch-linux-{arch}")
            config = metadata / f"nfpm-linux-{arch}.json"
            config.write_text(json.dumps(nfpm_config(stage, root, outpost_root, base.removeprefix("v") + "-dev", arch), indent=2))
            for fmt in ("deb", "rpm", "apk"):
                destination = assets / f"bashy_{base.removeprefix('v')}-dev_linux_{arch}.{fmt}"
                command = [str(nfpm), "package", "--config", str(config), "--packager", fmt, "--target", str(destination)]
                if run:
                    run("package-" + arch + "-" + fmt, command)
                else:
                    subprocess.run(command, check=True)
    for target in otel_platforms:
        goos, arch = target.split("/")
        suffix = ".exe" if goos == "windows" else ""
        with (staging / goos / arch / ("otel" + suffix)).open("rb") as src:
            with gzip.open(assets / f"otel-{goos}-{arch}.gz", "wb", compresslevel=6) as out:
                shutil.copyfileobj(src, out)
    (assets / "otel-blob-NOTICE.txt").write_text(
        "bashy otel blob — the embedded observability stack orchestrator\n"
        "(yoke/external/otel), built from source in this project.\n\n"
        "It is fetched at runtime and exec'd as a separate process; it is not linked\n"
        "into bashy. The storage/query components it supervises are NOT in this blob —\n"
        "VictoriaLogs, VictoriaMetrics and VictoriaTraces (all Apache-2.0) are fetched\n"
        "from their upstream releases on first use and run as subprocesses.\n")
    files = sorted(assets.iterdir())
    if len(files) != 33:
        raise ValueError(f"expected 33 assets before checksums, found {len(files)}")
    (assets / "checksums.txt").write_text("".join(digest(p) + "  " + p.name + "\n" for p in files))
    return layouts, [{"name": p.name, "sha256": digest(p), "size": p.stat().st_size}
                     for p in sorted(assets.iterdir())]

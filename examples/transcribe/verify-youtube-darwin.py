#!/usr/bin/env python3
"""Run the pinned S503.5 acquisition adapter on a claimed native arm64 Mac.

Keeps downloaded media under --work-root for downstream transcription. The
summary contains hashes and metadata only, so it can be committed as evidence.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import signal
import subprocess
import sys
import zipfile


YTDLP_VERSION = "2026.10.21"
YTDLP_SHA256 = "b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14"
DENO_VERSION = "2.9.8"
DENO_ARCHIVE_SHA256 = "9f0bf660393da2eaff044cdc7444cb08c73f235849b61d224d825a194ce7c98e"
DENO_EXECUTABLE_SHA256 = "d1e99265e5e6ee096c5780d88ff7d3e6cfdf33b1953a118b4889cab8ca6980ba"
EJS_VERSION = "0.8.0"
URLS = {
    "canonical": "https://www.youtube.com/watch?v=_j_PixI3Muo",
    "shortened": "https://youtu.be/_j_PixI3Muo?si=if_lqvXvU5GaM1nb",
}


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def run(args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, **kwargs).stdout.strip()


def verified_download(path, url, expected):
    if not path.exists():
        partial = path.with_name(path.name + ".download")
        try:
            subprocess.run(["/usr/bin/curl", "-fL", "--retry", "2", "--max-time", "180", "-o", str(partial), url], check=True)
            if sha256(partial) != expected:
                raise RuntimeError("download hash mismatch: " + path.name)
            partial.replace(path)
        finally:
            partial.unlink(missing_ok=True)
    actual = sha256(path)
    if actual != expected:
        raise RuntimeError("cached hash mismatch: " + path.name + " " + actual)
    return actual


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--work-root", type=Path, required=True)
    parser.add_argument("--source", type=Path, default=Path(__file__).with_name("source_youtube.py"))
    args = parser.parse_args()
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise RuntimeError("requires native Darwin arm64")
    if run(["/usr/sbin/sysctl", "-n", "sysctl.proc_translated"]) != "0":
        raise RuntimeError("translated process is not native arm64")
    if sys.version_info < (3, 10):
        raise RuntimeError("yt-dlp requires Python 3.10 or newer")

    root = args.work_root.expanduser().resolve()
    root.mkdir(parents=True, exist_ok=True)
    dependencies = root / "dependencies"
    dependencies.mkdir(exist_ok=True)
    yt_dlp = dependencies / "yt-dlp"
    deno_archive = dependencies / "deno-aarch64-apple-darwin.zip"
    verified_download(yt_dlp, "https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.21/yt-dlp", YTDLP_SHA256)
    verified_download(deno_archive, "https://github.com/denoland/deno/releases/download/v2.9.8/deno-aarch64-apple-darwin.zip", DENO_ARCHIVE_SHA256)
    deno = dependencies / "deno"
    if not deno.exists():
        with zipfile.ZipFile(deno_archive) as archive:
            if archive.namelist() != ["deno"]:
                raise RuntimeError("unexpected Deno archive contents")
            deno.write_bytes(archive.read("deno"))
        deno.chmod(0o755)
    if sha256(deno) != DENO_EXECUTABLE_SHA256:
        raise RuntimeError("Deno executable hash mismatch")

    with zipfile.ZipFile(yt_dlp) as archive:
        version_source = archive.read("yt_dlp_ejs/_version.py").decode()
    match = re.search(r"^__version__ = version = '([^']+)'$", version_source, re.M)
    if not match or match.group(1) != EJS_VERSION:
        raise RuntimeError("bundled yt-dlp-ejs version mismatch")
    if run([sys.executable, "-I", str(yt_dlp), "--version"]) != YTDLP_VERSION:
        raise RuntimeError("yt-dlp version mismatch")
    deno_output = run([str(deno), "--version"])
    if not deno_output.startswith("deno " + DENO_VERSION + " "):
        raise RuntimeError("Deno version mismatch")

    summary = {
        "schema": "s503-darwin-arm64-youtube-acquisition-v1",
        "identity": {
            "hostname": platform.node(),
            "system": platform.system(),
            "machine": platform.machine(),
            "macos": platform.mac_ver()[0],
            "proc_translated": "0",
        },
        "dependencies": {
            "python": sys.version.split()[0],
            "yt_dlp_version": YTDLP_VERSION,
            "yt_dlp_sha256": sha256(yt_dlp),
            "ejs_version": match.group(1),
            "deno_version": deno_output.splitlines()[0],
            "deno_archive_sha256": sha256(deno_archive),
            "deno_executable_sha256": sha256(deno),
        },
        "cases": [],
    }
    env = os.environ.copy()
    env.update(BASHY_TRANSCRIBE_YTDLP=str(yt_dlp), BASHY_TRANSCRIBE_JS_RUNTIME=str(deno), BASHY_TRANSCRIBE_PYTHON=sys.executable)
    for name, url in URLS.items():
        work = root / ("acquire-" + name)
        work.mkdir(exist_ok=True)
        request = work / "request.json"
        request.write_text(json.dumps({"version": "bashy-transcribe-stage-v1", "stage": "acquire", "input": url, "work_dir": str(work)}))
        proc = subprocess.Popen(
            [sys.executable, "-I", str(args.source.resolve()), str(request)],
            env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            start_new_session=True,
        )
        try:
            stdout, stderr = proc.communicate(timeout=180)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGTERM)
            try:
                proc.communicate(timeout=3)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.communicate()
            raise RuntimeError(f"{name} adapter timed out; process group stopped")
        (work / "result.json").write_text(stdout)
        (work / "stderr.log").write_text(stderr)
        if proc.returncode:
            raise RuntimeError(f"{name} adapter failed ({proc.returncode}); see {work / 'stderr.log'}")
        payload = json.loads(stdout)
        artifact = payload["artifacts"][0]
        media = Path(artifact["path"])
        if payload["media"]["video_id"] != "_j_PixI3Muo" or not media.is_file():
            raise RuntimeError("resolved identity or media missing")
        actual = sha256(media)
        if actual != artifact["sha256"]:
            raise RuntimeError("media hash mismatch")
        summary["cases"].append({
            "variant": name, "url": url, "video_id": payload["media"]["video_id"],
            "format_id": payload["media"]["format_id"], "duration_ms": payload["media"]["duration_ms"],
            "language": payload["media"]["language"], "media_bytes": media.stat().st_size,
            "media_sha256": actual, "private_media_path": str(media),
        })
    if summary["cases"][0]["media_sha256"] != summary["cases"][1]["media_sha256"]:
        raise RuntimeError("canonical and shortened URL media differ")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""yt-dlp backed acquire-stage adapter for one finite media URL.

The registered command supplies the path to the S503.6 verified yt-dlp
zipapp (reviewed pin b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14)
in BASHY_TRANSCRIBE_YTDLP. URLs and filesystem paths are always argv values;
this adapter never constructs shell source.
"""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from typing import Any
from urllib.parse import urlparse


VERSION = "bashy-transcribe-stage-v1"
STAGE = "acquire"
YTDLP_SHA256 = "b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14"
VIDEO_ID = re.compile(r"^[A-Za-z0-9_-]{1,128}$")
MEDIA_TYPES = {
    ".m4a": "audio/mp4",
    ".mp3": "audio/mpeg",
    ".ogg": "audio/ogg",
    ".opus": "audio/ogg",
    ".wav": "audio/wav",
    ".webm": "audio/webm",
}


class AcquisitionError(Exception):
    def __init__(self, code: str, message: str, retryable: bool = False):
        super().__init__(message)
        self.code = code
        self.retryable = retryable


def _error(code: str, message: str, retryable: bool = False) -> dict[str, Any]:
    return {
        "version": VERSION,
        "stage": STAGE,
        "error": {"code": code, "message": message, "retryable": retryable},
    }


def _run(command: list[str], cwd: Path) -> subprocess.CompletedProcess[str]:
    """Run yt-dlp with argv, forwarding cancellation to its process."""
    proc = subprocess.Popen(
        command,
        cwd=cwd,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    try:
        stdout, stderr = proc.communicate()
    except KeyboardInterrupt:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        raise AcquisitionError("cancelled", "yt-dlp download cancelled", True)
    return subprocess.CompletedProcess(command, proc.returncode, stdout, stderr)


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _youtube_host(url: str) -> bool:
    host = (urlparse(url).hostname or "").lower().rstrip(".")
    return host in {"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be", "www.youtu.be"}


def _yt_dlp(request: dict[str, Any], work_dir: Path, url: str) -> Path:
    zipapp = os.environ.get("BASHY_TRANSCRIBE_YTDLP", "")
    if not zipapp:
        raise AcquisitionError(
            "dependency_unavailable",
            "verified yt-dlp zipapp is unavailable; install the reviewed S503.6 dependency",
            True,
        )
    zipapp_path = Path(zipapp).expanduser().resolve()
    if not zipapp_path.is_file():
        raise AcquisitionError("dependency_unavailable", "configured yt-dlp zipapp does not exist", True)
    if _sha256(zipapp_path) != YTDLP_SHA256:
        raise AcquisitionError("dependency_unavailable", "yt-dlp zipapp SHA-256 does not match the reviewed S503.6 pin")

    # YouTube extraction needs an admitted JavaScript runtime and matching EJS
    # package. Never ask yt-dlp to fetch an unpinned remote EJS component.
    js_options = ["--no-remote-components"]
    if _youtube_host(url):
        runtime = os.environ.get("BASHY_TRANSCRIBE_JS_RUNTIME", "")
        runtime_path = Path(runtime).expanduser().resolve() if runtime else None
        if runtime_path is None or not runtime_path.is_file():
            raise AcquisitionError(
                "dependency_unavailable",
                "YouTube requires an admitted Deno runtime and matching yt-dlp-ejs package; set BASHY_TRANSCRIBE_JS_RUNTIME",
            )
        js_options += ["--no-js-runtimes", "--js-runtimes", "deno:" + str(runtime_path)]

    python = os.environ.get("BASHY_TRANSCRIBE_PYTHON") or sys.executable
    # Probe metadata without downloading. This rejects playlist-only and live
    # inputs before yt-dlp can create partial media files.
    inspect = _run(
        [
            python,
            "-I",
            str(zipapp_path),
            "--ignore-config",
            "--no-cookies",
            "--no-netrc",
            "--no-cache-dir",
            *js_options,
            "--no-playlist",
            "--dump-single-json",
            "--skip-download",
            "--no-warnings",
            url,
        ],
        work_dir,
    )
    if inspect.returncode != 0:
        message = (inspect.stderr or inspect.stdout).strip()
        raise AcquisitionError("download_failed", message or "yt-dlp could not resolve this URL", True)
    try:
        info = json.loads(inspect.stdout)
    except (json.JSONDecodeError, TypeError):
        raise AcquisitionError("download_failed", "yt-dlp returned invalid video metadata", True)
    if not isinstance(info, dict):
        raise AcquisitionError("download_failed", "yt-dlp returned invalid video metadata", True)
    if info.get("_type") in {"playlist", "multi_video"} or "entries" in info:
        raise AcquisitionError("unsupported_input", "playlists are not supported; provide one video URL")
    if info.get("is_live") or info.get("live_status") in {"is_live", "is_upcoming"}:
        raise AcquisitionError("unsupported_input", "live streams are not supported")
    video_id = info.get("id")
    if not isinstance(video_id, str) or not VIDEO_ID.fullmatch(video_id):
        raise AcquisitionError("download_failed", "yt-dlp returned an invalid video identifier", True)

    def remove_partial_downloads() -> None:
        for partial in work_dir.glob(video_id + ".*"):
            try:
                if partial.is_file():
                    partial.unlink()
            except OSError:
                pass

    try:
        downloaded = _run(
            [
                python,
                "-I",
                str(zipapp_path),
                "--ignore-config",
                "--no-cookies",
                "--no-netrc",
                "--no-cache-dir",
                *js_options,
                "--no-playlist",
                "--match-filter",
                "!is_live",
                "--format",
                "bestaudio",
                "--output",
                "%(id)s.%(ext)s",
                "--print",
                "after_move:filepath",
                "--no-progress",
                "--no-warnings",
                url,
            ],
            work_dir,
        )
    except AcquisitionError:
        remove_partial_downloads()
        raise
    if downloaded.returncode != 0:
        message = (downloaded.stderr or downloaded.stdout).strip()
        remove_partial_downloads()
        raise AcquisitionError("download_failed", message or "yt-dlp failed to download this video", True)

    # Require exactly one reported artifact, matching the probed video ID.
    lines = [line.strip() for line in downloaded.stdout.splitlines() if line.strip()]
    if len(lines) != 1:
        remove_partial_downloads()
        raise AcquisitionError("download_failed", "yt-dlp did not report exactly one media file", True)
    candidate = Path(lines[0])
    if not candidate.is_absolute():
        candidate = work_dir / candidate
    try:
        resolved = candidate.resolve(strict=True)
        resolved.relative_to(work_dir.resolve())
    except (OSError, ValueError):
        remove_partial_downloads()
        raise AcquisitionError("download_failed", "yt-dlp reported a missing or out-of-workspace file", True)
    if not resolved.is_file() or resolved.stat().st_size == 0:
        remove_partial_downloads()
        raise AcquisitionError("download_failed", "yt-dlp produced an empty media file", True)
    if resolved.parent != work_dir.resolve() or resolved.stem != video_id:
        remove_partial_downloads()
        raise AcquisitionError("download_failed", "yt-dlp output does not match the resolved video", True)
    return resolved


def acquire(request: dict[str, Any]) -> dict[str, Any]:
    if request.get("version") != VERSION or request.get("stage") != STAGE:
        raise AcquisitionError("invalid_request", "expected a bashy-transcribe-stage-v1 acquire request")
    url = request.get("input")
    if not isinstance(url, str) or not url.strip():
        raise AcquisitionError("invalid_request", "acquire request requires a nonempty URL")
    if "\x00" in url:
        raise AcquisitionError("invalid_request", "URL contains a NUL character")
    try:
        parsed = urlparse(url)
        if parsed.scheme not in {"http", "https"} or not parsed.hostname:
            raise AcquisitionError("invalid_request", "acquire input must be an HTTP(S) URL")
    except ValueError as exc:
        raise AcquisitionError("invalid_request", f"invalid URL: {exc}") from exc
    work_dir = request.get("work_dir")
    if not isinstance(work_dir, str) or not work_dir:
        raise AcquisitionError("invalid_request", "acquire request requires work_dir")
    directory = Path(work_dir).expanduser().resolve()
    if not directory.is_dir():
        raise AcquisitionError("invalid_request", "acquire work_dir must be an existing directory")
    media_path = _yt_dlp(request, directory, url)
    return {
        "version": VERSION,
        "stage": STAGE,
        "artifacts": [
            {
                "path": str(media_path),
                "media_type": MEDIA_TYPES.get(media_path.suffix.lower(), "application/octet-stream"),
                "sha256": _sha256(media_path),
            }
        ],
        "media": {"source": url},
    }


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: source_youtube.py REQUEST.json", file=sys.stderr)
        return 2
    try:
        request = json.loads(Path(argv[1]).read_text(encoding="utf-8"))
        if not isinstance(request, dict):
            raise AcquisitionError("invalid_request", "stage request must be a JSON object")
        result = acquire(request)
        print(json.dumps(result, separators=(",", ":")))
        return 0
    except AcquisitionError as exc:
        print(json.dumps(_error(exc.code, str(exc), exc.retryable), separators=(",", ":")))
        print(f"youtube source: {exc}", file=sys.stderr)
        return 130 if exc.code == "cancelled" else 1
    except (OSError, json.JSONDecodeError) as exc:
        result = _error("invalid_request", str(exc))
        print(json.dumps(result, separators=(",", ":")))
        print(f"youtube source: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))

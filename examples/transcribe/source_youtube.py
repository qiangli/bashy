#!/usr/bin/env python3
"""yt-dlp-backed acquisition adapter for the Bashy transcription stage v1."""

from __future__ import annotations

import hashlib
import json
import mimetypes
import os
import shutil
import subprocess
import sys
import uuid
from pathlib import Path
from urllib.parse import parse_qs, urlsplit


STAGE_VERSION = "bashy-transcribe-stage-v1"
YTDLP_SHA256 = "b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14"
YOUTUBE_HOSTS = {"youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be"}


class AcquisitionError(Exception):
    def __init__(self, code: str, message: str, retryable: bool = False):
        super().__init__(message)
        self.code = code
        self.retryable = retryable


def _fail(code: str, message: str, retryable: bool = False) -> None:
    raise AcquisitionError(code, message, retryable)


def _sha256_path(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _validate_url(value: str) -> str:
    if not isinstance(value, str) or not value:
        _fail("invalid_url", "media URL must be a nonempty string")
    try:
        parsed = urlsplit(value)
        host = (parsed.hostname or "").lower().rstrip(".")
        query = parse_qs(parsed.query, keep_blank_values=True)
    except ValueError as exc:
        _fail("invalid_url", f"invalid media URL: {exc}")
    if parsed.scheme != "https" or host not in YOUTUBE_HOSTS:
        _fail("unsupported_url", "the YouTube source accepts HTTPS youtube.com and youtu.be video URLs")
    if "list" in query or parsed.path.rstrip("/").lower() in {"/playlist", "/live"}:
        _fail("unsupported_input", "playlists and live streams are not supported")
    if host == "youtu.be":
        video_id = parsed.path.strip("/")
    elif parsed.path.rstrip("/").lower() == "/watch":
        video_id = (query.get("v") or [""])[0]
    elif parsed.path.lower().startswith("/shorts/"):
        video_id = parsed.path.split("/", 2)[2]
    else:
        video_id = ""
    if not video_id or any(c in video_id for c in "/?#"):
        _fail("invalid_url", "URL must identify one YouTube video")
    return value


def _zipapp_path() -> Path:
    configured = os.environ.get("YTDLP_ZIPAPP", "")
    if not configured:
        _fail("dependency_missing", "set YTDLP_ZIPAPP to the reviewed yt-dlp zipapp")
    path = Path(configured)
    if not path.is_file():
        _fail("dependency_missing", f"yt-dlp zipapp is unavailable: {path}")
    digest = _sha256_path(path)
    if digest != YTDLP_SHA256:
        _fail("dependency_tampered", f"yt-dlp zipapp SHA-256 does not match the reviewed pin: {path}")
    return path


def _run(argv: list[str]) -> tuple[int, str, str]:
    """Run one yt-dlp command without a shell; terminate it on cancellation."""
    process = subprocess.Popen(
        argv,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        shell=False,
    )
    try:
        stdout, stderr = process.communicate()
    except KeyboardInterrupt:
        process.terminate()
        try:
            process.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.communicate()
        raise
    return process.returncode, stdout, stderr


def _run_metadata(ytdlp: Path, url: str, runner=None) -> dict:
    runner = runner or _run
    rc, stdout, stderr = runner([
        sys.executable, str(ytdlp), "--ignore-config", "--no-cache-dir", "--no-warnings",
        "--no-progress", "--no-playlist", "--dump-single-json", url,
    ])
    if rc != 0:
        _raise_download_error(stderr, "yt-dlp could not resolve the video")
    try:
        info = json.loads(stdout)
    except (json.JSONDecodeError, TypeError):
        _fail("download_failed", "yt-dlp returned invalid video metadata", True)
    if info.get("_type") in {"playlist", "multi_video"} or info.get("entries") is not None:
        _fail("unsupported_input", "playlists and multi-video inputs are not supported")
    if info.get("is_live") is True or info.get("live_status") in {"is_live", "is_upcoming", "post_live"}:
        _fail("unsupported_input", "live streams and upcoming streams are not supported")
    return info


def _diagnostic(stderr: str, fallback: str) -> str:
    message = (stderr or "").strip().splitlines()
    return message[-1][:1000] if message else fallback


def _raise_download_error(stderr: str, fallback: str) -> None:
    message = _diagnostic(stderr, fallback)
    lowered = message.lower()
    unavailable = ("video unavailable", "video is unavailable", "has been removed", "private video",
                   "not available in your country", "members-only")
    if any(fragment in lowered for fragment in unavailable):
        _fail("video_unavailable", message, False)
    _fail("download_failed", message, True)


def _download(ytdlp: Path, url: str, folder: Path, runner=None) -> Path:
    runner = runner or _run
    template = str(folder / "media.%(ext)s")
    rc, stdout, stderr = runner([
        sys.executable, str(ytdlp), "--ignore-config", "--no-cache-dir", "--no-warnings",
        "--no-progress", "--no-playlist", "--no-simulate", "--format", "bestaudio",
        "--output", template, url,
    ])
    if rc != 0:
        _raise_download_error(stderr, "yt-dlp failed to download the video's audio")
    # The fixed template prevents titles and URL characters from becoming paths.
    files = [p for p in folder.glob("media.*") if p.is_file() and not p.name.endswith((".part", ".ytdl"))]
    if len(files) != 1 or files[0].stat().st_size == 0:
        _fail("download_failed", "yt-dlp did not produce exactly one nonempty audio artifact", True)
    return files[0]


def acquire(request: dict, ytdlp: Path | None = None, runner=None) -> dict:
    runner = runner or _run
    if request.get("version") != STAGE_VERSION or request.get("stage") != "acquire":
        _fail("invalid_request", "expected a bashy-transcribe-stage-v1 acquire request")
    url = _validate_url(request.get("input", ""))
    work_dir_value = request.get("work_dir")
    if not isinstance(work_dir_value, str) or not work_dir_value:
        _fail("invalid_request", "acquire request requires work_dir for owned download files")
    work_dir = Path(work_dir_value)
    if not work_dir.is_dir():
        _fail("invalid_request", f"work_dir does not exist: {work_dir}")
    ytdlp = ytdlp or _zipapp_path()

    owned_dir = work_dir / f"youtube-{uuid.uuid4().hex}"
    owned_dir.mkdir(mode=0o700)
    try:
        info = _run_metadata(ytdlp, url, runner)
        artifact = _download(ytdlp, url, owned_dir, runner)
        media_type = {
            ".webm": "audio/webm",
            ".m4a": "audio/mp4",
            ".opus": "audio/ogg",
            ".ogg": "audio/ogg",
            ".mp3": "audio/mpeg",
            ".wav": "audio/wav",
        }.get(artifact.suffix.lower(), mimetypes.guess_type(artifact.name)[0] or "application/octet-stream")
        media = {"source": "youtube"}
        for source_key, result_key in (("id", "video_id"), ("title", "title"), ("format_id", "format_id"), ("ext", "extension")):
            value = info.get(source_key)
            if isinstance(value, str) and value:
                media[result_key] = value
        result = {
            "version": STAGE_VERSION,
            "stage": "acquire",
            "artifacts": [{
                "path": str(artifact.resolve()),
                "media_type": media_type,
                "sha256": _sha256_path(artifact),
            }],
            "media": media,
        }
        duration = info.get("duration")
        if isinstance(duration, (float, int)) and duration >= 0:
            result["media"]["duration_ms"] = round(duration * 1000)
        language = info.get("language") or info.get("subtitles_language")
        if isinstance(language, str) and language:
            result["media"]["language"] = language
        return result
    except BaseException:
        shutil.rmtree(owned_dir, ignore_errors=True)
        raise


def main(argv: list[str] | None = None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    if len(argv) != 1:
        print("usage: source_youtube.py request.json", file=sys.stderr)
        return 2
    try:
        request = json.loads(Path(argv[0]).read_text(encoding="utf-8"))
        if not isinstance(request, dict):
            _fail("invalid_request", "request must be a JSON object")
        result = acquire(request)
    except AcquisitionError as exc:
        print(json.dumps({
            "version": STAGE_VERSION,
            "stage": "acquire",
            "error": {"code": exc.code, "message": str(exc), "retryable": exc.retryable},
        }, separators=(",", ":")))
        print(f"youtube source: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        error = AcquisitionError("cancelled", "YouTube audio download cancelled")
        print(json.dumps({
            "version": STAGE_VERSION,
            "stage": "acquire",
            "error": {"code": error.code, "message": str(error), "retryable": False},
        }, separators=(",", ":")))
        print(f"youtube source: {error}", file=sys.stderr)
        return 130
    except (OSError, json.JSONDecodeError) as exc:
        print(json.dumps({
            "version": STAGE_VERSION,
            "stage": "acquire",
            "error": {"code": "invalid_request", "message": str(exc), "retryable": False},
        }, separators=(",", ":")))
        print(f"youtube source: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(result, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

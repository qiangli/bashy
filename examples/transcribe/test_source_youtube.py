"""Controlled tests for the yt-dlp transcription source adapter."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


ADAPTER = Path(__file__).with_name("source_youtube.py")
VERSION = "bashy-transcribe-stage-v1"


FAKE_YTDLP = r'''import json, os, pathlib, sys, time
args = sys.argv[1:]
with open(os.environ["BASHY_TEST_FAKE_LOG"], "a", encoding="utf-8") as log:
    log.write(json.dumps(args) + "\n")
mode = os.environ.get("BASHY_TEST_MODE", "ok")
url = args[-1]
if mode == "hold":
    if "--dump-single-json" not in args:
        pathlib.Path.cwd().joinpath("fixture01.webm.part").write_bytes(b"partial")
    time.sleep(60)
if mode == "resolve_fail" or url.startswith("invalid:"):
    print("video unavailable", file=sys.stderr)
    raise SystemExit(1)
if "--dump-single-json" in args:
    if mode == "playlist":
        print(json.dumps({"_type": "playlist", "entries": [], "id": "list"}))
    elif mode == "live":
        print(json.dumps({"id": "fixture01", "is_live": True}))
    else:
        print(json.dumps({"id": "fixture01", "title": "controlled fixture"}))
    raise SystemExit(0)
if mode == "download_fail":
    pathlib.Path.cwd().joinpath("fixture01.webm.part").write_bytes(b"partial")
    print("download failed", file=sys.stderr)
    raise SystemExit(1)
path = pathlib.Path.cwd() / "fixture01.webm"
path.write_bytes(b"controlled audio bytes")
print(str(path))
'''


class YouTubeSourceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="yt fixture ; [safe] ")
        self.root = Path(self.temp.name)
        self.work = self.root / "work ; [1]"
        self.work.mkdir()
        self.zipapp = self.root / "controlled yt-dlp.pyz"
        self.zipapp.write_text(FAKE_YTDLP, encoding="utf-8")
        self.log = self.root / "argv.jsonl"
        self.request = self.root / "request.json"
        self.env = os.environ.copy()
        self.env.update(
            {
                "BASHY_TRANSCRIBE_YTDLP": str(self.zipapp),
                "BASHY_TRANSCRIBE_PYTHON": sys.executable,
                "BASHY_TEST_FAKE_LOG": str(self.log),
                "BASHY_TEST_MODE": "ok",
            }
        )

    def tearDown(self) -> None:
        self.temp.cleanup()

    def invoke(self, url: str, *, mode: str = "ok", timeout: float = 10) -> subprocess.CompletedProcess[str]:
        self.env["BASHY_TEST_MODE"] = mode
        self.request.write_text(
            json.dumps(
                {
                    "version": VERSION,
                    "stage": "acquire",
                    "input": url,
                    "work_dir": str(self.work),
                }
            ),
            encoding="utf-8",
        )
        return subprocess.run(
            [sys.executable, str(ADAPTER), str(self.request)],
            text=True,
            capture_output=True,
            env=self.env,
            timeout=timeout,
        )

    def test_download_produces_artifact_hash_and_preserves_url_as_one_argument(self) -> None:
        url = "https://youtu.be/fixture01?x=a;b&y=$(touch nope) [literal]"
        result = self.invoke(url)
        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        artifact = payload["artifacts"][0]
        media_path = Path(artifact["path"])
        self.assertEqual(payload["version"], VERSION)
        self.assertEqual(payload["stage"], "acquire")
        self.assertTrue(media_path.is_file())
        self.assertEqual(media_path.parent, self.work.resolve())
        self.assertEqual(artifact["media_type"], "audio/webm")
        self.assertEqual(artifact["sha256"], hashlib.sha256(media_path.read_bytes()).hexdigest())
        calls = [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines()]
        self.assertEqual(calls[0][-1], url)
        self.assertEqual(calls[1][-1], url)
        self.assertIn("bestaudio", calls[1])
        self.assertIn("--no-playlist", calls[1])
        self.assertIn("--format", calls[1])
        self.assertEqual([path.resolve() for path in self.work.iterdir()], [media_path.resolve()])

    def test_adapter_accepts_other_ytdlp_supported_sites_when_selected(self) -> None:
        url = "https://media.example.test/watch?id=fixture01"
        result = self.invoke(url)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["stage"], "acquire")
        calls = [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines()]
        self.assertEqual(calls[0][-1], url)

    def test_unavailable_url_is_reported_as_stage_error(self) -> None:
        result = self.invoke("invalid://unavailable", mode="resolve_fail")
        self.assertNotEqual(result.returncode, 0)
        payload = json.loads(result.stdout)
        self.assertEqual(payload["error"]["code"], "download_failed")
        self.assertIn("unavailable", payload["error"]["message"])
        self.assertEqual(list(self.work.iterdir()), [])

    def test_playlist_is_rejected_before_download(self) -> None:
        result = self.invoke("https://youtu.be/playlist?list=fixture", mode="playlist")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["error"]["code"], "unsupported_input")
        self.assertEqual(len(self.log.read_text(encoding="utf-8").splitlines()), 1)
        self.assertEqual(list(self.work.iterdir()), [])

    def test_live_stream_is_rejected_before_download(self) -> None:
        result = self.invoke("https://youtube.com/watch?v=fixture01", mode="live")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["error"]["code"], "unsupported_input")
        self.assertEqual(len(self.log.read_text(encoding="utf-8").splitlines()), 1)

    def test_download_failure_reports_error_without_partial_success(self) -> None:
        result = self.invoke("https://youtube.com/watch?v=fixture01", mode="download_fail")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["error"]["code"], "download_failed")
        self.assertNotIn("artifacts", json.loads(result.stdout))
        self.assertEqual(list(self.work.iterdir()), [])

    def test_cancellation_terminates_yt_dlp_child_and_reports_cancelled(self) -> None:
        self.env["BASHY_TEST_MODE"] = "hold"
        self.request.write_text(
            json.dumps(
                {
                    "version": VERSION,
                    "stage": "acquire",
                    "input": "https://youtube.com/watch?v=fixture01",
                    "work_dir": str(self.work),
                }
            ),
            encoding="utf-8",
        )
        proc = subprocess.Popen(
            [sys.executable, str(ADAPTER), str(self.request)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=self.env,
        )
        deadline = time.monotonic() + 5
        while not self.log.exists() and time.monotonic() < deadline:
            time.sleep(0.02)
        self.assertTrue(self.log.exists(), "controlled yt-dlp process did not start")
        proc.send_signal(signal.SIGINT)
        stdout, _ = proc.communicate(timeout=5)
        self.assertEqual(proc.returncode, 130)
        self.assertEqual(json.loads(stdout)["error"]["code"], "cancelled")
        # The child process is terminated before the stage returns.
        self.assertEqual(list(self.work.iterdir()), [])

    def test_missing_verified_dependency_is_actionable(self) -> None:
        self.env.pop("BASHY_TRANSCRIBE_YTDLP")
        result = self.invoke("https://youtube.com/watch?v=fixture01")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)["error"]["code"], "dependency_unavailable")


if __name__ == "__main__":
    unittest.main()

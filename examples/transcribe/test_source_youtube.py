"""Controlled unit tests for the replaceable yt-dlp acquisition adapter."""

import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import source_youtube as adapter


class ControlledYtDlp:
    def __init__(self, metadata=None, download_rc=0, cancel_download=False):
        self.metadata = metadata or {"id": "abc123", "duration": 1.25, "language": "en"}
        self.download_rc = download_rc
        self.cancel_download = cancel_download
        self.calls = []

    def __call__(self, argv):
        self.calls.append(argv)
        if "--dump-single-json" in argv:
            return 0, json.dumps(self.metadata), ""
        if self.cancel_download:
            template = argv[argv.index("--output") + 1]
            Path(template.replace("%(ext)s", "webm.part")).write_bytes(b"partial")
            raise KeyboardInterrupt
        if self.download_rc:
            return self.download_rc, "", "video unavailable"
        template = argv[argv.index("--output") + 1]
        path = Path(template.replace("%(ext)s", "webm"))
        path.write_bytes(b"controlled audio bytes")
        return 0, str(path), ""


class YouTubeSourceTests(unittest.TestCase):
    def request(self, work_dir, url="https://www.youtube.com/watch?v=abc123"):
        return {"version": adapter.STAGE_VERSION, "stage": "acquire", "input": url,
                "work_dir": str(work_dir)}

    def test_download_result_uses_stage_contract_and_fixed_filename(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "work dir [1]"
            root.mkdir()
            runner = ControlledYtDlp()
            url = "https://youtu.be/abc123?si=x%26%3B%24%28touch%20PWNED%29"
            result = adapter.acquire(self.request(root, url), Path("yt-dlp.py"), runner)
            artifact = result["artifacts"][0]
            self.assertEqual(result["version"], adapter.STAGE_VERSION)
            self.assertEqual(result["stage"], "acquire")
            self.assertEqual(result["media"], {
                "source": "youtube", "video_id": "abc123", "duration_ms": 1250, "language": "en"
            })
            self.assertEqual(Path(artifact["path"]).name, "media.webm")
            self.assertIn("work dir [1]", artifact["path"])
            self.assertEqual(artifact["media_type"], "audio/webm")
            self.assertEqual(len(artifact["sha256"]), 64)
            self.assertEqual(len(runner.calls), 2)
            self.assertEqual(runner.calls[0][-1], url)
            self.assertEqual(runner.calls[1][-1], url)
            self.assertFalse((root / "PWNED").exists())
            # URL metacharacters remain data in one argv slot for each call.
            self.assertIsInstance(runner.calls[0][-1], str)
            self.assertFalse(any("touch PWNED" == arg for call in runner.calls for arg in call))

    def test_invalid_and_playlist_urls_are_rejected_before_process_execution(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for url, code in [
                ("not a URL; touch /tmp/pwned", "unsupported_url"),
                ("https://www.youtube.com/watch?v=abc123&list=PL123", "unsupported_input"),
                ("https://www.youtube.com/playlist?list=PL123", "unsupported_input"),
            ]:
                runner = ControlledYtDlp()
                with self.subTest(url=url), self.assertRaises(adapter.AcquisitionError) as caught:
                    adapter.acquire(self.request(root, url), Path("yt-dlp.py"), runner)
                self.assertEqual(caught.exception.code, code)
                self.assertEqual(runner.calls, [])

    def test_unavailable_video_and_live_metadata_fail_without_artifacts(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            runner = ControlledYtDlp(download_rc=1)
            with self.assertRaises(adapter.AcquisitionError) as caught:
                adapter.acquire(self.request(root), Path("yt-dlp.py"), runner)
            self.assertEqual(caught.exception.code, "video_unavailable")
            self.assertIn("video unavailable", str(caught.exception))
            self.assertEqual(list(root.iterdir()), [])

            live = ControlledYtDlp(metadata={"id": "abc123", "is_live": True})
            with self.assertRaises(adapter.AcquisitionError) as caught:
                adapter.acquire(self.request(root), Path("yt-dlp.py"), live)
            self.assertEqual(caught.exception.code, "unsupported_input")
            self.assertEqual(len(live.calls), 1)
            self.assertEqual(list(root.iterdir()), [])

    def test_cancellation_cleans_owned_partial_download(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            runner = ControlledYtDlp(cancel_download=True)
            with self.assertRaises(KeyboardInterrupt):
                adapter.acquire(self.request(root), Path("yt-dlp.py"), runner)
            self.assertEqual(list(root.iterdir()), [])

    def test_cli_reports_unavailable_and_cancellation_without_success_artifacts(self):
        for runner, expected_status, expected_code in [
            (ControlledYtDlp(download_rc=1), 1, "video_unavailable"),
            (ControlledYtDlp(cancel_download=True), 130, "cancelled"),
        ]:
            with self.subTest(expected_code=expected_code), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                request = root / "request.json"
                request.write_text(json.dumps(self.request(root)), encoding="utf-8")
                stdout, stderr = io.StringIO(), io.StringIO()
                with mock.patch.object(adapter, "_zipapp_path", return_value=Path("yt-dlp.py")), \
                     mock.patch.object(adapter, "_run", side_effect=runner), \
                     contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                    status = adapter.main([str(request)])
                result = json.loads(stdout.getvalue())
                self.assertEqual(status, expected_status)
                self.assertEqual(result["error"]["code"], expected_code)
                self.assertNotIn("artifacts", result)
                self.assertTrue(stderr.getvalue())
                self.assertEqual(list(root.glob("youtube-*")), [])


if __name__ == "__main__":
    unittest.main()

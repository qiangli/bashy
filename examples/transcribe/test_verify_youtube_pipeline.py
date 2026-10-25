#!/usr/bin/env python3
"""Pure validation tests; no native inference or substitute transcript."""

import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest import mock


path = Path(__file__).with_name("verify-youtube-pipeline.py")
spec = importlib.util.spec_from_file_location("youtube_pipeline_gate", path)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class GateValidationTest(unittest.TestCase):
    def test_reference_tolerance_requires_meaningful_speech(self):
        reference = "the model runs four real tests on a macbook"
        self.assertEqual(gate.reference_score("The model runs four real tests on a MacBook.", reference), 1)
        self.assertLess(gate.reference_score("music and silence with no relevant speech", reference), .5)
        with self.assertRaisesRegex(ValueError, "six words"):
            gate.reference_score("some words", "too short")

    def test_subtitle_intervals_and_bounds(self):
        srt = "1\n00:00:00,100 --> 00:00:01,200\nhello\n\n2\n00:00:01,200 --> 00:00:02,000\nworld\n"
        vtt = "WEBVTT\n\n00:00:00.100 --> 00:00:01.200\nhello\n"
        self.assertEqual(gate.subtitle_intervals(srt, "srt", 2000), [(100, 1200), (1200, 2000)])
        self.assertEqual(gate.subtitle_intervals(vtt, "vtt", 2000), [(100, 1200)])
        with self.assertRaisesRegex(ValueError, "outside media duration"):
            gate.subtitle_intervals(srt, "srt", 1999)
        with self.assertRaisesRegex(ValueError, "overlap"):
            gate.subtitle_intervals(srt.replace("00:00:01,200 --> 00:00:02,000",
                                                "00:00:01,100 --> 00:00:02,000"), "srt", 2000)
        with self.assertRaisesRegex(ValueError, "header"):
            gate.subtitle_intervals(srt, "vtt", 2000)

    def test_missing_native_manifest_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "unknown manifest schema"):
            gate.validated_manifest({})
        with self.assertRaisesRegex(ValueError, "candidate commit"):
            gate.validated_manifest({"schema": "s503-youtube-pipeline-v1"})

    def test_manifest_binds_two_url_forms_and_file_hashes(self):
        with tempfile.TemporaryDirectory() as root:
            base = Path(root)
            files = {}
            for name in ("bashy", "helper", "python", "yt_dlp", "deno", "whisper", "controlled_webm"):
                file = base / name
                file.write_bytes(name.encode())
                files[name] = str(file)
                if name != "python":
                    files[name + "_sha256"] = gate.digest(file)
            model = base / "models" / "whisper" / "ggml-base.bin"
            model.parent.mkdir(parents=True)
            model.write_bytes(b"model")
            files.update(model=str(model), model_sha256=gate.digest(model), model_id="base")
            media = base / "operator.webm"
            media.write_bytes(b"real acquired bytes are required in the native run")
            media_sha = gate.digest(media)
            case = {"duration_ms": 499000, "reference": "six reviewed words from actual speech here",
                    "min_score": .7}
            m = dict(files, schema="s503-youtube-pipeline-v1", candidate_commit="c32bf04d",
                     os=gate.platform.system().lower(), arch=gate.platform.machine().lower(),
                     acquisition_evidence={"windows_amd64": "run165", "darwin_arm64": "run168"},
                     decoder={"name": "transcribe-webm", "exec": [str(base / "helper"), "webm"],
                              "sha256": gate.digest(base / "helper"), "license_evidence": "reviewed"},
                     controlled=dict(case), operator=dict(case),
                     operator_media={label: {"url": url, "video_id": "_j_PixI3Muo",
                                             "duration_ms": 499000, "sha256": media_sha,
                                             "path": str(media)}
                                     for label, url in gate.URLS.items()})
            with mock.patch.object(gate, "YOUTUBE_SHA256", media_sha), \
                 mock.patch.object(gate, "YTDLP_SHA256", gate.digest(base / "yt_dlp")):
                self.assertIs(gate.validated_manifest(m), m)
                m["operator_media"]["shortened"]["video_id"] = "wrong"
                with self.assertRaisesRegex(ValueError, "shortened URL identity"):
                    gate.validated_manifest(m)

    def test_cleanup_detects_both_owned_locations(self):
        with tempfile.TemporaryDirectory() as root:
            outdir, tempdir = Path(root) / "out", Path(root) / "tmp"
            outdir.mkdir()
            tempdir.mkdir()
            gate.assert_no_owned_temp(outdir, tempdir)
            (outdir / ".transcribe.orphan").write_text("x")
            with self.assertRaisesRegex(ValueError, "output temporary"):
                gate.assert_no_owned_temp(outdir, tempdir)
            (outdir / ".transcribe.orphan").unlink()
            (tempdir / "transcribe-orphan").mkdir()
            with self.assertRaisesRegex(ValueError, "owned work"):
                gate.assert_no_owned_temp(outdir, tempdir)


if __name__ == "__main__":
    unittest.main(verbosity=2)

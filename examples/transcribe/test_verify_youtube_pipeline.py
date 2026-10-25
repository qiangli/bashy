#!/usr/bin/env python3
"""Pure validation tests; no native inference or substitute transcript."""

import importlib.util
import json
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
        for broken in (
            srt + "\nmalformed extra cue\n",
            srt.replace("\nhello\n", "\n\n"),
            srt.replace("\n2\n", "\n3\n"),
            srt.replace("00:00:01,200 -->", "00:00:01.200 -->"),
            vtt + "\n00:bad --> 00:00:02.000\nextra\n",
        ):
            with self.assertRaises(ValueError):
                gate.subtitle_intervals(broken, "vtt" if broken.startswith("WEBVTT") else "srt", 2000)

    def test_missing_native_manifest_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "unknown manifest schema"):
            gate.validated_manifest({})
        with self.assertRaisesRegex(ValueError, "candidate commit"):
            gate.validated_manifest({"schema": "s503-youtube-pipeline-v1",
                                     "candidate_commit": "deadbeef"})

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
            media = {}
            for label in gate.URLS:
                file = base / (label + ".webm")
                file.write_bytes((label + " acquired bytes").encode())
                media[label] = file
            case = {"duration_ms": 499000, "reference": "six reviewed words from actual speech here",
                    "min_score": .7}
            source_sha = {source: gate.digest(gate.ROOT / source) for source in gate.SOURCE_FILES}
            m = dict(files, schema="s503-youtube-pipeline-v1", candidate_commit="a" * 40,
                     source_sha256=dict(source_sha),
                     os=gate.platform.system().lower(), arch=gate.platform.machine().lower(),
                     acquisition_evidence={"windows_amd64": "run165", "darwin_arm64": "run168"},
                     decoder={"name": "transcribe-webm", "exec": [str(base / "helper"), "webm"],
                              "sha256": gate.digest(base / "helper"), "license_evidence": "reviewed"},
                     controlled=dict(case), operator=dict(case),
                     operator_media={label: {"url": url, "video_id": "_j_PixI3Muo",
                                             "duration_ms": 499000, "sha256": gate.digest(media[label]),
                                             "path": str(media[label])}
                                     for label, url in gate.URLS.items()})
            with mock.patch.object(gate, "checkout_commit", return_value="a" * 40), \
                 mock.patch.object(gate, "committed_digest", side_effect=source_sha.get), \
                 mock.patch.object(gate, "YTDLP_SHA256", gate.digest(base / "yt_dlp")):
                self.assertIs(gate.validated_manifest(m), m)
                self.assertNotEqual(m["operator_media"]["canonical"]["sha256"],
                                    m["operator_media"]["shortened"]["sha256"])
                m["source_sha256"][gate.SOURCE_FILES[0]] = "0" * 64
                with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
                    gate.validated_manifest(m)
                m["source_sha256"] = dict(source_sha)
                with mock.patch.object(gate, "committed_digest", return_value="0" * 64):
                    with self.assertRaisesRegex(ValueError, "differs from candidate commit"):
                        gate.validated_manifest(m)
                m["operator_media"]["shortened"]["sha256"] = "0" * 64
                with self.assertRaisesRegex(ValueError, "shortened retained media"):
                    gate.validated_manifest(m)
                m["operator_media"]["shortened"]["sha256"] = gate.digest(media["shortened"])
                m["operator_media"]["shortened"]["video_id"] = "wrong"
                with self.assertRaisesRegex(ValueError, "shortened URL identity"):
                    gate.validated_manifest(m)

    def test_offline_requires_external_denial_report(self):
        with tempfile.TemporaryDirectory() as root:
            report_path = Path(root) / "network-denial.json"
            manifest = {"os": "darwin", "arch": "arm64", "candidate_commit": "a" * 40}
            with self.assertRaisesRegex(ValueError, "external network-denial"):
                gate.offline_canary(dict(manifest, offline_enforcement="network disabled"))
            report = {"schema": "s503-network-denial-v1", "os": "darwin", "arch": "arm64",
                      "candidate_commit": "a" * 40, "network_denied": True,
                      "probe_command": ["curl", "https://www.youtube.com/"],
                      "exit_code": 7, "error": "OS firewall denied connection"}
            report_path.write_text(json.dumps(report))
            enforcement = {"method": "host firewall", "canary_report": str(report_path),
                           "canary_sha256": gate.digest(report_path)}
            self.assertEqual(gate.offline_canary(dict(manifest, offline_enforcement=enforcement)),
                             enforcement)
            report["network_denied"] = False
            report_path.write_text(json.dumps(report))
            enforcement["canary_sha256"] = gate.digest(report_path)
            with self.assertRaisesRegex(ValueError, "failed network probe"):
                gate.offline_canary(dict(manifest, offline_enforcement=enforcement))

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

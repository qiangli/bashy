"""Focused policy tests; native inference runs only on the hosted Linux matrix."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import probe_linux_musl_inference as probe


class InferenceEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.case = {"language": "en", "reference": "a text-to-speech tool, to make Wikimedia's projects more accessible",
                     "min_score": 0.75}
        self.transcript = self.root / "transcript.json"

    def write_transcript(self, language="en", start=0, end=9000, text=None):
        self.transcript.write_text(json.dumps({"result": {"language": language},
            "transcription": [{"offsets": {"from": start, "to": end},
                               "text": text or self.case["reference"]}]}))

    def test_real_transcript_contract_requires_language_excerpt_and_timing(self):
        self.write_transcript()
        result = probe.inspect_transcript(self.transcript, self.case, 10000)
        self.assertEqual(result["language"], "en")
        self.assertEqual(result["intervals_ms"], [[0, 9000]])
        self.assertGreaterEqual(result["reference_score"], 0.75)
        for overrides, reason in [({"language": "auto"}, "detected language"),
                                  ({"end": 11000}, "timestamp"),
                                  ({"text": "the unrelated words"}, "excerpt score")]:
            self.write_transcript(**overrides)
            with self.assertRaisesRegex(RuntimeError, reason):
                probe.inspect_transcript(self.transcript, self.case, 10000)

    def test_bad_or_empty_cli_output_cannot_pass(self):
        self.transcript.write_text("{}")
        with self.assertRaisesRegex(RuntimeError, "detected language"):
            probe.inspect_transcript(self.transcript, self.case, 10000)
        self.transcript.write_text("")
        with self.assertRaisesRegex(RuntimeError, "missing or oversized"):
            probe.inspect_transcript(self.transcript, self.case, 10000)

    def test_overlapping_segments_cannot_pass_timing_gate(self):
        self.transcript.write_text(json.dumps({"result": {"language": "en"},
            "transcription": [
                {"offsets": {"from": 0, "to": 5000}, "text": "a text-to-speech tool"},
                {"offsets": {"from": 4000, "to": 9000},
                 "text": "to make Wikimedia's projects more accessible"}]}))
        with self.assertRaisesRegex(RuntimeError, "overlap"):
            probe.inspect_transcript(self.transcript, self.case, 10000)

    def test_input_identity_is_rechecked_before_replay(self):
        evidence = self.root / "evidence" / "inference"
        evidence.mkdir(parents=True)
        binary = evidence.parent / "whisper-cli"
        binary.write_bytes(b"candidate ELF")
        binary.chmod(0o755)
        report = evidence.parent / "probe-report.json"
        report.write_text(json.dumps({"probe_passed": True, "product_admitted": False,
                                      "elf": {"static_elf": True},
                                      "binary_sha256": probe.sha256(binary)}))
        model = self.root / "ggml-tiny.bin"
        model.write_bytes(b"model")
        wav = self.root / "speech.wav"
        wav.write_bytes(b"wav")
        source = probe.CASES["en-audio"]
        plan = {"build_report_sha256": probe.sha256(report), "binary": str(binary),
                "binary_sha256": probe.sha256(binary), "model": str(model),
                "model_sha256": probe.sha256(model), "model_bytes": model.stat().st_size,
                "fixture_index_sha256": probe.sha256(probe.FIXTURE_INDEX),
                "admission_sha256": probe.sha256(probe.ADMISSION),
                "cases": {"en-audio": {"source": str(source),
                                        "source_sha256": probe.sha256(source),
                                        "wav": str(wav), "sha256": probe.sha256(wav)}}}
        probe.verify_inputs(plan, evidence)
        binary.write_bytes(b"changed ELF")
        with self.assertRaisesRegex(RuntimeError, "inputs changed"):
            probe.verify_inputs(plan, evidence)

    def test_timeout_kills_and_reaps_process_group(self):
        log = self.root / "timeout.log"
        with self.assertRaises(subprocess.TimeoutExpired):
            probe.run_bounded([sys.executable, "-c",
                               "import os,time; print(os.getpid(),flush=True); time.sleep(20)"],
                              log, 0.2)
        self.assertTrue(log.is_file())
        with self.assertRaises(ProcessLookupError):
            os.kill(int(log.read_text().strip()), 0)


if __name__ == "__main__":
    unittest.main()

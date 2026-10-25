#!/usr/bin/env python3
"""Implementation smoke using the real registry, helper and pipeline.

Requires BASHY_BIN and TRANSCRIBE_ADAPTER absolute native executable paths.
Synthetic PCM and explicitly labelled fixture inference are NOT native speech
acceptance. This test never downloads tools/models or touches the user's ring.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import unittest
import wave


HERE = Path(__file__).resolve().parent


class RegistrationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.bashy = str(Path(os.environ["BASHY_BIN"]).resolve(strict=True))
        cls.helper = Path(os.environ["TRANSCRIBE_ADAPTER"]).resolve(strict=True)

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="transcribe registration ")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ)
        for key in ("WHISPER_PATH", "WHISPER_SHA256", "FFMPEG_PATH", "FFMPEG_SHA256"):
            self.env.pop(key, None)
        self.env.update(BASHY_BIN=self.bashy, BASHY_HINTS="off",
                        PATH=str(Path(self.bashy).parent) + os.pathsep + os.environ.get("PATH", ""),
                        BASHY_HOME=str(self.root / "home"),
                        BASHY_COMMANDS_DIR=str(self.root / "commands"),
                        BASHY_SKILLS_DIR=str(self.root / "skills"),
                        BASHY_BIN_CACHE=str(self.root / "cache"))
        self.helper_path = self.root / ("helper ; 日本" + self.helper.suffix)
        shutil.copy2(self.helper, self.helper_path)
        self.audio = self.root / "speech ; ' 日本.WAV"
        with wave.open(str(self.audio), "wb") as f:
            f.setparams((2, 2, 44100, 0, "NONE", "not compressed"))
            f.writeframes(struct.pack("<hh", 1000, 1000) * 44100)

    def run_cmd(self, *args, ok=True):
        p = subprocess.run([self.bashy, *map(str, args)], env=self.env,
                           text=True, capture_output=True, timeout=40)
        if ok:
            self.assertEqual(p.returncode, 0, p.stdout + p.stderr)
        else:
            self.assertNotEqual(p.returncode, 0, p.stdout + p.stderr)
        return p

    def register(self, ok=True):
        return self.run_cmd(HERE / "register.sh", self.helper_path, ok=ok)

    def request(self, stage, **kwargs):
        path = self.root / (stage + " request.json")
        path.write_text(json.dumps(dict(version="bashy-transcribe-stage-v1",
                                        stage=stage, **kwargs)), encoding="utf-8")
        return path

    def select(self, stage, path, provider=None, ok=True):
        args = ["transcribe", "select", stage]
        if provider:
            args += ["--provider", provider]
        return self.run_cmd(*args, path, ok=ok)

    def stage(self, stage, request, provider, ok=True):
        p = self.run_cmd("transcribe", "stage", stage, "--provider", provider,
                         request, ok=ok)
        return json.loads(p.stdout)

    def normalize(self):
        acq = self.stage("acquire", self.request("acquire", input=str(self.audio)),
                         "transcribe-local")
        self.assertEqual(acq["artifacts"][0]["sha256"],
                         hashlib.sha256(self.audio.read_bytes()).hexdigest())
        work = self.root / "work"
        work.mkdir()
        req = self.request("decode", artifacts=acq["artifacts"],
                           media=acq["media"], work_dir=str(work))
        self.assertEqual(self.select("decode", req).stdout.strip(), "transcribe-wav")
        decoded = self.stage("decode", req, "transcribe-wav")
        with wave.open(decoded["artifacts"][0]["path"], "rb") as f:
            self.assertEqual((f.getnchannels(), f.getframerate(), f.getnframes()),
                             (1, 16000, 16000))
            self.assertEqual(f.readframes(1), struct.pack("<h", 1000))
        return decoded, work

    def test_selection_normalization_and_missing_model(self):
        self.register()
        for path in (str(self.audio), "relative.wav", "C:\\media dir\\clip.wav",
                     "\\\\server\\share\\clip.wav", "./colon:name.wav"):
            req = self.request("acquire", input=path)
            self.assertEqual(self.select("acquire", req).stdout.strip(), "transcribe-local")
        req = self.request("acquire", input="https://example.invalid/audio.wav")
        self.select("acquire", req, ok=False)
        self.select("acquire", req, provider="transcribe-local", ok=False)
        decoded, work = self.normalize()
        req = self.request("infer", artifacts=decoded["artifacts"], media=decoded["media"],
                           work_dir=str(work), model="base", language="auto")
        self.assertEqual(self.select("infer", req).stdout.strip(), "transcribe-whisper")
        before = set(work.iterdir())
        result = self.stage("infer", req, "transcribe-whisper", ok=False)
        self.assertEqual(result["error"]["code"], "missing_model")
        self.assertNotIn("segments", result)
        self.assertNotIn("artifacts", result)
        self.assertEqual(set(work.iterdir()), before)
        self.assertFalse((self.root / "cache").exists())

    def test_exports_and_invalid_media(self):
        self.register()
        segments = [dict(start_ms=0, end_ms=1000, text="fixture Hola 日本 <text>")]
        for fmt in ("txt", "srt", "vtt"):
            output = self.root / ("output " + fmt)
            req = self.request("export", segments=segments, format=fmt, output=str(output))
            self.assertEqual(self.select("export", req).stdout.strip(), "transcribe-export")
            result = self.stage("export", req, "transcribe-export")
            self.assertEqual(result["artifacts"][0]["sha256"],
                             hashlib.sha256(output.read_bytes()).hexdigest())
            text = output.read_text(encoding="utf-8")
            self.assertIn("Hola 日本", text)
            if fmt != "txt":
                self.assertIn("&lt;text&gt;", text)
                self.assertIn("00:00:00", text)
            if fmt == "vtt":
                self.assertTrue(text.startswith("WEBVTT\n\n"))
            failed = self.stage("export", req, "transcribe-export", ok=False)
            self.assertEqual(failed["error"]["code"], "output_exists")
            self.assertEqual(output.read_text(encoding="utf-8"), text)
        self.assertFalse(list(self.root.glob(".transcribe-export-*")))
        work = self.root / "failed work"
        work.mkdir()
        self.audio.write_bytes(b"invalid media")
        req = self.request("decode", artifacts=[dict(path=str(self.audio))], work_dir=str(work))
        result = self.stage("decode", req, "transcribe-wav", ok=False)
        self.assertIn("error", result)
        self.assertEqual(list(work.iterdir()), [])

    def test_optional_configuration_and_ambiguity(self):
        # Configuration points to fixture bytes for registry inspection only;
        # never execute these bytes as whisper.cpp or FFmpeg.
        digest = hashlib.sha256(self.helper_path.read_bytes()).hexdigest()
        self.env.update(WHISPER_PATH=str(self.helper_path), WHISPER_SHA256=digest,
                        FFMPEG_PATH=str(self.helper_path), FFMPEG_SHA256=digest)
        self.register()
        record = json.loads(self.run_cmd("commands", "show", "transcribe-whisper", "--json").stdout)
        self.assertEqual(record["exec"][1:], ["--whisper", str(self.helper_path),
                                             "--whisper-sha256", digest, "whisper"])
        self.assertNotIn("net", record["effects"])
        req = self.request("decode", artifacts=[dict(path=str(self.audio))])
        self.assertIn("ambiguous", self.select("decode", req, ok=False).stderr)
        self.assertEqual(self.select("decode", req, "transcribe-wav").stdout.strip(), "transcribe-wav")
        req = self.request("decode", artifacts=[dict(path="video.mp4")])
        self.assertEqual(self.select("decode", req).stdout.strip(), "transcribe-ffmpeg")

    def test_invalid_deployment_does_not_create_records(self):
        self.env.update(FFMPEG_PATH=str(self.helper_path), FFMPEG_SHA256="invalid")
        self.register(ok=False)
        self.assertFalse((self.root / "commands").exists())

    def test_pipeline_with_fixture_inference_and_failure_cleanup(self):
        self.register()
        pipeline = HERE.parent / "pipeline.bsh"
        self.run_cmd("commands", "add", "transcribe-pipeline", "--set", "exec.0=" + self.bashy,
                     "--set", "exec.1=--bashsharp", "--set", "exec.2=" + str(pipeline),
                     "--set", "effects.0=read", "--set", "effects.1=write",
                     "--set", "effects.2=destroy", "--set", "effects.3=exec", "--set", "effects.4=net")
        output = self.root / "pipeline.txt"
        p = self.run_cmd("transcribe", "--output", output, self.audio, ok=False)
        self.assertIn("infer provider transcribe-whisper failed", p.stderr)
        self.assertFalse(output.exists())
        self.assertEqual(p.stdout, "")
        self.assertFalse(list(self.root.glob(".transcribe.*")))
        fixture = self.root / "fixture inference.py"
        fixture.write_text('import json\nprint(json.dumps({"version":"bashy-transcribe-stage-v1",'
                           '"stage":"infer","segments":[{"start_ms":0,"end_ms":1000,'
                           '"text":"fixture inference only"}]}))\n', encoding="utf-8")
        # Ordinary local override; the production inference implementation stays intact.
        self.run_cmd("commands", "set", "transcribe-whisper", "--set", "exec.0=" + sys.executable,
                     "--set", "exec.1=" + str(fixture))
        skill = self.root / "skill"
        skill.mkdir()
        source = HERE.parents[2] / "skills" / "bashy" / "commands"
        shutil.copy2(source / "transcribe.md", skill / "SKILL.md")
        shutil.copy2(source / "transcribe.dhnt", skill / "skill.dhnt")
        self.run_cmd("skill", "add", skill)
        for fmt in ("txt", "srt", "vtt"):
            output = self.root / ("pipeline." + fmt)
            p = self.run_cmd("transcribe", "--format", fmt, "--output", output, self.audio)
            self.assertEqual(Path(p.stdout.strip()), output)
            self.assertIn("fixture inference only", output.read_text(encoding="utf-8"))
            self.assertFalse(list(self.root.glob(".transcribe.*")))
            skill_output = self.root / ("skill." + fmt)
            result_path = self.root / "skill result.txt"
            self.env.update(BASHY_TRANSCRIBE_INPUT=str(self.audio),
                            BASHY_TRANSCRIBE_OUTPUT=str(skill_output),
                            BASHY_TRANSCRIBE_RESULT=str(result_path),
                            BASHY_TRANSCRIBE_FORMAT=fmt,
                            BASHY_TRANSCRIBE_SOURCE="", BASHY_TRANSCRIBE_DECODER="",
                            BASHY_TRANSCRIBE_PROVIDER="", BASHY_TRANSCRIBE_EXPORTER="")
            receipt = json.loads(self.run_cmd("skill", "run", "transcribe", "--json").stdout)
            self.assertTrue(receipt["attest"]["Valid"])
            self.assertEqual(Path(result_path.read_text().strip()), skill_output)
            self.assertEqual(skill_output.read_bytes(), output.read_bytes())
            self.assertFalse(list(self.root.glob(".transcribe.*")))


if __name__ == "__main__":
    for name in ("BASHY_BIN", "TRANSCRIBE_ADAPTER"):
        if not os.environ.get(name) or not Path(os.environ[name]).is_absolute():
            sys.exit(name + " must name an absolute native executable; no skipped smoke gate")
    unittest.main(verbosity=2)

#!/usr/bin/env python3
"""Evidence-only CPU inference and OS-offline replay for the exact musl CLI."""

from __future__ import annotations

import argparse
import difflib
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import sys
import time
import urllib.request
import wave


ROOT = Path(__file__).resolve().parents[3]
FIXTURE_INDEX = ROOT / "examples/transcribe/fixtures/controlled-native.json"
ADMISSION = ROOT / "docs/sprint-503/evidence/native-transcription-admission.json"
CANARY = ROOT / ".github/scripts/s503-offline-enforcer-probe.py"
MODEL_URL = ("https://huggingface.co/ggerganov/whisper.cpp/resolve/"
             "5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.bin")
CASES = {
    "en-audio": ROOT / "examples/transcribe/webm-opus/testdata/speech-long.webm",
    "es-audio": ROOT / "examples/transcribe/fixtures/spanish-proverb.wav",
}
CASE_TIMEOUT_SECONDS = 480


def require(condition: bool, reason: str) -> None:
    if not condition:
        raise RuntimeError(reason)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def read(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def write(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def reference_score(text: str, reference: str) -> float:
    # Same word-window comparison as verify-youtube-pipeline.py, kept local to
    # avoid loading the full Bashy pipeline for this direct-CLI experiment.
    import re
    words = lambda s: re.findall(r"[\w']+", s.casefold())
    actual, expected = words(text), words(reference)
    require(len(expected) >= 6 and bool(actual), "missing reference or transcript words")
    width = len(expected)
    return max(difflib.SequenceMatcher(None, expected, actual[i:i + width]).ratio()
               for i in range(max(1, len(actual) - width + 1)))


def inspect_transcript(path: Path, case: dict, duration_ms: int) -> dict:
    require(path.is_file() and 0 < path.stat().st_size <= 2 << 20,
            "missing or oversized CLI JSON transcript")
    doc = read(path)
    language = doc.get("result", {}).get("language")
    require(language == case["language"], f"detected language {language!r} differs from {case['language']}")
    segments = doc.get("transcription")
    require(isinstance(segments, list) and bool(segments), "CLI returned no transcript segments")
    texts = []
    intervals = []
    for item in segments:
        text = item.get("text", "").strip()
        offsets = item.get("offsets", {})
        start, end = offsets.get("from"), offsets.get("to")
        require(text and type(start) is int and type(end) is int and
                0 <= start < end <= duration_ms + 250,
                "empty segment or invalid media-relative timestamp")
        require(not intervals or start >= intervals[-1][1], "segment timestamps overlap")
        intervals.append([start, end])
        texts.append(text)
    transcript = " ".join(texts)
    score = reference_score(transcript, case["reference"])
    require(score >= case["min_score"],
            f"reference excerpt score {score:.3f} below {case['min_score']}")
    return {"language": language, "segments": len(segments), "intervals_ms": intervals,
            "transcript": transcript, "reference": case["reference"],
            "reference_score": round(score, 4), "minimum_score": case["min_score"],
            "json_sha256": sha256(path)}


def fetch_model(destination: Path, expected_sha: str, expected_bytes: int) -> None:
    request = urllib.request.Request(MODEL_URL, headers={"User-Agent": "s503-musl-inference/1"})
    with urllib.request.urlopen(request, timeout=120) as source, destination.open("wb") as target:
        size = 0
        for block in iter(lambda: source.read(1 << 20), b""):
            size += len(block)
            require(size <= expected_bytes, "model download exceeded pinned byte count")
            target.write(block)
    require(destination.stat().st_size == expected_bytes and sha256(destination) == expected_sha,
            "model bytes differ from reviewed S503 pin")


def run_bounded(command: list[str], log: Path, timeout: int) -> int:
    """Bound one process tree and kill/reap it on timeout or cancellation."""
    with log.open("wb") as stream:
        process = subprocess.Popen(command, stdout=stream, stderr=subprocess.STDOUT,
                                   start_new_session=True)
        previous = signal.getsignal(signal.SIGTERM)
        def cancelled(_signum, _frame):
            raise InterruptedError("hosted job cancelled")
        signal.signal(signal.SIGTERM, cancelled)
        try:
            return process.wait(timeout=timeout)
        except (subprocess.TimeoutExpired, KeyboardInterrupt, InterruptedError):
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
            raise
        finally:
            signal.signal(signal.SIGTERM, previous)


def checked_wav(path: Path, source_duration: int) -> dict:
    with wave.open(str(path), "rb") as stream:
        require(stream.getnchannels() == 1 and stream.getsampwidth() == 2 and
                stream.getframerate() == 16000 and stream.getcomptype() == "NONE",
                "fixture conversion is not mono 16 kHz PCM16")
        duration_ms = round(stream.getnframes() * 1000 / stream.getframerate())
    require(abs(duration_ms - source_duration) <= 100,
            "converted fixture duration differs from pinned source")
    return {"sha256": sha256(path), "bytes": path.stat().st_size,
            "duration_ms": duration_ms, "format": "mono 16kHz PCM16 WAV"}


def prepare(work: Path, evidence: Path, arch: str) -> None:
    probe = read(evidence.parent / "probe-report.json")
    binary = evidence.parent / "whisper-cli"
    require(probe.get("probe_passed") is True and probe.get("product_admitted") is False and
            probe.get("platform") == f"linux/{arch}" and
            probe.get("binary_sha256") == sha256(binary) and os.access(binary, os.X_OK),
            "built static musl CLI report or exact executable bytes missing")
    require(probe.get("elf", {}).get("static_elf") is True,
            "static ELF audit was not recorded")
    fixture_index = read(FIXTURE_INDEX)
    admission = read(ADMISSION)
    require(fixture_index.get("schema") == "s503-controlled-fixtures-v1" and
            admission.get("schema") == "s503-native-transcription-admission-v2", "reviewed fixture/model schema")
    require(admission.get("model_sha256") ==
            "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21" and
            admission.get("model_bytes") == 77691713, "reviewed multilingual tiny model pin")
    expected_arch = {"amd64": "x86_64", "arm64": "aarch64"}[arch]
    require(platform.system() == "Linux" and platform.machine() == expected_arch,
            "inference runner is not native target")
    enforcer = Path("/usr/bin/unshare")
    label = {"amd64": "ubuntu-24.04", "arm64": "ubuntu-24.04-arm"}[arch]
    enforcer_pin = admission["offline_canary"][label]
    require(enforcer.is_file() and enforcer_pin["method"] == "linux-unshare-net" and
            sha256(enforcer) == enforcer_pin["enforcer_sha256"],
            "OS network enforcer differs from reviewed Linux runner bytes")
    work.mkdir(parents=True, exist_ok=True)
    model = work / "ggml-tiny.bin"
    fetch_model(model, admission["model_sha256"], admission["model_bytes"])
    ffmpeg = shutil.which("ffmpeg")
    require(ffmpeg is not None, "hosted fixture converter ffmpeg missing")
    converted = {}
    for label, source in CASES.items():
        case = fixture_index["cases"][label]
        require(source.is_file() and source.stat().st_size == case["bytes"] and
                sha256(source) == case["sha256"] and case["license_evidence"].startswith(
                    "https://commons.wikimedia.org/"), f"pinned CC0 {label} source mismatch")
        target = work / f"{label}.wav"
        command = [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
                   "-i", str(source), "-vn", "-ac", "1", "-ar", "16000",
                   "-c:a", "pcm_s16le", str(target)]
        require(run_bounded(command, evidence / f"convert-{label}.log", 60) == 0,
                f"{label} conversion failed")
        converted[label] = {"source": str(source), "source_sha256": case["sha256"],
                            "license_evidence": case["license_evidence"],
                            "command": command, "wav": str(target),
                            **checked_wav(target, case["duration_ms"])}
    plan = {"schema": "s503-linux-musl-inference-plan-v1", "product_admitted": False,
            "arch": arch, "commit_sha": probe["commit_sha"],
            "binary": str(binary), "binary_sha256": sha256(binary),
            "build_report_sha256": sha256(evidence.parent / "probe-report.json"),
            "model": str(model), "model_url": MODEL_URL,
            "model_sha256": sha256(model), "model_bytes": model.stat().st_size,
            "fixture_index_sha256": sha256(FIXTURE_INDEX),
            "admission_sha256": sha256(ADMISSION), "cases": converted,
            "ffmpeg": {"path": ffmpeg, "sha256": sha256(Path(ffmpeg)),
                       "version": subprocess.check_output([ffmpeg, "-version"], text=True,
                                                          timeout=10).splitlines()[0]},
            "enforcer": {"path": str(enforcer), "sha256": sha256(enforcer)},
            "host_network_namespace": os.readlink("/proc/self/ns/net"),
            "case_timeout_seconds": CASE_TIMEOUT_SECONDS}
    write(evidence / "inference-plan.json", plan)


def verify_inputs(plan: dict, evidence: Path) -> None:
    probe = read(evidence.parent / "probe-report.json")
    require(probe.get("probe_passed") is True and probe.get("product_admitted") is False and
            probe.get("elf", {}).get("static_elf") is True and
            sha256(evidence.parent / "probe-report.json") == plan["build_report_sha256"] and
            os.access(plan["binary"], os.X_OK) and
            sha256(Path(plan["binary"])) == plan["binary_sha256"] == probe["binary_sha256"] and
            sha256(Path(plan["model"])) == plan["model_sha256"] and
            Path(plan["model"]).stat().st_size == plan["model_bytes"] and
            sha256(FIXTURE_INDEX) == plan["fixture_index_sha256"] and
            sha256(ADMISSION) == plan["admission_sha256"], "inference inputs changed after setup")
    for label, item in plan["cases"].items():
        require(label in CASES and sha256(Path(item["source"])) == item["source_sha256"] and
                sha256(Path(item["wav"])) == item["sha256"],
                f"{label} source or converted WAV changed")


def canary(phase: str, evidence: Path, offline: bool) -> dict:
    out = evidence / ("network-" + phase)
    command = "policy" if offline else "baseline"
    result = subprocess.run([sys.executable, str(CANARY), command, "--out", str(out)],
                            capture_output=True, text=True, timeout=20)
    require(result.returncode == 0, f"{phase} network canary failed: {result.stderr}")
    if offline:
        before, after = read(out / "denied-before.json"), read(out / "denied-after.json")
        require(before.get("denied") is True and after.get("denied") is True,
                "network namespace did not deny both socket probes")
        return {"before": before, "after": after}
    baseline = read(out / "baseline.json")
    require(baseline.get("connected") is True, "baseline TCP connectivity absent")
    return baseline


def infer_phase(phase: str, evidence: Path) -> None:
    plan = read(evidence / "inference-plan.json")
    verify_inputs(plan, evidence)
    offline = phase == "offline"
    namespace = os.readlink("/proc/self/ns/net")
    require((namespace != plan["host_network_namespace"]) == offline,
            "offline replay is not isolated in a new OS network namespace")
    phase_dir = evidence / phase
    phase_dir.mkdir(exist_ok=False)
    result = {"schema": "s503-linux-musl-inference-phase-v1", "phase": phase,
              "product_admitted": False, "passed": False,
              "network_namespace": namespace, "binary_sha256": plan["binary_sha256"],
              "model_sha256": plan["model_sha256"], "cases": {}}
    try:
        result["network_before"] = canary(phase + "-before", evidence, offline)
        fixture_index = read(FIXTURE_INDEX)["cases"]
        for label, fixture in plan["cases"].items():
            base = phase_dir / label
            command = [plan["binary"], "--model", plan["model"], "--file", fixture["wav"],
                       "--language", "auto", "--no-gpu", "--output-json", "--output-file", str(base)]
            started = time.monotonic()
            status = run_bounded(command, phase_dir / f"{label}.log", CASE_TIMEOUT_SECONDS)
            require(status == 0, f"{phase} {label} whisper-cli exited {status}")
            require((phase_dir / f"{label}.log").stat().st_size <= 2 << 20,
                    f"{phase} {label} CLI log exceeds evidence ceiling")
            transcript = inspect_transcript(Path(str(base) + ".json"),
                                            fixture_index[label], fixture["duration_ms"])
            result["cases"][label] = {"command": command, "exit_code": status,
                                      "elapsed_seconds": round(time.monotonic() - started, 3),
                                      "wav_sha256": fixture["sha256"],
                                      "log_sha256": sha256(phase_dir / f"{label}.log"),
                                      **transcript}
            write(phase_dir / "phase-report.json", result)
        result["network_after"] = canary(phase + "-after", evidence, offline)
        verify_inputs(plan, evidence)
        result["passed"] = True
    except BaseException as exc:
        result["error"] = f"{type(exc).__name__}: {exc}"
        raise
    finally:
        write(phase_dir / "phase-report.json", result)


def check(evidence: Path) -> None:
    plan = read(evidence / "inference-plan.json")
    verify_inputs(plan, evidence)
    for phase in ("online", "offline"):
        result = read(evidence / phase / "phase-report.json")
        require(result.get("passed") is True and result.get("product_admitted") is False and
                result.get("binary_sha256") == plan["binary_sha256"] and
                result.get("model_sha256") == plan["model_sha256"] and
                set(result.get("cases", {})) == set(CASES), f"{phase} evidence incomplete")
        for label, item in result["cases"].items():
            require(item["wav_sha256"] == plan["cases"][label]["sha256"] and
                    sha256(evidence / phase / f"{label}.json") == item["json_sha256"] and
                    sha256(evidence / phase / f"{label}.log") == item["log_sha256"],
                    f"{phase} {label} transcript or log changed")
        if phase == "offline":
            require(result["network_namespace"] != plan["host_network_namespace"] and
                    all(result[key][part]["denied"] is True
                        for key in ("network_before", "network_after")
                        for part in ("before", "after")), "offline OS network denial missing")
        else:
            require(result["network_namespace"] == plan["host_network_namespace"] and
                    result["network_before"]["connected"] and
                    result["network_after"]["connected"], "online network baseline missing")
    write(evidence / "inference-result.json", {
        "schema": "s503-linux-musl-inference-result-v1", "passed": True,
        "product_admitted": False, "arch": plan["arch"],
        "binary_sha256": plan["binary_sha256"], "model_sha256": plan["model_sha256"],
        "online_report_sha256": sha256(evidence / "online/phase-report.json"),
        "offline_report_sha256": sha256(evidence / "offline/phase-report.json"),
        "remaining_gates": ["independent notice and source-closure review",
                            "release packaging and artifact review",
                            "supported host and product-dependency-policy review"]})


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("prepare", "online", "offline", "check"))
    parser.add_argument("--work-dir", type=Path, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--expected-arch", choices=("amd64", "arm64"), required=True)
    args = parser.parse_args()
    work, evidence = args.work_dir.resolve(), args.evidence_dir.resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    attempt = {"schema": "s503-linux-musl-inference-attempt-v1",
               "action": args.action, "arch": args.expected_arch,
               "product_admitted": False, "passed": False}
    try:
        if args.action == "prepare":
            prepare(work, evidence, args.expected_arch)
        else:
            plan = read(evidence / "inference-plan.json")
            require(plan.get("arch") == args.expected_arch and
                    plan.get("product_admitted") is False, "wrong target or admitted plan")
            if args.action == "check":
                check(evidence)
            else:
                infer_phase(args.action, evidence)
        attempt["passed"] = True
    except BaseException as exc:
        attempt["error"] = f"{type(exc).__name__}: {exc}"
        raise
    finally:
        write(evidence / f"attempt-{args.action}.json", attempt)


if __name__ == "__main__":
    main()

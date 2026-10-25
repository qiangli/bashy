#!/usr/bin/env python3
"""Native, manifest-driven gate for the registered transcription pipeline.

This gate never substitutes inference or downloads dependencies. It reuses
already acquired media and requires a reviewed, native decoder and model.
"""

import argparse
import difflib
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
FORMATS = ("txt", "srt", "vtt")
STAMP = re.compile(r"^(\d{2}):(\d{2}):(\d{2})[,.](\d{3})$")
YOUTUBE_SHA256 = "f6aba46f43523ad83a604cec1a05821d4f4288fc55b18db9ad2dea7ccac50ff3"
YTDLP_SHA256 = "b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14"
URLS = {
    "canonical": "https://www.youtube.com/watch?v=_j_PixI3Muo",
    "shortened": "https://youtu.be/_j_PixI3Muo?si=if_lqvXvU5GaM1nb",
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def words(value):
    return re.findall(r"[\w']+", value.casefold())


def reference_score(text, reference):
    actual, expected = words(text), words(reference)
    require(len(expected) >= 6, "reviewed reference excerpt needs at least six words")
    require(bool(actual), "transcript contains no words")
    width = len(expected)
    return max(difflib.SequenceMatcher(None, expected, actual[i:i + width]).ratio()
               for i in range(max(1, len(actual) - width + 1)))


def milliseconds(value):
    match = STAMP.fullmatch(value)
    require(match is not None, "invalid subtitle timestamp: " + value)
    h, m, s, ms = map(int, match.groups())
    require(m < 60 and s < 60, "invalid subtitle clock: " + value)
    return ((h * 60 + m) * 60 + s) * 1000 + ms


def subtitle_intervals(text, fmt, duration_ms):
    if fmt == "vtt":
        require(text.startswith("WEBVTT\n"), "VTT header missing")
    intervals = []
    for line in text.splitlines():
        if " --> " not in line:
            continue
        left, right = line.split(" --> ", 1)
        start, end = milliseconds(left), milliseconds(right.split()[0])
        require(0 <= start < end <= duration_ms, "subtitle timestamp outside media duration")
        require(not intervals or start >= intervals[-1][1], "subtitle timestamps overlap")
        intervals.append((start, end))
    require(bool(intervals), fmt + " has no timed cues")
    return intervals


def validated_manifest(m):
    require(m.get("schema") == "s503-youtube-pipeline-v1", "unknown manifest schema")
    require(re.fullmatch(r"[0-9a-f]{7,40}", m.get("candidate_commit", "")),
            "candidate commit is required")
    require(m.get("acquisition_evidence") == {"windows_amd64": "run165", "darwin_arm64": "run168"},
            "preserved native acquisition evidence must be linked")
    require(m.get("os") == platform.system().lower(), "manifest OS differs from native host")
    require(m.get("arch") == platform.machine().lower(), "manifest arch differs from native host")
    for key in ("bashy", "helper", "python", "yt_dlp", "deno", "whisper", "model", "controlled_webm"):
        value = m.get(key)
        require(isinstance(value, str) and Path(value).is_absolute() and Path(value).is_file(),
                key + " must be an existing absolute file")
    for key in ("bashy", "helper", "yt_dlp", "deno", "whisper", "model", "controlled_webm"):
        require(digest(m[key]) == m.get(key + "_sha256"), key + " SHA-256 mismatch")
    require(Path(m["model"]).name == "ggml-" + m.get("model_id", "") + ".bin" and
            Path(m["model"]).parent.name == "whisper" and
            Path(m["model"]).parent.parent.name == "models",
            "model must be in the verified binmgr models/whisper cache")
    require(m.get("yt_dlp_sha256") == YTDLP_SHA256,
            "yt-dlp is not the reviewed zipapp pin")
    decoder = m.get("decoder")
    require(isinstance(decoder, dict) and decoder.get("name") and isinstance(decoder.get("exec"), list),
            "decoder requires a registered name and argv")
    require(all(isinstance(x, str) and x for x in decoder["exec"]), "decoder argv contains an empty value")
    require(Path(decoder["exec"][0]).is_absolute(), "decoder executable must be absolute")
    require(digest(decoder["exec"][0]) == decoder.get("sha256"), "decoder SHA-256 mismatch")
    require(decoder.get("license_evidence"), "decoder license/provenance evidence is required")
    controlled = m.get("controlled")
    operator = m.get("operator")
    for name, item in (("controlled", controlled), ("operator", operator)):
        require(isinstance(item, dict), name + " case missing")
        require(isinstance(item.get("duration_ms"), int) and item["duration_ms"] > 0,
                name + " duration missing")
        require(isinstance(item.get("reference"), str) and len(words(item["reference"])) >= 6,
                name + " reviewed excerpt missing")
        require(0.5 <= item.get("min_score", 0) <= 1, name + " reference tolerance invalid")
    require(m.get("controlled_webm_sha256") != YOUTUBE_SHA256,
            "controlled speech fixture must differ from operator video")
    media = m.get("operator_media")
    require(isinstance(media, dict) and set(media) == set(URLS), "both preserved URL variants required")
    for label, url in URLS.items():
        item = media[label]
        require(item.get("url") == url and item.get("video_id") == "_j_PixI3Muo",
                label + " URL identity mismatch")
        require(item.get("duration_ms") == operator["duration_ms"], label + " duration mismatch")
        require(item.get("sha256") == YOUTUBE_SHA256, label + " evidence hash mismatch")
        require(Path(item.get("path", "")).is_file() and digest(item["path"]) == YOUTUBE_SHA256,
                label + " retained media missing or changed")
    return m


def run(argv, env, timeout=60, check=True):
    proc = subprocess.run(list(map(str, argv)), env=env, text=True,
                          capture_output=True, timeout=timeout)
    if check and proc.returncode:
        raise RuntimeError(f"command failed ({proc.returncode}): {argv[:3]}\n{proc.stderr[-3000:]}")
    return proc


def add_record(bashy, env, name, executable, role, effects, accepts=None):
    args = [bashy, "commands", "add", name]
    for index, value in enumerate(executable):
        args += ["--set", f"exec.{index}={value}"]
    for index, value in enumerate(effects):
        args += ["--set", f"effects.{index}={value}"]
    if role:
        args += ["--set", "provides.0=transcribe." + role]
    if accepts:
        args += ["--set", "accepts.0.path=/input", "--set", "accepts.0.pattern=" + accepts]
    run(args, env)


def setup(m, env):
    bashy = m["bashy"]
    run([bashy, HERE / "adapters" / "register.sh", m["helper"]], env)
    add_record(bashy, env, "transcribe-youtube", [m["python"], str(HERE / "source_youtube.py")],
               "acquire", ["read", "write", "destroy", "exec", "net"],
               r"^https://(www\.youtube\.com/watch\?.*|youtu\.be/.*)$")
    add_record(bashy, env, m["decoder"]["name"], m["decoder"]["exec"],
               "decode", ["read", "write", "destroy", "exec"], None)
    add_record(bashy, env, "transcribe-pipeline", [bashy, "--bashsharp", str(HERE / "pipeline.bsh")],
               None, ["read", "write", "destroy", "exec", "net"])
    skill = Path(env["BASHY_HOME"]) / "transcribe-source"
    skill.mkdir(parents=True)
    shutil.copy2(ROOT / "skills/bashy/commands/transcribe.md", skill / "SKILL.md")
    shutil.copy2(ROOT / "skills/bashy/commands/transcribe.dhnt", skill / "skill.dhnt")
    run([bashy, "skill", "add", str(skill)], env)
    record = json.loads(run([bashy, "skill", "show", "transcribe", "--json"], env).stdout)
    require(record.get("identity") and record.get("capability"), "skill identity missing")
    return record


def invoke(m, env, source, media, label, orientation, fmt, outdir, skill_record):
    output = outdir / f"{label}-{orientation}.{fmt}"
    result = outdir / f"{label}-{orientation}.{fmt}.path"
    args = ["--source", source, "--decoder", m["decoder"]["name"],
            "--provider", "transcribe-whisper", "--exporter", "transcribe-export",
            "--model", m["model_id"], "--language", "auto", "--format", fmt,
            "--output", str(output), media]
    if orientation == "script":
        cmd = [m["bashy"], "--bashsharp", str(HERE / "pipeline.bsh"), *args]
        proc = run(cmd, env, timeout=m.get("case_timeout_seconds", 1800))
        result.write_text(proc.stdout)
    else:
        local = dict(env, BASHY_TRANSCRIBE_INPUT=media, BASHY_TRANSCRIBE_RESULT=str(result),
                     BASHY_TRANSCRIBE_OUTPUT=str(output), BASHY_TRANSCRIBE_SOURCE=source,
                     BASHY_TRANSCRIBE_DECODER=m["decoder"]["name"],
                     BASHY_TRANSCRIBE_PROVIDER="transcribe-whisper",
                     BASHY_TRANSCRIBE_EXPORTER="transcribe-export",
                     BASHY_TRANSCRIBE_MODEL=m["model_id"], BASHY_TRANSCRIBE_LANGUAGE="auto",
                     BASHY_TRANSCRIBE_FORMAT=fmt)
        receipt = json.loads(run([m["bashy"], "skill", "run", "transcribe", "--json"], local,
                                 timeout=m.get("case_timeout_seconds", 1800)).stdout)
        attest = receipt.get("attest", {})
        require(attest.get("Valid") is True and attest.get("Skill") == skill_record["identity"],
                "skill receipt identity/validity mismatch")
        require(receipt.get("capability") == skill_record["capability"],
                "skill receipt capability mismatch")
    require(result.read_text().strip() == str(output), "pipeline returned wrong output path")
    require(output.is_file() and output.stat().st_size > 0, "published transcript missing")
    return output


def verify_artifacts(paths, case):
    texts = {fmt: paths[fmt].read_text(encoding="utf-8") for fmt in FORMATS}
    for fmt, body in texts.items():
        require(len(words(body)) >= 12, fmt + " transcript is too short")
        require(reference_score(body, case["reference"]) >= case["min_score"],
                fmt + " transcript misses reviewed excerpt")
    for fmt in ("srt", "vtt"):
        subtitle_intervals(texts[fmt], fmt, case["duration_ms"])
    return {fmt: {"sha256": digest(paths[fmt]), "bytes": paths[fmt].stat().st_size}
            for fmt in FORMATS}


def assert_no_owned_temp(outdir, tempdir):
    require(not list(outdir.glob(".transcribe.*")), "pipeline left output temporary files")
    require(not list(tempdir.iterdir()), "pipeline left owned work directories")


def verify_function_receipts(env):
    ledger = Path(env["BASHY_SKILLS_DIR"]) / "attest" / "transcribe_pipeline.jsonl"
    require(ledger.is_file(), "Bash# function receipt ledger missing")
    records = [json.loads(line) for line in ledger.read_text().splitlines() if line.strip()]
    require(len(records) >= 2, "script/skill function receipts missing")
    projection = lambda r: (r["attest"]["Skill"], tuple(r["attest"]["Passed"]),
                            tuple(r["attest"]["Failed"]), tuple(r["attest"]["Effects"]))
    require(all(r.get("status") == 0 and r.get("attest", {}).get("Valid") is True for r in records),
            "function receipt invalid")
    require(len(set(map(projection, records))) == 1, "script/skill function receipt identity differs")
    return {"count": len(records), "identity": records[0]["attest"]["Skill"],
            "effects": records[0]["attest"]["Effects"]}


def failure_and_cancel(m, env, outdir, tempdir, online):
    bad = outdir / "invalid-url.txt"
    args = [m["bashy"], "transcribe", "--source", "transcribe-youtube" if online else "transcribe-local",
            "--decoder", m["decoder"]["name"], "--provider", "transcribe-whisper",
            "--exporter", "transcribe-export", "--output", str(bad),
            "https://www.youtube.com/watch?v=%24%28invalid%29" if online else str(outdir / "missing.webm")]
    failed = run(args, env, timeout=60, check=False)
    require(failed.returncode != 0 and not bad.exists() and not failed.stdout.strip(),
            "invalid URL produced a success artifact")
    assert_no_owned_temp(outdir, tempdir)
    if os.name != "posix":
        return "cancellation requires a native process-group implementation on this host"
    cancel = outdir / "cancelled.txt"
    cmd = [m["bashy"], "transcribe", "--source", "transcribe-local",
           "--decoder", m["decoder"]["name"], "--provider", "transcribe-whisper",
           "--exporter", "transcribe-export", "--model", m["model_id"],
           "--output", str(cancel), m["controlled_webm"]]
    proc = subprocess.Popen(cmd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, start_new_session=True)
    time.sleep(m.get("cancel_after_seconds", 2))
    require(proc.poll() is None, "cancel case completed before signal; no cancellation was tested")
    os.killpg(proc.pid, signal.SIGINT)
    try:
        stdout, _ = proc.communicate(timeout=15)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.communicate()
        raise ValueError("cancelled pipeline did not stop promptly")
    require(proc.returncode != 0 and not cancel.exists() and not stdout.strip(),
            "cancelled pipeline published success")
    assert_no_owned_temp(outdir, tempdir)
    return "pass"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--offline", action="store_true", help="repeat local media only under externally disabled network")
    parser.add_argument("--online", action="store_true", help="also run both real URL forms through the source and pipeline; downloads media")
    parser.add_argument("--validate-only", action="store_true")
    parser.add_argument("--output-dir", type=Path, help="new private directory to retain transcripts and result JSON")
    args = parser.parse_args()
    m = validated_manifest(json.loads(args.manifest.read_text()))
    require(not (args.offline and args.online), "online acquisition and offline repeat are separate runs")
    if args.validate_only:
        print("manifest and retained media validated; no pipeline run")
        return
    require(args.output_dir is not None and args.output_dir.is_absolute(), "--output-dir must be absolute")
    require(not args.output_dir.exists(), "--output-dir must be new; existing evidence is never overwritten")
    require(ROOT not in args.output_dir.parents, "transcripts must stay outside the source checkout")
    require(m.get("model_id"), "model_id is required")
    args.output_dir.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix="s503-pipeline-") as owned:
        root = Path(owned)
        outdir, tempdir = args.output_dir, root / "tmp"
        tempdir.mkdir()
        env = dict(os.environ, BASHY_BIN=m["bashy"], BASHY_HINTS="off",
                   BASHY_HOME=str(root / "home"), BASHY_COMMANDS_DIR=str(root / "commands"),
                   BASHY_SKILLS_DIR=str(root / "skills"), BASHY_BIN_CACHE=str(Path(m["model"]).parents[2] / "bin"),
                   BASHY_TRANSCRIBE_YTDLP=m["yt_dlp"], BASHY_TRANSCRIBE_JS_RUNTIME=m["deno"],
                   BASHY_TRANSCRIBE_PYTHON=m["python"], WHISPER_PATH=m["whisper"],
                   WHISPER_SHA256=m["whisper_sha256"], TMPDIR=str(tempdir),
                   PATH=str(Path(m["bashy"]).parent) + os.pathsep + os.environ.get("PATH", ""))
        env.pop("FFMPEG_PATH", None)
        env.pop("FFMPEG_SHA256", None)
        skill = setup(m, env)
        results = []
        cases = [("controlled", m["controlled_webm"], m["controlled"], "transcribe-local"),
                 ("operator", m["operator_media"]["canonical"]["path"],
                  m["operator"], "transcribe-local")]
        if args.offline:
            require(m.get("offline_enforcement"), "offline run needs recorded external network disablement")
        for label, media, expectation, source in cases:
            for orientation in ("script", "skill"):
                paths = {fmt: invoke(m, env, source, media, label, orientation, fmt, outdir, skill)
                         for fmt in FORMATS}
                artifacts = verify_artifacts(paths, expectation)
                assert_no_owned_temp(outdir, tempdir)
                results.append({"case": label, "orientation": orientation, "source": source,
                                "formats": artifacts})
        if args.online:
            # One acquisition per URL form, with both orientations represented.
            # The retained local media covers the other exports without repeats.
            for label, orientation in (("canonical", "script"), ("shortened", "skill")):
                path = invoke(m, env, "transcribe-youtube", URLS[label], "url-" + label,
                              orientation, "txt", outdir, skill)
                text = path.read_text(encoding="utf-8")
                require(reference_score(text, m["operator"]["reference"]) >= m["operator"]["min_score"],
                        label + " URL pipeline transcript misses reviewed excerpt")
                assert_no_owned_temp(outdir, tempdir)
                results.append({"case": "url-" + label, "orientation": orientation,
                                "source": "transcribe-youtube", "formats": {"txt": {
                                    "sha256": digest(path), "bytes": path.stat().st_size}}})
        function_receipts = verify_function_receipts(env)
        cancellation = failure_and_cancel(m, env, outdir, tempdir, args.online)
        summary = {"schema": "s503-youtube-pipeline-result-v1", "candidate_commit": m["candidate_commit"],
                          "candidate_sha256": digest(m["bashy"]),
                          "os": m["os"], "arch": m["arch"], "offline": args.offline,
                          "online_acquisition": args.online,
                          "offline_enforcement": m.get("offline_enforcement") if args.offline else None,
                          "operator_media_sha256": YOUTUBE_SHA256, "skill_identity": skill["identity"],
                          "function_receipts": function_receipts,
                          "results": results, "failure_cleanup": "pass", "cancellation": cancellation}
        (outdir / "result.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as exc:
        sys.exit("gate failed: " + str(exc))

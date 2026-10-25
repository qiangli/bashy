#!/usr/bin/env python3
"""Bounded, real native local-media gate independent of URL acquisition."""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
spec = importlib.util.spec_from_file_location('transcription_checks', HERE / 'verify-youtube-pipeline.py')
checks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checks)

SOURCE_FILES = (
    'examples/transcribe/verify-controlled-native.py',
    'examples/transcribe/verify-youtube-pipeline.py',
    'examples/transcribe/pipeline.bsh',
    'examples/transcribe/adapters/register.sh',
    'skills/bashy/commands/transcribe.md',
    'skills/bashy/commands/transcribe.dhnt',
)


def validate(m):
    checks.require(m.get('schema') == 's503-controlled-native-v1', 'controlled manifest schema')
    checks.require(m.get('candidate_commit') == checks.checkout_commit(), 'candidate commit mismatch')
    checks.require(m.get('os') == platform.system().lower() and
                   m.get('arch') == platform.machine().lower(), 'native runner identity mismatch')
    hashes = m.get('source_sha256')
    checks.require(isinstance(hashes, dict) and set(hashes) == set(SOURCE_FILES),
                   'candidate source hash inventory incomplete')
    for source in SOURCE_FILES:
        checks.require(hashes[source] == checks.digest(ROOT / source) ==
                       checks.committed_digest(source), source + ' differs from candidate')
    for name in ('bashy', 'helper', 'python', 'whisper', 'model', 'webm_decoder'):
        path = Path(m.get(name, ''))
        checks.require(path.is_absolute() and path.is_file(), name + ' is not an absolute file')
        checks.require(checks.digest(path) == m.get(name + '_sha256'), name + ' SHA-256 mismatch')
    checks.require(Path(m['model']).name == 'ggml-tiny.bin' and
                   Path(m['model']).parent.name == 'whisper' and
                   Path(m['model']).parent.parent.name == 'models', 'model cache layout')
    checks.require(m.get('model_id') == 'tiny', 'multilingual tiny model pin')
    checks.require(m.get('whisper_build_run_id') == 40201076574, 'admitted whisper build run')
    cases = m.get('cases')
    checks.require(isinstance(cases, dict) and set(cases) ==
                   {'en-audio', 'en-video', 'es-audio'}, 'three local cases required')
    for label, item in cases.items():
        path = Path(item.get('path', ''))
        checks.require(path.is_absolute() and path.is_file() and
                       checks.digest(path) == item.get('sha256'), label + ' fixture hash mismatch')
        checks.require(item.get('decoder') == ('transcribe-wav' if label == 'es-audio'
                       else 'transcribe-webm-opus'), label + ' decoder mismatch')
        checks.require(item.get('language') == ('es' if label == 'es-audio' else 'en'),
                       label + ' expected detected language')
        checks.require(isinstance(item.get('duration_ms'), int) and item['duration_ms'] > 2000,
                       label + ' duration')
        checks.require(len(checks.words(item.get('reference', ''))) >= 6 and
                       0.5 <= item.get('min_score', 0) <= 1, label + ' reviewed reference')
        checks.require(item.get('license_evidence'), label + ' CC0 provenance')
    return m


def setup(m, env):
    bashy = m['bashy']
    checks.run([bashy, HERE / 'adapters' / 'register.sh', m['helper']], env)
    checks.add_record(bashy, env, 'transcribe-webm-opus', [m['webm_decoder']],
                      'decode', ['read', 'write', 'destroy', 'exec'])
    checks.add_record(bashy, env, 'transcribe-pipeline',
                      [bashy, '--bashsharp', str(HERE / 'pipeline.bsh')], None,
                      ['read', 'write', 'destroy', 'exec', 'net'])
    skill_dir = Path(env['BASHY_HOME']) / 'transcribe-source'
    skill_dir.mkdir(parents=True)
    shutil.copy2(ROOT / 'skills/bashy/commands/transcribe.md', skill_dir / 'SKILL.md')
    shutil.copy2(ROOT / 'skills/bashy/commands/transcribe.dhnt', skill_dir / 'skill.dhnt')
    checks.run([bashy, 'skill', 'add', str(skill_dir)], env)
    record = json.loads(checks.run([bashy, 'skill', 'show', 'transcribe', '--json'], env).stdout)
    checks.require(record.get('identity') and record.get('capability'), 'skill identity missing')
    return record


def detected_language(m, env, item, root):
    """Inspect the same registered decode/infer stage results before pipeline cleanup."""
    work = root / ('language-' + item['language'])
    work.mkdir()
    decode = {'version': 'bashy-transcribe-stage-v1', 'stage': 'decode',
              'artifacts': [{'path': item['path'], 'media_type':
                             'audio/wav' if item['decoder'] == 'transcribe-wav' else 'audio/webm'}],
              'media': {'source': item['path']}, 'work_dir': str(work)}
    input_path = work / 'decode.json'
    input_path.write_text(json.dumps(decode))
    argv = ([m['helper'], 'wav'] if item['decoder'] == 'transcribe-wav'
            else [m['webm_decoder']]) + [str(input_path)]
    decoded = json.loads(checks.run(argv, env, timeout=120).stdout)
    checks.require(not decoded.get('error') and decoded.get('artifacts'), 'direct decode failed')
    infer = {'version': 'bashy-transcribe-stage-v1', 'stage': 'infer',
             'artifacts': decoded['artifacts'], 'media': decoded['media'],
             'model': m['model_id'], 'language': 'auto', 'work_dir': str(work)}
    input_path.write_text(json.dumps(infer))
    result = json.loads(checks.run([m['helper'], '--whisper', m['whisper'],
                                   '--whisper-sha256', m['whisper_sha256'],
                                   'whisper', str(input_path)], env,
                                   timeout=m.get('case_timeout_seconds', 1800)).stdout)
    checks.require(not result.get('error') and result.get('media', {}).get('language') ==
                   item['language'], 'automatic language detection differs from ' + item['language'])
    checks.require(result.get('segments'), 'automatic language probe lacks real segments')
    shutil.rmtree(work)
    return {'language': result['media']['language'], 'segments': len(result['segments'])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('--offline', action='store_true')
    parser.add_argument('--validate-only', action='store_true')
    parser.add_argument('--output-dir', type=Path)
    args = parser.parse_args()
    m = validate(json.loads(args.manifest.read_text()))
    enforcement = checks.offline_canary(m) if args.offline else None
    if args.validate_only:
        print('controlled native manifest validated')
        return
    outdir = args.output_dir
    checks.require(outdir is not None and outdir.is_absolute() and not outdir.exists() and
                   ROOT not in outdir.parents, 'output directory must be new, absolute, private')
    outdir.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix='s503-controlled-') as owned:
        root = Path(owned)
        tempdir = root / 'tmp'
        tempdir.mkdir()
        env = dict(os.environ, BASHY_BIN=m['bashy'], BASHY_HINTS='off',
                   BASHY_HOME=str(root / 'home'), BASHY_COMMANDS_DIR=str(root / 'commands'),
                   BASHY_SKILLS_DIR=str(root / 'skills'),
                   BASHY_BIN_CACHE=str(Path(m['model']).parents[2] / 'bin'),
                   WHISPER_PATH=m['whisper'], WHISPER_SHA256=m['whisper_sha256'],
                   TMPDIR=str(tempdir), PATH=str(Path(m['bashy']).parent) +
                   os.pathsep + os.environ.get('PATH', ''))
        env.pop('FFMPEG_PATH', None)
        env.pop('FFMPEG_SHA256', None)
        skill = setup(m, env)
        results = []
        for label, item in m['cases'].items():
            case_m = dict(m, decoder={'name': item['decoder']})
            for orientation in ('script', 'skill'):
                paths = {fmt: checks.invoke(case_m, env, 'transcribe-local', item['path'],
                                             label, orientation, fmt, outdir, skill)
                         for fmt in checks.FORMATS}
                artifacts = checks.verify_artifacts(paths, item)
                checks.assert_no_owned_temp(outdir, tempdir)
                results.append({'case': label, 'orientation': orientation,
                                'formats': artifacts})
        languages = {label: detected_language(m, env, m['cases'][label], root)
                     for label in ('en-audio', 'es-audio')}
        receipts = checks.verify_function_receipts(env)
        cancel_m = dict(m, controlled_webm=m['cases']['en-video']['path'],
                        decoder={'name': 'transcribe-webm-opus'})
        cleanup = checks.failure_and_cancel(cancel_m, env, outdir, tempdir, False)
        summary = {'schema': 's503-controlled-native-result-v1',
                   'candidate_commit': m['candidate_commit'], 'candidate_sha256':
                   checks.digest(m['bashy']), 'os': m['os'], 'arch': m['arch'],
                   'offline': args.offline, 'offline_enforcement': enforcement,
                   'fixture_sha256': {k: v['sha256'] for k, v in m['cases'].items()},
                   'source_sha256': m['source_sha256'], 'skill_identity': skill['identity'],
                   'function_receipts': receipts, 'detected_languages': languages,
                   'results': results, 'failure_cleanup': 'pass',
                   'local_pipeline_cancellation': cleanup}
        (outdir / 'result.json').write_text(json.dumps(summary, indent=2) + '\n')
        print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as exc:
        sys.exit('controlled gate failed: ' + str(exc))

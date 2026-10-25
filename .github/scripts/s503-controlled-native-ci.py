#!/usr/bin/env python3
"""Admission and evidence glue for the hosted S503 local-media gate."""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import shutil
import socket
import subprocess
import sys
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
ADMISSION = ROOT / 'docs/sprint-503/evidence/native-transcription-admission.json'
FIXTURES = ROOT / 'examples/transcribe/fixtures/controlled-native.json'
EVIDENCE = ROOT / 'evidence'
LABELS = {'ubuntu-24.04', 'ubuntu-24.04-arm', 'macos-15-intel',
          'macos-15', 'windows-2025', 'windows-11-arm'}
WHISPER_SOURCE = '7abe1fd1ae59c78c21bd04bed469df092c0da1f7'
WHISPER_ARCHIVE_SHA256 = 'ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720'
MODEL_URL = ('https://huggingface.co/ggerganov/whisper.cpp/resolve/'
             '5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny.bin')


def require(ok, message):
    if not ok:
        raise SystemExit('BLOCKED: ' + message)


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda: f.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def read(path):
    return json.loads(Path(path).read_text())


def save(name, document):
    EVIDENCE.mkdir(exist_ok=True)
    (EVIDENCE / name).write_text(json.dumps(document, indent=2) + '\n')


def admission():
    a = read(ADMISSION)
    label = os.environ['TARGET_LABEL']
    require(label in LABELS and a['schema'] == 's503-native-transcription-admission-v2',
            'six-target reviewed admission')
    require(a['whisper_build_run_id'] == 40201076574 and
            a['whisper_build_commit'] == WHISPER_SOURCE, 'pinned whisper source build')
    require(set(a['whisper']) == LABELS and set(a['python']) == LABELS and
            set(a['offline_canary']) == LABELS, 'incomplete native target admission')
    require(a['model_sha256'] ==
            'be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21'
            and a['model_bytes'] == 77691713, 'reviewed multilingual ggml-tiny model')
    require(a['whisper'][label]['executable_sha256'] ==
            os.environ['EXPECTED_WHISPER_SHA256'], 'native whisper digest from matrix')
    return a


def identity():
    a = admission()
    raw = platform.machine()
    arch_map = {'x86_64': 'amd64', 'AMD64': 'amd64', 'X64': 'amd64',
                'aarch64': 'arm64', 'arm64': 'arm64', 'ARM64': 'arm64'}
    name = {'Darwin': 'macOS'}.get(platform.system(), platform.system())
    process_arch = arch_map.get(raw, raw)
    os_arch = process_arch
    if name == 'Windows':
        native = subprocess.check_output(['pwsh', '-NoProfile', '-Command',
            '[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()'],
            text=True).strip()
        os_arch = arch_map.get(native.upper(), native.lower())
    translated = '0'
    if name == 'macOS':
        proc = subprocess.run(['sysctl', '-n', 'sysctl.proc_translated'],
                              capture_output=True, text=True)
        if proc.returncode == 0:
            translated = proc.stdout.strip()
    go_arch = subprocess.check_output(['go', 'env', 'GOHOSTARCH'], text=True).strip()
    require(name == os.environ['TARGET_OS'] and os_arch == os.environ['TARGET_ARCH']
            and process_arch == os_arch and go_arch == os_arch and translated != '1',
            'runner, Python process or Go toolchain is not native')
    runtime = json.loads(subprocess.check_output(
        [sys.executable, str(ROOT / 'examples/transcribe/dependencies/probe_runtime.py')],
        text=True, timeout=60))
    reviewed = a['python'][os.environ['TARGET_LABEL']]
    require(all(runtime['python'][k] == reviewed[k] for k in
                ('source', 'version', 'sha256')), 'external Python differs from reviewed bytes')
    vc = a['windows_vc_runtime'].get(os.environ['TARGET_LABEL'], {})
    require(set(runtime['windows_vc_runtime']) == set(vc) and all(
        runtime['windows_vc_runtime'][n][k] == vc[n][k]
        for n in vc for k in ('source', 'version', 'sha256')),
        'external Windows VC runtime differs from reviewed bytes')
    save('external-runtime.json', runtime)
    save('runner-identity.json', {
        'schema': 's503-runner-identity-v1', 'runner_label': os.environ['TARGET_LABEL'],
        'actual_os': name, 'actual_arch': os_arch, 'process_arch': raw,
        'go_host_arch': go_arch, 'translated': translated,
        'python_version': platform.python_version(),
        'python_executable': sys.executable, 'python_sha256': sha(sys.executable),
        'commit_sha': os.environ['CANDIDATE_COMMIT']})


def fetch(url, path, expected):
    request = urllib.request.Request(url, headers={
        'User-Agent': 'S503-controlled-native/1.0 (github.com/qiangli/bashy)'})
    with urllib.request.urlopen(request, timeout=120) as src, path.open('wb') as dst:
        shutil.copyfileobj(src, dst, 1 << 20)
    require(sha(path) == expected, 'downloaded artifact SHA-256 mismatch: ' + str(path))


def fixtures():
    fixtures = read(FIXTURES)
    require(fixtures['schema'] == 's503-controlled-fixtures-v1', 'fixture admission schema')
    authoring = fixtures['video_authoring']
    report_path = ROOT / 'examples/transcribe/fixtures/video-authoring.json'
    require(authoring['run_id'] == 40199530099 and
            authoring['artifact_name'] == 's503-controlled-video' and
            authoring['source_commit'] ==
            '10d88eb38c89beeb0d4324425a08b084c401c82d',
            'hosted video authoring run coordinates')
    actual_report_sha256 = sha(report_path)
    require(actual_report_sha256 == authoring['report_sha256'],
            'hosted video authoring report SHA-256: expected ' +
            authoring['report_sha256'] + ', got ' + actual_report_sha256)
    video_report = read(report_path)
    require(video_report['schema'] == 's503-controlled-video-authoring-v1' and
            video_report['candidate_commit'] == authoring['source_commit'] and
            video_report['audio_source_sha256'] ==
            fixtures['cases']['en-audio']['sha256'] and
            video_report['fixture_sha256'] == fixtures['cases']['en-video']['sha256'] and
            video_report['bytes'] == fixtures['cases']['en-video']['bytes'] and
            video_report['duration_seconds'] == '11.000000' and
            video_report['codecs'] == {'video': 'vp9', 'audio': 'opus'} and
            video_report['ffmpeg_ubuntu_package_version'] == '7:6.1.1-3ubuntu5' and
            video_report['transformation_command'].startswith('ffmpeg -hide_banner'),
            'actual VP9+Opus hosted fixture authoring record')
    require(fixtures['cases']['es-audio']['source_sha256'] ==
            'bf2168307813beb1037c3ee4e9efceee62bfbd0e4920adcc09f74d03e4b5118b'
            and fixtures['cases']['es-audio']['sha256'] ==
            'df23f58c95b54215e136bd6b1865a9aba360010a02822ec56e8854bc8074e2bb',
            'CC0 Spanish source and authored output pin')
    source_cases = {
        'en-audio': ROOT / 'examples/transcribe/webm-opus/testdata/speech-long.webm',
        'en-video': ROOT / 'examples/transcribe/fixtures/speech-video.webm',
        'es-audio': ROOT / 'examples/transcribe/fixtures/spanish-proverb.wav',
    }
    require(set(fixtures['cases']) == set(source_cases), 'controlled fixture case inventory')
    for label, original in source_cases.items():
        record = fixtures['cases'][label]
        require(original.is_file() and sha(original) == record['sha256'] and
                original.stat().st_size == record['bytes'], label + ' fixture bytes')
        require(record['license_evidence'].startswith('https://commons.wikimedia.org/'),
                label + ' primary CC0 license page')
    print('PASS: pinned CC0 en audio/video and es audio fixture hashes, authoring and provenance')
    return fixtures


def prepare():
    a = admission()
    fixture_index = fixtures()
    root = Path(os.environ['RUNNER_TEMP']) / 's503-controlled-native'
    artifact = Path(os.environ['RUNNER_TEMP']) / 's503-whisper-admitted'
    suffix = '.exe' if os.environ['TARGET_OS'] == 'Windows' else ''
    binary = artifact / ('whisper-cli' + suffix)
    report, runner = read(artifact / 'build-report.json'), read(artifact / 'runner-identity.json')
    digest = os.environ['EXPECTED_WHISPER_SHA256']
    platform_name = {'Linux': 'linux', 'macOS': 'darwin', 'Windows': 'windows'}[os.environ['TARGET_OS']]
    require(sha(binary) == digest == report['executable_sha256'] and
            report['executable_bytes'] == binary.stat().st_size and
            report['platform'] == platform_name + '/' + os.environ['TARGET_ARCH'] and
            report['runner_label'] == os.environ['TARGET_LABEL'] and
            report['status'] == 'built_and_cached' and report['whisper_version'] == '1.9.5' and
            report['source_archive_sha256'] == WHISPER_ARCHIVE_SHA256 and
            report['cpu_only'] and report['portable_cpu'] and
            '-DGGML_NATIVE=OFF' in report['build_flags'] and
            '-DGGML_CPU_ALL_VARIANTS=OFF' in report['build_flags'] and
            (os.environ['TARGET_ARCH'] != 'amd64' or
             '-DGGML_SSE42:BOOL=OFF' in report['build_flags']) and
            report['openmp_disabled'] and report['gpl_free'] and
            runner['runner_label'] == os.environ['TARGET_LABEL'] and
            runner['commit_sha'] == WHISPER_SOURCE and
            (artifact / 'binary-sha256.txt').read_text().strip() == digest + '  ' + binary.name,
            'whisper build artifact identity, source or bytes')
    if os.environ['TARGET_OS'] == 'Windows':
        require({'msvcp140.dll', 'vcruntime140.dll'}.issubset(
            {s.lower() for s in report['imported_libraries']}), 'VC runtime imports changed')
    whisper = root / 'cache/tools/whisper-cpp/v1.9.5' / binary.name
    whisper.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(binary, whisper)
    whisper.chmod(0o755)
    require(sha(whisper) == digest, 'whisper cache copy differs')
    save('whisper-build.json', report)

    model = root / 'cache/models/whisper/ggml-tiny.bin'
    model.parent.mkdir(parents=True, exist_ok=True)
    fetch(MODEL_URL, model, a['model_sha256'])
    require(model.stat().st_size == a['model_bytes'], 'model byte count')
    save('model-admission.json', {'source_url': MODEL_URL, 'sha256': sha(model),
                                  'bytes': model.stat().st_size})

    path = root / 'bin'
    bashy = path / ('bashy' + suffix)
    helper = path / ('transcribe-adapter' + suffix)
    decoder = path / ('transcribe-webm-opus' + suffix)
    for p in (bashy, helper, decoder):
        require(p.is_file(), 'missing candidate native binary ' + str(p))
    cases = {}
    source_cases = {
        'en-audio': ROOT / 'examples/transcribe/webm-opus/testdata/speech-long.webm',
        'en-video': ROOT / 'examples/transcribe/fixtures/speech-video.webm',
        'es-audio': ROOT / 'examples/transcribe/fixtures/spanish-proverb.wav',
    }
    for label, original in source_cases.items():
        record = fixture_index['cases'][label]
        require(sha(original) == record['sha256'] and
                original.stat().st_size == record['bytes'], label + ' fixture bytes')
        retained = root / 'fixtures' / original.name
        retained.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, retained)
        require(sha(retained) == record['sha256'], label + ' copied fixture bytes')
        cases[label] = dict(record, path=str(retained.resolve()))
    spec = importlib.util.spec_from_file_location('controlled_gate',
        ROOT / 'examples/transcribe/verify-controlled-native.py')
    gate = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(gate)
    sources = {name: sha(ROOT / name) for name in gate.SOURCE_FILES}
    manifest = {'schema': 's503-controlled-native-v1',
                'candidate_commit': os.environ['CANDIDATE_COMMIT'],
                'whisper_build_run_id': 40201076574,
                'source_sha256': sources, 'os': platform.system().lower(),
                'arch': platform.machine().lower(),
                'bashy': str(bashy.resolve()), 'bashy_sha256': sha(bashy),
                'helper': str(helper.resolve()), 'helper_sha256': sha(helper),
                'webm_decoder': str(decoder.resolve()), 'webm_decoder_sha256': sha(decoder),
                'python': sys.executable, 'python_sha256': sha(sys.executable),
                'whisper': str(whisper.resolve()), 'whisper_sha256': digest,
                'model': str(model.resolve()), 'model_sha256': sha(model), 'model_id': 'tiny',
                'cases': cases, 'case_timeout_seconds': 1800, 'cancel_after_seconds': 0.05}
    (root / 'manifest-online.json').write_text(json.dumps(manifest, indent=2) + '\n')
    save('manifest-online.json', manifest)


def enforcer():
    a = admission()
    choices = {'Linux': ('linux-unshare-net', '/usr/bin/unshare'),
               'macOS': ('macos-sandbox-deny-network', '/usr/bin/sandbox-exec'),
               'Windows': ('windows-firewall-program-block', shutil.which('pwsh'))}
    method, path = choices[os.environ['TARGET_OS']]
    require(path and Path(path).is_file(), 'native OS network enforcer unavailable')
    plan = a['offline_canary'][os.environ['TARGET_LABEL']]
    require(plan['method'] == method and plan['enforcer_sha256'] == sha(path) and
            plan['probe_command'] == ['python-socket-connect', '1.1.1.1:443'],
            'network enforcer differs from reviewed admission')
    save('offline-enforcer.json', {'method': method, 'path': str(Path(path).resolve()),
                                  'sha256': sha(path), 'probe_command': plan['probe_command']})


def network_probe(phase, manifest):
    code = ('import socket,sys\ntry:\n'
            ' socket.create_connection(("1.1.1.1",443),timeout=5)\n'
            'except OSError as exc:\n'
            ' print("DENIED_ERRNO=%s DENIED_WINERROR=%s ERROR=%s" % '
            '(exc.errno,getattr(exc,"winerror",None),exc))\n sys.exit(23)\n'
            'sys.exit(0)\n')
    proc = subprocess.run([sys.executable, '-c', code], capture_output=True,
                          text=True, timeout=10)
    output = (proc.stdout + proc.stderr).strip()
    allowed = {'Linux': ('DENIED_ERRNO=1 ', 'DENIED_ERRNO=13 ', 'DENIED_ERRNO=101 '),
               'Darwin': ('DENIED_ERRNO=1 ', 'DENIED_ERRNO=13 ', 'DENIED_ERRNO=51 '),
               'Windows': ('DENIED_WINERROR=10013 ',)}
    require(proc.returncode == 23 and any(s in output for s in allowed[platform.system()]),
            'external socket probe did not prove OS denial: ' + output)
    enforcer_record = read(EVIDENCE / 'offline-enforcer.json')
    record = {'schema': 's503-network-denial-v1', 'os': manifest['os'],
              'arch': manifest['arch'], 'candidate_commit': manifest['candidate_commit'],
              'network_denied': True, 'probe_command': enforcer_record['probe_command'],
              'actual_argv': [sys.executable, '-c', code], 'exit_code': proc.returncode,
              'error': output, 'method': enforcer_record['method'],
              'enforcer_sha256': enforcer_record['sha256'], 'phase': phase}
    save('network-denial-' + phase + '.json', record)
    return EVIDENCE / ('network-denial-' + phase + '.json')


def offline():
    online = read(EVIDENCE / 'manifest-online.json')
    root = Path(online['bashy']).parent.parent
    require(sys.executable == online['python'] and sha(sys.executable) ==
            online['python_sha256'], 'Python changed before offline replay')
    before = network_probe('before', online)
    enforcer_record = read(EVIDENCE / 'offline-enforcer.json')
    offline_manifest = dict(online, offline_enforcement={
        'method': enforcer_record['method'], 'canary_report': str(before.resolve()),
        'canary_sha256': sha(before)})
    path = root / 'manifest-offline.json'
    path.write_text(json.dumps(offline_manifest, indent=2) + '\n')
    save('manifest-offline.json', offline_manifest)
    try:
        proc = subprocess.run([sys.executable,
            str(ROOT / 'examples/transcribe/verify-controlled-native.py'), str(path),
            '--offline', '--output-dir', str(root / 'pipeline-offline')], cwd=ROOT)
    finally:
        network_probe('after', online)
    require(proc.returncode == 0, 'offline controlled pipeline failed')


def check():
    root = Path(os.environ['RUNNER_TEMP']) / 's503-controlled-native'
    for phase in ('before', 'after'):
        r = read(EVIDENCE / ('network-denial-' + phase + '.json'))
        require(r['network_denied'] is True and r['exit_code'] == 23,
                'OS network denial canary ' + phase)
    for mode in ('online', 'offline'):
        r = read(root / ('pipeline-' + mode) / 'result.json')
        require(r['schema'] == 's503-controlled-native-result-v1' and
                r['candidate_commit'] == os.environ['CANDIDATE_COMMIT'] and
                r['offline'] == (mode == 'offline') and len(r['results']) == 6 and
                set(r['detected_languages']) == {'en-audio', 'es-audio'} and
                r['detected_languages']['en-audio']['language'] == 'en' and
                r['detected_languages']['es-audio']['language'] == 'es' and
                r['function_receipts']['count'] >= 2 and
                r['failure_cleanup'] == 'pass' and
                r['local_pipeline_cancellation'] == 'pass',
                mode + ' native result incomplete')
    print('PASS: local audio/video en/es automatic CPU inference, script/skill, '
          'TXT/SRT/VTT, OS-offline replay, failure/cancellation cleanup')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=('identity', 'fixtures', 'prepare', 'enforcer', 'offline', 'check'))
    args = parser.parse_args()
    globals()[args.action]()


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Hosted-runner network-policy preflight; no media or inference dependencies."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import socket
import sys
import time

TARGET = ('1.1.1.1', 443)
DENIED_ERRNOS = {
    'Linux': {1, 13, 101},
    'Darwin': {1, 13, 51},
    'Windows': {10013, 10051, 10060},
}


def sha256(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def write(path, data):
    Path(path).write_text(json.dumps(data, indent=2, sort_keys=True) + '\n')


def probe():
    started = time.monotonic()
    try:
        with socket.create_connection(TARGET, timeout=5):
            pass
        return {'connected': True, 'denied': False, 'error': '', 'errno': None,
                'elapsed_seconds': round(time.monotonic() - started, 3)}
    except OSError as exc:
        number = (getattr(exc, 'winerror', None) or exc.errno) if platform.system() == 'Windows' else exc.errno
        # Windows Firewall can report a socket timeout rather than WSAEACCES.
        if platform.system() == 'Windows' and isinstance(exc, TimeoutError):
            number = 10060
        return {'connected': False, 'denied': number in DENIED_ERRNOS[platform.system()],
                'error': str(exc), 'errno': number,
                'elapsed_seconds': round(time.monotonic() - started, 3)}


def observation(phase):
    data = {'schema': 's503-os-network-probe-v1', 'phase': phase,
            'os': platform.system(), 'arch': platform.machine(),
            'python': sys.executable, 'python_sha256': sha256(sys.executable),
            'target': '1.1.1.1:443', 'timestamp_utc': time.time()}
    data.update(probe())
    return data


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['identity', 'enforcer', 'baseline', 'policy',
                                            'restored', 'evaluate'])
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--path', type=Path)
    parser.add_argument('--label')
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    if args.command == 'identity':
        expected = {
            'ubuntu-24.04': ('Linux', 'amd64'),
            'ubuntu-24.04-arm': ('Linux', 'arm64'),
            'macos-15-intel': ('Darwin', 'amd64'),
            'macos-15': ('Darwin', 'arm64'),
            'windows-2025': ('Windows', 'amd64'),
            'windows-11-arm': ('Windows', 'arm64'),
        }
        arch = {'x86_64': 'amd64', 'amd64': 'amd64', 'aarch64': 'arm64',
                'arm64': 'arm64'}.get(platform.machine().lower())
        native = expected.get(args.label) == (platform.system(), arch)
        write(args.out / 'identity.json', {
            'schema': 's503-os-network-identity-v1', 'label': args.label,
            'os': platform.system(), 'arch': platform.machine(),
            'python': sys.executable, 'python_sha256': sha256(sys.executable),
            'native_target': native})
        if not native:
            raise SystemExit('runner process OS/architecture differs from target label')
    elif args.command == 'enforcer':
        if args.path is None:
            defaults = {'Linux': '/usr/bin/unshare',
                        'Darwin': '/usr/bin/sandbox-exec',
                        'Windows': shutil.which('pwsh')}
            value = defaults[platform.system()]
            args.path = Path(value) if value else None
        if not args.path or not args.path.is_file() or not args.path.is_absolute():
            raise SystemExit('enforcer path must be an existing absolute file')
        write(args.out / 'enforcer.json', {
            'schema': 's503-os-network-enforcer-v1',
            'path': str(args.path.resolve()), 'sha256': sha256(args.path),
            'bytes': args.path.stat().st_size})
    elif args.command in ('baseline', 'restored'):
        result = observation(args.command)
        write(args.out / (args.command + '.json'), result)
        if not result['connected']:
            raise SystemExit(args.command + ' TCP canary could not connect: ' + result['error'])
    elif args.command == 'policy':
        before = observation('denied-before')
        write(args.out / 'denied-before.json', before)
        # A second probe ensures denial persists while the same policy is active.
        time.sleep(1)
        after = observation('denied-after')
        write(args.out / 'denied-after.json', after)
        if not before['denied'] or not after['denied']:
            raise SystemExit('OS policy did not deny both live TCP probes')
    else:
        required = ['identity', 'enforcer', 'baseline', 'policy-attempt',
                    'denied-before', 'denied-after', 'restored']
        evidence = {}
        errors = []
        for name in required:
            path = args.out / (name + '.json')
            if path.is_file():
                evidence[name] = json.loads(path.read_text())
            else:
                errors.append(name + ' evidence missing')
        if 'baseline' in evidence and not evidence['baseline'].get('connected'):
            errors.append('baseline TCP connection failed')
        if 'restored' in evidence and not evidence['restored'].get('connected'):
            errors.append('post-policy TCP connection failed')
        for name in ('denied-before', 'denied-after'):
            if name in evidence and not evidence[name].get('denied'):
                errors.append(name + ' was not OS-denied')
        if 'policy-attempt' in evidence and not evidence['policy-attempt'].get('success'):
            errors.append('policy application or cleanup failed')
        if 'enforcer' in evidence and len(evidence['enforcer'].get('sha256', '')) != 64:
            errors.append('enforcer SHA-256 missing')
        if 'identity' in evidence:
            actual = evidence['identity']
            if not actual.get('native_target'):
                errors.append('runner process is not native target')
            for name in ('baseline', 'denied-before', 'denied-after', 'restored'):
                if name in evidence and (evidence[name].get('os') != actual['os'] or
                                         evidence[name].get('arch') != actual['arch'] or
                                         evidence[name].get('python_sha256') != actual['python_sha256']):
                    errors.append(name + ' process identity changed')
        write(args.out / 'result.json', {
            'schema': 's503-os-network-preflight-result-v1',
            'status': 'pass' if not errors else 'fail', 'errors': errors,
            'label': args.label, 'evidence_files': sorted(evidence)})
        if errors:
            raise SystemExit('; '.join(errors))
        print('PASS: OS denial, two live probes, and restored TCP connectivity')


if __name__ == '__main__':
    main()

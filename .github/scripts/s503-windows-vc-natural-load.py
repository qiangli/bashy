#!/usr/bin/env python3
"""Collect Windows Python VC load evidence without loading a named VC DLL.

This is hosted-runner research only. It reads the existing private Python
install receipt and writes bounded paths, hashes, versions and signatures.
It never changes an admission policy or copies a DLL into the artifact.
"""

import argparse
import base64
import hashlib
import json
from pathlib import Path
import subprocess


PROBE = r'''
import ctypes, hashlib, json, platform, sys

names = ('MSVCP140.dll', 'VCRUNTIME140.dll', 'VCRUNTIME140_1.dll',
         'CONCRT140.dll', 'ucrtbase.dll')
kernel = ctypes.WinDLL('kernel32', use_last_error=True)
kernel.GetModuleHandleW.argtypes = (ctypes.c_wchar_p,)
kernel.GetModuleHandleW.restype = ctypes.c_void_p
kernel.GetModuleFileNameW.argtypes = (ctypes.c_void_p, ctypes.c_wchar_p, ctypes.c_uint32)
kernel.GetModuleFileNameW.restype = ctypes.c_uint32

def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()

def snapshot():
    rows = {}
    for name in names:
        handle = kernel.GetModuleHandleW(name)  # Observes only; never LoadLibrary.
        if not handle:
            rows[name] = None
            continue
        buffer = ctypes.create_unicode_buffer(32768)
        length = kernel.GetModuleFileNameW(handle, buffer, len(buffer))
        if not length or length >= len(buffer) - 1:
            raise RuntimeError('module path unavailable: ' + name)
        rows[name] = {'path': buffer.value, 'sha256': digest(buffer.value)}
    return rows

before = snapshot()
import bz2, lzma, ssl, zlib
after = snapshot()
print(json.dumps({'machine': platform.machine(), 'python': sys.executable,
                  'python_sha256': digest(sys.executable),
                  'before_stdlib_imports': before, 'after_stdlib_imports': after},
                 sort_keys=True))
'''


def sha256(path):
    h = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def signatures(paths):
    quoted = ','.join("'" + str(path).replace("'", "''") + "'" for path in paths)
    script = (
        "$ErrorActionPreference='Stop'; $paths=@(" + quoted + "); "
        "$rows=@(foreach($p in $paths) { "
        "$s=Get-AuthenticodeSignature -LiteralPath $p; "
        "$f=Get-Item -LiteralPath $p; "
        "[PSCustomObject]@{path=$p;status=[string]$s.Status;"
        "signer=$(if($s.SignerCertificate){$s.SignerCertificate.Subject}else{$null});"
        "file_version=$f.VersionInfo.FileVersion} }); "
        "ConvertTo-Json -InputObject $rows -Depth 3 -Compress"
    )
    encoded = base64.b64encode(script.encode('utf-16-le')).decode('ascii')
    run = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive',
                          '-EncodedCommand', encoded], check=True, capture_output=True,
                         text=True, timeout=30)
    rows = json.loads(run.stdout)
    require(isinstance(rows, list) and len(rows) == len(paths),
            'signature result count differs from inventoried VC paths')
    return rows


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--installed-result', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    receipt_bytes = args.installed_result.read_bytes()
    receipt = json.loads(receipt_bytes)
    target = receipt.get('expected_target')
    require(target in ('windows/amd64', 'windows/arm64'),
            'expected native Windows target missing')
    require(receipt.get('result') == 'INSTALLED_EVIDENCE_CAPTURED_UNADMITTED',
            'fresh installed-Python research receipt missing')
    require(receipt.get('product_admitted') is False and
            receipt.get('operator_admitted') is False,
            'research receipt must not claim admission')
    interpreter = Path(receipt['installed_python']['path'])
    require(interpreter.is_file(), 'installed interpreter missing')
    require(sha256(interpreter) == receipt['installed_python']['sha256'],
            'installed interpreter differs from reviewed receipt')

    invent = receipt['windows_vc']
    paths = [Path(row['path']) for row in invent['install_local']]
    paths.extend(Path(row['path']) for row in invent['system32'].values())
    require(len(paths) == len(set(str(path).lower() for path in paths)),
            'duplicate VC inventory path')
    for row in invent['install_local'] + list(invent['system32'].values()):
        path = Path(row['path'])
        require(path.is_file() and sha256(path) == row['sha256'],
                'inventoried VC file missing or changed: ' + str(path))

    run = subprocess.run([str(interpreter), '-I', '-S', '-c', PROBE],
                         check=True, capture_output=True, text=True, timeout=30)
    probe = json.loads(run.stdout)
    machine = probe['machine'].lower()
    expected_machine = 'amd64' if target == 'windows/amd64' else 'arm64'
    require(machine in ((expected_machine, 'x64') if expected_machine == 'amd64'
                        else ('arm64',)), 'interpreter is not native target')
    require(Path(probe['python']).resolve() == interpreter.resolve() and
            probe['python_sha256'] == receipt['installed_python']['sha256'],
            'natural-load probe ran under different Python bytes')
    known = {str(path.resolve()).lower(): sha256(path) for path in paths}
    unreviewed = {}
    observed_paths = {str(path.resolve()).lower(): path for path in paths}
    for phase in ('before_stdlib_imports', 'after_stdlib_imports'):
        for name, row in probe[phase].items():
            if row is None:
                continue
            path = Path(row['path']).resolve()
            require(path.is_file() and sha256(path) == row['sha256'],
                    'naturally loaded module changed after observation: ' + name)
            observed_paths[str(path).lower()] = path
            if name.lower() != 'ucrtbase.dll':
                if (name in ('VCRUNTIME140_1.dll', 'CONCRT140.dll') or
                        known.get(str(path).lower()) != row['sha256']):
                    unreviewed[name] = row

    report = {
        'schema': 's503-windows-vc-natural-load-v1',
        'scope': 'native hosted research; GetModuleHandleW observes without explicit VC LoadLibrary',
        'target': target,
        'installed_result_sha256': hashlib.sha256(receipt_bytes).hexdigest(),
        'installed_python_sha256': receipt['installed_python']['sha256'],
        'vc_inventory': invent,
        'natural_load': probe,
        'authenticode': signatures(list(observed_paths.values())),
        'unreviewed_natural_vc_loads': unreviewed,
        'result': ('NATURAL_LOAD_EVIDENCE_CAPTURED_UNADMITTED' if not unreviewed
                   else 'UNREVIEWED_NATURAL_VC_LOADS_UNADMITTED'),
        'product_admitted': False,
        'operator_admitted': False,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2, sort_keys=True) + '\n',
                           encoding='utf-8')


if __name__ == '__main__':
    main()

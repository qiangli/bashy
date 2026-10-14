#!/usr/bin/env python3
"""Bounded real runner protocol gate. No mock, PATH runner or download fallback."""
import errno
import hashlib
import http.client
import importlib.util
import json
import math
import os
from pathlib import Path
import platform
import signal
import socket
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
SOURCE = '590109c8352e8d5a6206e8909b518a54a2b0a7b8'
CHECKS = ['bind_owner', 'launched_health', 'model_load', 'ready_health',
          'streaming_completion', 'embedding', 'cancellation_slot_reuse', 'shutdown']


def require(value, message):
    if not value:
        raise RuntimeError(message)


def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def deny_controls(port):
    """EPERM/EACCES, not timeout/refused/unreachable, proves sandbox enforcement."""
    for family, host, remote in [(socket.AF_INET, '192.0.2.1', 443),
                                  (socket.AF_INET, '127.0.0.1', port + 1),
                                  (socket.AF_INET6, '::1', port)]:
        for kind in (socket.SOCK_STREAM, socket.SOCK_DGRAM):
            with socket.socket(family, kind) as s:
                s.settimeout(2)
                try:
                    if kind == socket.SOCK_DGRAM:
                        s.sendto(b'control', (host, remote))
                    else:
                        s.connect((host, remote))
                except OSError as exc:
                    require(exc.errno in (errno.EPERM, errno.EACCES),
                            f'control failed without policy denial: {exc}')
                    print(f'policy denied {family}/{kind} {host}:{remote}: {exc}', flush=True)
                else:
                    raise RuntimeError('network denial control escaped')


def request(port, route, body=None, timeout=15):
    conn = http.client.HTTPConnection('127.0.0.1', port, timeout=timeout)
    try:
        conn.request('GET' if body is None else 'POST', route,
                     None if body is None else json.dumps(body),
                     {'Content-Type': 'application/json'})
        response = conn.getresponse()
        if response.status != 200:
            response.close()
            raise RuntimeError(f'{route}: HTTP {response.status}')
        return conn, response
    except Exception:
        conn.close()
        raise


def api(port, route, body=None, timeout=15):
    conn, response = request(port, route, body, timeout)
    try:
        data = response.read(1024 * 1024 + 1)
        require(len(data) <= 1024 * 1024, f'{route}: oversized JSON response')
        return json.loads(data)
    finally:
        response.close()
        conn.close()


def completion_body(cancel=False):
    body = {'Prompt': 'Once upon a time', 'Shift': True,
            'Options': {'num_predict': -1 if cancel else 24, 'num_keep': 0,
                        'temperature': 0, 'seed': 42, 'top_k': 40, 'top_p': 0.9,
                        'repeat_last_n': 0, 'repeat_penalty': 1.0}}
    if cancel:
        # Right-recursive grammar has no terminal production: no EOS. A healthy
        # single-slot server MUST cancel to service the subsequent completion.
        body['Grammar'] = 'root ::= "a" root'
    return body


def stream(port, cancel=False):
    conn, response = request(port, '/completion', completion_body(cancel), timeout=15)
    try:
        require(response.getheader('Transfer-Encoding') == 'chunked', 'not streamed HTTP')
        frames = []
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            line = response.readline(65537)
            require(line and len(line) <= 65536, 'missing/oversize completion frame')
            item = json.loads(line)
            frames.append(item)
            if cancel:
                require(not item.get('done'), 'cancellation request ended before cancellation')
                if item.get('content'):
                    # Shut down the live TCP connection, not just a buffered reader.
                    conn.sock.shutdown(socket.SHUT_RDWR)
                    return {'first_frame': item, 'terminal_received': False}
            elif item.get('done'):
                require(len(frames) >= 2 and any(f.get('content') for f in frames[:-1]),
                        'completion has no streamed content')
                require(item.get('eval_count', 0) > 0 and item.get('prompt_eval_count', 0) > 0,
                        'completion missing evaluation counts')
                return {'frames': frames}
        raise RuntimeError('completion deadline exceeded')
    finally:
        response.close()
        conn.close()


def check_windows_owner(output, pid, port):
    # Loopback-scoped, not exact-single: the gated runner binds 127.0.0.1
    # (pinned llammarunner/ollamarunner both Listen on 127.0.0.1:port), but
    # the hosted Windows TCP stack may also surface the ::1 companion. Every
    # listener on the gate port must be loopback AND owned by the gated
    # runner pid; a wildcard bind or a foreign owner fails. The exact-single
    # form failed CI runs 38448516095, 38450982320, 38452584047 while the
    # loopback-only property itself held.
    listeners = []
    for line in output.splitlines():
        fields = line.split()
        if len(fields) != 5 or fields[0] != 'TCP' or fields[3] != 'LISTENING':
            continue
        endpoint = fields[1]
        if endpoint == f'127.0.0.1:{port}' or endpoint == f'[::1]:{port}':
            listeners.append((endpoint, fields[4]))
        elif endpoint.rsplit(':', 1)[-1].rstrip(']') == str(port):
            listeners.append((endpoint, fields[4]))
    require(listeners, f'no loopback listener on port {port}: {output.strip()}')
    for endpoint, owner in listeners:
        require(owner == str(pid), f'wrong listener owner for {endpoint}: {owner} != {pid}')
        require(endpoint in (f'127.0.0.1:{port}', f'[::1]:{port}'),
                f'non-loopback bind on gate port: {endpoint}')
    require(len(set(listeners)) == len(listeners), f'duplicate listener rows: {listeners}')
    return '; '.join(f'TCP {endpoint} LISTENING {owner}' for endpoint, owner in listeners)


def check_owner(pid, port):
    if os.name == 'nt':
        output = subprocess.check_output(['netstat.exe', '-ano', '-p', 'tcp'],
                                         text=True, timeout=15)
        return check_windows_owner(output, pid, port)
    output = subprocess.check_output(['lsof', '-nP', '-a', '-p', str(pid),
                                      '-iTCP', '-sTCP:LISTEN', '-Fpn'], text=True, timeout=10)
    addresses = [line[1:] for line in output.splitlines() if line.startswith('n')]
    require(addresses == [f'127.0.0.1:{port}'], f'wrong listener ownership/bind: {output}')
    return output.strip()


def wait_health(proc, port, expected):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        require(proc.poll() is None, 'runner exited before health')
        try:
            health = api(port, '/health', timeout=2)
            if health.get('status') == expected:
                require(proc.poll() is None, 'runner exited during health')
                return health
        except (OSError, http.client.HTTPException):
            pass
        time.sleep(0.1)
    raise RuntimeError(f'health deadline: expected status {expected}')


def validate_embedding(embedding):
    vector = embedding.get('embedding', [])
    require(vector and all(type(v) in (int, float) and math.isfinite(v) for v in vector)
            and any(v != 0 for v in vector) and embedding.get('prompt_eval_count', 0) > 0,
            'invalid/empty/nonfinite embedding')


def exercise(binary, fixture, evidence_path, port):
    # Network enforcement on Windows comes from the Defender Firewall gate
    # (windows-defender-egress.ps1), which launches this exercise under
    # disposable outbound BLOCK rules: no launcher, no pidfile, no job
    # object. The runner is a direct child, so proc.pid is authoritative.
    binary, fixture, evidence_path = map(lambda p: Path(p).resolve(), (binary, fixture, evidence_path))
    spec = json.loads((HERE / 'fixture.json').read_text())
    module = importlib.util.spec_from_file_location('fixture', HERE / 'fetch-fixture.py')
    fetch = importlib.util.module_from_spec(module)
    module.loader.exec_module(fetch)
    fetch.verify(fixture, spec)
    target = {'Linux': 'linux', 'Darwin': 'darwin', 'Windows': 'windows'}[platform.system()]
    arch = {'x86_64': 'amd64', 'AMD64': 'amd64', 'arm64': 'arm64', 'aarch64': 'arm64'}[platform.machine()]
    asset = f'runner-{target}-{arch}'
    require(binary.name == asset + ('.exe' if target == 'windows' else ''), 'native target mismatch')
    metadata = json.loads(binary.with_name(asset + '.json').read_text())
    require(metadata['target'] == f'{target}/{arch}' and metadata['upstream_commit'] == SOURCE,
            'metadata source/target mismatch')
    require(metadata['runner_sha256'] == digest(binary), 'gated executable digest mismatch')
    evidence = {'schema': 'bashy-runner-native-v1', 'status': 'FAIL', 'target': f'{target}/{arch}',
                'runner_sha256': digest(binary), 'fixture': spec, 'upstream_commit': SOURCE,
                'platform': platform.platform(), 'checks': {},
                'network': {'linux': 'netns+strace', 'darwin': 'sandbox-exec+denial-controls',
                            'windows': 'defender-firewall+denial-controls'}[target]}
    logpath = evidence_path.with_suffix('.log')
    proc = None
    try:
        # No SO_REUSEADDR: occupied/wildcard endpoints fail before starting.
        with socket.socket() as bind_check:
            bind_check.bind(('127.0.0.1', port))
        with tempfile.TemporaryDirectory() as work, open(logpath, 'w') as log:
            argv = [str(binary), '--model', str(fixture), '--port', str(port)]
            # Preserve only OS loader essentials. No proxy/telemetry/model host env.
            env = {key: os.environ[key] for key in ('PATH', 'SystemRoot', 'WINDIR', 'TEMP', 'TMP',
                                                    'TMPDIR', 'HOME') if key in os.environ}
            proc = subprocess.Popen(argv, stdout=log, stderr=log, stdin=subprocess.PIPE,
                                    env=env, start_new_session=os.name != 'nt')
            evidence['checks']['launched_health'] = wait_health(proc, port, 2)
            evidence['pid'] = proc.pid
            pid = proc.pid
            evidence['checks']['bind_owner'] = check_owner(pid, port)
            loaded = api(port, '/load', {'Operation': 2, 'Parallel': 1, 'BatchSize': 64,
                         'KvSize': 512, 'KvCacheType': 'f16', 'NumThreads': 2,
                         'GPULayers': [], 'UseMmap': False})
            require(loaded.get('Success') is True, 'model load rejected')
            evidence['checks']['model_load'] = loaded
            evidence['checks']['ready_health'] = wait_health(proc, port, 0)
            evidence['checks']['streaming_completion'] = stream(port)
            embedding = api(port, '/embedding', {'content': 'Once upon a time'})
            validate_embedding(embedding)
            evidence['checks']['embedding'] = embedding
            canceled = stream(port, cancel=True)
            started = time.monotonic()
            canceled['slot_reuse'] = stream(port)
            canceled['seconds_to_reuse'] = time.monotonic() - started
            require(canceled['seconds_to_reuse'] < 15, 'cancellation failed to free single slot')
            evidence['checks']['cancellation_slot_reuse'] = canceled
            require(proc.poll() is None, 'runner died during protocol checks')
            if os.name == 'nt':
                proc.stdin.write(b'stop\n')
                proc.stdin.flush()
            else:
                proc.terminate()
            result = proc.wait(timeout=10)
            require(result == (0 if os.name == 'nt' else -signal.SIGTERM),
                    f'unexpected shutdown status {result}')
            with socket.socket() as probe:
                probe.settimeout(1)
                require(probe.connect_ex(('127.0.0.1', port)) != 0, 'listener survived shutdown')
            if os.name != 'nt':
                try:
                    os.killpg(proc.pid, 0)
                except ProcessLookupError:
                    pass
                else:
                    raise RuntimeError('runner descendants survived shutdown')
            evidence['checks']['shutdown'] = {'returncode': result, 'listener_closed': True,
                                             'process_waited': True}
            require(digest(binary) == evidence['runner_sha256'], 'executable changed during gate')
            evidence['status'] = 'PROTOCOL_PASS'  # shell must finalize network verdict
    except Exception as exc:
        evidence['error'] = str(exc)
        raise
    finally:
        if proc is not None:
            if os.name != 'nt':
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            if proc.poll() is None:
                if os.name == 'nt':
                    try:
                        proc.stdin.write(b'stop\n')
                        proc.stdin.flush()
                        proc.wait(timeout=5)
                    except (OSError, subprocess.TimeoutExpired):
                        proc.kill()
                proc.wait(timeout=10)
            proc.stdin.close()
        evidence_path.write_text(json.dumps(evidence, indent=2) + '\n')
        if logpath.exists():
            print(logpath.read_text(errors='replace'), flush=True)


def finalize(path, network_log):
    evidence = json.loads(Path(path).read_text())
    require(evidence['status'] == 'PROTOCOL_PASS', 'protocol did not pass')
    require(set(evidence['checks']) == set(CHECKS), 'missing protocol evidence')
    require(Path(network_log).stat().st_size > 0, 'missing network evidence')
    evidence['network_evidence_sha256'] = digest(network_log)
    evidence['status'] = 'PASS'
    Path(path).write_text(json.dumps(evidence, indent=2) + '\n')


if __name__ == '__main__':
    if sys.argv[1:2] == ['--deny-controls']:
        deny_controls(int(sys.argv[2]))
    elif sys.argv[1:2] == ['--finalize']:
        finalize(*sys.argv[2:])
    elif len(sys.argv) == 5:
        exercise(sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4]))
    else:
        raise SystemExit('usage: native-gate.py BINARY FIXTURE EVIDENCE PORT')

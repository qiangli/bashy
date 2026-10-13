#!/usr/bin/env python3
"""Bounded inference gate; diagnostics never turn a failed rank into success."""
import os
import pathlib
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time

LOG_LIMIT = 1024 * 1024
EXCERPT = 32768
_CHUNK = 65536


def dump_log(path):
    """Keep head and tail, with an explicit truncation marker and byte count."""
    with open(path, 'rb') as log:
        size = os.fstat(log.fileno()).st_size
        head = log.read(EXCERPT)
        print(head.decode('utf-8', errors='replace'), flush=True)
        if size > EXCERPT:
            if size > 2 * EXCERPT:
                print(f'... omitted {size - 2 * EXCERPT} bytes (total={size}) ...', flush=True)
            log.seek(max(EXCERPT, size - EXCERPT))
            print(log.read(EXCERPT).decode('utf-8', errors='replace'), flush=True)


def _drain(label, pipe, path, stats, lock, kill):
    """Copy one rank's piped stdout to path, keeping at most LOG_LIMIT bytes.

    Only captured stdout is bounded. The engine process itself runs with no
    file-size limit, so its own files (Metal cache, model writes) are
    unconstrained. On excess the rank is killed and the stage fails closed.
    """
    observed = 0
    written = 0
    error = None
    try:
        # Unbuffered: the pre-cleanup snapshot reads this file through a new
        # descriptor, so drained bytes must reach disk immediately, not sit
        # in this writer's buffer until the rank exits.
        with open(path, 'wb', buffering=0) as log:
            while True:
                # read1: a single underlying read. Plain read(n) may wait
                # for more bytes than the child has written, which would
                # stall the drain (and the warmup/evaluation evidence with
                # it) until the rank exits or is killed.
                chunk = pipe.read1(_CHUNK)
                if not chunk:
                    break
                observed += len(chunk)
                with lock:
                    stats[label]['observed'] = observed
                if written < LOG_LIMIT:
                    keep = chunk[:LOG_LIMIT - written]
                    log.write(keep)
                    written += len(keep)
                if observed > LOG_LIMIT:
                    with lock:
                        if not stats[label]['exceeded']:
                            stats[label]['exceeded'] = True
                            kill()
                        stats[label]['observed'] = observed
                # Past the limit the rest is discarded so the child never
                # blocks on a full pipe while the killer acts.
    except Exception as exc:  # a dead pump must never read as success
        error = str(exc)
    finally:
        try:
            pipe.close()
        except OSError:
            pass
        with lock:
            stats[label]['observed'] = observed
            if error is not None:
                stats[label]['pump_error'] = error


def _kill_peer(peer):
    """Terminate one rank's process group (POSIX) or process (Windows).

    os.killpg does not exist on Windows; the exercise never spawns
    grandchildren, so terminating the process itself is equivalent there.
    """
    killpg = getattr(os, 'killpg', None)
    try:
        if killpg is None:
            peer.kill()
        else:
            killpg(peer.pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError, OSError):
        pass  # already exited; the monitor below still fails closed


def _over_limit(peers, stats, lock):
    with lock:
        return [label for label, _, _, _ in peers if stats[label]['exceeded']]


def _log_bound_addresses():
    """Best-effort bound-socket snapshot for Windows gate diagnostics.

    Runs BEFORE hung peers are reaped, so a stalled rank's listeners are
    still visible, and names IPv4 loopback vs IPv6 binds for the next gate
    failure. Windows only; a no-op elsewhere. Never fails the stage:
    diagnostics must not become a second gate.
    """
    if os.name != 'nt':
        return
    try:
        viewer = shutil.which('netstat')
        if viewer is None:
            return
        probe = subprocess.run([viewer, '-ano', '-p', 'TCP'], capture_output=True,
                               text=True, timeout=10, errors='replace')
        for line in (probe.stdout or '').splitlines():
            lower = line.lower()
            if '127.0.0.1' in lower or '::1' in lower or \
                    '0.0.0.0' in lower or '[::]' in lower or 'listening' in lower:
                print(f'bound: {line.strip()}', flush=True)
    except Exception as exc:
        print(f'bound: snapshot unavailable ({exc})', flush=True)


def run_stage(name, commands, timeout, evaluation=None):
    with tempfile.TemporaryDirectory() as directory:
        peers = []
        stats = {}
        lock = threading.Lock()
        started = time.monotonic()
        failure = None
        try:
            for label, command in commands:
                path = pathlib.Path(directory) / f'{label}.log'
                stats[label] = {'exceeded': False, 'observed': 0}
                peer = subprocess.Popen(command, stdout=subprocess.PIPE,
                                        stderr=subprocess.STDOUT,
                                        start_new_session=True)

                def kill(peer=peer):
                    _kill_peer(peer)

                pump = threading.Thread(target=_drain,
                                        args=(label, peer.stdout, path, stats, lock, kill),
                                        daemon=True)
                peers.append((label, peer, path, pump))
                pump.start()
                print(f'stage={name} rank={label} pid={peer.pid} started', flush=True)
            while True:
                statuses = [peer.poll() for _, peer, _, _ in peers]
                over = _over_limit(peers, stats, lock)
                if over:
                    raise RuntimeError(
                        f'{name}: log limit exceeded ({LOG_LIMIT} bytes): ranks={over} '
                        f'statuses={statuses}')
                if any(code is not None and code != 0 for code in statuses):
                    raise RuntimeError(f'{name}: child failed: {statuses}')
                if all(code == 0 for code in statuses):
                    break
                if time.monotonic() - started >= timeout:
                    raise TimeoutError(f'{name}: deadline={timeout}s statuses={statuses}')
                time.sleep(0.05)
            # Ranks exited; let the drains reach EOF so a burst that outran
            # the last poll is still enforced before success is declared.
            for _, _, _, pump in peers:
                pump.join(timeout=5)
            over = _over_limit(peers, stats, lock)
            if over:
                raise RuntimeError(
                    f'{name}: log limit exceeded ({LOG_LIMIT} bytes): ranks={over}')
            errors = {label: stats[label]['pump_error'] for label, _, _, _ in peers
                      if stats[label].get('pump_error')}
            if errors:
                raise RuntimeError(f'{name}: log capture failed: {errors}')
            if evaluation is not None:
                log = next(path for label, _, path, _ in peers if label == evaluation)
                if 'eval time' not in log.read_text(errors='replace'):
                    raise RuntimeError(f'{name}: master did not evaluate')
        except BaseException as error:
            failure = str(error)
            raise
        finally:
            # Snapshot both ranks BEFORE killing either; don't mistake cleanup
            # SIGKILL for a native failure or infer master success from its log.
            print(f'stage={name} elapsed={time.monotonic()-started:.3f}s '
                  f'result={failure or "completed"}', flush=True)
            for label, peer, path, _ in peers:
                with lock:
                    snap = dict(stats[label])
                try:
                    text = path.read_text(errors='replace')
                except FileNotFoundError:
                    text = ''
                print(f'rank={label} pid={peer.pid} pre_cleanup_status={peer.poll()} '
                      f'warmup={"warming up" in text} evaluation={"eval time" in text} '
                      f'log_limit_exceeded={snap["exceeded"]} log_bytes={snap["observed"]}',
                      flush=True)
            # Hung peers are still bound here; snapshot before the kill loop
            # below reaps them. Successful stages stay quiet.
            if failure is not None:
                _log_bound_addresses()
            for label, peer, path, pump in peers:
                if peer.poll() is None:
                    _kill_peer(peer) # exited between poll and kill; still reap below
                peer.wait(timeout=5)
                pump.join(timeout=5)
                print(f'---- rank={label} final_status={peer.returncode} log ----', flush=True)
                try:
                    dump_log(path)
                except FileNotFoundError:
                    print(f'(no log captured for rank={label})', flush=True)
        print(f'PASS: {name}', flush=True)


def exercise(binary, model, ring):
    binary, model, ring = (str(pathlib.Path(x).resolve()) for x in (binary, model, ring))
    args = [binary, '-m', model, '-p', 'a', '-n', '2', '-c', '64',
            '-t', '2', '--seed', '1', '--ignore-eos', '-ngl', '0']
    run_stage('nng loopback multipart and timeout', [('ring', [ring])], 30)
    run_stage('model load + prompt evaluation + generation', [('single', args)], 90, 'single')
    ranks = []
    for rank in (1, 0):
        print(f'rank={rank} configured_endpoints: data_bind=tcp://*:{19000+rank} '
              f'data_next=tcp://127.0.0.1:{19000+(1-rank)} '
              f'signal_bind=tcp://*:{19100+rank} '
              f'signal_next=tcp://127.0.0.1:{19100+(1-rank)} '
              'master=tcp://127.0.0.1:19000 world=2 layers=1,1', flush=True)
        ranks.append((str(rank), args + ['--world', '2', '--rank', str(rank),
                      '--layer-window', '1,1', '--master-ip', '127.0.0.1',
                      '--next-node-ip', '127.0.0.1', '--data-port', '19000',
                      '--signal-port', '19100']))
    run_stage('two-rank loopback engine inference with explicit 1,1 layer split',
              ranks, 90, '0')


if __name__ == '__main__':
    if len(sys.argv) == 3 and sys.argv[1] == '--trace':
        dump_log(sys.argv[2])
    else:
        exercise(*sys.argv[1:])

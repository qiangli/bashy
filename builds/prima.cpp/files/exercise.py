#!/usr/bin/env python3
"""Bounded inference gate; diagnostics never turn a failed rank into success."""
import os
import pathlib
import resource
import signal
import subprocess
import sys
import tempfile
import time

LOG_LIMIT = 1024 * 1024
EXCERPT = 32768


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


def limit_log():
    # Native Unix lanes only. A runaway child fails (SIGXFSZ), never fills disk.
    resource.setrlimit(resource.RLIMIT_FSIZE, (LOG_LIMIT, LOG_LIMIT))


def run_stage(name, commands, timeout, evaluation=None):
    with tempfile.TemporaryDirectory() as directory:
        peers = []
        started = time.monotonic()
        failure = None
        try:
            for label, command in commands:
                path = pathlib.Path(directory) / f'{label}.log'
                with path.open('wb') as log:
                    peer = subprocess.Popen(command, stdout=log, stderr=subprocess.STDOUT,
                                            start_new_session=True, preexec_fn=limit_log)
                peers.append((label, peer, path))
                print(f'stage={name} rank={label} pid={peer.pid} started', flush=True)
            while True:
                statuses = [peer.poll() for _, peer, _ in peers]
                if any(code is not None and code != 0 for code in statuses):
                    raise RuntimeError(f'{name}: child failed: {statuses}')
                if all(code == 0 for code in statuses):
                    break
                if time.monotonic() - started >= timeout:
                    raise TimeoutError(f'{name}: deadline={timeout}s statuses={statuses}')
                time.sleep(0.05)
            if evaluation is not None:
                log = next(path for label, _, path in peers if label == evaluation)
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
            for label, peer, path in peers:
                text = path.read_text(errors='replace')
                print(f'rank={label} pid={peer.pid} pre_cleanup_status={peer.poll()} '
                      f'warmup={"warming up" in text} evaluation={"eval time" in text}', flush=True)
            for label, peer, path in peers:
                if peer.poll() is None:
                    try:
                        os.killpg(peer.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass # exited between poll and kill; still reap below
                peer.wait(timeout=5)
                print(f'---- rank={label} final_status={peer.returncode} log ----', flush=True)
                dump_log(path)
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

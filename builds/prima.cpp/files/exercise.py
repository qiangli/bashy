#!/usr/bin/env python3
"""Bounded single-node inference plus the pinned nng multipart/timeout test."""
import pathlib
import subprocess
import sys

binary, model, ring = map(lambda x: str(pathlib.Path(x).resolve()), sys.argv[1:])
subprocess.run([ring], check=True, timeout=30)
result = subprocess.run([binary, '-m', model, '-p', 'a', '-n', '2', '-c', '64',
                         '-t', '2', '--seed', '1', '--ignore-eos', '-ngl', '0'],
                        capture_output=True, text=True, timeout=90)
print(result.stdout)
print(result.stderr)
assert result.returncode == 0, 'inference failed'
assert 'eval time' in result.stderr, 'no evaluation evidence'
print('PASS: model load + prompt evaluation + generation; nng loopback multipart and timeout')

# Exercise the shipped engine's two-rank path, not only the transport shim.
import tempfile
with tempfile.TemporaryDirectory() as d:
    logs=[]
    peers=[]
    try:
        for rank in [1,0]:
            log=open(pathlib.Path(d)/f'rank{rank}.log','w+')
            logs.append(log)
            peers.append(subprocess.Popen([binary,'-m',model,'-p','a','-n','2','-c','64',
                          '-t','2','--seed','1','--ignore-eos','-ngl','0',
                          '--world','2','--rank',str(rank),'--layer-window','1,1',
                          '--master-ip','127.0.0.1','--next-node-ip','127.0.0.1',
                          '--data-port','19000','--signal-port','19100'],
                          stdout=log,stderr=subprocess.STDOUT))
        for peer in peers:
            assert peer.wait(timeout=90)==0, 'two-rank inference failed'
        logs[1].seek(0)
        assert 'eval time' in logs[1].read(), 'master did not evaluate'
        print('PASS: two-rank loopback engine inference with explicit 1,1 layer split')
    finally:
        for peer in peers:
            if peer.poll() is None: peer.kill()
            peer.wait()
        for log in logs:
            log.seek(0)
            print(log.read())
            log.close()

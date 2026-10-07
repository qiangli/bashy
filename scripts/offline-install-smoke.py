#!/usr/bin/env python3
"""Run inside OS network isolation; never modifies host firewall or services."""
import argparse
import json
import os
import pathlib
import re
import socket
import subprocess
import time
import urllib.request

p = argparse.ArgumentParser()
p.add_argument('source', type=pathlib.Path)
p.add_argument('destination', type=pathlib.Path)
p.add_argument('work', type=pathlib.Path)
p.add_argument('repo', type=pathlib.Path)
a = p.parse_args()
for directory in [a.destination, a.work]:
    directory.mkdir(parents=True, exist_ok=True)
env = dict(os.environ, BASHY_OFFLINE='1', BASHY_BIN_CACHE=str(a.work / 'cache'),
           HOME=str(a.work), USERPROFILE=str(a.work), XDG_CONFIG_HOME=str(a.work / 'config'),
           XDG_CACHE_HOME=str(a.work / 'cache'), HTTP_PROXY='', HTTPS_PROXY='', ALL_PROXY='', NO_PROXY='*')
# The test cannot accidentally rely on Go installed on a hosted runner.
env['PATH'] = os.pathsep.join(x for x in env.get('PATH', '').split(os.pathsep) if not re.search(r'(^|[/\\])go([/\\]|$)', x, re.I))
os.environ.update(env)
local_http = urllib.request.build_opener(urllib.request.ProxyHandler({}))
suffix = '.exe' if os.name == 'nt' else ''
def run(args, check=True, overrides=None):
    return subprocess.run([str(x) for x in args], env=env | (overrides or {}), text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=90, check=check)
bashy = a.source / ('bashy' + suffix)
# Exercise a real network operation with the application offline guard disabled:
# the OS isolation, rather than BASHY_OFFLINE itself, must deny acquisition.
blocked = run([bashy, 'fetch', '--timeout', '5s', 'https://github.com'], check=False, overrides={'BASHY_OFFLINE': '0'})
if blocked.returncode == 0:
    raise SystemExit('network isolation ineffective: candidate fetched external content')
denial = re.compile(r'network is unreachable|operation not permitted|permission denied|connectex|i/o timeout|context deadline exceeded|no route to host|connection.*(?:timed out|refused)|lookup.*(?:denied|timeout|no such host)', re.I)
if not denial.search(blocked.stdout + blocked.stderr):
    raise SystemExit('external fetch failed without a recognizable network denial: ' + blocked.stderr)
run([bashy, 'self', 'install', '--dir', a.destination])
bashy, outpost = (a.destination / (name + suffix) for name in ('bashy', 'outpost'))
for name in ('bashy', 'outpost', 'bash', 'sh'):
    if not (a.destination / (name + suffix)).is_file():
        raise SystemExit('missing installed member: ' + name)
banner = run([bashy, '--version']); version = json.loads(run([outpost, 'version', '--json']).stdout)['version']
match = re.search(r'bashy-(v?\d+\.\d+\.\d+(?:-dev)?)', banner.stdout + banner.stderr)
normalize = lambda x: x.removeprefix('v').removesuffix('-dev')
if not match or normalize(match[1]) != normalize(version):
    raise SystemExit('installed versions differ')
run([bashy, 'outpost', 'version', '--json'])
run([outpost, 'bashy', '--version'])
if run([bashy, '-c', 'printf offline | cat']).stdout != 'offline':
    raise SystemExit('bundled coreutils smoke failed')
actual = run([bashy, '--bashsharp', a.repo / 'examples/quickstart/kwargs.bsh']).stdout
if actual != (a.repo / 'examples/quickstart/kwargs.expected').read_text():
    raise SystemExit('offline Bash# output differs')
children = []
logs = []
try:
    for args, name in [([outpost, 'start', '--admin-addr', '127.0.0.1:18777'], 'daemon'),
                       ([outpost, 'sshd', '--addr', '127.0.0.1:18222', '--no-mdns'], 'sshd')]:
        log = open(a.work / (name + '.log'), 'w'); logs.append(log)
        children.append(subprocess.Popen([str(x) for x in args], env=env, stdout=log, stderr=log))
    for _ in range(60):
        if any(child.poll() is not None for child in children):
            raise SystemExit('unpaired process exited; see work logs')
        try:
            with local_http.open('http://127.0.0.1:18777/healthz', timeout=1) as response:
                if response.status != 200:
                    raise OSError('health failed')
            with socket.create_connection(('127.0.0.1', 18222), timeout=1) as connection:
                if not connection.recv(256).startswith(b'SSH-'):
                    raise OSError('SSH banner missing')
            break
        except OSError:
            time.sleep(0.5)
    else:
        raise SystemExit('unpaired health/sshd timeout; see work logs')
finally:
    for child in children:
        child.terminate()
    for child in children:
        try:
            child.wait(timeout=10)
        except subprocess.TimeoutExpired:
            child.kill(); child.wait()
    for log in logs:
        log.close()
print('PASS: OS-isolated install, four members, paired resolver/version, coreutils, Bash#, unpaired health and sshd')

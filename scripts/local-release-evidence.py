#!/usr/bin/env python3
"""Collect or verify the security receipt for one frozen release commit."""
import argparse
import datetime as dt
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys

SCHEMA = 'bashy-local-gate-v1'

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()

def objects(text):
    decoder, pos = json.JSONDecoder(), 0
    while pos < len(text):
        while pos < len(text) and text[pos].isspace():
            pos += 1
        if pos == len(text):
            return
        value, pos = decoder.raw_decode(text, pos)
        yield value

VULNDB_INDEX = os.environ.get('GOVULNDB_INDEX', 'https://vuln.go.dev/index/db.json')


def upstream_database_modified():
    """The modification time the vulnerability database upstream serves right now,
    or None when it cannot be fetched (the seven-day rule then applies alone)."""
    import urllib.request
    try:
        with urllib.request.urlopen(VULNDB_INDEX, timeout=20) as response:
            return timestamp(json.loads(response.read())['modified'])
    except Exception:
        return None


def timestamp(value):
    parsed = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
    if parsed.tzinfo is None:
        raise ValueError('timestamp must have a timezone')
    return parsed.astimezone(dt.timezone.utc)

def verify(record, root, commit, now=None):
    now = now or dt.datetime.now(dt.timezone.utc)
    if record.get('schema') != SCHEMA or record.get('commit') != commit or not re.fullmatch('[0-9a-f]{40}', commit):
        raise ValueError('receipt does not name the exact source commit')
    for name in ('go.mod', 'go.sum', 'tools/bashy-build/image.lock'):
        if record.get('locks', {}).get(name) != digest(root / name):
            raise ValueError('frozen source lock differs: ' + name)
    for gate in ('license', 'govulncheck'):
        result = record.get(gate, {})
        if result.get('verdict') != 'pass' or not result.get('reports'):
            raise ValueError(gate + ' needs passing retained reports')
        if any(not re.fullmatch('[0-9a-f]{64}', v) for v in result['reports'].values()):
            raise ValueError('invalid report digest')
    date = timestamp(record['govulncheck']['database_modified'])
    if date > now:
        raise ValueError('vulnerability database modification time is in the future')
    # Fresh means the scan used the database upstream serves NOW. Its modification
    # time is only a proxy: vuln.go.dev can go quiet for weeks (2026-11-11 was
    # still current on 2026-11-23). Accept a database at most seven days old, or
    # one proven current by the upstream index time recorded at collect time.
    upstream = record['govulncheck'].get('database_upstream_modified')
    if now - date > dt.timedelta(days=7) and (upstream is None or timestamp(upstream) != date):
        raise ValueError('vulnerability database must be no older than seven days or match the upstream index recorded at collect time')
    generated = timestamp(record['generated_at'])
    if generated > now + dt.timedelta(minutes=5) or generated < date:
        raise ValueError('invalid receipt generation time')
    graphs = record.get('module_graphs')
    if not isinstance(graphs, list) or len(graphs) < 3:
        raise ValueError('missing shipped module graphs')
    if record.get('module_graphs_sha256') != hashlib.sha256(canonical(graphs)).hexdigest():
        raise ValueError('module graphs digest differs')
    required = {'github.com/qiangli/bashy', 'github.com/qiangli/outpost', 'github.com/qiangli/yoke'}
    if {g.get('module') for g in graphs} != required:
        raise ValueError('shipped module graph scope differs')
    for graph in graphs:
        if not graph.get('modules') or not graph.get('locks'):
            raise ValueError('incomplete transitive module locks')
        for module in graph['modules']:
            if not module.get('main') and not module.get('version') and not module.get('replacement'):
                raise ValueError('unversioned dependency')
            downloaded = module.get('go_mod_downloaded', True)
            if not isinstance(downloaded, bool):
                raise ValueError('invalid go.mod download state')
            if not downloaded:
                if module.get('main'):
                    raise ValueError('main module missing go.mod content digest')
                if 'go_mod_sha256' in module:
                    raise ValueError('unexpected go.mod content digest for un-downloaded module')
                sum_value = module.get('go_mod_sum')
                if module.get('go_mod_pruned', False) is True:
                    if sum_value or module.get('sum') or 'go_sum_sha256' in module:
                        raise ValueError('pruned module carries lock data')
                elif not isinstance(sum_value, str) or not sum_value.startswith('h1:') or len(sum_value) <= len('h1:'):
                    raise ValueError('un-downloaded module missing h1: go.mod sum')
                continue
            if 'go_mod_pruned' in module:
                raise ValueError('downloaded module marked pruned')
            digest_value = module.get('go_mod_sha256', '')
            if not isinstance(digest_value, str) or not re.fullmatch('[0-9a-f]{64}', digest_value):
                raise ValueError('missing transitive go.mod content digest')
    return record

def collect(root, report_dir, scopes):
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=root, text=True).strip():
        raise ValueError('candidate has uncommitted tracked source changes')
    graphs = []
    go_env = dict(os.environ, GOWORK='off')
    for scope in scopes:
        scope = scope.resolve()
        raw = subprocess.check_output(['go', 'list', '-mod=readonly', '-m', '-json', 'all'], cwd=scope, env=go_env, text=True)
        modules, main = [], None
        for m in objects(raw):
            effective = m.get('Replace', m)
            gomod_path = effective.get('GoMod')
            gomod = pathlib.Path(gomod_path) if gomod_path else None
            item = {'path': m['Path'], 'version': m.get('Version', ''), 'main': m.get('Main', False),
                    'sum': m.get('Sum', ''), 'go_mod_sum': m.get('GoModSum', '')}
            if gomod is not None:
                item['go_mod_sha256'] = digest(gomod)
            else:
                item['go_mod_downloaded'] = False
                if not item['go_mod_sum']:
                    # `go list -m all` enumerates the full graph, but under module
                    # pruning a module no build needs has no go.sum line at all and
                    # is never downloaded; record it as pruned instead of pretending
                    # it has a lock. go verifies that such a module cannot reach a
                    # build; the receipt only records that fact.
                    item['go_mod_pruned'] = True
            module_dir = pathlib.Path(effective['Dir']) if effective.get('Dir') else (gomod.parent if gomod is not None else None)
            if module_dir is not None and (module_dir / 'go.sum').is_file():
                item['go_sum_sha256'] = digest(module_dir / 'go.sum')
            if 'Replace' in m:
                item['replacement'] = {k: effective[k] for k in ('Path', 'Version', 'Sum', 'GoModSum') if k in effective}
            if m.get('Main'):
                main = m['Path']
            modules.append(item)
        locks = {}
        # Capture nested module locks too: replacing a parent module must not
        # silently leave the scanned command/nested module's locks unbound.
        for path in sorted(scope.rglob('go.*')):
            if path.name in ('go.mod', 'go.sum') and '.git' not in path.parts and path.is_file():
                locks[str(path.relative_to(scope))] = digest(path)
        graphs.append({'module': main, 'locks': locks, 'modules': sorted(modules, key=lambda m: m['path'])})
    graphs.sort(key=lambda g: g['module'])
    reports = sorted((report_dir / 'govulncheck').glob('*.json'))
    dates = []
    for path in reports:
        configs = [o['config'] for o in objects(path.read_text()) if 'config' in o]
        if len(configs) != 1:
            raise ValueError('missing scanner configuration: ' + path.name)
        dates.append(timestamp(configs[0]['db_last_modified']))
    if not dates:
        raise ValueError('no vulnerability scanner reports')
    license_reports = sorted((report_dir / 'license').glob('*'))
    now = dt.datetime.now(dt.timezone.utc)
    govulncheck = {'verdict': 'pass', 'database_modified': min(dates).isoformat(),
                   'reports': {p.name: digest(p) for p in reports}}
    upstream = upstream_database_modified()
    if upstream is not None:
        govulncheck['database_upstream_modified'] = upstream.isoformat()
    record = {'schema': SCHEMA, 'commit': commit, 'generated_at': now.isoformat(),
              'locks': {n: digest(root / n) for n in ('go.mod', 'go.sum', 'tools/bashy-build/image.lock')},
              'license': {'verdict': 'pass', 'reports': {p.name: digest(p) for p in license_reports if p.is_file()}},
              'govulncheck': govulncheck,
              'module_graphs': graphs, 'module_graphs_sha256': hashlib.sha256(canonical(graphs)).hexdigest()}
    return verify(record, root, commit, now)

def verify_distribution(record, directory):
    distribution = record.get('distribution', {})
    if distribution.get('schema') != 'bashy-dist-v1' or distribution.get('source_sha') != record['commit']:
        raise ValueError('distribution is not bound to the frozen source')
    assets = distribution.get('assets', [])
    names = [a['name'] for a in assets]
    if not names or len(set(names)) != len(names) or any(pathlib.Path(n).name != n for n in names):
        raise ValueError('invalid distribution asset names')
    if set(names) != {p.name for p in directory.iterdir() if p.is_file()}:
        raise ValueError('distribution asset set differs')
    for asset in assets:
        path = directory / asset['name']
        if digest(path) != asset['sha256'] or path.stat().st_size != asset['size']:
            raise ValueError('distribution digest differs: ' + asset['name'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('collect', 'verify'))
    parser.add_argument('receipt', type=pathlib.Path)
    parser.add_argument('--root', type=pathlib.Path, default=pathlib.Path('.'))
    parser.add_argument('--commit')
    parser.add_argument('--reports', type=pathlib.Path)
    parser.add_argument('--scope', type=pathlib.Path, action='append')
    parser.add_argument('--dist-manifest', type=pathlib.Path)
    parser.add_argument('--dist', type=pathlib.Path)
    a = parser.parse_args()
    if a.action == 'collect':
        if not a.reports or not a.scope:
            parser.error('collect requires --reports and --scope')
        record = collect(a.root.resolve(), a.reports, a.scope)
        if a.dist_manifest:
            record['distribution'] = json.loads(a.dist_manifest.read_text())
        a.receipt.write_text(json.dumps(record, indent=2, sort_keys=True) + '\n')
    else:
        record = verify(json.loads(a.receipt.read_text()), a.root, a.commit or '')
        if a.dist:
            verify_distribution(record, a.dist)
    print('local release security receipt: PASS')

if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as err:
        sys.exit('local release security receipt: ' + str(err))

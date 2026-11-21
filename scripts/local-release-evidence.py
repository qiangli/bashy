#!/usr/bin/env python3
"""Collect or verify the security receipt for one frozen release commit."""
import argparse
import datetime as dt
import hashlib
import json
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
        verdicts = ('pass', 'override') if gate == 'govulncheck' else ('pass',)
        if result.get('verdict') not in verdicts or not result.get('reports'):
            raise ValueError(gate + ' needs passing retained reports')
        if any(not re.fullmatch('[0-9a-f]{64}', v) for v in result['reports'].values()):
            raise ValueError('invalid report digest')
    if record['govulncheck']['verdict'] == 'override':
        verify_override(record['govulncheck'].get('override'))
    date = timestamp(record['govulncheck']['database_modified'])
    if date > now or now - date > dt.timedelta(days=7):
        raise ValueError('vulnerability database must be no older than seven days and not in the future')
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
            if not re.fullmatch('[0-9a-f]{64}', module.get('go_mod_sha256', '')):
                raise ValueError('missing transitive go.mod content digest')
    return record

def verify_override(override):
    """An explicit operator override of reachable findings: who, why, which ids."""
    if not isinstance(override, dict):
        raise ValueError('vulnerability override record missing')
    for key in ('reason', 'by'):
        if not isinstance(override.get(key), str) or not override[key].strip():
            raise ValueError('vulnerability override needs a non-empty ' + key)
    ids = override.get('overridden')
    if not isinstance(ids, dict) or not ids or not all(isinstance(v, list) and v for v in ids.values()):
        raise ValueError('vulnerability override names no overridden findings')
    if any(not isinstance(i, str) or not re.fullmatch(r'GO-\d{4}-\d+', i) for v in ids.values() for i in v):
        raise ValueError('invalid overridden vulnerability id')
    if not re.fullmatch('[0-9a-f]{64}', override.get('sha256', '')):
        raise ValueError('vulnerability override digest missing')

def collect(root, report_dir, scopes):
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=root, text=True).strip():
        raise ValueError('candidate has uncommitted tracked source changes')
    graphs = []
    for scope in scopes:
        scope = scope.resolve()
        raw = subprocess.check_output(['go', 'list', '-mod=readonly', '-m', '-json', 'all'], cwd=scope, text=True)
        modules, main = [], None
        for m in objects(raw):
            effective = m.get('Replace', m)
            gomod = pathlib.Path(effective['GoMod'])
            item = {'path': m['Path'], 'version': m.get('Version', ''), 'main': m.get('Main', False),
                    'sum': m.get('Sum', ''), 'go_mod_sum': m.get('GoModSum', ''),
                    'go_mod_sha256': digest(gomod)}
            module_sum = pathlib.Path(effective.get('Dir', gomod.parent)) / 'go.sum'
            if module_sum.is_file():
                item['go_sum_sha256'] = digest(module_sum)
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
    record = {'schema': SCHEMA, 'commit': commit, 'generated_at': now.isoformat(),
              'locks': {n: digest(root / n) for n in ('go.mod', 'go.sum', 'tools/bashy-build/image.lock')},
              'license': {'verdict': 'pass', 'reports': {p.name: digest(p) for p in license_reports if p.is_file()}},
              'govulncheck': {'verdict': 'pass', 'database_modified': min(dates).isoformat(),
                             'reports': {p.name: digest(p) for p in reports}},
              'module_graphs': graphs, 'module_graphs_sha256': hashlib.sha256(canonical(graphs)).hexdigest()}
    override = report_dir / 'vuln-override.json'
    if override.is_file():
        record['govulncheck']['verdict'] = 'override'
        record['govulncheck']['override'] = dict(json.loads(override.read_text()), sha256=digest(override))
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
    override = record['govulncheck'].get('override') if record['govulncheck'].get('verdict') == 'override' else None
    if override:
        ids = sorted({i for v in override['overridden'].values() for i in v})
        print('local release security receipt: PASS with vulnerability OVERRIDE by ' + override['by'] + ': ' + override['reason'])
        print('overridden: ' + ' '.join(ids))
    else:
        print('local release security receipt: PASS')

if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as err:
        sys.exit('local release security receipt: ' + str(err))

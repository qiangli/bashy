#!/usr/bin/env python3
"""Generate the v1.0 command evidence table; --check fails on every open gap."""
import argparse
import hashlib
import json
import os
import platform
from pathlib import Path
import re
import subprocess
import sys
import tempfile

OSES = ('darwin', 'linux', 'windows')
DISPATCH_TEST = 'TestE2EAllListedCommandsDispatch'
ROOT = Path(__file__).resolve().parents[1]


def envelope_version(payload):
    if not isinstance(payload, dict):
        return None
    # Existing contracts use either spelling. Arrays and nested payload fields
    # do not establish an envelope version.
    for key in ('schema_version', 'schema'):
        value = payload.get(key)
        if isinstance(value, str) and re.search(r'(?:^|[-/])v[1-9][0-9]*$', value):
            return value
    return None


def dispatch_covered(name, catalog, entry, goos):
    # Mirrors the default catalog + hidden-verb loops in the named E2E test.
    if name in catalog.get('hidden_verbs', []):
        return True
    return (name in catalog.get('verbs', []) + catalog.get('coreutils', [])
            and goos in entry.get('os', []))


def dispatch_pass(records, candidate, goos):
    return any(r.get('candidate') == candidate and r.get('os') == goos
               and r.get('test') == DISPATCH_TEST and r.get('passed') is True
               and bool(r.get('run_url')) for r in records)


def gaps_for(row):
    gaps = []
    if row.get('stability') not in ('experimental', 'preview', 'supported'):
        gaps.append('stability: no declared release tier')
    if not row.get('consumer'):
        gaps.append('consumer: no named consumer recorded')
    if not envelope_version({'schema_version': row.get('json_schema')}):
        gaps.append('json_schema: no successful versioned JSON envelope probe')
    if not row.get('atlas_entry'):
        gaps.append('atlas: no complete entry in the live catalog')
    for goos in OSES:
        if not row.get('dispatch_coverage', {}).get(goos):
            gaps.append('dispatch_coverage: ' + goos)
        if not row.get('dispatch_pass', {}).get(goos):
            gaps.append('dispatch_pass: no same-candidate ' + goos + ' evidence')
    if row.get('scope_gap'):
        gaps.append('scope: ' + row['scope_gap'])
    return gaps


def invoke(binary, args, cwd, env):
    try:
        result = subprocess.run([binary, *args], cwd=cwd, env=env, capture_output=True,
                                text=True, timeout=30)
    except subprocess.TimeoutExpired:
        return None, 'timeout after 30s'
    # Do not persist stdout/stderr: probe output may contain host identity or
    # local catalog data. Only the envelope version and exit status are evidence.
    if result.returncode:
        return None, 'exit ' + str(result.returncode)
    try:
        return json.loads(result.stdout), None
    except ValueError:
        return None, 'stdout is not a single JSON value'


def generate(binary, manifest, candidate, records):
    binary = str(Path(binary).resolve())
    rows = []
    with tempfile.TemporaryDirectory(prefix='bashy-release-bar-') as scratch:
        env = dict(os.environ)
        # Isolate the catalog and all probe stores from the operator's session.
        for key in list(env):
            if key.startswith(('BASHY_', 'DHNT_', 'WEAVE_', 'CLAUDE_', 'CODEX_')):
                env.pop(key)
        env.update(HOME=scratch, USERPROFILE=scratch, XDG_CONFIG_HOME=scratch,
                   BASHY_TELEMETRY_QUIET='1', BASHY_AGENTIC='1')
        catalog, error = invoke(binary, ['commands', '--json', '--all'], scratch, env)
        if error or not isinstance(catalog, dict) or not catalog.get('verbs'):
            raise ValueError('commands catalog failed: ' + str(error))
        atlas, error = invoke(binary, ['commands', '--atlas', '--json', '--all'], scratch, env)
        if error or not isinstance(atlas, dict) or not atlas.get('commands'):
            raise ValueError('atlas catalog failed: ' + str(error))
        entries = {r['name']: r for r in atlas['commands']}
        for spec in manifest['commands']:
            name = spec['command']
            entry = entries.get(name, {})
            stability = spec.get('stability')
            if not stability and entry.get('status') == 'experimental':
                stability = 'experimental'
            probe = spec.get('json_probe')
            schema, probe_error = None, 'no safe JSON probe declared'
            if probe and entry:
                payload, probe_error = invoke(binary, [name, *probe], scratch, env)
                schema = envelope_version(payload)
                if not probe_error and not schema:
                    probe_error = 'JSON response has no top-level versioned schema'
            coverage = {goos: dispatch_covered(name, catalog, entry, goos) for goos in OSES}
            row = dict(command=name, section=spec['section'], stability=stability,
                       stability_source=spec.get('stability_source') or ('atlas.status' if stability else None),
                       execution_tier=entry.get('tier'), consumer=spec.get('consumer'),
                       consumer_ref=spec.get('consumer_ref'), json_schema=schema,
                       json_probe=probe, json_probe_error=probe_error,
                       atlas_entry=bool(entry and all(entry.get(k) for k in ('group', 'tier', 'sdlc', 'effects', 'os'))),
                       alias_of=entry.get('alias_of'), dispatch_coverage=coverage,
                       dispatch_pass={goos: coverage[goos] and dispatch_pass(records, candidate, goos) for goos in OSES},
                       scope_gap=spec.get('scope_gap'))
            row['gaps'] = gaps_for(row)
            rows.append(row)
    return dict(schema_version='bashy-release-bar-v1', release='1.0.0', candidate=candidate,
                probe_os={'Darwin': 'darwin', 'Linux': 'linux', 'Windows': 'windows'}[platform.system()],
                binary_sha256=hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                dispatch_test=DISPATCH_TEST, commands=rows,
                gap_count=sum(len(r['gaps']) for r in rows))


def markdown(report):
    def cell(value):
        return str(value or '—').replace('|', '\\|').replace('\n', ' ')
    def oses(values):
        return ', '.join(os for os in OSES if values[os]) or 'none'
    lines = ['<!-- Generated by scripts/release-bar.py; do not edit. -->',
             '# v1.0 command evidence', '',
             'Candidate: `' + report['candidate'] + '`. A covered dispatch is not a recorded pass.', '',
             '| Command | Part 4 | Stability | Named consumer | JSON envelope schema (probe) | Atlas | Dispatch coverage | Recorded pass |',
             '|---|---|---|---|---|---|---|---|']
    for r in report['commands']:
        schema = r['json_schema'] or 'missing'
        if r['json_probe']:
            schema += ' (`' + ' '.join([r['command'], *r['json_probe']]) + '`)'
        lines.append('| ' + ' | '.join(map(cell, [r['command'], r['section'], r['stability'], r['consumer'], schema,
                                                   'yes' if r['atlas_entry'] else 'missing',
                                                   oses(r['dispatch_coverage']), oses(r['dispatch_pass'])])) + ' |')
    lines += ['', '## Named gaps', '', '| Command | Owner | Missing evidence / decision |', '|---|---|---|']
    for r in report['commands']:
        if r['gaps']:
            lines.append('| ' + cell(r['command']) + ' | Sprint 379 / Story 1533 | ' + cell('; '.join(r['gaps'])) + ' |')
    return '\n'.join(lines) + '\n'


def record_dispatch(log, candidate, goos, run_url):
    events = [json.loads(line) for line in Path(log).read_text(encoding="utf-8").splitlines() if line.strip().startswith('{')]
    package = 'github.com/qiangli/bashy/internal/agentos'
    relevant = [e for e in events if e.get('Package') == package and e.get('Test') == DISPATCH_TEST]
    # A package failure (including compile failure) invalidates the run.
    passed = (any(e.get('Action') == 'pass' for e in relevant)
              and not any(e.get('Action') in ('fail', 'skip') for e in relevant)
              and any(e.get('Package') == package and 'Test' not in e and e.get('Action') == 'pass' for e in events)
              and not any(e.get('Action') == 'fail' for e in events))
    return dict(candidate=candidate, os=goos, test=DISPATCH_TEST, passed=passed, run_url=run_url)


def merge_reports(reports, records, candidate):
    if not reports or any(r.get('candidate') != candidate for r in reports):
        raise ValueError('missing reports or candidate mismatch')
    names = [r['command'] for r in reports[0]['commands']]
    if any([r['command'] for r in report['commands']] != names for report in reports):
        raise ValueError('release inventories differ across runners')
    by_os = {report.get('probe_os'): report for report in reports}
    if set(by_os) != set(OSES) or len(reports) != len(OSES):
        raise ValueError('exactly one report from each OS is required')
    result = json.loads(json.dumps(reports[0]))
    result['probe_os'] = 'three-os'
    result.pop('binary_sha256', None)
    result['binary_sha256_by_os'] = {os: by_os[os]['binary_sha256'] for os in OSES}
    for i, row in enumerate(result['commands']):
        peers = [by_os[os]['commands'][i] for os in OSES]
        # All three runtimes must supply the same versioned envelope. Never
        # let a successful Linux sample conceal a Windows-only failure.
        if not row['json_schema'] or any(p['json_schema'] != row['json_schema'] for p in peers):
            row['json_schema'] = None
            row['json_probe_error'] = 'no common successful envelope across three OSes'
        for field in ('stability', 'consumer'):
            if any(p[field] != row[field] for p in peers):
                row[field] = None
        row['atlas_entry'] = all(p['atlas_entry'] for p in peers)
        row['dispatch_coverage'] = {os: by_os[os]['commands'][i]['dispatch_coverage'][os] for os in OSES}
        row['dispatch_pass'] = {os: row['dispatch_coverage'][os] and dispatch_pass(records, candidate, os) for os in OSES}
        row['gaps'] = gaps_for(row)
    result['gap_count'] = sum(len(r['gaps']) for r in result['commands'])
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bashy')
    p.add_argument('--merge-report', type=Path, action='append', default=[])
    p.add_argument('--manifest', type=Path, default=ROOT / 'docs/release-bar-v1.json')
    p.add_argument('--candidate', required=True)
    p.add_argument('--dispatch-evidence', type=Path, action='append', default=[])
    p.add_argument('--json-out', type=Path)
    p.add_argument('--markdown-out', type=Path)
    p.add_argument('--check', action='store_true')
    p.add_argument('--record-dispatch', type=Path, help='go test -json log; emit a candidate/OS attestation')
    p.add_argument('--os', choices=OSES)
    p.add_argument('--run-url')
    args = p.parse_args()
    if args.record_dispatch:
        if not args.os or not args.run_url:
            p.error('--record-dispatch requires --os and --run-url')
        record = record_dispatch(args.record_dispatch, args.candidate, args.os, args.run_url)
        output = json.dumps(record, indent=2) + '\n'
        if args.json_out:
            args.json_out.write_text(output, encoding="utf-8")
        else:
            print(output, end='')
        return 0 if record['passed'] else 1
    if not args.bashy and not args.merge_report:
        p.error('--bashy is required when generating evidence')
    records = []
    for path in args.dispatch_evidence:
        value = json.loads(path.read_text(encoding="utf-8"))
        records.extend(value if isinstance(value, list) else [value])
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    names = [r['command'] for r in manifest['commands']]
    if not names or len(names) != len(set(names)):
        p.error('release inventory is empty or contains duplicate commands')
    report = (merge_reports([json.loads(path.read_text(encoding="utf-8")) for path in args.merge_report], records, args.candidate)
              if args.merge_report else generate(args.bashy, manifest, args.candidate, records))
    if [r['command'] for r in report['commands']] != names:
        raise ValueError('reports do not match the declared release inventory')
    output = json.dumps(report, indent=2) + '\n'
    if args.json_out:
        args.json_out.write_text(output, encoding="utf-8")
    else:
        print(output, end='')
    if args.markdown_out:
        args.markdown_out.write_text(markdown(report), encoding="utf-8")
    if args.check and report['gap_count']:
        print(f"release bar: {report['gap_count']} named gaps; see generated table", file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, OSError) as error:
        print('release bar: ' + str(error), file=sys.stderr)
        sys.exit(2)

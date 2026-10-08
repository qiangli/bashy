"""Focused regressions for the five-point release gate (stdlib only)."""
import copy
import json
import subprocess
import sys
import tempfile
import importlib.util
from pathlib import Path
import unittest

sys.dont_write_bytecode = True

spec = importlib.util.spec_from_file_location('release_bar', Path(__file__).with_name('release-bar.py'))
bar = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bar)


class ReleaseBarTest(unittest.TestCase):
    def setUp(self):
        self.row = dict(command='commands', stability='preview', consumer='agent bootstrap',
                        json_schema='bashy-commands-v1', atlas_entry=True,
                        dispatch_coverage={os: True for os in bar.OSES},
                        dispatch_pass={os: True for os in bar.OSES})

    def test_each_of_five_requirements_fails_closed(self):
        self.assertEqual([], bar.gaps_for(self.row))
        for field in ('stability', 'consumer', 'json_schema', 'atlas_entry'):
            row = copy.deepcopy(self.row)
            row[field] = None
            self.assertTrue(bar.gaps_for(row), field)
        for field in ('dispatch_coverage', 'dispatch_pass'):
            for os in bar.OSES:
                row = copy.deepcopy(self.row)
                row[field][os] = False
                self.assertTrue(bar.gaps_for(row), (field, os))

    def test_execution_tier_is_not_stability(self):
        self.row['stability'] = 'workspace'
        self.assertTrue(bar.gaps_for(self.row))

    def test_unversioned_or_nested_schema_is_not_an_envelope(self):
        for payload in ([], {}, {'schema_version': ''}, {'result': {'schema_version': 'v1'}}):
            self.assertIsNone(bar.envelope_version(payload))
        self.assertEqual('bashy-commands-v1', bar.envelope_version({'schema_version': 'bashy-commands-v1'}))

    def test_dispatch_pass_requires_same_candidate_and_named_test(self):
        record = dict(candidate='abc', os='linux', test=bar.DISPATCH_TEST, passed=True, run_url='https://example.test/run')
        self.assertTrue(bar.dispatch_pass([record], 'abc', 'linux'))
        for field, value in [('candidate', 'old'), ('test', 'TestSomethingElse'), ('passed', False), ('run_url', '')]:
            bad = dict(record, **{field: value})
            self.assertFalse(bar.dispatch_pass([bad], 'abc', 'linux'), field)
        self.assertFalse(bar.dispatch_pass([], 'abc', 'linux'))

    def test_hidden_aliases_and_tools_are_covered_but_absent_names_are_not(self):
        catalog = {'verbs': ['commands'], 'hidden_verbs': ['models'], 'coreutils': ['graph']}
        for name in ('commands', 'models', 'graph'):
            self.assertTrue(bar.dispatch_covered(name, catalog, {'os': list(bar.OSES)}, 'windows'))
        self.assertFalse(bar.dispatch_covered('missing', catalog, {'os': list(bar.OSES)}, 'linux'))
        self.assertFalse(bar.dispatch_covered('graph', catalog, {'os': ['linux']}, 'windows'))


    def test_three_os_merge_rejects_missing_stale_and_mismatched_evidence(self):
        row = dict(self.row, section='4e', json_probe=['--json'], json_probe_error=None)
        reports = [dict(candidate='abc', probe_os=os, binary_sha256=os, commands=[copy.deepcopy(row)]) for os in bar.OSES]
        records = [dict(candidate='abc', os=os, test=bar.DISPATCH_TEST, passed=True, run_url='https://example.test/run') for os in bar.OSES]
        self.assertEqual(0, bar.merge_reports(reports, records, 'abc')['gap_count'])
        self.assertGreater(bar.merge_reports(reports, records[:2], 'abc')['gap_count'], 0)
        with self.assertRaises(ValueError):
            bar.merge_reports(reports[:2], records, 'abc')
        with self.assertRaises(ValueError):
            bar.merge_reports(reports, records, 'new')
        reports[2]['commands'][0]['json_schema'] = None
        self.assertGreater(bar.merge_reports(reports, records, 'abc')['gap_count'], 0)

    def test_record_requires_completed_test_and_package(self):
        package = 'github.com/qiangli/bashy/internal/agentos'
        passed = [dict(Package=package, Test=bar.DISPATCH_TEST, Action='pass'),
                  dict(Package=package, Action='pass')]
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'test.jsonl'
            for events, want in [(passed, True), (passed[:1], False), ([], False),
                                 (passed + [dict(Package=package, Action='fail')], False)]:
                path.write_text(''.join(json.dumps(e) + '\n' for e in events))
                self.assertEqual(want, bar.record_dispatch(path, 'abc', 'linux', 'url')['passed'])

    def test_cli_check_fails_without_allowlisting_named_gaps(self):
        with tempfile.TemporaryDirectory() as temp:
            manifest = Path(temp) / 'manifest.json'
            manifest.write_text(json.dumps({'commands': [{'command': 'commands'}]}))
            args = [sys.executable, str(Path(bar.__file__)), '--candidate', 'abc', '--check']
            args += ['--manifest', str(manifest)]
            for os in bar.OSES:
                row = dict(self.row, section='4e', json_probe=None, json_probe_error=None)
                report = dict(candidate='abc', probe_os=os, binary_sha256=os, commands=[row])
                path = Path(temp) / (os + '.json')
                path.write_text(json.dumps(report))
                args += ['--merge-report', str(path)]
            result = subprocess.run(args, capture_output=True, text=True)
            self.assertEqual(1, result.returncode, result.stderr)
            self.assertIn('3 named gaps', result.stderr)
            self.assertEqual(3, json.loads(result.stdout)['gap_count'])

    def test_inventory_has_unique_names_and_real_consumer_references(self):
        manifest = json.loads((bar.ROOT / 'docs/release-bar-v1.json').read_text())
        names = [r['command'] for r in manifest['commands']]
        self.assertEqual(len(names), len(set(names)))
        self.assertTrue(set('sprint todo weave dag foreman supervise mb meet ping inbox bus notify whois app models tools agents context run commands llm mcp install-agent out genie kb graph skill craft ask limit oci sandbox loom sshd proxy'.split()) <= set(names))
        for row in manifest['commands']:
            if row['consumer']:
                self.assertIn('bashy ' + row['command'], (bar.ROOT / row['consumer_ref']).read_text())


if __name__ == '__main__':
    unittest.main()

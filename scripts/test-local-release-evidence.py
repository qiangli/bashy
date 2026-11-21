#!/usr/bin/env python3
import copy
import datetime as dt
import hashlib
import importlib.util
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('evidence', ROOT / 'scripts/local-release-evidence.py')
evidence = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evidence)

class ReceiptTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        for name in ('go.mod', 'go.sum', 'tools/bashy-build/image.lock'):
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(name)
        self.now = dt.datetime.now(dt.timezone.utc)
        self.commit = 'a' * 40
        graphs = [{'module': 'github.com/qiangli/' + name, 'locks': {'go.mod': 'b' * 64, 'go.sum': 'c' * 64},
                   'modules': [{'path': 'example.org/dependency', 'version': 'v1.0.0', 'go_mod_sha256': 'd' * 64}]}
                  for name in ('bashy', 'outpost', 'yoke')]
        self.record = {'schema': evidence.SCHEMA, 'commit': self.commit, 'generated_at': self.now.isoformat(),
                       'locks': {n: evidence.digest(self.root / n) for n in ('go.mod', 'go.sum', 'tools/bashy-build/image.lock')},
                       'license': {'verdict': 'pass', 'reports': {'license.json': 'e' * 64}},
                       'govulncheck': {'verdict': 'pass', 'reports': {'scan.json': 'f' * 64}, 'database_modified': (self.now - dt.timedelta(days=1)).isoformat()},
                       'module_graphs': graphs, 'module_graphs_sha256': hashlib.sha256(evidence.canonical(graphs)).hexdigest()}

    def check(self):
        evidence.verify(self.record, self.root, self.commit, self.now)

    def test_current_exact_receipt_passes(self):
        self.check()

    def test_source_lock_tampering_fails(self):
        for name in self.record['locks']:
            with self.subTest(name=name):
                path = self.root / name
                original = path.read_bytes()
                path.write_bytes(b'changed')
                with self.assertRaises(ValueError): self.check()
                path.write_bytes(original)

    def test_wrong_commit_or_failed_gate_fails(self):
        original = copy.deepcopy(self.record)
        for key in ('commit', 'license', 'govulncheck'):
            with self.subTest(key=key):
                self.record = copy.deepcopy(original)
                if key == 'commit': self.record[key] = 'b' * 40
                else: self.record[key]['verdict'] = 'fail'
                with self.assertRaises(ValueError): self.check()

    def test_operator_vulnerability_override(self):
        override = {'reason': 'disclosed advisories', 'by': 'operator', 'sha256': '1' * 64,
                    'overridden': {'yoke': ['GO-2026-6859']}}
        self.record['govulncheck'].update(verdict='override', override=override)
        self.check()
        for key, bad in (('reason', ' '), ('by', ''), ('overridden', {}), ('overridden', {'yoke': []}),
                         ('overridden', {'yoke': ['CVE-1']}), ('sha256', '')):
            with self.subTest(key=key, bad=bad):
                self.record['govulncheck']['override'] = dict(override, **{key: bad})
                with self.assertRaises(ValueError): self.check()
        del self.record['govulncheck']['override']
        with self.assertRaises(ValueError): self.check()
        self.record['govulncheck']['override'] = override
        self.record['license']['verdict'] = 'override'
        with self.assertRaises(ValueError): self.check()

    def test_old_future_missing_database_date_fails(self):
        for date in (self.now - dt.timedelta(days=8), self.now + dt.timedelta(seconds=1)):
            self.record['govulncheck']['database_modified'] = date.isoformat()
            with self.assertRaises(ValueError): self.check()
        del self.record['govulncheck']['database_modified']
        with self.assertRaises(KeyError): self.check()

    def test_transitive_graph_tampering_fails(self):
        self.record['module_graphs'][1]['modules'][0]['version'] = 'v2.0.0'
        with self.assertRaises(ValueError): self.check()

    def test_missing_transitive_lock_fails(self):
        self.record['module_graphs'][1]['modules'][0].pop('go_mod_sha256')
        self.record['module_graphs_sha256'] = hashlib.sha256(evidence.canonical(self.record['module_graphs'])).hexdigest()
        with self.assertRaises(ValueError): self.check()

    def test_undownloaded_module_verify_guards(self):
        unfetched = {'path': 'example.org/pruned', 'version': 'v0.4.0', 'main': False, 'go_mod_downloaded': False,
                     'go_mod_sum': 'h1:pruned='}
        self.record['module_graphs'][0]['modules'].append(unfetched)
        self.record['module_graphs_sha256'] = hashlib.sha256(evidence.canonical(self.record['module_graphs'])).hexdigest()
        self.check()
        for bad in (
            dict(unfetched, main=True),
            dict(unfetched, go_mod_sha256='d' * 64),
            dict(unfetched, go_mod_downloaded='no'),
            {'path': 'example.org/pruned', 'version': 'v0.4.0', 'go_mod_sum': 'h1:pruned=', 'go_mod_downloaded': None},
            {'path': 'example.org/pruned', 'version': 'v0.4.0', 'go_mod_sum': 'h1:pruned=', 'go_mod_sha256': 'not-downloaded'},
            {'path': 'example.org/pruned', 'version': 'v0.4.0', 'go_mod_sum': 'h1:pruned=', 'go_mod_sha256': None},
        ):
            with self.subTest(bad=bad):
                self.record['module_graphs'][0]['modules'][-1] = bad
                self.record['module_graphs_sha256'] = hashlib.sha256(evidence.canonical(self.record['module_graphs'])).hexdigest()
                with self.assertRaises(ValueError): self.check()

    def test_undownloaded_module_requires_h1_go_mod_sum(self):
        row = {'path': 'example.org/pruned', 'version': 'v0.4.0', 'main': False, 'go_mod_downloaded': False}
        self.record['module_graphs'][0]['modules'].append(row)
        def reseal():
            self.record['module_graphs_sha256'] = hashlib.sha256(evidence.canonical(self.record['module_graphs'])).hexdigest()
        row['go_mod_sum'] = 'h1:pruned='
        reseal()
        self.check()
        for bad in (None, '', 'h1:', 'sha256:abc', 'abc=', 7):
            with self.subTest(go_mod_sum=bad):
                row['go_mod_sum'] = bad
                reseal()
                with self.assertRaises(ValueError): self.check()
        del row['go_mod_sum']
        reseal()
        with self.assertRaises(ValueError): self.check()

    def test_collect_records_undownloaded_modules_honestly(self):
        work = pathlib.Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, work)
        cached_mod = work / 'modcache' / 'downloaded.mod'
        cached_mod.parent.mkdir(parents=True)
        cached_mod.write_text('module example.org/downloaded\n\ngo 1.27\n')
        scopes = []
        for name in ('bashy', 'outpost', 'yoke'):
            scope = self.root if name == 'bashy' else work / 'siblings' / name
            scope.mkdir(parents=True, exist_ok=True)
            (scope / 'go.mod').write_text(f'module github.com/qiangli/{name}\n\ngo 1.27\n')
            (scope / 'go.sum').write_text('example.org/downloaded v1.2.0/go.mod h1:test=\n')
            scopes.append(scope)
        env = dict(os.environ, GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                   GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
        subprocess.run(['git', 'init', '-q', '-b', 'main'], cwd=self.root, env=env, check=True)
        subprocess.run(['git', 'add', '.'], cwd=self.root, env=env, check=True)
        subprocess.run(['git', 'commit', '-qm', 'frozen candidate'], cwd=self.root, env=env, check=True)
        commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=self.root, env=env, text=True).strip()
        reports = work / 'reports'
        (reports / 'govulncheck').mkdir(parents=True)
        (reports / 'license').mkdir(parents=True)
        (reports / 'govulncheck' / 'bashy.govulncheck.json').write_text(
            json.dumps({'config': {'db_last_modified': (self.now - dt.timedelta(hours=2)).isoformat()}}) + '\n')
        (reports / 'license' / 'license.json').write_text('{"status":"pass"}\n')
        fake_go = work / 'bin' / 'go'
        fake_go.parent.mkdir(parents=True)
        fake_go.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
assert sys.argv[1:] == ['list', '-mod=readonly', '-m', '-json', 'all'], sys.argv
if os.environ.get('CHECK_GOWORK') == '1':
    assert os.environ.get('GOWORK') == 'off', 'collect must isolate module graph from parent go.work'
cwd = pathlib.Path.cwd()
mod_line = (cwd / 'go.mod').read_text().splitlines()[0].split()[1]
entries = [
    {'Path': mod_line, 'Main': True, 'Dir': str(cwd), 'GoMod': str(cwd / 'go.mod')},
    {'Path': 'example.org/downloaded', 'Version': 'v1.2.0', 'GoMod': os.environ['FAKE_CACHED_MOD'],
     'Sum': 'h1:dl=', 'GoModSum': 'h1:dlmod='},
    {'Path': 'example.org/pruned', 'Version': 'v0.4.0', 'Indirect': True, 'GoModSum': 'h1:prunedmod='},
    {'Path': 'example.org/replaced', 'Version': 'v0.1.0', 'Indirect': True, 'GoModSum': 'h1:replacedmod=',
     'Replace': {'Path': 'example.org/fork', 'Version': 'v0.1.1'}},
]
for entry in entries:
    print(json.dumps(entry))
''')
        fake_go.chmod(0o755)
        receipt = work / 'evidence.json'
        run_env = dict(env, FAKE_CACHED_MOD=str(cached_mod), PATH=str(fake_go.parent) + os.pathsep + env['PATH'])
        cmd = [sys.executable, str(ROOT / 'scripts/local-release-evidence.py'), 'collect', str(receipt),
               '--root', str(self.root), '--reports', str(reports)]
        for scope in scopes:
            cmd.extend(['--scope', str(scope)])
        subprocess.run(cmd, env=run_env, check=True, stdout=subprocess.DEVNULL)
        subprocess.run(cmd, env=dict(run_env, CHECK_GOWORK='1', GOWORK='auto'), check=True, stdout=subprocess.DEVNULL)
        subprocess.run([sys.executable, str(ROOT / 'scripts/local-release-evidence.py'), 'verify', str(receipt),
                        '--root', str(self.root), '--commit', commit], env=run_env, check=True, stdout=subprocess.DEVNULL)
        collected = json.loads(receipt.read_text())
        for graph in collected['module_graphs']:
            by_path = {m['path']: m for m in graph['modules']}
            self.assertEqual(by_path['example.org/downloaded']['go_mod_sha256'], evidence.digest(cached_mod))
            self.assertNotIn('go_mod_downloaded', by_path['example.org/downloaded'])
            for name in ('example.org/pruned', 'example.org/replaced'):
                self.assertIs(by_path[name].get('go_mod_downloaded'), False)
                self.assertNotIn('go_mod_sha256', by_path[name])
                self.assertNotIn('go_sum_sha256', by_path[name])
            self.assertEqual(by_path['example.org/replaced']['replacement'],
                             {'Path': 'example.org/fork', 'Version': 'v0.1.1'})

    def test_distribution_binds_exact_source_and_each_asset(self):
        asset = self.root / 'asset.bin'
        asset.write_bytes(b'reviewed artifact')
        directory = self.root / 'dist'
        directory.mkdir()
        target = directory / asset.name
        shutil.copy(asset, target)
        self.record['distribution'] = {'schema': 'bashy-dist-v1', 'source_sha': self.commit,
                                       'assets': [{'name': asset.name, 'sha256': evidence.digest(asset), 'size': asset.stat().st_size}]}
        evidence.verify_distribution(self.record, directory)
        target.write_bytes(b'changed artifact')
        with self.assertRaises(ValueError): evidence.verify_distribution(self.record, directory)
        shutil.copy(asset, target)
        self.record['distribution']['source_sha'] = 'b' * 40
        with self.assertRaises(ValueError): evidence.verify_distribution(self.record, directory)

    def test_preflight_offline_approved_note_and_tampering(self):
        env = dict(os.environ, GIT_AUTHOR_NAME='Fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                   GIT_COMMITTER_NAME='Fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
        def git(*args, cwd=self.root):
            return subprocess.check_output(['git', *args], cwd=cwd, env=env, text=True, stderr=subprocess.DEVNULL).strip()
        git('init', '-q', '-b', 'main')
        (self.root / 'scripts').mkdir()
        for name in ('verify-local-release-gate.sh', 'local-release-evidence.py'):
            shutil.copy(ROOT / 'scripts' / name, self.root / 'scripts' / name)
        git('add', '.')
        git('commit', '-qm', 'fixture candidate')
        self.commit = git('rev-parse', 'HEAD')
        self.record['commit'] = self.commit
        note = self.root / 'receipt.json'
        note.write_text(json.dumps(self.record))
        git('notes', '--ref=refs/notes/bashy-local-gates', 'add', '-F', str(note), self.commit)
        notes_commit = git('rev-parse', 'refs/notes/bashy-local-gates')
        note.unlink()
        git('remote', 'add', 'origin', str(self.root))
        runenv = dict(env, COMMIT=self.commit, LOCAL_GATE_EVIDENCE_COMMIT=notes_commit)
        command = ['bash', 'scripts/verify-local-release-gate.sh']
        subprocess.run(command, cwd=self.root, env=runenv, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        runenv['LOCAL_GATE_EVIDENCE_COMMIT'] = 'f' * 40
        self.assertNotEqual(subprocess.run(command, cwd=self.root, env=runenv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode, 0)

if __name__ == '__main__': unittest.main()

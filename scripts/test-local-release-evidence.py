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

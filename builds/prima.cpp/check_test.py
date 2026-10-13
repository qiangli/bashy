#!/usr/bin/env python3
import importlib.util
import hashlib
import io
import json
import re
import tarfile
import runpy
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('verify',ROOT/'files/verify-asset.py')
verify=importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)

merge_spec=importlib.util.spec_from_file_location('merge',ROOT/'files/merge-inputs.py')
merge=importlib.util.module_from_spec(merge_spec)
merge_spec.loader.exec_module(merge)

class Audit(unittest.TestCase):
    def test_incomplete_license_scan_cannot_pass(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); (root/'out').mkdir()
            (root/'out/prima-licenses.json').write_text('{"files": []}')
            (root/'out/prima-inputs.json').write_text('[{"path":"prima-linux-amd64/LICENSE","sha256":"abc"}]')
            old=Path.cwd()
            try:
                os.chdir(root)
                with self.assertRaisesRegex(ValueError,'every compiler input'):
                    runpy.run_path(str(ROOT/'files/audit.py'))
            finally:
                os.chdir(old)

class LicenseReview(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.tree = Path(self.temp.name)
        self.review = runpy.run_path(str(ROOT/'files/license-review.py'))
        self.inventory = json.loads((ROOT/'files/dependencies.json').read_text())
        self.inputs, self.scan = [], {'files': []}
        for asset in merge.ASSETS:
            for p in (ROOT/'testdata/license-review').rglob('*'):
                if not p.is_file(): continue
                name = str(p.relative_to(ROOT/'testdata/license-review'))
                expression = ('mit AND cpal-1.0' if name.endswith(('tcp.h', 'stub.c')) else
                              'mit AND mpl-2.0' if name.endswith('shim.hpp') else 'mit')
                self.add(f'{asset}/{name}', p.read_bytes(), expression)

    def add(self, path, body, expression):
        p = self.tree/path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(body)
        self.inputs.append({'path': path, 'sha256': hashlib.sha256(body).hexdigest()})
        self.scan['files'].append({'path': 'audit-inputs/'+path, 'type': 'file',
                                  'license_detections': [{'license_expression': expression}]})

    def result(self):
        return self.review['audit_licenses'](self.scan, self.inputs, self.tree, self.inventory)

    def change_bytes(self, suffix, body):
        for row in self.inputs:
            if row['path'].endswith(suffix):
                (self.tree/row['path']).write_bytes(body)
                row['sha256'] = hashlib.sha256(body).hexdigest()

    def test_exact_reviewed_files_on_all_platforms(self):
        errors, corrections = self.result()
        self.assertEqual(errors, [])
        self.assertEqual(len(corrections), 12)
        self.assertEqual({r['reviewed'] for r in corrections}, {'mit'})
        self.assertEqual(self.scan['files'][0]['license_detections'][0]['license_expression'], 'mit')

    def test_websocket_disabled_stub_requires_exact_reviewed_bytes(self):
        name = 'deps/nng/src/supplemental/websocket/stub.c'
        expected = 'ba3d1139e784b4913095d0f9bb161e6d493513d0a6e96e6d8e507659a56099ee'
        body = (ROOT/'testdata/license-review'/name).read_bytes()
        self.assertEqual(hashlib.sha256(body).hexdigest(), expected)
        self.assertIn(b'terms of the MIT License', body)
        self.assertIn(b'when websocket support is unconfigured', body)
        errors, corrections = self.result()
        self.assertEqual(errors, [])
        self.assertEqual({(row['path'], row['sha256'], row['detected']) for row in corrections
                          if row['path'].endswith(name)},
                         {(f'{asset}/{name}', expected, 'mit AND cpal-1.0')
                          for asset in merge.ASSETS})
        for asset in merge.ASSETS:
            path = f'{asset}/{name}'
            row = next(row for row in self.inputs if row['path'] == path)
            scan = next(row for row in self.scan['files'] if row['path'] == 'audit-inputs/'+path)
            for changed in (body + b'\n// changed\n', body + b'\n// CPAL-1.0\n'):
                with self.subTest(asset=asset, changed=changed[-20:]):
                    (self.tree/path).write_bytes(changed)
                    row['sha256'] = hashlib.sha256(changed).hexdigest()
                    self.assertIn(path, '\n'.join(self.result()[0]))
                    (self.tree/path).write_bytes(body)
                    row['sha256'] = expected
            with self.subTest(asset=asset, expression='cpal-1.0'):
                scan['license_detections'][0]['license_expression'] = 'cpal-1.0'
                self.assertIn(path, '\n'.join(self.result()[0]))
                scan['license_detections'][0]['license_expression'] = 'mit AND cpal-1.0'

    def test_changed_source_bytes_even_with_updated_inventory_fail(self):
        for suffix, body in [('tcp.h', b'// MIT License CPAL-1.0'),
                             ('shim.hpp', b'// MPL-2.0'),
                             ('tcp.h', b'// MIT License changed')]:
            with self.subTest(suffix=suffix, body=body):
                self.change_bytes(suffix, body)
                self.assertTrue(self.result()[0])

    def test_changed_root_license_fails(self):
        self.change_bytes('LICENSE.txt', b'MIT')
        self.change_bytes('/LICENSE', b'MIT')
        self.assertEqual(len(self.result()[0]), 12)

    def test_changed_pins_fail(self):
        self.inventory['linked'][1]['commit'] = '0'*40
        self.assertEqual(len(self.result()[0]), 12)
        self.inventory = json.loads((ROOT/'files/dependencies.json').read_text())
        self.inventory['source']['commit'] = '0'*40
        self.assertEqual(len(self.result()[0]), 12)

    def test_other_paths_and_nonpermissive_expressions_fail(self):
        for expr in ['cpal-1.0', 'mpl-2.0', 'fpl', 'mit AND cpal-1.0 AND gpl-2.0',
                     'mit OR cpal-1.0', '']:
            self.scan['files'][0]['license_detections'][0]['license_expression'] = expr
            self.assertTrue(self.result()[0], expr)
        self.add('prima-linux-amd64/other/tcp.h',
                 (ROOT/'testdata/license-review/deps/nng/include/nng/compat/nanomsg/tcp.h').read_bytes(),
                 'mit AND cpal-1.0')
        self.assertTrue(self.result()[0])

    def test_sha1_closure_fails_even_if_scanner_reports_mit(self):
        self.add('prima-linux-amd64/deps/nng/src/supplemental/sha1/sha1.c', b'fixture', 'mit')
        self.assertIn('excluded SHA1', self.result()[0][0])

    def test_errors_missing_platform_duplicates_and_hash_mismatch_fail(self):
        self.scan['files'][0]['scan_errors'] = ['scanner failed']
        self.assertTrue(self.result()[0])
        self.scan['files'][0].pop('scan_errors')
        (self.tree/self.inputs[0]['path']).write_bytes(b'tampered')
        with self.assertRaisesRegex(ValueError, 'hash mismatch'): self.result()
        missing = merge.ASSETS[0] + '/'
        self.inputs = [row for row in self.inputs if not row['path'].startswith(missing)]
        self.scan['files'] = [row for row in self.scan['files']
                              if not row['path'].startswith('audit-inputs/'+missing)]
        with self.assertRaisesRegex(ValueError, 'all four platforms'): self.result()
        self.inputs.append(self.inputs[0])
        with self.assertRaisesRegex(ValueError, 'every compiler input'): self.result()

    def test_audit_preserves_raw_findings_sbom_and_osv_gate(self):
        import contextlib
        import shutil
        for vulnerabilities in [False, True]:
            with self.subTest(vulnerabilities=vulnerabilities):
                out = self.tree/'out'
                out.mkdir(exist_ok=True)
                tree = self.tree/'audit-inputs'
                tree.mkdir(exist_ok=True)
                for asset in merge.ASSETS:
                    shutil.copytree(self.tree/asset, tree/asset, dirs_exist_ok=True)
                (out/'prima-inputs.json').write_text(json.dumps(self.inputs))
                (out/'prima-licenses.json').write_text(json.dumps(self.scan))
                response = {'results': [{} for _ in range(4)]}
                if vulnerabilities: response['results'][0] = {'vulns': [{'id': 'fixture'}]}
                old = Path.cwd()
                try:
                    os.chdir(self.tree)
                    with patch('urllib.request.urlopen', return_value=io.BytesIO(json.dumps(response).encode())) as request:
                        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                            if vulnerabilities:
                                with self.assertRaises(SystemExit) as caught:
                                    runpy.run_path(str(ROOT/'files/audit.py'))
                                self.assertEqual(caught.exception.code, 1)
                            else:
                                runpy.run_path(str(ROOT/'files/audit.py'))
                        self.assertEqual(request.call_count, 1)
                finally:
                    os.chdir(old)
                report = json.loads((out/'prima-licenses.json').read_text())
                self.assertEqual(report['files'], self.scan['files'])
                self.assertEqual(len(report['bashy_reviewed_findings']), 12)
                self.assertTrue((out/'prima-sbom.cdx.json').is_file())
                self.assertTrue((out/'prima-osv.json').is_file())

    def test_overlay_disables_websockets_preserves_tcp(self):
        overlay = (ROOT/'files/overlay.cmake').read_text()
        for flag, value in [('NNG_TRANSPORT_WS', 'OFF'), ('NNG_TRANSPORT_WSS', 'OFF'),
                            ('NNG_SUPP_SHA1', 'OFF'), ('NNG_TRANSPORT_TCP', 'ON')]:
            setting = f'set({flag} {value} CACHE BOOL "" FORCE)'
            self.assertIn(setting, overlay)
            self.assertLess(overlay.index(setting), overlay.index('add_subdirectory'))

    def test_generated_review_parity(self):
        for name in ['audit.py', 'license-review.py', 'license-review.json', 'overlay.cmake']:
            self.assertEqual((ROOT/'files'/name).read_bytes(), (ROOT/'fork/bashy-build'/name).read_bytes())

class CompilerInputs(unittest.TestCase):
    def test_requires_all_platforms_and_exact_contents(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            sha=hashlib.sha256(b'fixture').hexdigest()
            for asset in merge.ASSETS:
                (root/f'{asset}-inputs.json').write_text(json.dumps([{'path':'LICENSE','sha256':sha}]))
                with tarfile.open(root/f'{asset}-inputs.tar.gz','w:gz') as t:
                    item=tarfile.TarInfo('./LICENSE'); item.size=7
                    t.addfile(item,io.BytesIO(b'fixture'))
            merge.merge(root,root/'result',root/'report.json')
            self.assertEqual(len(json.loads((root/'report.json').read_text())),4)
            (root/f'{merge.ASSETS[0]}-inputs.json').write_text(json.dumps([{'path':'LICENSE','sha256':'0'*64}]))
            with self.assertRaises(ValueError): merge.merge(root,root/'bad',root/'bad.json')
            (root/f'{merge.ASSETS[0]}-inputs.json').unlink()
            with self.assertRaises(FileNotFoundError): merge.merge(root,root/'missing',root/'bad.json')

    def test_rejects_archive_traversal_and_symlinks(self):
        for name,kind in [('../escape',tarfile.REGTYPE),('LICENSE',tarfile.SYMTYPE)]:
            with tempfile.TemporaryDirectory() as d:
                root=Path(d); asset=merge.ASSETS[0]
                (root/f'{asset}-inputs.json').write_text(json.dumps([{'path':'LICENSE','sha256':'0'*64}]))
                with tarfile.open(root/f'{asset}-inputs.tar.gz','w:gz') as t:
                    item=tarfile.TarInfo(name); item.type=kind; item.linkname='elsewhere'; t.addfile(item)
                with self.assertRaises(ValueError): merge.merge(root,root/'result',root/'report.json')

class Preparation(unittest.TestCase):
    def test_dependency_metadata_matches_pin(self):
        declaration=dict(line.split('=',1) for line in
                         (ROOT/'tool.env').read_text().splitlines())
        metadata=(ROOT/'files/dependencies.json').read_bytes()
        self.assertEqual(metadata,(ROOT/'fork/bashy-build/dependencies.json').read_bytes())
        dependencies=json.loads(metadata)
        self.assertEqual(dependencies['source']['tag'],declaration['UPSTREAM_TAG'])
        self.assertEqual(dependencies['source']['commit'],declaration['UPSTREAM_COMMIT'])
        core=next(item for item in dependencies['linked']
                  if item['name']=='prima.cpp/llama.cpp/ggml')
        self.assertEqual(core['version'],declaration['UPSTREAM_COMMIT'])
        self.assertEqual(core['purl'],f"pkg:github/qiangli/prima.cpp@{declaration['UPSTREAM_COMMIT']}")

    def test_reviewed_pin_and_fail_closed_guards(self):
        # Candidate 0.1.1-1 passed verify-pin.sh but all four prepare steps
        # rejected the reviewed HEAD because prepare.py still pinned 0.1.0.
        sha='542d01932a88372a184037c5939048f355dcbd57'
        old='f4ea9845dc91a18793921492e50ef23ec799e93e'
        pin=(ROOT/'fork/bashy-build/PIN').read_text()
        self.assertIn(f'UPSTREAM_COMMIT={sha}\n',pin)
        self.assertEqual(pin,(ROOT/'tool.env').read_text())
        self.assertEqual((ROOT/'files/prepare.py').read_bytes(),
                         (ROOT/'fork/bashy-build/prepare.py').read_bytes())
        for script in [ROOT/'files/prepare.py', ROOT/'fork/bashy-build/prepare.py']:
            spec=importlib.util.spec_from_file_location('prepare',script)
            module=importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            self.assertEqual(module.SOURCE,sha)
            cases=[(old,'',True,module.NNG,True,'wrong source',0),
                   ('0'*40,'',True,module.NNG,True,'wrong source',0),
                   (sha,' M source.cpp',True,module.NNG,True,'dirty source',0),
                   (sha,'',False,module.NNG,True,'',0),
                   (sha,'',True,'0'*40,True,'nng tag moved',1),
                   (sha,'',True,module.NNG,False,'nng license changed',1),
                   (sha,'',True,module.NNG,True,None,1)]
            for head,dirty,license_ok,nng,nng_license_ok,error,clones in cases:
                with self.subTest(script=script,head=head,dirty=dirty,
                                  license_ok=license_ok,nng=nng,nng_license_ok=nng_license_ok), \
                     tempfile.TemporaryDirectory() as d:
                    src=Path(d)
                    (src/'LICENSE').write_text('fixture')
                    (src/'common').mkdir()
                    profiler=src/'common/profiler.cpp'
                    original='static void external_fio_impl() {}\nvoid device_disk_rnd_bw() {}\n'
                    profiler.write_text(original)
                    (src/'CMakeLists.txt').write_text('# fixture\n')
                    license_sha='0ed0eea0d0f31b7f07774a7cac64f9fd20351faf51fab447e9444734448d0452'
                    nng_license='353da18316279d3eded2dd973f21af1cce87acf8'
                    with patch.object(module,'git',side_effect=[head,dirty,nng,
                                      nng_license if nng_license_ok else '0'*40]), \
                         patch.object(module.hashlib,'sha256') as digest, \
                         patch.object(module.subprocess,'run') as clone, \
                         patch('sys.stdout',new_callable=io.StringIO):
                        digest.return_value.hexdigest.return_value=license_sha if license_ok else '0'*64
                        if error is not None:
                            with self.assertRaisesRegex(AssertionError,error):
                                module.prepare(src)
                            self.assertEqual(profiler.read_text(),original)
                            self.assertFalse((src/'bashy-overlay.cmake').exists())
                        else:
                            module.prepare(src)
                            self.assertIn('*read_bw = 0.1f;',profiler.read_text())
                            self.assertEqual((src/'bashy-overlay.cmake').read_bytes(),
                                             (script.parent/'overlay.cmake').read_bytes())
                        self.assertEqual(clone.call_count,clones)

class Contract(unittest.TestCase):
    def test_source_gate_fails_closed(self):
        sha='542d01932a88372a184037c5939048f355dcbd57'
        tag='bashy-bashy-prima-0.1.1-1'
        with tempfile.TemporaryDirectory() as d:
            git=Path(d)/'git'
            git.write_text('#!/bin/sh\n'
                'case "$3 $4" in\n'
                ' "rev-parse HEAD") echo "$TEST_HEAD" ;;\n'
                ' "rev-parse refs/tags/bashy-prima-0.1.1^{commit}")\n'
                '  [ -n "$TEST_TAG" ] || exit 1; echo "$TEST_TAG" ;;\n'
                ' "status --porcelain") printf "%s" "$TEST_DIRTY" ;;\n'
                ' *) exit 99 ;;\nesac\n')
            git.chmod(0o755)
            cases=[(sha,sha,'',tag,True,' OK'),
                   ('0'*40,sha,'',tag,False,'HEAD'),
                   (sha,'0'*40,'',tag,False,'!= pinned'),
                   (sha,'','',tag,False,'missing from checkout'),
                   (sha,sha,' M source.cpp',tag,False,'not pristine'),
                   (sha,sha,'','bashy-bashy-prima-0.1.0-1',False,'does not build')]
            for head,source_tag,dirty,release,passes,message in cases:
                with self.subTest(head=head,source_tag=source_tag,dirty=dirty,release=release):
                    result=subprocess.run(['/bin/bash',str(ROOT/'fork/bashy-build/verify-pin.sh'),d,release],
                        env={**os.environ,'PATH':d+':'+os.environ['PATH'],
                             'TEST_HEAD':head,'TEST_TAG':source_tag,'TEST_DIRTY':dirty},
                        capture_output=True,text=True,timeout=5)
                    self.assertEqual(result.returncode==0,passes,result.stderr)
                    self.assertIn(message,result.stdout+result.stderr)

    def test_source_and_raw_asset_contract(self):
        declaration=(ROOT/'tool.env').read_text()
        self.assertEqual(declaration,(ROOT/'fork/bashy-build/PIN').read_text())
        self.assertIn('UPSTREAM_TAG=bashy-prima-0.1.1\n',declaration)
        self.assertIn('UPSTREAM_COMMIT=542d01932a88372a184037c5939048f355dcbd57\n',declaration)
        assets={'prima-linux-amd64','prima-linux-arm64',
                'prima-darwin-arm64','prima-darwin-amd64'}
        self.assertEqual(verify.ASSETS,assets)
        declared=re.search(r'^RELEASE_EXECUTABLES="([^"]+)"$',declaration,re.M)[1]
        self.assertEqual(set(declared.split()),assets)
        for asset in assets:
            verify.validate_inputs('bashy-bashy-prima-0.1.1-1',asset,'a'*64)
        for tag in ['bashy-bashy-prima-0.1.0-1','bashy-prima-0.1.1',
                    'bashy-bashy-prima-0.1.1-0','bashy-bashy-prima-0.1.1-1-rc']:
            with self.assertRaises(ValueError):
                verify.validate_inputs(tag,'prima-linux-amd64','a'*64)

    def test_reject_untrusted_inputs(self):
        for tag,asset,sha in [('main','prima-linux-amd64','a'*64),
                              ('bashy-bashy-prima-0.1.1-1','../prima','a'*64),
                              ('bashy-bashy-prima-0.1.1-1','prima-linux-amd64','latest')]:
            with self.assertRaises(ValueError): verify.validate_inputs(tag,asset,sha)

    def test_signed_checksum_still_requires_consumer_pin(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); asset='prima-linux-amd64'; data=b'fixture'
            sha=hashlib.sha256(data).hexdigest()
            (root/asset).write_bytes(data)
            (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n')
            with patch.object(verify.subprocess,'run') as run:
                verify.verify(root,'bashy-bashy-prima-0.1.1-1',asset,sha)
                self.assertEqual(run.call_count,2)
                self.assertIn('--certificate-identity',run.call_args.args[0])
                for call in run.call_args_list:
                    args=call.args[0]
                    self.assertEqual(args[args.index('--certificate-identity')+1],
                        'https://github.com/qiangli/prima.cpp/.github/workflows/bashy-build.yml'
                        '@refs/tags/bashy-bashy-prima-0.1.1-1')
                    self.assertEqual(args[args.index('--certificate-oidc-issuer')+1],
                                     'https://token.actions.githubusercontent.com')
                    self.assertTrue(call.kwargs['check'])
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.1-1',asset,'0'*64)
                (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n'*2)
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.1-1',asset,sha)

    def test_signature_failure_stops_verification(self):
        with tempfile.TemporaryDirectory() as d, patch.object(verify.subprocess,'run',side_effect=subprocess.CalledProcessError(1,'cosign')):
            with self.assertRaises(subprocess.CalledProcessError):
                verify.verify(Path(d),'bashy-bashy-prima-0.1.1-1','prima-linux-amd64','a'*64)

    def test_first_party_exception_is_narrow(self):
        text=(ROOT.parent/'render.sh').read_text()
        self.assertIn('$TOOL == prima.cpp && $UPSTREAM_SLUG == qiangli/prima.cpp',text)
        workflow=(ROOT/'fork/.github/workflows/bashy-build.yml').read_text()
        self.assertNotIn('/releases/tags/$UPSTREAM_TAG',workflow)
        self.assertIn('needs: [pin, build, audit]',workflow)
        self.assertEqual(workflow.count('name: verify HEAD == pinned commit'),2)

class NoEgressLinuxDiagnostics(unittest.TestCase):
    # Candidate -2 run 38411861522 failed every Linux gate with no cause in
    # the hosted logs: the namespace exercise runs redirected into
    # out/$asset-egress.txt, and the step exit skipped the artifact upload.
    # Every Linux failure path must therefore reprint the evidence file to
    # stderr, distinguish namespace-setup failure (no trace) from an exercise
    # failure (trace present), and stay fail-closed without weakening the
    # outbound-attempt verdict.
    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def _linux(self):
        text=self._script()
        start=text.index('Linux)')
        return text[start:text.index('Darwin)')]

    def test_failures_reprint_evidence_to_stderr(self):
        linux=self._linux()
        # Both the exercise-failure and the missing-trace paths dump the
        # buffered evidence file to the step log before exiting.
        self.assertEqual(linux.count('cat "out/$asset-egress.txt" >&2'),2)
        self.assertIn('---- out/$asset-egress.txt ----',linux)

    def test_setup_failure_distinguished_from_exercise_failure(self):
        linux=self._linux()
        self.assertIn('suspect unshare/namespace setup, strace, or setpriv',linux)
        self.assertIn('suspect the exercise (binary, ring test or model)',linux)

    def test_detection_not_weakened(self):
        linux=self._linux()
        # The exact Prima 0.1.1 ring and engine ports still use the fatal verdict.
        self.assertIn('egress_verdict "$work/trace" 5599 5600 5601 5602 5603 5604 5605 19000 19001 19100 19101',linux)
        self.assertIn('exit 1',linux)

    def test_fork_matches_files(self):
        self.assertEqual((ROOT/'fork/bashy-build/no-egress-test.sh').read_text(),
                         self._script())

class NoEgressEvidenceUpload(unittest.TestCase):
    # Gate evidence must upload even when the gate fails, or the next blind
    # failure repeats run 38411861522. The upload covers only the egress
    # evidence file; unsigned build outputs still upload on success only, and
    # signing still waits on every gate via the release job's needs.
    def _workflow(self):
        return (ROOT/'fork/.github/workflows/bashy-build.yml').read_text()

    def test_evidence_upload_runs_on_failure(self):
        workflow=self._workflow()
        self.assertIn('name: upload no-egress gate evidence',workflow)
        self.assertIn('if: always()',workflow)
        self.assertIn('out/*-egress.txt',workflow)

    def test_evidence_fragment_is_declared(self):
        self.assertEqual((ROOT/'egress-evidence.yml').read_text(),
                         '      - name: upload no-egress gate evidence\n'
                         '        if: always()\n'
                         '        uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2\n'
                         '        with:\n'
                         '          name: ${{ matrix.asset }}-egress\n'
                         '          path: out/*-egress.txt\n'
                         '          if-no-files-found: warn\n'
                         '          retention-days: 3\n')

    def test_unsigned_upload_still_success_only(self):
        workflow=self._workflow()
        unsigned=workflow[workflow.index('name: upload unsigned build outputs'):]
        self.assertNotIn('if: always()',unsigned.split('name: upload audit outputs')[0])

    def test_signing_still_waits_on_all_gates(self):
        workflow=self._workflow()
        self.assertIn('needs: [pin, build, audit]',workflow)

class NoEgressDarwinObserver(unittest.TestCase):
    # Hosted macos-15 run 38411861522 failed with 'sandbox observer cannot see
    # control denial': the unified log is not a reliable observer for sandbox
    # denies. The Darwin branch must therefore prove enforcement with a
    # directly-observed EPERM positive control, keep the sandbox policy
    # unchanged, and stay fail-closed on any observed engine denial.
    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def test_positive_control_is_directly_observed_eperm(self):
        text=self._script()
        self.assertIn('operation not permitted',text)
        self.assertIn('EPERM',text)
        # The control's own stderr is the observer: denial must be grepped
        # from the control output file, and control success must fail.
        self.assertRegex(text,r'2>"\$work/control"')
        self.assertIn('control connection succeeded under the deny profile',text)
        # The blind unified-log observer must not gate the control: its
        # absence used to fail the job with this message.
        self.assertNotIn('sandbox observer cannot see control denial',text)

    def test_policy_not_weakened(self):
        text=self._script()
        self.assertIn('(deny network-outbound)',text)
        for port in ['5599','5600','5601','5602','5603','5604','5605','19000','19001','19100','19101']:
            self.assertIn(f'localhost:{port}',text)
        for line in text.splitlines():
            if 'allow network-outbound' in line:
                self.assertIn('localhost',line)

    def test_engine_stays_fail_closed(self):
        text=self._script()
        # Engine failure under the deny profile fails the job.
        self.assertIn('engine failed under the deny profile',text)
        # A denied attempt the engine survived is still egress.
        self.assertIn('engine log shows a denied network attempt',text)
        # The unified-log sweep is retained as a fail-closed signal only.
        self.assertIn('unified log shows an engine network denial',text)

    def test_fork_matches_files(self):
        self.assertEqual((ROOT/'fork/bashy-build/no-egress-test.sh').read_text(),
                         (ROOT/'files/no-egress-test.sh').read_text())

class NoEgressRingPorts(unittest.TestCase):
    # First-party Prima tag bashy-prima-0.1.1 @ 542d0193 adds shim tests on
    # fixed loopback ports 5602-5605. Keep both enforcement points aligned
    # with the complete 5599-5605 ring, and allow no other destinations.
    RING_PORTS = ['5599', '5600', '5601', '5602', '5603', '5604', '5605']
    ENGINE_PORTS = ['19000', '19001', '19100', '19101']

    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def test_linux_verdict_allows_exact_loopback_ports(self):
        calls=[l for l in self._script().splitlines()
               if 'egress_verdict "$work/trace"' in l]
        self.assertEqual(len(calls),1)
        self.assertEqual(re.findall(r'\b\d{4,5}\b',calls[0]),
                         self.RING_PORTS+self.ENGINE_PORTS)

    def test_darwin_profile_allows_exact_loopback_ports(self):
        allows=[l for l in self._script().splitlines()
                if '(allow network-outbound' in l]
        self.assertEqual(len(allows),1)
        self.assertEqual(sorted(re.findall(r'localhost:(\d+)',allows[0]),key=int),
                         self.RING_PORTS+self.ENGINE_PORTS)

    def test_darwin_profile_allows_only_loopback(self):
        allows=[l for l in self._script().splitlines()
                if '(allow network-outbound' in l]
        self.assertEqual(len(allows),1)
        remotes=re.findall(r'remote ip "([^"]+)"',allows[0])
        self.assertTrue(remotes)
        for remote in remotes:
            self.assertTrue(remote.startswith('localhost:'),remote)

class ExerciseDiagnostics(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location('exercise', ROOT/'files/exercise.py')
        cls.exercise = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.exercise)

    def stage(self, commands, timeout=2, evaluation='0'):
        import contextlib
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            try:
                self.exercise.run_stage('fixture', commands, timeout, evaluation)
            except (RuntimeError, TimeoutError) as error:
                return output.getvalue(), error
        return output.getvalue(), None

    def command(self, body):
        import sys
        return [sys.executable, '-c', body]

    def test_both_ranks_must_exit_and_master_evaluate(self):
        output, error = self.stage([
            ('1', self.command('print("worker finished")')),
            ('0', self.command('print("eval time")'))])
        self.assertIsNone(error)
        self.assertIn('PASS: fixture', output)
        output, error = self.stage([('0', self.command('print("no evaluation")'))])
        self.assertIsInstance(error, RuntimeError)
        self.assertNotIn('PASS:', output)

    def test_master_evaluation_does_not_pass_stuck_worker(self):
        output, error = self.stage([
            ('1', self.command('import time; print("warming up", flush=True); time.sleep(30)')),
            ('0', self.command('print("eval time")'))], timeout=0.3)
        self.assertIsInstance(error, TimeoutError)
        self.assertIn('pre_cleanup_status=None warmup=True evaluation=False', output)
        self.assertIn('pre_cleanup_status=0 warmup=False evaluation=True', output)
        self.assertIn('final_status=-9', output)
        self.assertNotIn('PASS:', output)

    def test_worker_exit_failure_is_not_hidden_by_master(self):
        output, error = self.stage([
            ('1', self.command('raise SystemExit(7)')),
            ('0', self.command('print("eval time")'))])
        self.assertIsInstance(error, RuntimeError)
        self.assertIn('final_status=7', output)
        self.assertNotIn('PASS:', output)

    def test_timeout_retains_ring_or_single_output(self):
        output, error = self.stage([('ring', self.command(
            'import time; print("ring entered", flush=True); time.sleep(30)'))],
            timeout=0.3, evaluation=None)
        self.assertIsInstance(error, TimeoutError)
        self.assertIn('ring entered', output)
        self.assertNotIn('PASS:', output)

    def test_trace_excerpt_is_bounded_and_labeled(self):
        import contextlib
        with tempfile.TemporaryDirectory() as d:
            path = Path(d)/'trace'
            path.write_bytes(b'HEAD' + b'x'*200000 + b'TAIL')
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                self.exercise.dump_log(path)
            text = output.getvalue()
            self.assertIn('HEAD', text)
            self.assertIn('TAIL', text)
            self.assertIn('omitted', text)
            self.assertLess(len(text), 66000)

    def test_runaway_output_fails_closed(self):
        output, error = self.stage([('0', self.command(
            'import os; os.write(1, b"x"*2000000); os.write(1, b"more")'))])
        self.assertIsInstance(error, RuntimeError)
        self.assertNotIn('PASS:', output)
        self.assertLess(len(output), 68000)

    def test_stdout_bound_is_capture_only(self):
        # Candidate -5 run 38418158569: preexec_fn RLIMIT_FSIZE capped every
        # file the engine wrote (Metal/cache/model), killing the single-rank
        # Darwin engine with SIGXFSZ after 2.2s. The bound must apply only to
        # captured stdout, never to engine-owned files.
        text = (ROOT/'files/exercise.py').read_text()
        self.assertNotIn('RLIMIT_FSIZE', text)
        self.assertNotIn('setrlimit', text)
        self.assertNotIn('preexec_fn', text)
        self.assertIn('stdout=subprocess.PIPE', text)

    def test_engine_owned_files_are_not_limited(self):
        # A 2 MiB engine-owned file write must pass: under the old
        # process-wide RLIMIT_FSIZE=1MiB the same child died (SIGXFSZ on
        # Darwin, EFBIG exit 1 here) instead of evaluating.
        with tempfile.TemporaryDirectory() as d:
            target = Path(d)/'engine-owned.bin'
            output, error = self.stage([('0', self.command(
                f'import pathlib; pathlib.Path({str(target)!r})'
                '.write_bytes(b"m"*2000000); print("eval time")'))])
            self.assertIsNone(error, output)
            self.assertIn('PASS: fixture', output)
            self.assertEqual(target.stat().st_size, 2000000)

    def test_excess_stdout_reports_log_limit(self):
        output, error = self.stage([('0', self.command(
            'import os; os.write(1, b"x"*2000000); print("eval time")'))])
        self.assertIsInstance(error, RuntimeError)
        self.assertIn('log limit exceeded', str(error))
        self.assertIn('log_limit_exceeded=True', output)
        self.assertNotIn('PASS:', output)
        self.assertLess(len(output), 68000)

    def test_linux_retains_trace_before_exercise_failure_exit(self):
        text = (ROOT/'files/no-egress-test.sh').read_text()
        failure = text[text.index('namespace exercise failed'):text.index('if [[ ! -s $work/trace ]]')]
        self.assertIn('python3 "$here/exercise.py" --trace "$work/trace"', failure)
        self.assertLess(failure.index('--trace'), failure.index('exit 1'))
        self.assertIn('-e trace=connect,sendto,sendmsg,sendmmsg,shutdown', text)
        self.assertIn('ulimit -f 16384', text)
        self.assertIn('egress_verdict "$work/trace"', text)

    def test_linux_exercise_failure_artifact_contains_trace(self):
        with tempfile.TemporaryDirectory() as d:
            work = Path(d)
            (work/'out').mkdir()
            (work/'bin').mkdir()
            (work/'bin/uname').write_text('#!/bin/sh\necho Linux\n')
            (work/'bin/sudo').write_text(
                '#!/usr/bin/env python3\n'
                'import pathlib, sys\n'
                'for arg in sys.argv[1:]:\n'
                ' p = pathlib.Path(arg)\n'
                ' if len(arg) < 200 and "\\n" not in arg and (p/"model.gguf").is_file():\n'
                '  (p/"trace").write_text("123 connect signal port 19101 = EINPROGRESS\\n")\n'
                'print("fixture: both ranks entered warmup")\n'
                'raise SystemExit(1)\n')
            for path in (work/'bin').iterdir():
                path.chmod(0o755)
            result = subprocess.run(['/bin/bash', str(ROOT/'fork/bashy-build/no-egress-test.sh'),
                                     str(work/'prima-linux-amd64')], cwd=work,
                                    env={**os.environ, 'PATH': str(work/'bin')+':'+os.environ['PATH']},
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 1, result.stderr)
            evidence = (work/'out/prima-linux-amd64-egress.txt').read_text()
            self.assertIn('fixture: both ranks entered warmup', evidence)
            self.assertIn('bounded syscall trace', evidence)
            self.assertIn('connect signal port 19101', evidence)
            self.assertIn('connect signal port 19101', result.stderr)

    def test_fork_matches_exercise(self):
        self.assertEqual((ROOT/'files/exercise.py').read_bytes(),
                         (ROOT/'fork/bashy-build/exercise.py').read_bytes())

if __name__=='__main__': unittest.main()

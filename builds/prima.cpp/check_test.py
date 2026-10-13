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
        self.assertEqual(len(corrections), 15)
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
        self.assertEqual(len(self.result()[0]), 15)

    def test_changed_pins_fail(self):
        self.inventory['linked'][1]['commit'] = '0'*40
        self.assertEqual(len(self.result()[0]), 15)
        self.inventory = json.loads((ROOT/'files/dependencies.json').read_text())
        self.inventory['source']['commit'] = '0'*40
        self.assertEqual(len(self.result()[0]), 15)

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
        with self.assertRaisesRegex(ValueError, 'all five platforms'): self.result()
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
                self.assertEqual(len(report['bashy_reviewed_findings']), 15)
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
            self.assertEqual(len(json.loads((root/'report.json').read_text())),5)
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

    def test_windows_style_entries_merge_and_unknown_still_rejected(self):
        # CI run 38483154929 (tag 0.1.2-11): the Windows leg declared
        # manifest paths with backslash separators (native WindowsPath str
        # from collect-inputs.py) while MSYS2 tar stores forward slashes,
        # so the audit failed with a nameless 'unexpected archive entry'.
        # Declared, hashed Windows entries (TOOLCHAIN.json, libomp.dll,
        # backslash src paths) must merge into POSIX report paths; anything
        # undeclared is still rejected, and the error names the entry.
        bodies={'LICENSE':b'MIT','deps/nng/LICENSE.txt':b'MIT',
                'TOOLCHAIN.json':b'{"schema": "fixture"}','libomp.dll':b'dll'}
        declared={'LICENSE':'LICENSE','deps/nng/LICENSE.txt':'deps\\nng\\LICENSE.txt',
                  'TOOLCHAIN.json':'TOOLCHAIN.json','libomp.dll':'libomp.dll'}
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            for asset in merge.ASSETS:
                if asset=='prima-windows-amd64':
                    rows=[{'path':declared[name],
                           'sha256':hashlib.sha256(body).hexdigest()}
                          for name,body in bodies.items()]
                    names=list(bodies)
                else:
                    rows=[{'path':'LICENSE','sha256':hashlib.sha256(b'MIT').hexdigest()}]
                    names=['LICENSE']
                (root/f'{asset}-inputs.json').write_text(json.dumps(rows))
                with tarfile.open(root/f'{asset}-inputs.tar.gz','w:gz') as t:
                    for name in names:
                        body=bodies[name] if asset=='prima-windows-amd64' else b'MIT'
                        item=tarfile.TarInfo('./'+name); item.size=len(body)
                        t.addfile(item,io.BytesIO(body))
            merge.merge(root,root/'result',root/'report.json')
            report=json.loads((root/'report.json').read_text())
            windows={row['path'] for row in report if row['path'].startswith('prima-windows-amd64/')}
            self.assertEqual(windows,{f'prima-windows-amd64/{name}' for name in bodies})
            self.assertTrue((root/'result/prima-windows-amd64/deps/nng/LICENSE.txt').is_file())
            self.assertTrue((root/'result/prima-windows-amd64/TOOLCHAIN.json').is_file())
            self.assertTrue((root/'result/prima-windows-amd64/libomp.dll').is_file())
            with tarfile.open(root/'prima-windows-amd64-inputs.tar.gz','w:gz') as t:
                for name in list(bodies)+['crt/unknown.dll']:
                    body=bodies.get(name,b'unknown')
                    item=tarfile.TarInfo('./'+name); item.size=len(body)
                    t.addfile(item,io.BytesIO(body))
            with self.assertRaisesRegex(ValueError,r'unexpected archive entry: crt/unknown\.dll'):
                merge.merge(root,root/'bad',root/'bad.json')

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
        # Repinned to the reviewed 0.1.2 source (Windows ring shims + server
        # sources carried); the superseded 0.1.1 pin is the wrong-source case.
        sha='a0d55d961c8fc1b694de63bd0b2aebdb8365cf2b'
        old='542d01932a88372a184037c5939048f355dcbd57'
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
        sha='a0d55d961c8fc1b694de63bd0b2aebdb8365cf2b'
        tag='bashy-bashy-prima-0.1.2-2'
        with tempfile.TemporaryDirectory() as d:
            git=Path(d)/'git'
            git.write_text('#!/bin/sh\n'
                'case "$3 $4" in\n'
                ' "rev-parse HEAD") echo "$TEST_HEAD" ;;\n'
                ' "rev-parse refs/tags/bashy-prima-0.1.2^{commit}")\n'
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
        self.assertIn('UPSTREAM_TAG=bashy-prima-0.1.2\n',declaration)
        self.assertIn('UPSTREAM_COMMIT=a0d55d961c8fc1b694de63bd0b2aebdb8365cf2b\n',declaration)
        assets={'prima-linux-amd64','prima-linux-arm64',
                'prima-darwin-arm64','prima-darwin-amd64',
                'prima-server-linux-amd64','prima-server-linux-arm64',
                'prima-server-darwin-arm64','prima-server-darwin-amd64',
                'prima-windows-amd64.exe','prima-server-windows-amd64.exe'}
        self.assertEqual(verify.ASSETS,assets)
        declared=re.search(r'^RELEASE_EXECUTABLES="([^"]+)"$',declaration,re.M)[1]
        self.assertEqual(set(declared.split()),assets)
        for asset in assets:
            verify.validate_inputs('bashy-bashy-prima-0.1.2-2',asset,'a'*64)
        for tag in ['bashy-bashy-prima-0.1.0-1','bashy-prima-0.1.2',
                    'bashy-bashy-prima-0.1.2-0','bashy-bashy-prima-0.1.2-2-rc']:
            with self.assertRaises(ValueError):
                verify.validate_inputs(tag,'prima-linux-amd64','a'*64)

    def test_reject_untrusted_inputs(self):
        for tag,asset,sha in [('main','prima-linux-amd64','a'*64),
                              ('bashy-bashy-prima-0.1.2-2','../prima','a'*64),
                              ('bashy-bashy-prima-0.1.2-2','prima-linux-amd64','latest')]:
            with self.assertRaises(ValueError): verify.validate_inputs(tag,asset,sha)

    def test_signed_checksum_still_requires_consumer_pin(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); asset='prima-linux-amd64'; data=b'fixture'
            sha=hashlib.sha256(data).hexdigest()
            (root/asset).write_bytes(data)
            (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n')
            with patch.object(verify.subprocess,'run') as run:
                verify.verify(root,'bashy-bashy-prima-0.1.2-2',asset,sha)
                self.assertEqual(run.call_count,2)
                self.assertIn('--certificate-identity',run.call_args.args[0])
                for call in run.call_args_list:
                    args=call.args[0]
                    self.assertEqual(args[args.index('--certificate-identity')+1],
                        'https://github.com/qiangli/prima.cpp/.github/workflows/bashy-build.yml'
                        '@refs/tags/bashy-bashy-prima-0.1.2-2')
                    self.assertEqual(args[args.index('--certificate-oidc-issuer')+1],
                                     'https://token.actions.githubusercontent.com')
                    self.assertTrue(call.kwargs['check'])
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.2-2',asset,'0'*64)
                (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n'*2)
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.2-2',asset,sha)

    def test_signature_failure_stops_verification(self):
        with tempfile.TemporaryDirectory() as d, patch.object(verify.subprocess,'run',side_effect=subprocess.CalledProcessError(1,'cosign')):
            with self.assertRaises(subprocess.CalledProcessError):
                verify.verify(Path(d),'bashy-bashy-prima-0.1.2-2','prima-linux-amd64','a'*64)

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

class WindowsTarget(unittest.TestCase):
    # Windows-amd64 restores the proven win-build recipe (MinGW Makefiles +
    # clang, static toolchain, nng static, libomp.dll alongside) as a fifth
    # signed release leg on the free windows-2025 runner: CLI
    # prima-windows-amd64.exe, server prima-server-windows-amd64.exe and the
    # compiler OpenMP runtime. Same pin, audit, no-egress and Sigstore gates
    # as the other platforms. Nothing here compiles or runs Windows code;
    # hosted CI owns that proof.
    ALLOWED = [5599, 5600, 5601, 5602, 5603, 5604, 5605, 19000, 19001, 19100, 19101]

    def test_release_declares_windows_asset(self):
        declaration=(ROOT/'tool.env').read_text()
        self.assertIn('prima-windows-amd64=windows-2025',declaration)
        self.assertIn('WINDOWS_SHELL=bash',declaration)
        executables=re.search(r'^RELEASE_EXECUTABLES="([^"]+)"$',declaration,re.M)[1].split()
        self.assertIn('prima-windows-amd64.exe',executables)
        self.assertIn('prima-server-windows-amd64.exe',executables)
        assets=re.search(r'^RELEASE_ASSETS="([^"]+)"$',declaration,re.M)[1].split()
        for name in ['prima-windows-amd64.exe','prima-server-windows-amd64.exe','libomp.dll']:
            self.assertIn(name,assets)
        self.assertIn('five native builds',declaration)

    def test_workflow_runs_bash_on_windows(self):
        workflow=(ROOT/'fork/.github/workflows/bashy-build.yml').read_text()
        self.assertIn('- asset: prima-windows-amd64\n            runner: windows-2025',workflow)
        self.assertIn('shell: bash',workflow)
        self.assertIn('native Windows MinGW + clang compilers',workflow)
        self.assertIn('python3 fork/bashy-build/windows-compilers.py',workflow)

    def test_setup_discovers_windows_compilers(self):
        setup=(ROOT/'setup.yml').read_text()
        self.assertIn('python3 fork/bashy-build/prepare.py src',setup)
        self.assertIn("if: runner.os == 'Windows'",setup)
        self.assertIn('python3 fork/bashy-build/windows-compilers.py',setup)
        discovery=(ROOT/'files/windows-compilers.py').read_text()
        self.assertIn('x86_64-w64-windows-gnu',discovery)
        self.assertNotIn('x86_64-w64-mingw32',discovery)
        self.assertIn('-dumpmachine',discovery)
        self.assertIn('__clang__',discovery)
        self.assertIn('BASHY_WINDOWS_CC',discovery)
        for manager in ['choco install','scoop install','apt-get install']:
            self.assertNotIn(manager,discovery)

    def test_setup_provisions_pinned_clang64_before_probing(self):
        setup=(ROOT/'setup.yml').read_text()
        self.assertIn('msys2/setup-msys2@ec48f7c5447b3140e2b088413ae3a55687bccb6e # v2.33.0',setup)
        self.assertIn('msystem: CLANG64',setup)
        for package in ['mingw-w64-clang-x86_64-clang','mingw-w64-clang-x86_64-compiler-rt',
                        'mingw-w64-clang-x86_64-lld','mingw-w64-clang-x86_64-cmake',
                        'mingw-w64-clang-x86_64-make','mingw-w64-clang-x86_64-ninja',
                        'mingw-w64-clang-x86_64-llvm-openmp',
                        'mingw-w64-clang-x86_64-llvm-tools']:
            self.assertIn(package,setup)
        self.assertLess(setup.index('provision pinned MinGW clang toolchain'),
                        setup.index('native Windows MinGW + clang compilers'))
        workflow=(ROOT/'fork/.github/workflows/bashy-build.yml').read_text()
        self.assertIn('msys2/setup-msys2@ec48f7c5447b3140e2b088413ae3a55687bccb6e # v2.33.0',workflow)
        self.assertIn('msystem: CLANG64',workflow)
        self.assertLess(workflow.index('provision pinned MinGW clang toolchain'),
                        workflow.index('native Windows MinGW + clang compilers'))

    def test_probe_inventory_and_toolchain_record_agree(self):
        spec=importlib.util.spec_from_file_location(
            'windows_compilers',ROOT/'files/windows-compilers.py')
        module=importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        setup=(ROOT/'setup.yml').read_text()
        inventory=json.loads((ROOT/'files/dependencies.json').read_text())
        windows=inventory['toolchain']['windows']
        self.assertEqual(module.TARGET,'x86_64-w64-windows-gnu')
        self.assertEqual(module.MSYSTEM,windows['msystem'])
        self.assertIn(module.MSYSTEM,setup)
        self.assertIn(module.SETUP_ACTION,setup)
        self.assertEqual(windows['provisioner'],module.SETUP_ACTION)
        self.assertEqual(list(module.PACKAGES),windows['packages'])
        for package in module.PACKAGES:
            self.assertIn(package,setup)
        self.assertEqual(windows['generator'],'MinGW Makefiles')
        self.assertEqual(windows['generator'],module.GENERATOR)
        self.assertEqual(windows['make_program'],'clang64/bin/mingw32-make.exe')
        self.assertEqual(windows['objdump'],'clang64/bin/llvm-objdump.exe')
        self.assertEqual(windows['openmp_runtime'],'clang64/bin/libomp.dll')
        self.assertEqual(module.TOOLS['make'],'mingw32-make.exe')
        self.assertEqual(module.TOOLS['objdump'],'llvm-objdump.exe')
        self.assertEqual(module.OMP_DLL,'libomp.dll')

    def test_build_uses_mingw_clang_static_with_shipped_omp(self):
        script=(ROOT/'files/build.sh').read_text()
        self.assertIn("prima-windows-amd64) [[ $(uname -m) == x86_64 ]]; windows=true ;;",script)
        self.assertIn("-G 'MinGW Makefiles'",script)
        self.assertIn('BASHY_WINDOWS_CC',script)
        self.assertIn('-DCMAKE_EXE_LINKER_FLAGS=-static',script)
        self.assertIn('openmp=ON',script)
        self.assertIn('build/bin/llama-cli.exe "out/$asset.exe"',script)
        self.assertIn('build/bin/llama-server.exe "out/$server_asset.exe"',script)
        self.assertIn('libomp.dll',script)
        self.assertIn('unexpected Windows dynamic dependency',script)
        self.assertIn('cygpath -m "$src"',script)

    def test_windows_gate_proves_no_egress(self):
        # Rework 8, CI run 38473601546 (tag 0.1.2-9): the custom WFP-API
        # harness segfaulted. The Windows leg now drives the built-in
        # Windows Defender Firewall from an elevated PowerShell step, with
        # no custom native code anywhere in the gate.
        script=(ROOT/'files/no-egress-test.sh').read_text()
        self.assertIn('MINGW*|MSYS*)',script)
        self.assertIn('native gate requires a GitHub-hosted runner',script)
        self.assertIn('Dragon native gate prohibited',script)
        self.assertIn('powershell.exe',script)
        self.assertIn('windows-defender-egress.ps1',script)
        self.assertIn('Defender deny policy',script)
        # The firewall API rejects forward slashes in -Program paths, so
        # rule programs are converted to backslashes (live noviwin1 proof).
        self.assertIn('winpath',script)
        self.assertIn("tr '/' '\\\\'",script)
        for proof in ['Defender control blocked',
                      'pfirewall.log',
                      'no non-loopback connections observed',
                      'Defender block rules removed; firewall logging restored']:
            self.assertIn(proof,script)
        self.assertIn("grep -c '^PASS:'",script)
        self.assertIn('CONTROL-CONNECTED|allowed non-loopback connection',script)
        for token in ['net-guard','wfp-verdict.h','windows-net-guard.c',
                      '-lfwpuclnt','BASHY_WINDOWS_CC:?',"pid_win=$(cygpath -m"]:
            self.assertNotIn(token,script)

    def test_exercise_kill_is_portable(self):
        exercise=(ROOT/'files/exercise.py').read_text()
        self.assertIn('def _kill_peer(peer):',exercise)
        self.assertEqual(exercise.count('_kill_peer(peer)'),3)
        self.assertNotIn('os.killpg(peer.pid',exercise)

    def test_windows_files_ship_in_fork(self):
        for name in ['build.sh','exercise.py','merge-inputs.py','verify-asset.py',
                     'license-review.py','dependencies.json','collect-inputs.py',
                     'windows-compilers.py','windows-defender-egress.ps1','no-egress-test.sh']:
            self.assertEqual((ROOT/'fork/bashy-build'/name).read_bytes(),
                             (ROOT/'files'/name).read_bytes(),name)
        self.assertIn('prima-windows-amd64',merge.ASSETS)
        self.assertIn('prima-windows-amd64.exe',verify.ASSETS)

class WindowsDefenderGate(unittest.TestCase):
    # Rework 8, CI run 38473601546 (tag 0.1.2-9): the custom WFP-API
    # harness segfaulted in fork/bashy-build/no-egress-test.sh. Three CI
    # rounds had gone to that custom native code. The Windows leg now
    # drives the built-in Windows Defender Firewall from an elevated
    # PowerShell step: disposable outbound BLOCK rules on the exact
    # exercise programs. Loopback is not filtered by Windows Firewall, so
    # the ring/exercise loopback works unimpeded with no allowlist.
    def _gate(self):
        return (ROOT/'files/windows-defender-egress.ps1').read_text()

    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def _exercise(self):
        spec = importlib.util.spec_from_file_location(
            'prima_exercise', ROOT/'files/exercise.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_no_custom_native_code(self):
        # The gate compiles nothing and ships no C sources or headers.
        self.assertFalse((ROOT/'files/windows-net-guard.c').exists())
        self.assertFalse((ROOT/'files/wfp-verdict.h').exists())
        self.assertFalse((ROOT/'fork/bashy-build/windows-net-guard.c').exists())
        self.assertFalse((ROOT/'fork/bashy-build/wfp-verdict.h').exists())
        self.assertFalse((ROOT/'fork/bashy-build/net-guard.exe').exists())
        for token in ['Fwpm', 'FWPM_', '-lfwpuclnt', 'net-guard.exe',
                      'windows-net-guard.c', 'wfp-verdict.h']:
            self.assertNotIn(token, self._gate())
            self.assertNotIn(token, self._script())

    def test_gate_script_ships_in_fork(self):
        self.assertTrue((ROOT/'files/windows-defender-egress.ps1').is_file())
        self.assertEqual((ROOT/'fork/bashy-build/windows-defender-egress.ps1').read_bytes(),
                         (ROOT/'files/windows-defender-egress.ps1').read_bytes())

    def test_block_rules_cover_every_exercise_program(self):
        gate = self._gate()
        # One disposable outbound BLOCK rule per program, keyed on the
        # exact executable path, installed for every profile.
        self.assertIn('New-NetFirewallRule', gate)
        self.assertIn('-Direction Outbound', gate)
        self.assertIn('-Action Block', gate)
        self.assertIn('-Program', gate)
        self.assertIn('-Profile Any', gate)
        for param in ['$PythonExe', '$CliExe', '$RingExe']:
            self.assertIn(param, gate)
        self.assertIn('$ServerExe', gate)
        # Installed count is verified before any control runs.
        self.assertIn('block rules installed', gate)
        self.assertIn('block rules missing', gate)

    def test_loopback_needs_no_allowlist(self):
        # Windows Firewall does not filter loopback, so there is no port
        # allowlist to tile, permit, or drift from exercise.py.
        gate = self._gate()
        self.assertIn('not filtered', gate)
        for token in ['PERMIT', 'allowed_ports', 'allowed_ranges',
                      '5599', '19000', 'FwpmFilterAdd']:
            self.assertNotIn(token, gate)

    def test_control_must_fail_and_appear_in_firewall_log(self):
        gate = self._gate()
        # The negative control is an outbound TCP 443 connect from the
        # blocked interpreter to unroutable TEST-NET-1.
        self.assertIn('192.0.2.1', gate)
        self.assertIn('443', gate)
        self.assertIn('CONTROL-CONNECTED', gate)
        # A connected control means no enforcement: fail.
        self.assertIn('despite the block rule', gate)
        # Dropped-packet logging is enabled for the gate (known log path)
        # and the control failure must appear there as DROP.
        self.assertIn('Set-NetFirewallProfile', gate)
        self.assertIn('-LogBlocked True', gate)
        self.assertIn('pfirewall.log', gate)
        self.assertIn('DROP TCP', gate)
        self.assertIn('is not in the firewall log', gate)

    def test_sampler_fails_on_allowed_non_loopback(self):
        gate = self._gate()
        # A Get-NetTCPConnection sampler watches the whole gate: any
        # Established/SynSent connection from an exercise program to a
        # non-loopback peer fails it. The control's own TEST-NET-1 attempt
        # is proven via the firewall log instead, so the sampler ignores it.
        self.assertIn('Get-NetTCPConnection', gate)
        self.assertIn('Established', gate)
        self.assertIn('SynSent', gate)
        self.assertIn('127.0.0.1', gate)
        self.assertIn('::1', gate)
        self.assertIn('OwningProcess', gate)
        self.assertIn('allowed non-loopback connection', gate)

    def test_rules_and_logging_restored(self):
        gate = self._gate()
        # Cleanup is unconditional: rules removed and per-profile logging
        # restored in a finally block, with a proof line the bash leg
        # requires in the evidence file.
        self.assertIn('finally', gate)
        self.assertIn('Remove-NetFirewallRule', gate)
        self.assertIn('LogBlocked', gate)
        self.assertIn('LogFileName', gate)
        # Profiles are enabled for the gate and restored: rules enforce
        # nothing under a disabled profile (live noviwin1 proof: both
        # networks are Public with that firewall off).
        self.assertIn('-Enabled True', gate)
        self.assertIn('profiles enabled', gate)
        self.assertIn('Defender block rules removed; firewall logging restored', gate)
        # The gate refuses to run unelevated instead of silently
        # installing no rules.
        self.assertIn('Administrator', gate)

    def test_reporting_never_throws(self):
        # Under the script's Stop preference a bare Write-Error throws,
        # which would skip the evidence append and the failed flag. Every
        # error report carries -ErrorAction Continue (live noviwin1 proof:
        # the first repro failed inside New-NetFirewallRule and the catch
        # itself threw, hiding the cause from the evidence file).
        gate = self._gate()
        writers = [line for line in gate.splitlines()
                   if line.lstrip().startswith('Write-Error')]
        self.assertTrue(writers)
        for line in writers:
            self.assertIn('-ErrorAction Continue', line)
        # Native stderr merged with 2>&1 throws under Stop with the first
        # stderr line as the message (live noviwin1 proof: the control's
        # own traceback masked its outcome). Both child runs use file
        # redirects instead.
        code = [line for line in gate.splitlines()
                if not line.lstrip().startswith('#')]
        self.assertNotIn('2>&1', '\n'.join(code))
        self.assertIn('-RedirectStandardError', gate)

    def test_exercise_must_fully_pass(self):
        gate = self._gate()
        # The exit code travels as a marker line: .ExitCode stays empty
        # after a manual .WaitForExit() on some hosts (live noviwin1
        # proof), so the gate never reads it. The 600s Wait-Job is the
        # outer backstop over the exercise's own per-stage deadlines.
        self.assertIn('EXERCISE-RC=', gate)
        self.assertIn('Wait-Job', gate)
        self.assertIn('-Timeout 600', gate)
        # The exercise never reads .ExitCode; the single remaining use is
        # the control's Start-Process -Wait, which does populate it.
        self.assertEqual(gate.count('.ExitCode'), 1)
        self.assertIn('^PASS:', gate)
        self.assertIn('did not all PASS', gate)
        self.assertIn('shows a denied network attempt', gate)
        script = self._script()
        self.assertIn('Defender control blocked', script)
        self.assertIn('no non-loopback connections observed', script)

    def test_exercise_snapshots_bounds_before_reap(self):
        text = (ROOT/'files/exercise.py').read_text()
        self.assertIn('def _log_bound_addresses():', text)
        self.assertIn("os.name != 'nt'", text)
        self.assertIn('if failure is not None:\n                _log_bound_addresses()', text)
        body = text[text.index('def _log_bound_addresses():'):text.index('def run_stage(')]
        for token in ['WSAEACCES', 'Firewall gate failure', 'EPERM']:
            self.assertNotIn(token, body)

    def test_bound_snapshot_is_quiet_off_windows(self):
        if os.name == 'nt':
            self.skipTest('Windows runs the real netstat path')
        import contextlib
        module = self._exercise()
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            module._log_bound_addresses()
        self.assertEqual(out.getvalue(), '')

    def test_bound_snapshot_filters_windows_listeners(self):
        import contextlib
        import types
        module = self._exercise()
        output = ('Active Connections\r\n'
                  '  Proto  Local Address          Foreign Address        State           PID\r\n'
                  '  TCP    127.0.0.1:19000        0.0.0.0:0              LISTENING       1234\r\n'
                  '  TCP    [::1]:19001            [::]:0                 LISTENING       1234\r\n'
                  '  TCP    10.0.0.4:443           93.184.216.34:443      ESTABLISHED     99\r\n')
        with patch.object(module.shutil, 'which', return_value='netstat.exe'), \
             patch.object(module.os, 'name', 'nt'), \
             patch.object(module.subprocess, 'run',
                          return_value=types.SimpleNamespace(stdout=output)):
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                module._log_bound_addresses()
        lines = out.getvalue().splitlines()
        self.assertTrue(lines)
        self.assertTrue(all(line.startswith('bound: ') for line in lines))
        self.assertTrue(any('127.0.0.1:19000' in line for line in lines))
        self.assertTrue(any('[::1]:19001' in line for line in lines))
        self.assertFalse(any('ESTABLISHED' in line for line in lines))

    def test_failed_stage_logs_bounds_and_still_fails(self):
        import contextlib
        import sys
        module = self._exercise()
        fail = [sys.executable, '-c', 'import sys; sys.exit(1)']
        with patch.object(module, '_log_bound_addresses') as bound:
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                with self.assertRaisesRegex(RuntimeError, 'child failed'):
                    module.run_stage('fixture-bound', [('rank', fail)], 30)
        self.assertEqual(bound.call_count, 1)

    def test_passing_stage_stays_quiet(self):
        import contextlib
        import sys
        module = self._exercise()
        ok = [sys.executable, '-c', 'pass']
        with patch.object(module, '_log_bound_addresses') as bound:
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                module.run_stage('fixture-quiet', [('rank', ok)], 30)
        self.assertEqual(bound.call_count, 0)
        self.assertIn('PASS: fixture-quiet', out.getvalue())

class WindowsCompilerProbe(unittest.TestCase):
    # The exact run 38462299969 failure: an MSVC-target clang must be
    # refused, never used for the MinGW static link.
    def _module(self):
        spec = importlib.util.spec_from_file_location(
            'windows_compilers', ROOT/'files/windows-compilers.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def _pair(self, directory, triple, macros, version='clang version 19.1.7'):
        for name in ('clang.exe', 'clang++.exe'):
            script = directory/name
            script.write_text('#!/bin/sh\n'
                              f'if [ "$1" = -dumpmachine ]; then echo "{triple}"; exit 0; fi\n'
                              f'if [ "$1" = --version ]; then echo "{version}"; exit 0; fi\n'
                              f'printf "%s\\n" {macros}\n')
            script.chmod(0o755)

    def test_mingw_clang_pair_passes(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            macros = '"#define __clang__ 1" "#define __MINGW64__ 1" "#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" "#define __cplusplus 202002L"'
            self._pair(root, 'x86_64-w64-windows-gnu', macros)
            records = module.probe(root/'clang.exe', root/'clang++.exe')
            self.assertEqual([r['target'] for r in records],
                             ['x86_64-w64-windows-gnu']*2)

    def test_msvc_target_clang_is_refused(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            macros = '"#define __clang__ 1" "#define _MSC_VER 1940" "#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" "#define __cplusplus 202002L"'
            self._pair(root, 'x86_64-pc-windows-msvc', macros)
            with self.assertRaisesRegex(ValueError, 'x86_64-pc-windows-msvc|MSVC'):
                module.probe(root/'clang.exe', root/'clang++.exe')

    def test_gnu_triple_without_mingw_macros_is_refused(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            macros = '"#define __clang__ 1" "#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" "#define __cplusplus 202002L"'
            self._pair(root, 'x86_64-w64-windows-gnu', macros)
            with self.assertRaisesRegex(ValueError, 'MinGW'):
                module.probe(root/'clang.exe', root/'clang++.exe')

    def test_mismatched_pair_is_refused(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            macros = '"#define __clang__ 1" "#define __MINGW64__ 1" "#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" "#define __cplusplus 202002L"'
            self._pair(root, 'x86_64-w64-windows-gnu', macros)
            (root/'clang++.exe').write_text((root/'clang++.exe').read_text().replace(
                'clang version 19.1.7', 'clang version 18.1.8'))
            with self.assertRaisesRegex(ValueError, 'mismatch'):
                module.probe(root/'clang.exe', root/'clang++.exe')

class WindowsCompilerSelection(unittest.TestCase):
    # Run 38465115827: setup-msys2 installed CLANG64, but find_toolchain
    # consulted PATH first and selected C:\Program Files\LLVM\bin\clang.exe
    # (x86_64-pc-windows-msvc). Selection must be <msys2-location>/clang64/bin
    # FIRST and ONLY: no PATH lookup, no Program Files fallback, no other
    # MSYS2 shell.
    def _module(self):
        spec = importlib.util.spec_from_file_location(
            'windows_compilers', ROOT/'files/windows-compilers.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def _pair(self, directory, triple='x86_64-w64-windows-gnu'):
        macros = ('"#define __clang__ 1" "#define __MINGW64__ 1" '
                  '"#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" '
                  '"#define __cplusplus 202002L"')
        for name in ('clang.exe', 'clang++.exe'):
            script = directory/name
            script.write_text('#!/bin/sh\n'
                              f'if [ "$1" = -dumpmachine ]; then echo "{triple}"; exit 0; fi\n'
                              'if [ "$1" = --version ]; then echo "clang version 19.1.7"; exit 0; fi\n'
                              f'printf "%s\\n" {macros}\n')
            script.chmod(0o755)

    def test_selects_msys2_clang64_ahead_of_path_llvm(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as msys, tempfile.TemporaryDirectory() as llvm:
            clang64 = Path(msys)/'clang64'/'bin'
            clang64.mkdir(parents=True)
            self._pair(clang64)
            llvm_bin = Path(llvm)/'bin'
            llvm_bin.mkdir()
            self._pair(llvm_bin, triple='x86_64-pc-windows-msvc')
            old_path, old_location = os.environ.get('PATH', ''), os.environ.get(module.MSYS2_LOCATION_ENV)
            os.environ['PATH'] = str(llvm_bin) + os.pathsep + old_path
            os.environ[module.MSYS2_LOCATION_ENV] = msys
            try:
                self.assertEqual(module.find_toolchain(), clang64)
                records = module.probe(clang64/'clang.exe', clang64/'clang++.exe')
                self.assertEqual([r['target'] for r in records],
                                 ['x86_64-w64-windows-gnu']*2)
            finally:
                os.environ['PATH'] = old_path
                if old_location is None:
                    os.environ.pop(module.MSYS2_LOCATION_ENV, None)
                else:
                    os.environ[module.MSYS2_LOCATION_ENV] = old_location

    def test_missing_clang64_pair_fails_without_path_fallback(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as msys, tempfile.TemporaryDirectory() as llvm:
            llvm_bin = Path(llvm)/'bin'
            llvm_bin.mkdir()
            self._pair(llvm_bin, triple='x86_64-pc-windows-msvc')
            old_path, old_location = os.environ.get('PATH', ''), os.environ.get(module.MSYS2_LOCATION_ENV)
            os.environ['PATH'] = str(llvm_bin) + os.pathsep + old_path
            os.environ[module.MSYS2_LOCATION_ENV] = msys
            try:
                with self.assertRaisesRegex(ValueError, 'clang64'):
                    module.find_toolchain()
            finally:
                os.environ['PATH'] = old_path
                if old_location is None:
                    os.environ.pop(module.MSYS2_LOCATION_ENV, None)
                else:
                    os.environ[module.MSYS2_LOCATION_ENV] = old_location

    def test_no_path_or_program_files_lookup_in_source(self):
        import inspect
        module = self._module()
        discovery = (ROOT/'files/windows-compilers.py').read_text()
        self.assertIn('MSYS2_LOCATION', discovery)
        self.assertIn('clang64/bin', discovery)
        self.assertNotIn('import shutil', discovery)
        # The selection body itself must name only the CLANG64 dir: no PATH
        # lookup, no Program Files literal, no scan of other MSYS2 shells.
        # (Prose comments elsewhere may cite the old LLVM path as history.)
        body = inspect.getsource(module.find_toolchain)
        self.assertIn('clang64_dir()', body)
        for token in ['which', 'PATH', 'Program Files', 'glob']:
            self.assertNotIn(token, body)

    def test_setup_wires_msys2_location_into_probe(self):
        setup = (ROOT/'setup.yml').read_text()
        self.assertIn('id: msys2', setup)
        self.assertIn('steps.msys2.outputs.msys2-location', setup)
        self.assertIn('MSYS2_LOCATION', setup)
        self.assertLess(setup.index('id: msys2'),
                        setup.index('steps.msys2.outputs.msys2-location'))
        workflow = (ROOT/'fork/.github/workflows/bashy-build.yml').read_text()
        self.assertIn('id: msys2', workflow)
        self.assertIn('steps.msys2.outputs.msys2-location', workflow)
        self.assertIn('MSYS2_LOCATION', workflow)

class WindowsPinnedBuildTools(unittest.TestCase):
    # Run 38466935835: the probe required clang64/bin/make.exe, but the
    # MSYS2 package mingw-w64-clang-x86_64-make installs mingw32-make.exe.
    # The pinned build tool is mingw32-make.exe (CMAKE_MAKE_PROGRAM under
    # MinGW Makefiles); the same audit pinned llvm-objdump.exe
    # (mingw-w64-clang-x86_64-llvm-tools) for the DLL evidence step and
    # libomp.dll (mingw-w64-clang-x86_64-llvm-openmp) for the shipped
    # runtime, and rooted every later Windows step at those verified paths
    # instead of PATH-resolved names or a C:/msys64 literal.
    def _module(self):
        spec = importlib.util.spec_from_file_location(
            'windows_compilers', ROOT/'files/windows-compilers.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def _clang64(self, root, triple='x86_64-w64-windows-gnu', tools=('cmake.exe', 'mingw32-make.exe', 'ninja.exe', 'llvm-objdump.exe'), omp=True):
        macros = ('"#define __clang__ 1" "#define __MINGW64__ 1" '
                  '"#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" '
                  '"#define __cplusplus 202002L"')
        for name in ('clang.exe', 'clang++.exe'):
            script = root/name
            script.write_text('#!/bin/sh\n'
                              f'if [ "$1" = -dumpmachine ]; then echo "{triple}"; exit 0; fi\n'
                              'if [ "$1" = --version ]; then echo "clang version 19.1.7"; exit 0; fi\n'
                              f'printf "%s\\n" {macros}\n')
            script.chmod(0o755)
        for name in tools:
            tool = root/name
            tool.write_text('#!/bin/sh\necho "fixture 1.0"\n')
            tool.chmod(0o755)
        if omp:
            (root/'libomp.dll').write_bytes(b'fixture')

    def test_make_exe_alone_is_not_accepted(self):
        # The 38466935835 layout has mingw32-make.exe but no make.exe;
        # conversely, a bare make.exe must never satisfy the probe. The
        # refusal names the pinned executable.
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            clang64 = Path(d)/'clang64'/'bin'
            clang64.mkdir(parents=True)
            self._clang64(clang64, tools=('cmake.exe', 'make.exe', 'ninja.exe', 'llvm-objdump.exe'))
            with self.assertRaisesRegex(ValueError, r'mingw32-make\.exe'):
                module.tool_versions(clang64)

    def test_pinned_tool_set_passes_and_records_exe_names(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            clang64 = Path(d)/'clang64'/'bin'
            clang64.mkdir(parents=True)
            self._clang64(clang64)
            versions = module.tool_versions(clang64)
            pinned = clang64.resolve()
            self.assertEqual(versions['make']['exe'], 'mingw32-make.exe')
            self.assertEqual(versions['objdump']['exe'], 'llvm-objdump.exe')
            self.assertEqual(versions['openmp_runtime']['exe'], 'libomp.dll')
            for key in ('cmake', 'make', 'ninja', 'objdump'):
                self.assertEqual(Path(versions[key]['path']).parent, pinned)
            self.assertEqual(Path(versions['openmp_runtime']['path']).parent, pinned)
            self.assertEqual(versions['pacman'], 'unavailable')

    def test_missing_objdump_or_omp_runtime_fails_closed(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            clang64 = Path(d)/'clang64'/'bin'
            clang64.mkdir(parents=True)
            self._clang64(clang64, tools=('cmake.exe', 'mingw32-make.exe', 'ninja.exe'))
            with self.assertRaisesRegex(ValueError, r'llvm-objdump\.exe'):
                module.tool_versions(clang64)
            self._clang64(clang64)
            (clang64/'libomp.dll').unlink()
            with self.assertRaisesRegex(ValueError, r'libomp\.dll'):
                module.tool_versions(clang64)

    def test_probe_records_generator_and_make_program(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as msys, tempfile.TemporaryDirectory() as work:
            clang64 = Path(msys)/'clang64'/'bin'
            clang64.mkdir(parents=True)
            self._clang64(clang64)
            path_file, env_file = Path(work)/'path', Path(work)/'env'
            path_file.write_text('')
            env_file.write_text('')
            old = {key: os.environ.get(key) for key in
                   ('PATH', module.MSYS2_LOCATION_ENV, 'GITHUB_PATH', 'GITHUB_ENV')}
            os.environ[module.MSYS2_LOCATION_ENV] = msys
            os.environ['GITHUB_PATH'] = str(path_file)
            os.environ['GITHUB_ENV'] = str(env_file)
            cwd = Path.cwd()
            try:
                os.chdir(work)
                module.main()
            finally:
                os.chdir(cwd)
                for key, value in old.items():
                    if value is None:
                        os.environ.pop(key, None)
                    else:
                        os.environ[key] = value
            toolchain = json.loads((Path(work)/'out'/'windows-toolchain.json').read_text())
            pinned = clang64.resolve()
            self.assertEqual(toolchain['generator'], 'MinGW Makefiles')
            self.assertEqual(toolchain['make_program'], str(pinned/'mingw32-make.exe'))
            env = env_file.read_text()
            for var, exe in [('BASHY_WINDOWS_CMAKE', 'cmake.exe'),
                             ('BASHY_WINDOWS_MAKE', 'mingw32-make.exe'),
                             ('BASHY_WINDOWS_NINJA', 'ninja.exe'),
                             ('BASHY_WINDOWS_OBJDUMP', 'llvm-objdump.exe')]:
                self.assertIn(f'{var}={pinned}', env)
                self.assertIn(exe, env)

    def test_build_pins_make_program_and_verified_tools(self):
        script = (ROOT/'files/build.sh').read_text()
        self.assertIn('-DCMAKE_MAKE_PROGRAM="$BASHY_WINDOWS_MAKE"', script)
        self.assertIn('"${BASHY_WINDOWS_CMAKE:-cmake}" -S "$src" -B build', script)
        self.assertIn('objdump=${BASHY_WINDOWS_OBJDUMP:?', script)
        self.assertNotIn('command -v objdump', script)
        self.assertNotIn('command -v llvm-objdump', script)
        # The hosted MSYS2 root is per-run (D:\a\_temp\msys64 in run
        # 38466935835): no quoted install-root path may survive (comments
        # may still name the old literal as history).
        self.assertNotIn("'C:/msys64", script)
        self.assertNotIn('"C:/msys64', script)
        self.assertIn('BASHY_MINGW_ROOT', script)

    def test_no_egress_gate_compiles_nothing(self):
        # Rework 8: the Defender gate compiles nothing -- no verified-clang
        # compile line, no bare gcc, no WFP link libraries anywhere in the
        # Windows leg. (windows-compilers.py still provisions the toolchain
        # for build.sh; the gate itself just stopped consuming it.)
        script = (ROOT/'files/no-egress-test.sh').read_text()
        windows = script[script.index('MINGW*|MSYS*)'):]
        for token in ['BASHY_WINDOWS_CC', '"$cc" -std=c11', '-lfwpuclnt',
                      'windows-net-guard.c', 'net-guard.exe']:
            self.assertNotIn(token, windows)
        for line in windows.splitlines():
            stripped = line.lstrip()
            self.assertFalse(stripped.startswith('gcc '), line)
        self.assertIn('powershell.exe', windows)

    def test_rendered_workflow_installs_all_pinned_packages(self):
        module = self._module()
        workflow = (ROOT/'fork/.github/workflows/bashy-build.yml').read_text()
        for package in module.PACKAGES:
            self.assertIn(package, workflow)

class WindowsNngWarningDowngrade(unittest.TestCase):
    # CI run 38468420048 (tag 0.1.2-6): the Windows leg fails at 45% in
    # the pinned nng dep, win_tcpconn.c:263-264 passing SOCKET to
    # CancelIoEx's HANDLE parameter, which newer clang promotes from
    # -Wint-conversion to an error by default. The reviewed overlay
    # downgrades just the clang-default-error classes to warnings, for the
    # nng target on Windows only; pinned source bytes stay untouched.
    CLASSES = ['int-conversion', 'incompatible-pointer-types',
               'implicit-function-declaration']

    def _overlay(self):
        return (ROOT/'files/overlay.cmake').read_text()

    def test_downgrade_is_windows_only_and_nng_only(self):
        overlay = self._overlay()
        self.assertIn('if(WIN32)', overlay)
        guarded = overlay[overlay.index('if(WIN32)'):]
        self.assertIn('target_compile_options(nng PRIVATE', guarded)
        for cls in self.CLASSES:
            self.assertIn(f'-Wno-error={cls}', guarded)
        # The nng target exists by then: the downgrade follows its
        # add_subdirectory, and every -Wno-error flag stays inside the
        # WIN32 guard (the rationale comment above the guard may name the
        # classes, but no flag may precede it).
        self.assertLess(overlay.index('add_subdirectory'),
                        overlay.index('if(WIN32)'))
        head = overlay[:overlay.index('if(WIN32)')]
        self.assertNotIn('-Wno-error=', head)

    def test_downgrade_stays_narrow(self):
        overlay = self._overlay()
        # No global downgrade, no silencing: warnings stay visible, and no
        # other target (llama, common, prima-ring-test) is touched.
        self.assertNotIn('add_compile_options', overlay)
        self.assertNotIn('-Wno-int-conversion', overlay)
        self.assertNotIn('-w', overlay.replace('-Wno-error', ''))
        self.assertEqual(overlay.count('target_compile_options(nng'), 1)
        self.assertNotIn('target_compile_options(llama', overlay)
        self.assertNotIn('target_compile_options(common', overlay)

    def test_pins_still_assert_exact_source_bytes(self):
        # The flag change must not loosen the source pins: prepare.py still
        # asserts the reviewed source/nng SHAs and license blob.
        prepare = (ROOT/'files/prepare.py').read_text()
        self.assertIn("SOURCE = 'a0d55d961c8fc1b694de63bd0b2aebdb8365cf2b'", prepare)
        self.assertIn("NNG = '29aff2bacc9e2cfa6dcc52155d2816582617f918'", prepare)
        self.assertIn('nng license changed', prepare)

class CollectInputsDepfiles(unittest.TestCase):
    def _module(self):
        spec = importlib.util.spec_from_file_location(
            'collect', ROOT/'files/collect-inputs.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_rule_split_keeps_drive_letter_targets(self):
        module = self._module()
        tail = module.rule_prerequisites('CMakeFiles/a.dir/b.obj: C:/src/a.cpp C:/build/c.h')
        self.assertIn('C:/src/a.cpp', tail)
        self.assertIn('C:/build/c.h', tail)
        tail = module.rule_prerequisites('CMakeFiles/a.dir/b.o: /src/a.cpp \\\n /build/c.h')
        self.assertIn('/src/a.cpp', tail)
        self.assertIn('/build/c.h', tail)

    def test_obj_d_and_toolchain_record_join_closure(self):
        module = self._module()
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            src, build, dest = root/'src', root/'build', root/'dest'
            (src/'deps/nng').mkdir(parents=True)
            (src/'LICENSE').write_bytes(b'MIT')
            (src/'deps/nng/LICENSE.txt').write_bytes(b'MIT')
            (src/'a.cpp').write_bytes(b'int main() {}')
            depdir = build/'CMakeFiles/a.dir'
            depdir.mkdir(parents=True)
            (depdir/'a.obj.d').write_text(f'a.obj: {src}/a.cpp\n')
            toolchain = root/'toolchain.json'
            toolchain.write_bytes(b'{"schema": "fixture"}')
            old = os.environ.get('BASHY_TOOLCHAIN_JSON')
            os.environ['BASHY_TOOLCHAIN_JSON'] = str(toolchain)
            try:
                module.collect(src, build, dest, root/'report.json')
            finally:
                if old is None:
                    del os.environ['BASHY_TOOLCHAIN_JSON']
                else:
                    os.environ['BASHY_TOOLCHAIN_JSON'] = old
            report = json.loads((root/'report.json').read_text())
            by_path = {row['path']: row['sha256'] for row in report}
            self.assertEqual(by_path['TOOLCHAIN.json'], hashlib.sha256(b'{"schema": "fixture"}').hexdigest())
            self.assertTrue((dest/'TOOLCHAIN.json').is_file())
            self.assertIn('a.cpp', by_path)

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

class WindowsDefenderRunnerMismatch(unittest.TestCase):
    # Rework 9, CI run 38481498544 (tag 0.1.2-10): the gate PASSED live on
    # noviwin1 but failed on the windows-2025 runner with "no DROP for
    # 192.0.2.1:443" after a TimeoutError, not an immediate WinError 10013
    # access denied: the block rule most likely did not match the control
    # process (hostedtoolcache python vs a shim, 8.3 short names, case).
    # The gate now canonicalizes every rule program, blocks the control
    # interpreter's self-reported image too, reads the installed rule
    # programs back for comparison, polls the log through a share-read
    # handle, records the ActiveStore policy, and lets the control prove
    # the rule matched via immediate access-denied OR a logged DROP.
    def _gate(self):
        return (ROOT/'files/windows-defender-egress.ps1').read_text()

    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def test_rule_programs_are_canonicalized_and_compared(self):
        gate = self._gate()
        # Raw vs resolved paths are both logged for the exact program list.
        self.assertIn('Defender rule program raw=', gate)
        self.assertIn('resolved=', gate)
        self.assertIn('FullName', gate)
        # The control interpreter reports its own image; both paths are
        # blocked when they differ (shim re-exec case).
        self.assertIn('self-reported=', gate)
        self.assertIn('realpath(sys.executable)', gate)
        self.assertIn('blocking both paths', gate)
        # Installed rules are read back and compared case-insensitively
        # before the control runs, instead of surfacing as a timeout later.
        self.assertIn('Get-NetFirewallApplicationFilter', gate)
        self.assertIn('ToLowerInvariant', gate)
        self.assertIn('no rule carries', gate)
        # The bash leg resolves symlinks first; the gate re-canonicalizes,
        # so this is the first of two resolutions, not the only one.
        self.assertIn('os.path.realpath(sys.executable)', self._script())

    def test_firewall_log_uses_share_read_poll(self):
        gate = self._gate()
        # pfirewall.log is held open and buffered: share-read handle, and
        # the ~30s poll is retained.
        self.assertIn('FileShare', gate)
        self.assertIn('ReadWrite', gate)
        self.assertIn('StreamReader', gate)
        self.assertIn('-lt 30', gate)
        # Both dropped and allowed logging are switched on (cmdlet plus
        # netsh belt and braces) and restored afterwards.
        self.assertIn('droppedconnections enable', gate)
        self.assertIn('allowedconnections enable', gate)
        self.assertIn('LogAllowed', gate)

    def test_active_store_policy_is_recorded_and_enforced(self):
        gate = self._gate()
        # A GPO/managed override can leave a profile disabled in the
        # ActiveStore while the persistent store looks enabled: record the
        # effective state and fail closed on any managed disable.
        self.assertIn('-PolicyStore ActiveStore', gate)
        self.assertIn('ActiveStore profile', gate)
        self.assertIn('GPO', gate)

    def test_control_proves_rule_matched(self):
        gate = self._gate()
        # Immediate access-denied (10013) is rule-matched proof even when
        # the DROP line never lands; a bare timeout with no DROP still
        # fails, now with the compared paths attached.
        self.assertIn('10013', gate)
        self.assertIn('TimeoutError', gate)
        self.assertIn('rule matched', gate)
        self.assertIn('is not in the firewall log', gate)
        self.assertIn('Defender control blocked', gate)
        self.assertIn('control image:', gate)

if __name__=='__main__': unittest.main()

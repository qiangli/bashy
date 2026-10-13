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

class Contract(unittest.TestCase):
    def test_reject_untrusted_inputs(self):
        for tag,asset,sha in [('main','prima-linux-amd64','a'*64),
                              ('bashy-bashy-prima-0.1.0-1','../prima','a'*64),
                              ('bashy-bashy-prima-0.1.0-1','prima-linux-amd64','latest')]:
            with self.assertRaises(ValueError): verify.validate_inputs(tag,asset,sha)

    def test_signed_checksum_still_requires_consumer_pin(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); asset='prima-linux-amd64'; data=b'fixture'
            sha=hashlib.sha256(data).hexdigest()
            (root/asset).write_bytes(data)
            (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n')
            with patch.object(verify.subprocess,'run') as run:
                verify.verify(root,'bashy-bashy-prima-0.1.0-1',asset,sha)
                self.assertEqual(run.call_count,2)
                self.assertIn('--certificate-identity',run.call_args.args[0])
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.0-1',asset,'0'*64)
                (root/'SHA256SUMS').write_text(f'{sha}  {asset}\n'*2)
                with self.assertRaises(ValueError): verify.verify(root,'bashy-bashy-prima-0.1.0-1',asset,sha)

    def test_signature_failure_stops_verification(self):
        with tempfile.TemporaryDirectory() as d, patch.object(verify.subprocess,'run',side_effect=subprocess.CalledProcessError(1,'cosign')):
            with self.assertRaises(subprocess.CalledProcessError):
                verify.verify(Path(d),'bashy-bashy-prima-0.1.0-1','prima-linux-amd64','a'*64)

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
        # Same verdict, same loopback ports plus ring-test 5601, still fatal.
        self.assertIn('egress_verdict "$work/trace" 5599 5600 5601 19000 19001 19100 19101',linux)
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
        for port in ['5599','5600','5601','19000','19001','19100','19101']:
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

class NoEgressRingPort5601(unittest.TestCase):
    # Candidate -3 run 38414127942: all four native builds passed but every
    # no-egress leg failed. Both allowlists carried only ring-test ports
    # 5599/5600, yet the pinned ring test (common/zmq_nng_shim_test.cpp @
    # f4ea9845dc91a18793921492e50ef23ec799e93e) binds/connects three
    # loopback ports: 5599 (multipart round-trip), 5600 (rcvtimeo), 5601
    # (single-message send/recv). Under the Darwin sandbox the denied 5601
    # connect hangs the ring test's blocking recv until the 30s exercise
    # timeout; on Linux the same loopback connect trips the strace verdict.
    # Both enforcement points must therefore allow loopback 5601, and the
    # Darwin profile must allow nothing but loopback.
    RING_PORTS = ['5599', '5600', '5601']
    ENGINE_PORTS = ['19000', '19001', '19100', '19101']

    def _script(self):
        return (ROOT/'files/no-egress-test.sh').read_text()

    def test_linux_verdict_allows_all_seven_loopback_ports(self):
        calls=[l for l in self._script().splitlines()
               if 'egress_verdict "$work/trace"' in l]
        self.assertEqual(len(calls),1)
        self.assertEqual(re.findall(r'\b\d{4,5}\b',calls[0]),
                         self.RING_PORTS+self.ENGINE_PORTS)

    def test_darwin_profile_allows_all_seven_loopback_ports(self):
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

if __name__=='__main__': unittest.main()

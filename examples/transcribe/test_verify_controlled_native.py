#!/usr/bin/env python3
"""Focused receipt-retention checks; no model or native inference."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('controlled_native_gate',
                                              HERE / 'verify-controlled-native.py')
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class ReceiptRetentionTest(unittest.TestCase):
    def test_validated_projections_bind_ledgers_without_private_paths(self):
        with tempfile.TemporaryDirectory() as owned:
            root = Path(owned)
            ledgers = root / 'skills' / 'attest'
            ledgers.mkdir(parents=True)
            outdir = root / 'evidence'
            outdir.mkdir()
            private = str(root / 'private token')
            skill = {'identity': 'skill:transcribe', 'capability': 'transcribe-capability'}
            for name, count, identity in (('transcribe', 9, skill['identity']),
                                          ('transcribe_pipeline', 18,
                                           'fn:transcribe_pipeline')):
                records = []
                for index in range(count):
                    record = {'name': name, 'context_key': private,
                              'attest': {'Skill': identity, 'Valid': True,
                                         'Passed': ['contract check'], 'Failed': None,
                                         'Effects': ['read']}}
                    if name == 'transcribe':
                        record['capability'] = skill['capability']
                    else:
                        record['status'] = 0
                    records.append(record)
                (ledgers / (name + '.jsonl')).write_text(
                    ''.join(json.dumps(record) + '\n' for record in records))

            summary, evidence = gate.retain_receipts({'BASHY_SKILLS_DIR': str(root / 'skills')},
                                                      outdir, skill)
            self.assertEqual(summary['count'], 18)
            for kind, expected in (('skill', 9), ('function', 18)):
                item = evidence[kind]
                retained = outdir / item['file']
                self.assertEqual(item['count'], expected)
                self.assertEqual(item['sha256'], hashlib.sha256(retained.read_bytes()).hexdigest())
                self.assertNotIn(private, retained.read_text())
                self.assertNotIn('context_key', retained.read_text())
                self.assertEqual(retained.read_text().count('source_record_sha256'), expected)

            bad = ledgers / 'transcribe.jsonl'
            rows = [json.loads(line) for line in bad.read_text().splitlines()]
            rows[0]['attest']['Valid'] = False
            bad.write_text(''.join(json.dumps(row) + '\n' for row in rows))
            with self.assertRaisesRegex(ValueError, 'skill receipt differs'):
                gate.retain_receipts({'BASHY_SKILLS_DIR': str(root / 'skills')}, outdir, skill)


if __name__ == '__main__':
    unittest.main()

"""Format-defined roles, progress and visibility across the command boundary."""
import contextlib
import io
import json
from unittest.mock import patch

from tests.helpers import TempBashy, HERE
import common
import nudge
import plan
import run


class FormatFlows(TempBashy):
    def apply_format(self, name):
        draft = json.loads(self.tutor('plan', 'Produce a checkable result', '--format', name,
                                     '--people', 'alex', '--json').stdout)
        applied = json.loads(self.tutor('apply', '-', '--json', stdin=json.dumps(draft)).stdout)
        return draft, str(applied['sprint'])

    def test_meeting_plan_apply_next_status(self):
        draft, sprint = self.apply_format('meeting')
        self.assertEqual(draft['roles'], ['facilitator', 'participant'])
        value = json.loads(self.tutor('next', sprint, '--json').stdout)
        self.assertEqual(value['format'], 'meeting')
        self.assertEqual(value['roles'], {'owner': 'facilitator', 'participants': ['participant']})
        total = sum(s['role'] == 'participant' for s in draft['steps'])
        self.assertEqual(value['current']['total'], total)
        self.assertGreater(len(value['steps']), total)
        self.tutor('next', sprint)
        self.assertTrue(self.tutor('status', sprint, '--line').stdout.startswith('Step 2/%d' % total))
        meta = common.metadata(int(sprint))
        self.assertEqual(meta['tone'], plan.load_format('meeting')['tone'])

    def test_solo_plan_run_to_completion(self):
        draft, sprint = self.apply_format('solo')
        self.assertEqual({s['role'] for s in draft['steps']}, {'self'})
        self.tutor('next', sprint)
        value = run.status(int(sprint))
        self.assertEqual(value['roles'], {'owner': 'self', 'participants': []})
        self.assertEqual(value['current']['total'], len(draft['steps']))
        for _ in draft['steps']:
            self.tutor('next', sprint)
        self.assertEqual(self.tutor('status', sprint, '--line').stdout.strip(),
                         'Done · %d/%d steps' % (len(draft['steps']), len(draft['steps'])))

    def test_named_private_editions(self):
        item = self.repo / 'item.md'
        item.write_text('---\nformat: meeting\nanswer: hidden key\n---\nPublic\n'
                        '~~~private\nOwner secret\n~~~\n'
                        '~~~private participant\nRole secret\n~~~\n')
        for edition in ('owner', 'facilitator', 'participant', 'shared'):
            with self.subTest(edition=edition):
                out = self.tutor('render', str(item), '--edition', edition).stdout
                self.assertIn('Public', out)
                self.assertEqual('Owner secret' in out, edition in ('owner', 'facilitator'))
                self.assertEqual('hidden key' in out, edition in ('owner', 'facilitator'))
                self.assertEqual('Role secret' in out, edition != 'shared')
        self.assertNotEqual(self.tutor('render', str(item), '--edition', 'stranger', check=False).returncode, 0)
        item.write_text('---\nformat: solo\n---\n~~~private\nReflection\n~~~\n')
        self.assertIn('Reflection', self.tutor('render', str(item), '--edition', 'self').stdout)
        self.assertNotIn('Reflection', self.tutor('render', str(item), '--edition', 'shared').stdout)

    def test_multiple_participant_roles_share_progress(self):
        fmt = self.repo / 'format.md'
        fmt.write_text('---\nname: workshop\nroles:\n- host\n- speaker\n- listener\n'
                       'phases:\n- discuss 30\n---\n')
        draft = json.loads(self.tutor('plan', 'Discuss', '--format', str(fmt), '--json').stdout)
        draft['steps'] = [dict(title='Part %d' % i, role=role, owner='alex', minutes=5)
                          for i, role in enumerate(['host', 'speaker', 'listener', 'host'])]
        applied = json.loads(self.tutor('apply', '-', '--json', stdin=json.dumps(draft)).stdout)
        sprint = str(applied['sprint'])
        self.tutor('next', sprint)
        value = json.loads(self.tutor('next', sprint, '--json').stdout)
        self.assertEqual((value['current']['index'], value['current']['total']), (2, 2))
        self.tutor('next', sprint)
        self.assertEqual(self.tutor('status', sprint, '--line').stdout.strip(), 'Done · 2/2 steps')

    def test_prep_preserves_local_solo_format(self):
        target = self.repo / 'solo-content'
        self.tutor('prep', str(target), '--format', 'solo', '--goal', 'Reflect', '--json')
        self.tutor('prep', str(target), '--json')
        draft = json.loads((target / 'draft.json').read_text())
        self.assertEqual(draft['roles'], ['self'])
        self.assertTrue(draft['steps'])
        self.assertEqual({s['role'] for s in draft['steps']}, {'self'})
        item = next((target / 'content').glob('*.md'))
        with item.open('a') as stream:
            stream.write('\n~~~private\nPrivate reflection\n~~~\n')
        self.assertIn('Private reflection', self.tutor('render', str(item), '--edition', 'self').stdout)

    def test_audience_only_from_format(self):
        template, _ = nudge.load_prompt_template()
        text = nudge.fill_prompt(template, 'Do it', 'Evidence', {'audience': 'new volunteers', 'tone': 'Be concise.'})
        self.assertIn('Audience: new volunteers.', text)
        self.assertIn('Be concise.', text)
        self.assertNotIn('Audience:', nudge.fill_prompt(template, 'Do it', 'Evidence'))
        for roles in ([], ['host', 'host'], ['host', 'bad role'], ['host', 'UPPER']):
            with self.subTest(roles=roles), self.assertRaises(SystemExit):
                common.validate_roles(roles)

    def test_class_is_an_example_format(self):
        draft, sprint = self.apply_format('class')
        self.assertEqual(draft['roles'], ['teacher', 'student'])
        self.assertEqual(draft['audience'], 'learners; keep language plain and encouraging')
        value = json.loads(self.tutor('next', sprint, '--json').stdout)
        self.assertEqual(value['roles'], {'owner': 'teacher', 'participants': ['student']})
        self.assertIn('learners; keep language plain and encouraging', common.metadata(int(sprint))['audience'])
        item = self.repo / 'example.md'
        item.write_text('---\nformat: class\n---\n~~~private\nAnswer\n~~~\n')
        self.assertIn('Answer', self.tutor('render', str(item), '--edition', 'teacher').stdout)
        self.assertNotIn('Answer', self.tutor('render', str(item), '--edition', 'student').stdout)

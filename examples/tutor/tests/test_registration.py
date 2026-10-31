"""Registration and manager smoke regressions, all stores isolated."""
import json
from tests.helpers import TempBashy
from common import bashy
import run


class SmokeRegressions(TempBashy):
    def test_student_progress_excludes_teacher_steps(self):
        bashy('sprint', 'add', 'Week 2')
        ids = []
        for i, role in enumerate(['teacher', 'student', 'teacher', 'student',
                                   'student', 'teacher', 'student', 'student'], 1):
            ids.append(bashy('todo', 'add', 'Step %d' % i, '--sprint', '1',
                             '--label', 'step-%02d,timebox-10m,role-%s' % (i, role),
                             '--json', json_out=True)['id'])
        bashy('todo', 'status', ids[3], 'doing')
        value = run.status(1)
        self.assertEqual(len(value['steps']), 8)
        self.assertEqual(value['current']['total'], 5)
        self.assertEqual(value['current']['index'], 2)
        self.assertTrue(run.one_line(value).startswith('Step 2/5'))
        self.tutor('next', '1', '--json')
        self.assertEqual(run.status(1)['current']['id'], ids[4])

    def test_apply_nudge_done_when_roundtrip(self):
        draft = json.loads(self.tutor('plan', 'Week 2', '--people', 'ana', '--json').stdout)
        draft['steps'][0]['done_when'] = 'Sketch shared'
        result = json.loads(self.tutor('apply', '-', '--json', stdin=json.dumps(draft)).stdout)
        self.tutor('next', str(result['sprint']), '--json')
        hint = json.loads(self.tutor('nudge', str(result['sprint']), '--person', 'ana', '--json').stdout)
        self.assertEqual(hint['done_when'], 'Sketch shared')


class RegistrationTest(TempBashy):
    def test_real_registration_duality_and_rehearsal(self):
        import subprocess
        import sys
        from tests.helpers import HERE
        result = subprocess.run([sys.executable, str(HERE / 'verify-duality.py')],
                                text=True, capture_output=True, timeout=180)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('PASS none:', result.stdout)
        self.assertIn('PASS stub:', result.stdout)

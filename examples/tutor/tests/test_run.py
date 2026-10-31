"""Run verbs use isolated stores; clock tests never launch an agent."""
import contextlib
import io
import json
import os
from unittest.mock import patch

from tests.helpers import TempBashy
from common import bashy, log_path
import run


class RunTests(TempBashy):
    def setUp(self):
        super().setUp()
        os.environ['TUTOR_NOW'] = '2026-10-30T10:00:00Z'
        bashy('sprint', 'add', 'Class')
        self.ids = []
        for i, title in enumerate(['Warmup', 'Prototype sketch'], 1):
            obj = bashy('todo', 'add', title, '--sprint', '1', '--owner', 'Ana',
                        '--label', 'step-%02d,timebox-10m,role-student' % i,
                        '--json', json_out=True)
            self.ids.append(obj['id'])

    def invoke(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            run.main(list(args))
        return out.getvalue()

    def status(self):
        return json.loads(self.tutor('status', '1', '--json').stdout)

    def test_not_started_and_filtering(self):
        bashy('todo', 'add', 'Unrelated', '--sprint', '1')
        status = self.status()
        self.assertEqual(set(status), {'sprint', 'title', 'clock', 'current', 'steps'})
        self.assertEqual(status['clock'], dict(state='not-started', deadline=None, remaining_s=None))
        self.assertIsNone(status['current'])
        self.assertEqual(len(status['steps']), 2)
        self.assertEqual(self.tutor('status', '1', '--line').stdout, 'Not started · 2 steps\n')

    def test_next_extend_and_overdue(self):
        self.tutor('next', '1', '--json')
        os.environ['TUTOR_NOW'] = '2026-10-30T10:13:10Z'
        status = self.status()
        self.assertEqual(status['current']['remaining_s'], -190)
        self.assertTrue(status['current']['overdue'])
        self.assertEqual(self.tutor('status', '1', '--line').stdout,
                         'Step 1/2 · Warmup · 3:10 over · ana\n')
        self.tutor('extend', '1', '1', '--by', '5m', '--json')
        status = self.status()
        self.assertEqual(status['current']['timebox_s'], 900)
        self.assertEqual(status['current']['remaining_s'], 110)
        self.assertFalse(status['current']['overdue'])
        self.assertEqual(status['clock']['state'], 'not-started')
        events = [json.loads(line) for line in log_path(1).read_text().splitlines()]
        self.assertEqual(events[-1]['by_s'], 300)

    def test_next_done_round_trip(self):
        self.tutor('next', '1', '--json')
        os.environ['TUTOR_NOW'] = '2026-10-30T10:02:00Z'
        self.tutor('next', '1', '--json')
        self.assertEqual(self.status()['current']['id'], self.ids[1])
        self.tutor('done', '1', self.ids[1], '-m', 'Sketch shared', '--json')
        self.assertIsNone(self.status()['current'])
        self.assertEqual(self.status()['steps'][1]['evidence'], 'Sketch shared')
        self.assertEqual(self.tutor('status', '1', '--line').stdout, 'Done · 2/2 steps\n')

    def test_no_agent_start_refusal_does_not_start_step(self):
        result = self.tutor('start', '1', '--json', check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('--owner', result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        self.assertIsNone(self.status()['current'])
        self.assertFalse(log_path(1).exists())

    def test_running_clock_start_is_idempotent(self):
        real = run.bashy
        card = {'id': 1, 'title': 'Class', 'boxes': [
            {'started_at': '2026-10-30T09:00:00Z', 'cutoff': '2026-10-30T10:30:00Z'}]}
        def runner(*args, **kwargs):
            if args[:2] == ('sprint', 'show'):
                return json.dumps({'t': 'node', 'kind': 'sprint', 'attrs': card}) + '\n'
            if args[:2] == ('sprint', 'start'):
                self.fail('already running sprint must not start again')
            return real(*args, **kwargs)
        with patch.object(run, 'bashy', side_effect=runner):
            self.invoke('start', '1', '--json')
            self.invoke('start', '1', '--json')
            status = json.loads(self.invoke('status', '1', '--json'))
            self.assertEqual(status['clock']['remaining_s'], 1800)
            self.assertEqual(status['clock']['state'], 'running')
            card['boxes'][0]['stopped_at'] = '2026-10-30T10:00:00Z'
            self.assertEqual(json.loads(self.invoke('status', '1', '--json'))['clock']['state'], 'stopped')
        self.assertEqual(len(log_path(1).read_text().splitlines()), 1)

    def test_invalid_step_and_duration_are_clean_errors(self):
        for args in [('done', '1', '99'), ('extend', '1', '1', '--by', '-5m')]:
            result = self.tutor(*args, '--json', check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn('Traceback', result.stderr)
        self.assertFalse(log_path(1).exists())

    def test_command_boundary_done_does_not_advance(self):
        # A command-boundary test of tutor independent of the installed bashy's
        # generic sprint closure policy; the real round trip above stays mandatory.
        real = run.bashy
        completed = set()
        calls = []
        def runner(*args, **kwargs):
            if args[:2] == ('todo', 'status') and args[-1] == 'done':
                calls.append(args)
                completed.add(args[2])
                return ''
            if args[:2] == ('todo', 'list'):
                items = run.records(real(*args, **kwargs), 'todo')
                for item in items:
                    if item['id'] in completed:
                        item['state'] = 'done'
                return json.dumps(items)
            return real(*args, **kwargs)
        with patch.object(run, 'bashy', side_effect=runner):
            self.invoke('next', '1', '--json')
            os.environ['TUTOR_NOW'] = '2026-10-30T10:02:00Z'
            value = json.loads(self.invoke('done', '1', '1', '-m', 'Shared', '--json'))
            self.assertIsNone(value['current'])
            self.assertEqual(value['steps'][1]['status'], 'assigned')
            self.assertEqual(value['steps'][0]['evidence'], 'Shared')
            self.invoke('next', '1', '--json')
            self.invoke('next', '1', '--json')
            self.assertEqual(self.invoke('status', '1', '--line'), 'Done · 2/2 steps\n')
        self.assertEqual(calls, [('todo', 'status', ident, 'done') for ident in self.ids])

    def test_exact_line_and_native_json_envelope(self):
        value = dict(current=dict(index=2, total=5, title='Prototype sketch',
                                 remaining_s=760, overdue=False, owners=['Ana', 'Ben']))
        self.assertEqual(run.one_line(value), 'Step 2/5 · Prototype sketch · 12:40 left · Ana, Ben')
        card = {'id': 1, 'title': 'Class', 'boxes': []}
        self.assertEqual(run.records(json.dumps({'result': {'sprint': card}}), 'sprint'), [card])

    def test_start_forwards_owner_and_duration(self):
        real = run.bashy
        calls = []
        def runner(*args, **kwargs):
            if args[:2] == ('sprint', 'start'):
                calls.append(args)
                return ''
            return real(*args, **kwargs)
        with patch.object(run, 'bashy', side_effect=runner):
            value = json.loads(self.invoke('start', '1', '--for', '45m', '--owner', 'teacher', '--json'))
        self.assertEqual(calls, [('sprint', 'start', '1', '--for', '45m', '--owner', 'teacher')])
        self.assertEqual(value['current']['id'], self.ids[0])

    def test_real_sprint_clock_projection(self):
        # Seed only this test's private store: no managed agent is launched.
        path = self.root / 'home' / 'sprint' / 'queue.json'
        board = json.loads(path.read_text())
        box = dict(started_at='2026-10-30T09:00:00Z', cutoff='2026-10-30T10:30:00Z')
        board['stories'][0]['boxes'] = [box]
        path.write_text(json.dumps(board))
        self.assertEqual(self.status()['clock'], dict(state='running',
                         deadline='2026-10-30T10:30:00Z', remaining_s=1800))
        box['stopped_at'] = '2026-10-30T10:00:00Z'
        path.write_text(json.dumps(board))
        self.assertEqual(self.status()['clock']['state'], 'stopped')

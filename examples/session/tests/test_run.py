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
        os.environ['SESSION_NOW'] = '2030-01-01T10:00:00Z'
        bashy('sprint', 'add', 'Session')
        bashy('person', 'add', 'alice')
        self.ids = []
        for i, title in enumerate(['Warmup', 'Prototype sketch'], 1):
            obj = bashy('todo', 'add', "--user", title, '--sprint', '1', '--owner', 'alice',
                        '--label', 'step-%02d,timebox-10m,role-participant' % i,
                        '--json', json_out=True)
            self.ids.append(obj['id'])

    def invoke(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            run.main(list(args))
        return out.getvalue()

    def status(self):
        return json.loads(self.session('status', '1', '--json').stdout)

    def test_not_started_and_filtering(self):
        bashy('todo', 'add', "--user", 'Unrelated', '--sprint', '1')
        status = self.status()
        self.assertEqual(set(status), {'sprint', 'title', 'clock', 'paused', 'current', 'steps', 'format', 'roles'})
        self.assertEqual(status['clock'], dict(state='not-started', deadline=None, remaining_s=None))
        self.assertIsNone(status['current'])
        self.assertEqual(len(status['steps']), 2)
        self.assertEqual(self.session('status', '1', '--line').stdout, 'Not started · 2 steps\n')

    def test_next_time_and_overdue(self):
        self.session('next', '1', '--json')
        os.environ['SESSION_NOW'] = '2030-01-01T10:13:10Z'
        status = self.status()
        self.assertEqual(status['current']['remaining_s'], -190)
        self.assertTrue(status['current']['overdue'])
        self.assertEqual(self.session('status', '1', '--line').stdout,
                         'Step 1/2 · Warmup · 3:10 over · alice\n')
        self.session('time', '1', '1', '--add', '5m', '--json')
        status = self.status()
        self.assertEqual(status['current']['timebox_s'], 900)
        self.assertEqual(status['current']['remaining_s'], 110)
        self.assertFalse(status['current']['overdue'])
        self.assertEqual(status['clock']['state'], 'not-started')
        events = [json.loads(line) for line in log_path(1).read_text().splitlines()]
        self.assertEqual(events[-1]['by_s'], 300)

    def at(self, hhmmss):
        os.environ['SESSION_NOW'] = '2030-01-01T%sZ' % hhmmss

    def order(self):
        return [(s['index'], s['title']) for s in self.status()['steps']]

    def test_pause_freezes_elapsed_and_resume_continues(self):
        self.session('next', '1')
        self.at('10:02:00')
        self.assertEqual(self.session('pause', '1', '--json').returncode, 0)
        self.at('10:30:00')
        status = self.status()
        self.assertTrue(status['paused'])
        self.assertEqual(status['current']['elapsed_s'], 120)
        self.assertEqual(self.session('status', '1', '--line').stdout, 'Paused · Step 1/2 · Warmup\n')
        self.session('pause', '1')
        self.session('resume', '1')
        self.at('10:31:00')
        status = self.status()
        self.assertFalse(status['paused'])
        self.assertEqual(status['current']['elapsed_s'], 180)
        self.assertEqual(status['current']['remaining_s'], 420)
        self.assertEqual([json.loads(l)['event'] for l in log_path(1).read_text().splitlines()].count('pause'), 1)

    def test_time_adds_and_subtracts_never_below_a_minute(self):
        self.session('next', '1')
        self.session('time', '1', '--add', '5m')
        self.assertEqual(self.status()['current']['timebox_s'], 900)
        self.session('time', '1', '--add', '-1m')
        self.assertEqual(self.status()['current']['timebox_s'], 840)
        self.session('time', '1', self.ids[0], '--add', '-1h')
        self.assertEqual(self.status()['current']['timebox_s'], 60)
        self.session('time', '1', '--add', '-1m')
        self.assertEqual(self.status()['current']['timebox_s'], 60)
        self.session('time', '1', '2', '--add', '2m')
        self.assertEqual(self.status()['steps'][1]['timebox_s'], 720)

    def test_goto_back_and_forward(self):
        self.session('next', '1')
        self.at('10:05:00')
        self.session('goto', '1', '2')
        status = self.status()
        self.assertEqual(status['current']['id'], self.ids[1])
        self.assertEqual(status['steps'][0]['status'], 'assigned')
        self.assertEqual(status['current']['elapsed_s'], 0)
        self.session('done', '1', '2')
        self.session('goto', '1', self.ids[0])
        status = self.status()
        self.assertEqual(status['current']['id'], self.ids[0])
        self.assertEqual(status['steps'][1]['status'], 'done')
        self.session('goto', '1', '2')
        status = self.status()
        self.assertEqual(status['current']['id'], self.ids[1])
        self.assertEqual(status['steps'][0]['status'], 'assigned')

    def test_skip_counts_as_finished_and_starts_next(self):
        self.session('next', '1')
        self.session('skip', '1')
        status = self.status()
        self.assertEqual(status['steps'][0]['status'], 'skipped')
        self.assertEqual(status['current']['id'], self.ids[1])
        self.session('done', '1', '2')
        self.assertEqual(self.session('status', '1', '--line').stdout, 'Done · 2/2 steps\n')
        self.assertIn('step-skipped', bashy('todo', 'list', '--user', '--all', '--json'))

    def test_move_up_down_to_rewrites_labels(self):
        bashy('todo', 'add', '--user', 'Third', '--sprint', '1', '--owner', 'alice',
              '--label', 'step-03,timebox-10m,role-participant')
        bashy('todo', 'add', '--user', 'Fourth', '--sprint', '1', '--owner', 'alice',
              '--label', 'step-04,timebox-10m,role-participant')
        self.session('next', '1')
        self.session('move', '1', '4', '--up')
        self.assertEqual(self.order(), [(1, 'Warmup'), (2, 'Prototype sketch'), (3, 'Fourth'), (4, 'Third')])
        self.session('move', '1', "3", '--down')
        self.assertEqual(self.order(), [(1, 'Warmup'), (2, 'Prototype sketch'), (3, 'Third'), (4, 'Fourth')])
        self.session('move', '1', '4', '--to', '2')
        self.assertEqual(self.order(), [(1, 'Warmup'), (2, 'Fourth'), (3, 'Prototype sketch'), (4, 'Third')])
        self.assertEqual(self.status()['current']['title'], 'Warmup')
        labels = bashy('todo', 'list', '--user', '--all', '--json')
        self.assertEqual(labels.count('"step-02"'), 1)
        self.assertEqual(self.session('move', '1', '1', '--up', check=False).returncode, 1)
        self.assertEqual(self.session('move', '1', '3', '--to', '1', check=False).returncode, 1)

    def test_next_done_round_trip(self):
        self.session('next', '1', '--json')
        os.environ['SESSION_NOW'] = '2030-01-01T10:02:00Z'
        self.session('next', '1', '--json')
        self.assertEqual(self.status()['current']['id'], self.ids[1])
        self.session('done', '1', self.ids[1], '-m', 'Sketch shared', '--json')
        self.assertIsNone(self.status()['current'])
        self.assertEqual(self.status()['steps'][1]['evidence'], 'Sketch shared')
        self.assertEqual(self.session('status', '1', '--line').stdout, 'Done · 2/2 steps\n')

    def test_progress_between_steps(self):
        self.session('next', '1')
        self.session('done', '1', '1')
        self.assertEqual(self.session('status', '1', '--line').stdout, '1/2 done\n')

    def test_invalid_step_and_duration_are_clean_errors(self):
        for args in [('done', '1', '99'), ('time', '1', '1', '--add', 'x')]:
            result = self.session(*args, '--json', check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn('Traceback', result.stderr)
        self.assertFalse(log_path(1).exists())

    def test_command_boundary_done_does_not_advance(self):
        # A command-boundary test of session independent of the installed bashy's
        # generic sprint closure policy; the real round trip above stays mandatory.
        real = run.bashy
        calls = []
        def runner(*args, **kwargs):
            if args[:2] in (('todo', 'status'), ('todo', 'edit')):
                calls.append(args)
            return real(*args, **kwargs)
        with patch.object(run, 'bashy', side_effect=runner):
            self.invoke('next', '1', '--json')
            os.environ['SESSION_NOW'] = '2030-01-01T10:02:00Z'
            value = json.loads(self.invoke('done', '1', '1', '-m', 'Shared', '--json'))
            self.assertIsNone(value['current'])
            self.assertEqual(value['steps'][1]['status'], 'assigned')
            self.assertEqual(value['steps'][0]['evidence'], 'Shared')
            self.invoke('next', '1', '--json')
            self.invoke('next', '1', '--json')
            self.assertEqual(self.invoke('status', '1', '--line'), 'Done · 2/2 steps\n')
        # People never close sprint stories; the session manager accepts them.
        self.assertNotIn('done', [c[-1] for c in calls if c[:2] == ('todo', 'status')])
        self.assertEqual([c for c in calls if c[:2] == ('todo', 'edit')],
                         [('todo', 'edit', "--user", ident, '--label', 'step-done') for ident in self.ids])

    def test_exact_line_and_native_json_envelope(self):
        value = dict(current=dict(index=2, total=5, title='Prototype sketch',
                                 remaining_s=760, overdue=False, owners=['alice', 'bob']))
        self.assertEqual(run.one_line(value), 'Step 2/5 · Prototype sketch · 12:40 left · alice, bob')
        card = {'id': 1, 'title': 'Session', 'boxes': []}
        self.assertEqual(run.records(json.dumps({'result': {'sprint': card}}), 'sprint'), [card])

    def test_clock_falls_back_to_plain_thread(self):
        # The JSON card omits boxes today; the plain thread carries the cutoff.
        plain = ("sprint #1 [doing] — Session\n"
                 "  [10-30 09:00] t (stage): started a 90m box (backlog → doing), cutoff 2030-01-01T10:30:00Z\n")
        self.assertEqual(run.thread_box(plain), {'cutoff': '2030-01-01T10:30:00Z'})
        self.assertEqual(run.thread_box(plain + "  [10-30 10:00] t (stage): stopped the box\n")['stopped_at'], 'thread')
        self.assertIsNone(run.thread_box("sprint #1 [backlog] — Session\n"))
        at = run.timestamp('2030-01-01T10:00:00Z')
        real = run.bashy
        thread = [plain]
        def runner(*args, **kwargs):
            if args[:2] == ('sprint', 'show') and '--plain' in args:
                return thread[0]
            return real(*args, **kwargs)
        with patch.object(run, 'bashy', side_effect=runner):
            self.assertEqual(run.session_clock({}, at, 1),
                             dict(state='running', deadline='2030-01-01T10:30:00Z', remaining_s=1800))
            thread[0] = "sprint #1 [doing] — Session\n"
            self.assertEqual(run.session_clock({}, at, 1)['state'], 'not-started')

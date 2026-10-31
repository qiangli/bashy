"""Registration and manager smoke regressions, all stores isolated."""
import json
from tests.helpers import TempBashy
from common import bashy
import run


class SmokeRegressions(TempBashy):
    def test_participant_progress_excludes_owner_steps(self):
        bashy('sprint', 'add', 'Session')
        ids = []
        for i, role in enumerate(['owner', 'participant', 'owner', 'participant',
                                   'participant', 'owner', 'participant', 'participant'], 1):
            ids.append(bashy('todo', 'add', "--user", 'Step %d' % i, '--sprint', '1',
                             '--label', 'step-%02d,timebox-10m,role-%s' % (i, role),
                             '--json', json_out=True)['id'])
        bashy('todo', 'status', "--user", ids[3], 'doing')
        value = run.status(1)
        self.assertEqual(len(value['steps']), 8)
        self.assertEqual(value['current']['total'], 5)
        self.assertEqual(value['current']['index'], 2)
        self.assertTrue(run.one_line(value).startswith('Step 2/5'))
        self.tutor('next', '1', '--json')
        self.assertEqual(run.status(1)['current']['id'], ids[4])

    def test_apply_nudge_done_when_roundtrip(self):
        draft = json.loads(self.tutor('plan', 'Session', '--people', 'alice', '--json').stdout)
        draft['steps'][0]['done_when'] = 'Sketch shared'
        result = json.loads(self.tutor('apply', '-', '--json', stdin=json.dumps(draft)).stdout)
        self.tutor('next', str(result['sprint']), '--json')
        hint = json.loads(self.tutor('nudge', str(result['sprint']), '--person', 'alice', '--json').stdout)
        self.assertEqual(hint['done_when'], 'Sketch shared')


class RegistrationTest(TempBashy):
    def test_server_build_ignores_unrelated_go_workspace(self):
        import os
        import subprocess
        from tests.helpers import HERE

        unrelated = self.root / 'unrelated'
        unrelated.mkdir()
        (unrelated / 'go.mod').write_text('module unrelated\n\ngo 1.22\n')
        workspace = self.root / 'go.work'
        workspace.write_text('go 1.22\n\nuse ./unrelated\n')
        cache = self.root / 'cache'
        env = dict(os.environ, GOWORK=str(workspace), XDG_CACHE_HOME=str(cache))
        env.pop('TUTOR_SERVER_BIN', None)
        # Missing sprint exits at argument validation after building, without
        # opening a listener or reading any session state.
        result = subprocess.run(['bash', str(HERE / 'server' / 'run.sh'), 'serve'],
                                env=env, text=True, capture_output=True, timeout=120)
        self.assertTrue((cache / 'bashy' / 'tutor' / 'tutor-server').is_file(),
                        result.stdout + result.stderr)
        self.assertIn('usage: tutor serve N', result.stderr)
        self.assertNotIn('cannot build the server', result.stderr)

    def test_join_message_forwarding(self):
        import os
        import subprocess
        from tests.helpers import HERE

        os.environ['BASHY_APPS_DIR'] = str(self.root / 'apps')
        stub = self.root / 'join-stub'
        stub.write_text('#!/usr/bin/env python3\n'
                        'import json, sys\n'
                        'print(json.dumps(sys.argv[1:]))\n')
        stub.chmod(0o755)
        os.environ['TUTOR_SERVER_BIN'] = str(stub)
        registered = subprocess.run(['bash', str(HERE / 'register.sh')],
                                    text=True, capture_output=True, timeout=30)
        self.assertEqual(registered.returncode, 0, registered.stdout + registered.stderr)
        url = 'http://127.0.0.1:1/s/x'
        for flags in [('-m', 'ev'), ('--message=ev',), ('--message', 'ev')]:
            for registered_command in (True, False):
                with self.subTest(flags=flags, registered=registered_command):
                    command = (['bashy', 'tutor'] if registered_command else
                               ['bashy', '--bashsharp', str(HERE / 'tutor.bsh')])
                    result = subprocess.run(command + ['join', url, '--done', *flags],
                                            text=True, capture_output=True, timeout=30)
                    self.assertNotIn('flag provided but not defined', result.stderr)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    args = json.loads(result.stdout)
                    self.assertEqual(args[:2], ['join', url])
                    self.assertTrue('--done' in args or '--done=true' in args)
                    self.assertIn('-m', args)
                    self.assertEqual(args[args.index('-m') + 1], 'ev')
                    self.assertFalse(any(arg.startswith('--message') for arg in args))

    def test_real_registration_duality_and_rehearsal(self):
        import subprocess
        import sys
        from tests.helpers import HERE
        result = subprocess.run([sys.executable, str(HERE / 'verify-duality.py')],
                                text=True, capture_output=True, timeout=180)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('PASS none:', result.stdout)
        self.assertIn('PASS stub:', result.stdout)

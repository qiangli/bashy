#!/usr/bin/env python3
"""Short isolated development smoke. Requires existing BASHY_BIN and ADAPTER.
No inference/downloads; no default user registry mutations. Exercises the real
selector, explicit override, ambiguity and ordinary stage execution.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import wave

HERE = Path(__file__).resolve().parent

class Registration(unittest.TestCase):
    def test_registration_selection_execution_and_ambiguity(self):
        bashy = Path(os.environ['BASHY_BIN']).resolve(strict=True)
        helper = Path(os.environ['ADAPTER']).resolve(strict=True)
        with tempfile.TemporaryDirectory(prefix='webm registration ') as tmp:
            root = Path(tmp)
            env = dict(os.environ, BASHY_HOME=str(root/'home'),
                       BASHY_COMMANDS_DIR=str(root/'commands'),
                       BASHY_SKILLS_DIR=str(root/'skills'), BASHY_HINTS='off',
                       BASHY_BIN=str(bashy))
            def run(*args, ok=True):
                p = subprocess.run([str(bashy), *map(str,args)], env=env,
                                   text=True, capture_output=True, timeout=15)
                self.assertEqual(p.returncode == 0, ok, p.stdout+p.stderr)
                return p
            exe = root/'adapter ; 日本'
            shutil.copy2(helper,exe)
            run(HERE/'register.sh',exe)
            record = json.loads(run('commands','show','transcribe-webm-opus','--json').stdout)
            self.assertEqual(record['exec'],[str(exe)])
            self.assertEqual(record['provides'],['transcribe.decode'])
            self.assertEqual(set(record['effects']),{'read','write','destroy','exec'})
            work = root/'work'
            work.mkdir()
            (work/'unrelated').write_text('keep')
            req = root/'request.json'
            source = HERE/'testdata'/'speech.webm'
            digest = hashlib.sha256(source.read_bytes()).hexdigest()
            req.write_text(json.dumps(dict(version='bashy-transcribe-stage-v1', stage='decode',
                                          work_dir=str(work),artifacts=[dict(path=str(source),sha256=digest)])))
            self.assertEqual(run('transcribe','select','decode',req).stdout.strip(),'transcribe-webm-opus')
            result = json.loads(run('transcribe','stage','decode','--provider','transcribe-webm-opus',req).stdout)
            artifact = result['artifacts'][0]
            self.assertEqual(artifact['sha256'],hashlib.sha256(Path(artifact['path']).read_bytes()).hexdigest())
            with wave.open(artifact['path'],'rb') as wav:
                self.assertEqual((wav.getframerate(),wav.getnchannels(),wav.getsampwidth(),wav.getnframes()),(16000,1,2,31896))
            self.assertEqual((work/'unrelated').read_text(),'keep')
            self.assertEqual(digest,hashlib.sha256(source.read_bytes()).hexdigest())
            run('commands','add','another-webm','--set','exec.0='+str(exe),
                '--set','provides.0=transcribe.decode','--set','effects.0=exec',
                '--set','accepts.0.path=/artifacts/0/path','--set',r'accepts.0.pattern=(?i)\.webm$')
            self.assertIn('ambiguous',run('transcribe','select','decode',req,ok=False).stderr)
            self.assertEqual(run('transcribe','select','decode','--provider','transcribe-webm-opus',req).stdout.strip(),'transcribe-webm-opus')
            obj=json.loads(req.read_text());obj['artifacts'][0]['path']='unsupported.mp4';req.write_text(json.dumps(obj))
            run('transcribe','select','decode','--provider','transcribe-webm-opus',req,ok=False)
            obj['artifacts'][0]['path']=str(source)
            del obj['work_dir']
            req.write_text(json.dumps(obj))
            run('transcribe','select','decode','--provider','transcribe-webm-opus',req,ok=False)

if __name__ == '__main__':
    unittest.main(verbosity=2)

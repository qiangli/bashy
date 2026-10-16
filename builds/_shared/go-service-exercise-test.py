#!/usr/bin/env python3
"""Bounded mock tests: never start a service or change firewall policy."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch, Mock

spec = importlib.util.spec_from_file_location("exercise", Path(__file__).with_name("go-service-exercise.py"))
exercise = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exercise)
BUILDS = Path(__file__).resolve().parent.parent


class ServiceExercise(unittest.TestCase):
    def test_configuration_preserves_no_tracking_without_ambient_secrets(self):
        for tool, port in (("gitea", 13880), ("seaweedfs", 15333), ("zot", 15000)):
            with self.subTest(tool=tool), tempfile.TemporaryDirectory() as temp:
                work = Path(temp)
                with patch.dict(exercise.os.environ, {"HTTP_PROXY": "bad", "SECRET_TOKEN": "secret"}):
                    argv, env, urls = exercise.configuration(tool, "service.exe", work, port, BUILDS/tool/"files")
                self.assertEqual(env["HTTP_PROXY"], "")
                self.assertNotIn("SECRET_TOKEN", env)
                self.assertEqual(env["OTEL_SDK_DISABLED"], "true")
                self.assertTrue(all(url.startswith("http://127.0.0.1:") for url in urls))
                if tool == "gitea":
                    text = (work/"app.ini").read_text()
                    self.assertNotIn("{{", text)
                    self.assertIn("[cron.update_checker]\n", text)
                    self.assertIn("PROXY_ENABLED = false", text)
                elif tool == "seaweedfs":
                    self.assertIn("-master.telemetry=false", argv)
                    self.assertIn("-ip.bind=127.0.0.1", argv)
                else:
                    config = json.loads((work/"config.json").read_text())
                    self.assertEqual(config["extensions"], {})
                    self.assertEqual(config["http"]["address"], "127.0.0.1")

    def test_victoria_exercises_health_query_and_post_ingest(self):
        for tool in ("victoria-logs", "victoria-metrics", "victoria-traces"):
            with self.subTest(tool=tool), tempfile.TemporaryDirectory() as temp:
                work = Path(temp)
                proc = Mock()
                proc.poll.return_value = None
                with patch.object(exercise.subprocess, "Popen", return_value=proc) as start, \
                     patch.object(exercise, "request", return_value=200) as probe, \
                     patch.object(exercise.time, "sleep") as sleep:
                    exercise.exercise(tool, "service.exe", work, 19428, BUILDS/tool/"files")
                argv = start.call_args.args[0]
                self.assertIn("--httpListenAddr=127.0.0.1:19428", argv)
                self.assertFalse(any("pushmetrics" in arg for arg in argv))
                env = start.call_args.kwargs["env"]
                self.assertEqual(env["OTEL_SDK_DISABLED"], "true")
                self.assertEqual(env["HTTP_PROXY"], "")
                posts = [call.args[0] for call in probe.call_args_list
                         if isinstance(call.args[0], exercise.urllib.request.Request)]
                self.assertEqual(len(posts), 2)
                self.assertTrue(all(req.method == "POST" and req.data for req in posts))
                self.assertTrue(all(req.full_url.startswith("http://127.0.0.1:19428/")
                                    for req in posts))
                sleep.assert_called_once_with(30)
                proc.terminate.assert_called_once()
                proc.wait.assert_called_once()

    def run_mock(self, proc, statuses):
        with tempfile.TemporaryDirectory() as temp:
            work = Path(temp)
            with patch.object(exercise.subprocess, "Popen", return_value=proc), \
                 patch.object(exercise, "request", side_effect=statuses), \
                 patch.object(exercise.time, "sleep"):
                exercise.exercise("zot", "service.exe", work, 15000, BUILDS/"zot/files")

    def test_dead_service_fails_and_is_reaped(self):
        proc = Mock()
        proc.poll.return_value = 1
        with self.assertRaisesRegex(RuntimeError, "before readiness"):
            self.run_mock(proc, [])
        proc.terminate.assert_called_once()
        proc.wait.assert_called_once()

    def test_probe_failure_fails_and_is_reaped(self):
        proc = Mock()
        proc.poll.return_value = None
        with self.assertRaises(OSError):
            self.run_mock(proc, [200, OSError("connection lost")])
        proc.terminate.assert_called_once()

    def test_success_observes_then_reaps_even_if_termination_hangs(self):
        proc = Mock()
        proc.poll.return_value = None
        proc.wait.side_effect = [subprocess.TimeoutExpired("service", 15), 0]
        self.run_mock(proc, [200] * 9)
        proc.kill.assert_called_once()
        self.assertEqual(proc.wait.call_count, 2)

    def test_failed_exercise_never_publishes_pass(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture, evidence = Path(temp)/"service.txt", Path(temp)/"evidence.json"
            fixture.write_text("zot")
            with patch.object(exercise.sys, "argv", ["gate", "service.exe", str(fixture), str(evidence), "15000"]), \
                 patch.object(exercise, "exercise", side_effect=RuntimeError("failed")):
                with self.assertRaises(RuntimeError):
                    exercise.main()
            self.assertEqual(json.loads(evidence.read_text())["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()

#!/usr/bin/env python3
"""Offline contract regressions. HTTP fixtures are not inference evidence."""
import contextlib
import hashlib
import http.server
import importlib.util
import json
from pathlib import Path
import socket
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
FILES = Path(__file__).resolve().parent / 'files'


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, FILES / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


gate = module('gate', 'native-gate.py')
fixture = module('fixture', 'fetch-fixture.py')


@contextlib.contextmanager
def server(frames, chunked=True, status=200):
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'

        def do_POST(self):
            self.rfile.read(int(self.headers['Content-Length']))
            self.send_response(status)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Transfer-Encoding', 'chunked' if chunked else 'identity')
            self.end_headers()
            try:
                for frame in frames:
                    data = (json.dumps(frame) + '\n').encode()
                    self.wfile.write(f'{len(data):x}\r\n'.encode() + data + b'\r\n')
                    self.wfile.flush()
                self.wfile.write(b'0\r\n\r\n')
            except (BrokenPipeError, ConnectionResetError):
                pass

        def handle(self):
            try:
                super().handle()
            except ConnectionResetError:
                pass  # expected when the client rejects a malformed response

        def log_message(self, *_args):
            pass

    with http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler) as httpd:
        thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        thread.start()
        try:
            yield httpd.server_port
        finally:
            httpd.shutdown()
            thread.join()


class NativeContract(unittest.TestCase):
    def test_stream_requires_tokens_and_terminal_counts(self):
        frames = [{'content': 'Once'}, {'content': ' upon'},
                  {'done': True, 'eval_count': 2, 'prompt_eval_count': 3}]
        with server(frames) as port:
            self.assertEqual(gate.stream(port)['frames'], frames)

    def test_empty_stream_rejected(self):
        with server([]) as port, self.assertRaisesRegex(RuntimeError, 'missing'):
            gate.stream(port)

    def test_terminal_without_content_rejected(self):
        with server([{'done': True}]) as port, self.assertRaisesRegex(RuntimeError, 'no streamed'):
            gate.stream(port)

    def test_missing_counts_rejected(self):
        with server([{'content': 'x'}, {'done': True}]) as port:
            with self.assertRaisesRegex(RuntimeError, 'evaluation counts'):
                gate.stream(port)

    def test_nonstreaming_response_rejected(self):
        with server([], chunked=False) as port, self.assertRaisesRegex(RuntimeError, 'not streamed'):
            gate.stream(port)

    def test_http_error_rejected(self):
        with server([], status=503) as port, self.assertRaisesRegex(RuntimeError, 'HTTP 503'):
            gate.stream(port)

    def test_cancel_closes_before_terminal(self):
        with server([{'content': 'a'}]) as port:
            self.assertFalse(gate.stream(port, cancel=True)['terminal_received'])

    def test_completed_request_is_not_cancellation(self):
        with server([{'done': True}]) as port:
            with self.assertRaisesRegex(RuntimeError, 'ended before cancellation'):
                gate.stream(port, cancel=True)

    def test_cancel_has_no_eos_or_token_limit(self):
        body = gate.completion_body(cancel=True)
        self.assertEqual(body['Grammar'], 'root ::= "a" root')
        self.assertEqual(body['Options']['num_predict'], -1)
        self.assertTrue(body['Shift'])

    def test_fixture_hash_size_and_magic_fail_closed(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / 'fixture'
            data = b'GGUFtest'
            path.write_bytes(data)
            spec = {'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()}
            fixture.verify(path, spec)
            for bad in [b'GGUFtesx', data + b'x', b'NOPEtest']:
                path.write_bytes(bad)
                with self.assertRaises(RuntimeError):
                    fixture.verify(path, spec)
            path.unlink()
            with self.assertRaises(FileNotFoundError):
                fixture.verify(path, spec)

    def test_network_denial_requires_permission_error(self):
        for error in [PermissionError(1, 'denied'), PermissionError(13, 'denied')]:
            with patch.object(socket.socket, 'connect', side_effect=error), \
                 patch.object(socket.socket, 'sendto', side_effect=error):
                gate.deny_controls(18997)
        for error in [ConnectionRefusedError(61, 'refused'), TimeoutError('timeout')]:
            with patch.object(socket.socket, 'connect', side_effect=error):
                with self.assertRaises(RuntimeError):
                    gate.deny_controls(18997)
        with patch.object(socket.socket, 'connect', return_value=None):
            with self.assertRaisesRegex(RuntimeError, 'escaped'):
                gate.deny_controls(18997)

    def test_embedding_rejects_empty_zero_nan_and_wrong_types(self):
        for vector in [[], [0, 0], [float('nan')], [float('inf')], ['1'], [True]]:
            with self.subTest(vector=vector), self.assertRaises(RuntimeError):
                gate.validate_embedding({'embedding': vector, 'prompt_eval_count': 1})
        with self.assertRaises(RuntimeError):
            gate.validate_embedding({'embedding': [1]})
        gate.validate_embedding({'embedding': [0.1, -0.3], 'prompt_eval_count': 2})

    def test_bind_owner_rejects_wrong_pid_endpoint_wildcard(self):
        for output in ['p99\nn*:18997\n', 'p99\nn127.0.0.1:8080\n', 'p99\n']:
            with patch.object(gate.subprocess, 'check_output', return_value=output):
                with self.assertRaises(RuntimeError):
                    gate.check_owner(99, 18997)

    def test_final_verdict_requires_both_gates(self):
        with tempfile.TemporaryDirectory() as root:
            path, log = Path(root) / 'evidence', Path(root) / 'network'
            evidence = {'status': 'PROTOCOL_PASS', 'checks': dict.fromkeys(gate.CHECKS, True)}
            path.write_text(json.dumps(evidence))
            with self.assertRaises(FileNotFoundError):
                gate.finalize(path, log)
            log.write_text('')
            with self.assertRaises(RuntimeError):
                gate.finalize(path, log)
            log.write_text('network proof')
            for status in ['FAIL', 'PASS', 'missing']:
                path.write_text(json.dumps(dict(evidence, status=status)))
                with self.assertRaises(RuntimeError):
                    gate.finalize(path, log)
            missing = dict(evidence, checks={})
            path.write_text(json.dumps(missing))
            with self.assertRaises(RuntimeError):
                gate.finalize(path, log)
            path.write_text(json.dumps(evidence))
            gate.finalize(path, log)
            result = json.loads(path.read_text())
            self.assertEqual(result['status'], 'PASS')
            self.assertEqual(result['network_evidence_sha256'], gate.digest(log))


if __name__ == '__main__':
    unittest.main(verbosity=2)

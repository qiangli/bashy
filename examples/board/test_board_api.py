"""Pinned-route contract tests; no fork, containers, or credentials required."""
import csv
import io
import json
import os
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import patch

import board_api as api


class BoardTests(unittest.TestCase):
    def invoke(self, args, status=200, raw=b'{"id":"result"}'):
        calls = []
        def transport(*request):
            calls.append(request)
            return status, raw
        out = io.StringIO()
        with patch.dict(os.environ, {'BOARD_URL': 'http://127.0.0.1:22751/board/api', 'BOARD_TOKEN': 'local-test-token'}):
            rc = api.run(args, out, transport)
        return rc, out.getvalue(), calls

    def test_pinned_routes_and_bodies(self):
        cases = [
            (['columns', 'list', '--project', 'p'], 'GET', '/column/p', None),
            (['columns', 'add', '--project', 'p', '--name', 'Done', '--final', 'true'], 'POST', '/column/p', {'name': 'Done', 'isFinal': True}),
            (['comments', 'list', 't'], 'GET', '/comment/t', None),
            (['projects', 'list', '--workspace', 'w'], 'GET', '/project/?workspaceId=w', None),
            (['projects', 'add', '--workspace', 'w', '--name', 'Board', '--icon', 'X', '--slug', 'BRD'], 'POST', '/project/', {'workspaceId': 'w', 'name': 'Board', 'icon': 'X', 'slug': 'BRD'}),
            (['projects', 'show', 'p'], 'GET', '/project/p', None),
            (['projects', 'set', 'p', '--name', 'Board', '--icon', 'X', '--slug', 'BRD', '--description', '', '--public', 'false'], 'PUT', '/project/p', {'name': 'Board', 'icon': 'X', 'slug': 'BRD', 'description': '', 'isPublic': False}),
            (['projects', 'rm', 'p'], 'DELETE', '/project/p', None),
            (['tasks', 'list', '--project', 'p', '--assignee', 'u', '--page', '2', '--limit', '10'], 'GET', '/task/tasks/p?page=2&limit=10&assigneeId=u', None),
            (['tasks', 'add', '--project', 'p', '--title', 'Card', '--description', '', '--priority', 'low', '--status', 'planned'], 'POST', '/task/p', {'title': 'Card', 'description': '', 'priority': 'low', 'status': 'planned'}),
            (['tasks', 'show', 't'], 'GET', '/task/t', None),
            (['tasks', 'set', 't', '--title', 'Card', '--priority', 'high', '--status', 'done', '--project', 'p', '--position', '0'], 'PUT', '/task/t', {'title': 'Card', 'priority': 'high', 'status': 'done', 'projectId': 'p', 'position': 0}),
            (['tasks', 'move', 't', '--column', 'done'], 'PUT', '/task/status/t', {'status': 'done'}),
            (['tasks', 'move', 't', '--status', 'planned'], 'PUT', '/task/status/t', {'status': 'planned'}),
            (['tasks', 'rm', 't'], 'DELETE', '/task/t', None),
            (['comments', 'add', 't', '--content', 'hello'], 'POST', '/comment/t', {'content': 'hello'}),
            (['export', 'p', '--format', 'json'], 'GET', '/task/export/p', None),
        ]
        for args, method, path, body in cases:
            with self.subTest(args=args):
                rc, output, calls = self.invoke(args + ['--json'])
                self.assertEqual(rc, 0, output)
                self.assertEqual(calls, [('http://127.0.0.1:22751/board/api' + path, method, body, 'local-test-token')])
                self.assertEqual(json.loads(output), {'id': 'result'})
                if method != 'GET':
                    rc, output, calls = self.invoke(args + ['--dry-run'])
                    self.assertEqual(rc, 0, output)
                    self.assertEqual(calls, [])
                    self.assertEqual(json.loads(output), {'dryRun': True, 'method': method, 'url': 'http://127.0.0.1:22751/board/api' + path, 'body': body})

    def test_narrow_edits_and_clearing(self):
        for flag, value, path, body in [
            ('title', 'new', 'title', {'title': 'new'}),
            ('description', '', 'description', {'description': ''}),
            ('priority', 'urgent', 'priority', {'priority': 'urgent'}),
            ('status', 'done', 'status', {'status': 'done'}),
            ('assignee', '', 'assignee', {'userId': None}),
            ('due-date', '', 'due-date', {}),
        ]:
            with self.subTest(flag=flag):
                rc, _, calls = self.invoke(['tasks', 'set', 't', '--' + flag, value])
                self.assertEqual(rc, 0)
                self.assertEqual(calls[0][0].split('/api')[1], '/task/' + path + '/t')
                self.assertEqual(calls[0][2], body)

    def test_invalid_input_never_calls_api(self):
        cases = [
            ['projects', 'set', 'p', '--name', 'missing replacement fields'],
            ['tasks', 'set', 't'], ['tasks', 'set', 't', '--title', 'x', '--priority', 'high'],
            ['tasks', 'move', '../x', '--status', 'done'],
            ['tasks', 'move', 't', '--column', 'x', '--status', 'y'],
            ['projects', 'show', 'p', '--dry-run'], ['projects', 'show', 'p', '--format', 'csv'],
            ['tasks', 'list', '--project', 'p', '--limit', '101'],
            ['tasks', 'list', '--project', 'p', '--page', 'oops'],
            ['tasks', 'set', 't', '--priority', 'bogus'],
            ['comments', 'add', 't', '--content', ''],
            ['projects', 'show', 'p', '--workspace', 'ignored'],
            ['export', 'p', 'extra'], ['tasks', 'list', 'p'],
            ['tasks', 'show', 't', '--js'],
        ]
        for args in cases:
            with self.subTest(args=args):
                rc, output, calls = self.invoke(args)
                self.assertEqual(rc, 2, output)
                self.assertEqual(calls, [])
                self.assertEqual(json.loads(output)['code'], 'invalid_argument')

    def test_local_only_and_credential_free_dry_run(self):
        for url in ['https://example.invalid/api', 'http://localhost:80/api', 'http://127.0.0.1/api', 'http://user@127.0.0.1:80/api', 'http://127.0.0.1:80/api?x=1', 'http://127.0.0.1:80/%2e%2e/api', 'http://127.0.0.1:80/\napi']:
            rc, _, calls = self.invoke(['projects', 'show', 'p', '--url', url])
            self.assertEqual((rc, calls), (2, []), url)
        with patch.dict(os.environ, {}, clear=True):
            out = io.StringIO()
            rc = api.run(['tasks', 'rm', 't', '--dry-run', '--url', 'http://[::1]:8080/api'], out, lambda *args: self.fail('dry-run contacted API'))
            self.assertEqual(rc, 0)
            self.assertNotIn('token', out.getvalue())
            out = io.StringIO()
            self.assertEqual(api.run(['tasks', 'show', 't', '--url', 'http://127.0.0.1:8080/api'], out), 1)
            self.assertEqual(json.loads(out.getvalue())['code'], 'unauthenticated')

    def test_denials_pass_through_without_retry(self):
        for status, raw in [(401, b'Unauthorized'), (403, b'{"message":"Insufficient permissions","details":{"resource":"task"}}'), (404, b'{"message":"Task not found"}'), (409, b'Conflict')]:
            rc, output, calls = self.invoke(['tasks', 'move', 't', '--status', 'done', '--json'], status, raw)
            self.assertEqual(rc, 1)
            self.assertEqual(output, raw.decode() + '\n')
            self.assertEqual(len(calls), 1)

    def test_export_csv_and_json_override(self):
        response = {'project': {'name': 'Board'}, 'tasks': [{'title': 'comma, quote"\n漢字', 'description': 'two\nlines', 'status': 'done', 'labels': [{'name': 'a,b', 'color': 'red'}]}]}
        raw = json.dumps(response).encode()
        rc, output, calls = self.invoke(['export', 'p', '--format', 'csv'], raw=raw)
        self.assertEqual(rc, 0)
        rows = list(csv.DictReader(io.StringIO(output)))
        self.assertEqual(rows[0]['title'], response['tasks'][0]['title'])
        self.assertEqual(json.loads(rows[0]['labels']), response['tasks'][0]['labels'])
        self.assertEqual(len(calls), 1)
        rc, output, _ = self.invoke(['export', 'p', '--format', 'csv', '--json'], raw=raw)
        self.assertEqual(json.loads(output), response)
        rc, output, _ = self.invoke(['tasks', 'list', '--project', 'p'], raw=b'[]')
        self.assertEqual((rc, json.loads(output)), (0, []))
        rc, output, _ = self.invoke(['export', 'p', '--format', 'csv'], raw=b'{}')
        self.assertEqual((rc, json.loads(output)['code']), (1, 'data_loss'))
        rc, output, _ = self.invoke(['tasks', 'show', 't'], raw=b'<html>login</html>')
        self.assertEqual((rc, json.loads(output)['code']), (1, 'data_loss'))

    def test_real_transport_no_redirect_or_proxy(self):
        requests = []
        class Handler(BaseHTTPRequestHandler):
            def do_PUT(self):
                requests.append((self.path, self.headers.get('Authorization'), json.loads(self.rfile.read(int(self.headers['Content-Length'])))))
                self.send_response(302)
                self.send_header('Location', '/unexpected')
                self.end_headers()
                self.wfile.write(b'{"message":"redirect refused"}')
            def log_message(self, *args):
                pass
        server = HTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            with patch.dict(os.environ, {'HTTP_PROXY': 'http://127.0.0.1:1', 'http_proxy': 'http://127.0.0.1:1', 'NO_PROXY': '', 'no_proxy': ''}):
                status, raw = api.call('http://127.0.0.1:' + str(server.server_port) + '/api/task/status/t', 'PUT', {'status': 'done'}, 'local-test-token')
            self.assertEqual(status, 302)
            self.assertEqual(json.loads(raw)['message'], 'redirect refused')
            self.assertEqual(requests, [('/api/task/status/t', 'Bearer local-test-token', {'status': 'done'})])
        finally:
            server.shutdown()
            thread.join()
            server.server_close()


if __name__ == '__main__':
    unittest.main()

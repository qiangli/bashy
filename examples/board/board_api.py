"""Local API client for board.bsh; stdlib only, no database or identity store."""
import argparse
import csv
import ipaddress
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request


class UsageError(Exception):
    pass


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise UsageError(message)


FIELDS = {
    'workspace': 'workspaceId', 'project': 'projectId', 'name': 'name',
    'icon': 'icon', 'slug': 'slug', 'description': 'description',
    'public': 'isPublic', 'include-subtasks': 'includeSubtasks', 'title': 'title',
    'status': 'status', 'priority': 'priority', 'assignee': 'userId',
    'start-date': 'startDate', 'due-date': 'dueDate', 'position': 'position',
    'content': 'content', 'color': 'color', 'final': 'isFinal', 'include-archived': 'includeArchived', 'page': 'page',
    'limit': 'limit', 'sort-by': 'sortBy', 'sort-order': 'sortOrder',
    'due-before': 'dueBefore', 'due-after': 'dueAfter', 'related-page': 'relatedPage',
}
PROJECT_CREATE = {'workspace', 'name', 'icon', 'slug'}
PROJECT_UPDATE = {'name', 'icon', 'slug', 'description', 'public'}
TASK_CREATE = {'title', 'description', 'priority', 'status'}
TASK_UPDATE = {'title', 'priority', 'status', 'project', 'position'}
TASK_OPTIONAL = {'description', 'start-date', 'due-date', 'assignee'}
FILTERS = {'status', 'priority', 'assignee', 'page', 'limit', 'related-page', 'sort-by', 'sort-order', 'due-before', 'due-after'}
NARROW = {'title': 'title', 'description': 'description', 'priority': 'priority',
          'assignee': 'assignee', 'due-date': 'due-date', 'status': 'status'}


def parse(argv):
    p = Parser(prog='board', allow_abbrev=False)
    p.add_argument('noun', choices=['projects', 'tasks', 'comments', 'columns', 'export'])
    p.add_argument('verb')
    p.add_argument('item', nargs='?')
    p.add_argument('--json', action='store_true')
    p.add_argument('--dry-run', action='store_true')
    p.add_argument('--url', default=os.environ.get('BOARD_URL', ''))
    p.add_argument('--format', choices=['json', 'csv'])
    for name in FIELDS:
        p.add_argument('--' + name, dest=name)
    p.add_argument('--column')
    a = p.parse_args(argv)
    if a.noun == 'export':
        if a.item is not None:
            raise UsageError('export takes one project ID')
        a.item, a.verb = a.verb, 'export'
    valid = {'projects': {'list', 'add', 'show', 'set', 'rm'},
             'tasks': {'list', 'add', 'show', 'set', 'move', 'rm'},
             'comments': {'add', 'list'}, 'columns': {'add', 'list'}, 'export': {'export'}}
    if a.verb not in valid[a.noun]:
        raise UsageError('unknown ' + a.noun + ' verb')
    if a.verb in ('list', 'add') and a.noun != 'comments':
        if a.item is not None:
            raise UsageError(a.verb + ' does not take an ID; use --workspace or --project')
    elif not a.item or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', a.item):
        raise UsageError('a stable app ID is required (not a URL or ticket number)')
    if a.column is not None:
        if a.noun != 'tasks' or a.verb != 'move' or a.status is not None:
            raise UsageError('--column is an alias for move --status; supply only one')
        a.status = a.column
    allowed = {
        ('projects', 'list'): {'workspace', 'include-archived'},
        ('projects', 'add'): PROJECT_CREATE,
        ('projects', 'set'): PROJECT_UPDATE | {'include-subtasks'},
        ('tasks', 'list'): FILTERS | {'project'},
        ('tasks', 'add'): TASK_CREATE | TASK_OPTIONAL | {'project'},
        ('tasks', 'set'): TASK_UPDATE | TASK_OPTIONAL,
        ('tasks', 'move'): {'status'},
        ('comments', 'add'): {'content'},
        ('columns', 'list'): {'project'},
        ('columns', 'add'): {'project', 'name', 'icon', 'color', 'final'},
    }.get((a.noun, a.verb), set())
    values = {k: getattr(a, k) for k in FIELDS if getattr(a, k) is not None}
    if values.keys() - allowed:
        raise UsageError('unexpected fields: ' + ', '.join(sorted(values.keys() - allowed)))
    if a.format is not None and a.noun != 'export':
        raise UsageError('--format is only valid for export')
    if a.dry_run and a.verb not in ('add', 'set', 'move', 'rm'):
        raise UsageError('--dry-run is only valid for writes')
    required = {('projects', 'list'): {'workspace'}, ('projects', 'add'): PROJECT_CREATE,
                ('projects', 'set'): PROJECT_UPDATE, ('tasks', 'list'): {'project'},
                ('tasks', 'add'): TASK_CREATE | {'project'}, ('tasks', 'move'): {'status'},
                ('columns', 'list'): {'project'}, ('columns', 'add'): {'project', 'name'},
                ('comments', 'add'): {'content'}}.get((a.noun, a.verb), set())
    if a.noun == 'tasks' and a.verb == 'set':
        if len(values) != 1 or not values.keys() <= NARROW.keys():
            required = TASK_UPDATE
    if required - values.keys():
        raise UsageError('required fields: ' + ', '.join('--' + k for k in sorted(required - values.keys())))
    for field in ('workspace', 'project'):
        if field in values and not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', values[field]):
            raise UsageError('invalid --' + field + ' ID')
    for field in ('public', 'include-subtasks', 'include-archived', 'final'):
        if field in values:
            if values[field] not in ('true', 'false'):
                raise UsageError('--' + field + ' expects true or false')
            if field != 'include-archived':
                values[field] = values[field] == 'true'
    for field, low, high in [('page', 1, 1000000), ('related-page', 1, 1000000), ('limit', 1, 100), ('position', 0, 2147483646)]:
        if field in values:
            try:
                values[field] = int(values[field])
                if not low <= values[field] <= high:
                    raise ValueError()
            except ValueError:
                raise UsageError('--' + field + ' is out of range')
    if 'priority' in values and values['priority'] not in ('no-priority', 'low', 'medium', 'high', 'urgent'):
        raise UsageError('invalid priority')
    if 'sort-by' in values and values['sort-by'] not in ('createdAt', 'priority', 'dueDate', 'position', 'title', 'number'):
        raise UsageError('invalid sort field')
    if 'sort-order' in values and values['sort-order'] not in ('asc', 'desc'):
        raise UsageError('invalid sort order')
    if 'content' in values and not 1 <= len(values['content']) <= 10000:
        raise UsageError('comment content must contain 1..10000 characters')
    return a, values


def plan(a, v):
    body = {FIELDS[k]: value for k, value in v.items()}
    if a.noun == 'export':
        return 'GET', '/task/export/' + a.item, None
    if a.noun == 'columns':
        project = body.pop('projectId')
        return ('GET' if a.verb == 'list' else 'POST'), '/column/' + project, (None if a.verb == 'list' else body)
    if a.noun == 'comments' and a.verb == 'list':
        return 'GET', '/comment/' + a.item, None
    base = {'projects': '/project', 'tasks': '/task', 'comments': '/comment'}[a.noun]
    if a.verb == 'list':
        if a.noun == 'tasks':
            base += '/tasks/' + body.pop('projectId')
            if 'userId' in body:
                body['assigneeId'] = body.pop('userId')
        else:
            base += '/'
        return 'GET', base + '?' + urllib.parse.urlencode(body), None
    if a.verb in ('show', 'rm'):
        return ('GET' if a.verb == 'show' else 'DELETE'), base + '/' + a.item, None
    if a.verb == 'add':
        suffix = body.pop('projectId') if a.noun == 'tasks' else (a.item or '')
        return 'POST', base + '/' + suffix, body
    if a.verb == 'move':
        return 'PUT', '/task/status/' + a.item, body
    if a.noun == 'tasks' and len(v) == 1 and v.keys() <= NARROW.keys():
        key = next(iter(v))
        if key == 'assignee' and v[key] == '':
            body['userId'] = None
        if key == 'due-date' and v[key] == '':
            body = {}
        return 'PUT', '/task/' + NARROW[key] + '/' + a.item, body
    return 'PUT', base + '/' + a.item, body


def local_url(value):
    try:
        u = urllib.parse.urlsplit(value)
        if (u.scheme not in ('http', 'https') or not ipaddress.ip_address(u.hostname or '').is_loopback
                or not u.port or u.username or u.password or u.query or u.fragment
                or any(c.isspace() or ord(c) < 32 for c in value)
                or any(p in ('.', '..') for p in urllib.parse.unquote(u.path).split('/'))):
            raise ValueError()
    except ValueError:
        raise UsageError('set BOARD_URL or --url to the numeric loopback API base with port, including /api (or /board/api for the adapter)')
    return value.rstrip('/')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def call(url, method, body, token):
    request = urllib.request.Request(url, method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=30) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        with error:
            return error.code, error.read()


def emit_error(code, message, out):
    print(json.dumps({'code': code, 'message': message}), file=out)


def run(argv, out=sys.stdout, transport=call):
    try:
        a, values = parse(argv)
        method, path, body = plan(a, values)
        url = local_url(a.url) + path
        if a.dry_run:
            print(json.dumps({'dryRun': True, 'method': method, 'url': url, 'body': body}), file=out)
            return 0
        token = os.environ.get('BOARD_TOKEN', '').strip()
        if not token or any(c.isspace() or ord(c) < 32 for c in token):
            emit_error('unauthenticated', "set BOARD_TOKEN to the principal's existing local app bearer token", out)
            return 1
        status, raw = transport(url, method, body, token)
        # The fork's denials may be text or JSON. Preserve either, without retry,
        # impersonation, or any client-side replacement of its authorization.
        if not 200 <= status < 300:
            out.write(raw.decode() + ('' if raw.endswith(b'\n') else '\n'))
            return 1
        try:
            response = json.loads(raw)
        except (ValueError, UnicodeError):
            emit_error('data_loss', 'local app returned invalid JSON', out)
            return 1
        if a.noun == 'export' and a.format == 'csv' and not a.json:
            fields = ['title', 'description', 'status', 'priority', 'startDate', 'dueDate', 'userId', 'labels']
            if not isinstance(response, dict) or not isinstance(response.get('tasks'), list) or any(not isinstance(t, dict) for t in response['tasks']):
                emit_error('data_loss', 'local app returned an invalid export', out)
                return 1
            writer = csv.DictWriter(out, fieldnames=fields, lineterminator='\n')
            writer.writeheader()
            for task in response['tasks']:
                row = {k: task.get(k) for k in fields}
                row['labels'] = json.dumps(row['labels'], ensure_ascii=False)
                writer.writerow(row)
        else:
            out.write(raw.decode() + ('' if raw.endswith(b'\n') else '\n'))
        return 0
    except UsageError as error:
        emit_error('invalid_argument', str(error), out)
        return 2
    except (OSError, ValueError):
        emit_error('unavailable', 'cannot contact the local board API', out)
        return 1


if __name__ == '__main__':
    sys.exit(run(sys.argv[1:]))

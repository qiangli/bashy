"""People's step timeboxes layered on the sprint's existing session clock."""
import argparse
from datetime import datetime, timedelta, timezone
import json
import os
import re
import sys

from common import bashy, emit, fail, log_path


def timestamp(value):
    dt = datetime.fromisoformat(value.replace('Z', '+00:00'))
    if dt.tzinfo is None:
        raise ValueError('timestamps need a timezone')
    return dt


def now():
    value = os.environ.get('TUTOR_NOW')
    return timestamp(value) if value else datetime.now(timezone.utc)


def records(text, kind):
    """Accept current graph JSONL and the older ordinary JSON envelopes."""
    try:
        value = json.loads(text)
    except json.JSONDecodeError:
        value = [json.loads(line) for line in text.splitlines() if line.strip()]
    if isinstance(value, dict) and 'result' in value:
        value = value['result']
    if isinstance(value, dict) and isinstance(value.get('sprint'), dict):
        value = value['sprint']
    if isinstance(value, dict) and 'items' in value:
        value = value['items']
    values = value if isinstance(value, list) else [value]
    return [v['attrs'] if v.get('t') == 'node' else v for v in values
            if 't' not in v or (v['t'] == 'node' and v.get('kind') == kind)]


def read_card(sprint):
    cards = records(bashy('sprint', 'show', str(sprint), '--json'), 'sprint')
    if not cards:
        fail('sprint %s not found' % sprint)
    return cards[0]


CUTOFF = re.compile(r'cutoff (\d{4}-\d\d-\d\dT[\d:.]+(?:Z|[+-]\d\d:\d\d))')


def thread_box(plain):
    """Last session box from the plain card thread ('... cutoff <ISO>')."""
    box = None
    for line in plain.splitlines():
        match = CUTOFF.search(line)
        if match:
            box = dict(cutoff=match[1])
        elif box and re.search(r'\(stage\): (stopped|ended)', line):
            box['stopped_at'] = 'thread'
    return box


def log_box(events):
    """Session box recorded by `tutor start --for` when the card shows none."""
    starts = [e for e in events if e.get('event') == 'session_start']
    return dict(cutoff=starts[-1]['deadline']) if starts else None


def session_clock(card, at, sprint=None, events=()):
    boxes = card.get('boxes') or []
    box = boxes[-1] if boxes else None
    if box is None and sprint is not None:
        box = thread_box(bashy('sprint', 'show', str(sprint), '--plain', check=False))
    if box is None:
        box = log_box(events)
    if box is None:
        return dict(state='not-started', deadline=None, remaining_s=None)
    deadline = box.get('cutoff')
    stopped = box.get('stopped_at')
    return dict(state='stopped' if stopped else 'running', deadline=deadline,
                remaining_s=(int((timestamp(deadline) - at).total_seconds())
                             if deadline and not stopped else None))


def status(sprint, at=None):
    at = at or now()
    card = read_card(sprint)
    events = []
    path = log_path(sprint)
    if path.exists():
        events = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
    todos = records(bashy('todo', 'list', '--all', '--json'), 'todo')
    steps = []
    current = None
    for item in todos:
        if item.get('sprint') != sprint:
            continue
        labels = item.get('labels') or []
        indexes = [int(m[1]) for label in labels if (m := re.fullmatch(r'step-(\d+)', label))]
        if not indexes:
            continue
        minutes = next((int(m[1]) for label in labels
                        if (m := re.fullmatch(r'timebox-(\d+)m', label))), 0)
        role = next((label[5:] for label in labels if label.startswith('role-')), 'student')
        step = dict(index=indexes[0], id=str(item['id']), title=item['title'],
                    owner=item.get('assignee', item.get('owner', '')),
                    role=role, status=item.get('state', item.get('status', 'todo')),
                    timebox_s=minutes * 60, started_at=None, done_at=None,
                    overdue=False, evidence=None)
        if 'step-done' in labels:
            step['status'] = 'done'
        for event in events:
            if event.get('step') != step['id']:
                continue
            if event['event'] == 'step_start':
                step['started_at'] = event['ts']
                step['done_at'] = None
                step['evidence'] = None
            elif event['event'] == 'done':
                step['done_at'] = event['ts']
                step['evidence'] = event.get('evidence')
                step['status'] = 'done'
            elif event['event'] == 'extend':
                step['timebox_s'] += event['by_s']
        end = timestamp(step['done_at']) if step['done_at'] else at
        elapsed = max(0, int((end - timestamp(step['started_at'])).total_seconds())) if step['started_at'] else 0
        step['overdue'] = elapsed > step['timebox_s']
        steps.append((step, elapsed))
    steps.sort(key=lambda pair: (pair[0]['index'], pair[0]['id']))
    for step, elapsed in steps:
        if step['status'] == 'doing' and current is None:
            owners = [s.strip() for s in step['owner'].split(',') if s.strip()]
            current = dict(index=step['index'], total=len(steps), id=step['id'],
                           title=step['title'], owners=owners, timebox_s=step['timebox_s'],
                           elapsed_s=elapsed, remaining_s=step['timebox_s'] - elapsed,
                           overdue=step['overdue'])
    return dict(sprint=sprint, title=card['title'], clock=session_clock(card, at, sprint, events),
                current=current, steps=[s for s, _ in steps])


def one_line(value):
    step = value['current']
    if step is None:
        total = len(value['steps'])
        done = sum(s['status'] == 'done' for s in value['steps'])
        return 'Done · %d/%d steps' % (done, total) if total and done == total else 'Not started · %d steps' % total
    seconds = abs(step['remaining_s'])
    line = 'Step %d/%d · %s · %d:%02d %s' % (
        step['index'], step['total'], step['title'], seconds // 60, seconds % 60,
        'over' if step['overdue'] else 'left')
    if step['owners']:
        line += ' · ' + ', '.join(step['owners'])
    # User titles and owners must not introduce extra lines or terminal escapes.
    line = re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]', '', line)
    return ' '.join(''.join(c for c in line if c >= ' ' or c in '\r\n\t').split())


def log(sprint, step, event, at, **fields):
    with log_path(sprint).open('a', encoding='utf-8') as stream:
        stream.write(json.dumps(dict(ts=at.isoformat(), event=event, step=step['id'], **fields)) + '\n')


def transition(sprint, step, target, at, evidence=None):
    if step['status'] == target:
        return
    if target == 'done':
        # People cannot close sprint stories; the class manager (the teacher's
        # agent) accepts them later with `bashy sprint accept N ID -m EVIDENCE`.
        bashy('todo', 'edit', step['id'], '--label', 'step-done')
    else:
        bashy('todo', 'status', step['id'], target)
    log(sprint, step, 'step_start' if target == 'doing' else 'done', at,
        **({'evidence': evidence} if target == 'done' else {}))


def select_step(steps, selector):
    matches = [s for s in steps if s['id'] == selector]
    if not matches and selector.isdigit():
        matches = [s for s in steps if s['index'] == int(selector)]
    if len(matches) != 1:
        fail('unknown or ambiguous step %s' % selector)
    return matches[0]


def duration(value):
    match = re.fullmatch(r'([1-9]\d*)([smh])', value)
    if not match:
        raise argparse.ArgumentTypeError('duration must be positive, e.g. 5m')
    return int(match[1]) * {'s': 1, 'm': 60, 'h': 3600}[match[2]]


def main(argv):
    parser = argparse.ArgumentParser(prog='tutor')
    verbs = parser.add_subparsers(dest='verb', required=True)
    for name in ('start', 'next', 'done', 'extend', 'status'):
        sub = verbs.add_parser(name)
        sub.add_argument('sprint', type=int)
        output = sub.add_mutually_exclusive_group()
        output.add_argument('--json', action='store_true')
        if name == 'status':
            output.add_argument('--line', action='store_true')
        if name == 'start':
            sub.add_argument('--for', dest='for_', default='90m')
            sub.add_argument('--owner')
        if name in ('done', 'extend'):
            sub.add_argument('step')
        if name == 'done':
            sub.add_argument('-m', '--message', default=None)
        if name == 'extend':
            sub.add_argument('--by', required=True, type=duration)
    args = parser.parse_args(argv)
    try:
        at = now()
        value = status(args.sprint, at)
        steps = value['steps']
        if args.verb == 'start':
            if value['clock']['state'] != 'running':
                cmd = ['sprint', 'start', str(args.sprint), '--for', args.for_]
                if args.owner:
                    cmd += ['--owner', args.owner]
                bashy(*cmd)
                log(args.sprint, dict(id=None), 'session_start', at,
                    deadline=(at + timedelta(seconds=duration(args.for_))).isoformat())
            if steps and value['current'] is None and steps[0]['status'] in ('todo', 'assigned'):
                transition(args.sprint, steps[0], 'doing', at)
        elif args.verb == 'next':
            current = value['current']
            if current:
                transition(args.sprint, select_step(steps, current['id']), 'done', at)
            pending = [s for s in steps if s['status'] in ('todo', 'assigned')
                       and (not current or s['index'] > current['index'])]
            if pending:
                transition(args.sprint, pending[0], 'doing', at)
        elif args.verb == 'done':
            transition(args.sprint, select_step(steps, args.step), 'done', at, args.message)
        elif args.verb == 'extend':
            log(args.sprint, select_step(steps, args.step), 'extend', at, by_s=args.by)
        if args.verb != 'status':
            value = status(args.sprint, at)
        emit(value, args.json, one_line(value))
    except (ValueError, OSError, KeyError, TypeError) as exc:
        fail(str(exc))


if __name__ == '__main__':
    main(sys.argv[1:])

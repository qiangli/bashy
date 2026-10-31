"""People's step timeboxes layered on the sprint's existing session clock."""
import argparse
from datetime import datetime, timezone
import json
import os
import re
import sys

from nudge import fetch_body
from common import bashy, emit, fail, log_path, metadata, participant_roles


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


MIN_TIMEBOX_S = 60
FINISHED = ('done', 'skipped')
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


def session_clock(card, at, sprint=None):
    boxes = card.get('boxes') or []
    box = boxes[-1] if boxes else None
    if box is None and sprint is not None:
        box = thread_box(bashy('sprint', 'show', str(sprint), '--plain', check=False))
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
    meta = metadata(sprint)
    roles = meta["roles"]
    participants = participant_roles(roles)
    events = []
    path = log_path(sprint)
    if path.exists():
        events = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
    paused_since, intervals = None, []
    for event in events:
        if event['event'] == 'pause' and paused_since is None:
            paused_since = timestamp(event['ts'])
        elif event['event'] == 'resume' and paused_since is not None:
            intervals.append((paused_since, timestamp(event['ts'])))
            paused_since = None
    paused = paused_since is not None
    if paused:
        intervals.append((paused_since, at))
    todos = records(bashy('todo', 'list', "--user", '--all', '--json'), 'todo')
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
        role = next((label[5:] for label in labels if label.startswith('role-')), participants[0])
        step = dict(index=indexes[0], id=str(item['id']), title=item['title'],
                    owner=item.get('assignee', item.get('owner', '')),
                    role=role, status=item.get('state', item.get('status', 'todo')),
                    timebox_s=minutes * 60, started_at=None, done_at=None,
                    overdue=False, evidence=None,
                    done_when=fetch_body(str(item['id'])) or 'Show the outcome of this step.')
        if 'step-done' in labels:
            step['status'] = 'done'
        if 'step-skipped' in labels:
            step['status'] = 'skipped'
        for event in events:
            if event.get('step') != step['id']:
                continue
            if event['event'] == 'step_start':
                step['started_at'] = event['ts']
                step['done_at'] = None
                step['evidence'] = None
                if step['status'] == 'done' and 'step-done' not in labels:
                    step['status'] = 'doing'
            elif event['event'] == 'done':
                step['done_at'] = event['ts']
                step['evidence'] = event.get('evidence')
                step['status'] = 'done'
            elif event['event'] == 'leave':
                step['started_at'] = None
            elif event['event'] == 'time':
                step['timebox_s'] = max(MIN_TIMEBOX_S, step['timebox_s'] + event['by_s'])
        end = timestamp(step['done_at']) if step['done_at'] else at
        elapsed = 0
        if step['started_at']:
            begin = timestamp(step['started_at'])
            frozen = sum(max(0, (min(b, end) - max(a, begin)).total_seconds()) for a, b in intervals)
            elapsed = max(0, int((end - begin).total_seconds() - frozen))
        step['overdue'] = elapsed > step['timebox_s']
        steps.append((step, elapsed))
    steps.sort(key=lambda pair: (pair[0]['index'], pair[0]['id']))
    for step, elapsed in steps:
        if step['status'] == 'doing' and step['role'] in participants and current is None:
            owners = [s.strip() for s in step['owner'].split(',') if s.strip()]
            sequence = [s for s, _ in steps if s['role'] in participants]
            current = dict(index=sequence.index(step) + 1, total=len(sequence), id=step['id'],
                           title=step['title'], owners=owners, timebox_s=step['timebox_s'],
                           elapsed_s=elapsed, remaining_s=step['timebox_s'] - elapsed,
                           overdue=step['overdue'])
    return dict(sprint=sprint, title=card['title'], clock=session_clock(card, at, sprint),
                paused=paused, current=current, steps=[s for s, _ in steps], format=meta["format"],
                roles=dict(owner=roles[0], participants=roles[1:]))


def one_line(value):
    step = value['current']
    if value.get('paused') and step:
        return clean('Paused · Step %d/%d · %s' % (step['index'], step['total'], step['title']))
    if step is None:
        participants = [s for s in value['steps'] if s['role'] in (value.get('roles', {}).get('participants') or [value.get('roles', {}).get('owner', 'participant')])]
        total = len(participants)
        done = sum(s['status'] in FINISHED for s in participants)
        if total and done == total:
            return 'Done · %d/%d steps' % (done, total)
        if done:
            return '%d/%d done' % (done, total)
        return 'Not started · %d steps' % total
    seconds = abs(step['remaining_s'])
    line = 'Step %d/%d · %s · %d:%02d %s' % (
        step['index'], step['total'], step['title'], seconds // 60, seconds % 60,
        'over' if step['overdue'] else 'left')
    if step['owners']:
        line += ' · ' + ', '.join(step['owners'])
    return clean(line)


def clean(line):
    # User titles and owners must not introduce extra lines or terminal escapes.
    line = re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]', '', line)
    return ' '.join(''.join(c for c in line if c >= ' ' or c in '\r\n\t').split())


def log(sprint, step, event, at, **fields):
    with log_path(sprint).open('a', encoding='utf-8') as stream:
        stream.write(json.dumps(dict(ts=at.isoformat(), event=event,
                                     **({'step': step['id']} if step else {}), **fields)) + '\n')


def transition(sprint, step, target, at, evidence=None):
    if step['status'] == target:
        return
    if target == 'done':
        # People cannot close sprint stories; the session manager (the owner's
        # agent) accepts them later with `bashy sprint accept N ID -m EVIDENCE`.
        bashy('todo', 'edit', "--user", step['id'], '--label', 'step-done')
    else:
        bashy('todo', 'status', "--user", step['id'], target)
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
    match = re.fullmatch(r'([+-]?)([1-9]\d*)([smh])', value)
    if not match:
        raise argparse.ArgumentTypeError('duration must look like 5m, +5m or -1m')
    seconds = int(match[2]) * {'s': 1, 'm': 60, 'h': 3600}[match[3]]
    return -seconds if match[1] == '-' else seconds


def set_todo_state(step, target):
    bashy('todo', 'status', '--user', step['id'], target)


def goto(sprint, current, target, at):
    if current and current['id'] == target['id']:
        return
    if current:
        set_todo_state(current, 'assigned' if current['owner'] else 'todo')
        log(sprint, current, 'leave', at)
    drop = {'done': 'step-done', 'skipped': 'step-skipped'}.get(target['status'])
    if drop:
        bashy('todo', 'edit', '--user', target['id'], '--unlabel', drop)
    set_todo_state(target, 'doing')
    log(sprint, target, 'step_start', at)


def reorder(sprint, steps, step, mode, where, at):
    upcoming = [s for s in steps if s['status'] not in FINISHED + ('doing',)]
    if step not in upcoming:
        fail('only upcoming steps can be moved')
    slots = [s['index'] for s in upcoming]
    order = list(upcoming)
    at_pos = order.index(step)
    order.remove(step)
    if mode == 'to':
        if where not in slots:
            fail('%d is not an upcoming position (%s)' % (where, ', '.join(map(str, slots))))
        pos = slots.index(where)
    else:
        pos = max(0, min(len(order), at_pos + (-1 if mode == 'up' else 1)))
    order.insert(pos, step)
    changed = [(s, slot) for s, slot in zip(order, slots) if s['index'] != slot]
    for s, slot in changed:
        bashy('todo', 'edit', '--user', s['id'], '--label', 'step-%02d' % slot,
              '--unlabel', 'step-%02d' % s['index'])
    if changed:
        log(sprint, None, 'reorder', at, order=[s['id'] for s in order])


def main(argv):
    # argparse reads "-1m" as a flag; glue it to --add so `--add -1m` works.
    glued = []
    for arg in argv:
        if glued and glued[-1] == '--add' and arg[:1] == '-':
            glued[-1] = '--add=' + arg
        else:
            glued.append(arg)
    argv = glued
    parser = argparse.ArgumentParser(prog='tutor')
    verbs = parser.add_subparsers(dest='verb', required=True)
    for name in ('next', 'done', 'time', 'goto', 'skip', 'move', 'pause', 'resume', 'status'):
        sub = verbs.add_parser(name)
        sub.add_argument('sprint', type=int)
        output = sub.add_mutually_exclusive_group()
        output.add_argument('--json', action='store_true')
        if name == 'status':
            output.add_argument('--line', action='store_true')
        if name in ('done', 'goto', 'move'):
            sub.add_argument('step')
        if name == 'time':
            sub.add_argument('step', nargs='?')
            sub.add_argument('--add', required=True, type=duration)
        if name == 'done':
            sub.add_argument('-m', '--message', default=None)
        if name == 'move':
            where = sub.add_mutually_exclusive_group(required=True)
            where.add_argument('--up', action='store_true')
            where.add_argument('--down', action='store_true')
            where.add_argument('--to', type=int)
    args = parser.parse_args(argv)
    try:
        at = now()
        value = status(args.sprint, at)
        roles = value['roles']
        work_roles = roles['participants'] or [roles['owner']]
        steps = [s for s in value['steps'] if s['role'] in work_roles]
        current = select_step(steps, value['current']['id']) if value['current'] else None

        def start_next():
            pending = [s for s in steps if s['status'] in ('todo', 'assigned')
                       and (not current or s['index'] > current['index'])]
            if pending:
                transition(args.sprint, pending[0], 'doing', at)

        if args.verb == 'next':
            if current:
                transition(args.sprint, current, 'done', at)
            start_next()
        elif args.verb == 'done':
            transition(args.sprint, select_step(value['steps'], args.step), 'done', at, args.message)
        elif args.verb == 'time':
            if args.step is None and not current:
                fail('no step is running; name a step')
            step = select_step(value['steps'], args.step) if args.step else current
            by = max(args.add, MIN_TIMEBOX_S - step['timebox_s'])
            log(args.sprint, step, 'time', at, by_s=by)
        elif args.verb == 'pause' and not value['paused']:
            log(args.sprint, None, 'pause', at)
        elif args.verb == 'resume' and value['paused']:
            log(args.sprint, None, 'resume', at)
        elif args.verb == 'goto':
            goto(args.sprint, current, select_step(steps, args.step), at)
        elif args.verb == 'skip':
            if not current:
                fail('no step is running')
            bashy('todo', 'edit', '--user', current['id'], '--label', 'step-skipped')
            log(args.sprint, current, 'skip', at)
            start_next()
        elif args.verb == 'move':
            mode = 'up' if args.up else 'down' if args.down else 'to'
            reorder(args.sprint, steps, select_step(steps, args.step), mode, args.to, at)
        if args.verb != 'status':
            value = status(args.sprint, at)
        emit(value, args.json, one_line(value))
    except (ValueError, OSError, KeyError, TypeError) as exc:
        fail(str(exc))


if __name__ == '__main__':
    main(sys.argv[1:])

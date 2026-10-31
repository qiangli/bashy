"""Shared contract for every tutor verb (Sprint 547).

tutor composes plain bashy commands; it never imports klatch, form or any
other tool. State lives in existing primitives:

  session      = a sprint card       (bashy sprint add / start / extend / stop)
  step         = a todo on that card (bashy todo add --sprint N --owner PERSON)
  people       = registered persons  (bashy person add)
  run log      = $XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl (append-only events)

Step labels (bashy labels are one lowercase word, no colon):
  step-NN        1-based order of the step in the session ("step-01")
  timebox-Nm     the step's timebox in minutes ("timebox-20m")
  role-WORD   who the step belongs to
  phase-WORD     optional phase from the format ("phase-warmup")

Draft (output of `tutor plan`, input of `tutor apply`), version 1:
  {"version": 1, "title": str, "goal": str, "epic": str, "minutes": int,
   "format": str, "roles": [str], "audience": str, "tone": str, "people": [str],
   "steps": [{"title": str, "owner": str, "minutes": int,
              "done_when": str, "phase": str, "role": str}]}

Every verb prints JSON with --json; errors go to stderr with a non-zero exit.
"""
import json
import os
import re
import subprocess
import sys
from pathlib import Path

DRAFT_VERSION = 1
WORD = re.compile(r"[a-z0-9][a-z0-9._-]*")


def bashy_bin():
    return os.environ.get("BASHY_BIN", "bashy")


def bashy(*args, check=True, json_out=False, stdin=None):
    """Run a bashy command; return stdout text (or parsed JSON)."""
    env = dict(os.environ, BASHY_HINTS="off")
    proc = subprocess.run([bashy_bin(), *args], input=stdin, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    if check and proc.returncode != 0:
        fail("bashy %s failed: %s" % (" ".join(args[:2]), proc.stderr.strip()))
    if json_out:
        return json.loads(proc.stdout)
    return proc.stdout


def fail(message, code=1):
    print("tutor: " + message, file=sys.stderr)
    raise SystemExit(code)


def emit(obj, as_json, text=None):
    if as_json:
        print(json.dumps(obj, indent=2, sort_keys=True))
    else:
        print(text if text is not None else json.dumps(obj, indent=2, sort_keys=True))


def data_dir():
    base = os.environ.get("XDG_DATA_HOME") or os.path.join(Path.home(), ".local", "share")
    d = Path(base) / "bashy" / "tutor"
    d.mkdir(parents=True, exist_ok=True)
    return d


def log_path(sprint):
    return data_dir() / ("%s.jsonl" % sprint)


def label_word(prefix, value):
    word = "%s-%s" % (prefix, str(value).lower())
    if not WORD.fullmatch(word) or len(word) > 32:
        fail("invalid label %r" % word)
    return word


def step_labels(index, minutes, role, phase=""):
    labels = [label_word("step", "%02d" % index), label_word("timebox", "%dm" % minutes),
              label_word("role", role)]
    if phase:
        labels.append(label_word("phase", re.sub(r"[^a-z0-9]+", "", phase.lower()) or "main"))
    return labels


def validate_roles(roles):
    if not isinstance(roles, list) or not roles:
        fail("roles must be a non-empty ordered list of unique words")
    for role in roles:
        if not isinstance(role, str) or not WORD.fullmatch(role) or role != role.lower():
            fail("invalid role word %r" % role)
        label_word("role", role)
    if len(set(roles)) != len(roles):
        fail("roles must be unique")
    if "shared" in roles or ("owner" in roles and roles[0] != "owner"):
        fail("shared is reserved; owner can only be the first role")
    return roles


def participant_roles(roles):
    # In a one-person format the owner also performs the session work.
    return roles[1:] or roles[:1]


def metadata(sprint):
    path = log_path(sprint)
    if path.exists():
        for line in reversed(path.read_text().splitlines()):
            event = json.loads(line)
            if event.get("event") == "format":
                return event
    return dict(format="session", roles=["owner", "participant"], audience="", tone="")


def validate_draft(draft):
    """Return the draft if valid; fail with a precise message otherwise."""
    if not isinstance(draft, dict) or draft.get("version") != DRAFT_VERSION:
        fail("draft: version must be %d" % DRAFT_VERSION)
    for key in ("title", "goal"):
        if not isinstance(draft.get(key), str) or not draft[key].strip():
            fail("draft: %s is required" % key)
    if not isinstance(draft.get("minutes"), int) or draft["minutes"] <= 0:
        fail("draft: minutes must be a positive integer")
    roles = validate_roles(draft.get("roles", ["owner", "participant"]))
    steps = draft.get("steps")
    if not isinstance(steps, list) or not steps:
        fail("draft: steps must be a non-empty list")
    for i, s in enumerate(steps, 1):
        if not isinstance(s, dict) or not str(s.get("title", "")).strip():
            fail("draft: step %d needs a title" % i)
        if not isinstance(s.get("minutes"), int) or s["minutes"] <= 0:
            fail("draft: step %d minutes must be a positive integer" % i)
        if s.get("role", participant_roles(roles)[0]) not in roles:
            fail("draft: step %d role must be declared by the format" % i)
    return draft

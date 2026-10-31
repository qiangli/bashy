"""tutor plan / apply — owned by story bc05f6ea6e8e (Sprint 547).

`plan` shapes an editable draft (common.py schema v1); `apply` turns that draft
into a sprint card plus one labelled todo per step. Both compose plain bashy
commands only — no klatch, no form, Python 3 stdlib.

The format is the shaping surface: an editable Markdown file whose front-matter
(`key: value` and `- item` lists only) declares phases, timeboxes, pairing,
desk tools, tone and the teacher's own steps. The body is guidance for the
planning agent. With no agent configured the format's template is split
arithmetically, which is why a session can always be planned offline.
"""
import argparse
import contextlib
import io
import json
import os
import sys
from pathlib import Path

import common

HERE = Path(__file__).resolve().parent
FORMATS = HERE.parent / "formats"
PROMPTS = HERE.parent / "prompts"

LIST_FIELDS = ("phases", "nudge_points", "desk_tools", "teacher_steps")
DEFAULT_FORMAT = "class"
DEFAULT_DONE_WHEN = "the outcome of this step is shown to another person"


# --------------------------------------------------------------------------- #
# the format: a tiny front-matter parser, deliberately not YAML
# --------------------------------------------------------------------------- #

def parse_format(text):
    """Parse `key: value` / `- item` front-matter plus a Markdown body."""
    lines = text.splitlines()
    body_start, scalars, lists, key = 0, {}, {}, None
    if lines and lines[0].strip() == "---":
        for i, line in enumerate(lines[1:], 1):
            if line.strip() == "---":
                body_start = i + 1
                break
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            if line.lstrip().startswith("- "):
                if key is None:
                    common.fail("format: list item before any key")
                lists.setdefault(key, []).append(line.lstrip()[2:].strip())
                continue
            if ":" not in line:
                common.fail("format: cannot parse line %r" % line)
            key, value = line.split(":", 1)
            key, value = key.strip(), value.strip()
            if value:
                scalars[key] = value
            else:
                lists.setdefault(key, [])
    fmt = dict(scalars)
    for field in LIST_FIELDS:
        fmt[field] = lists.get(field, [])
    for extra, items in lists.items():
        fmt.setdefault(extra, items)
    fmt["name"] = scalars.get("name", DEFAULT_FORMAT)
    fmt["minutes"] = _positive_int(scalars.get("minutes", 90), "format minutes")
    fmt["default_step_minutes"] = _positive_int(
        scalars.get("default_step_minutes", 20), "default_step_minutes")
    fmt["teacher_step_minutes"] = _positive_int(
        scalars.get("teacher_step_minutes", 5), "teacher_step_minutes")
    fmt["pairing"] = scalars.get("pairing", "")
    fmt["tone"] = scalars.get("tone", "")
    fmt["default_done_when"] = scalars.get("default_done_when", DEFAULT_DONE_WHEN)
    fmt["phases"] = [_parse_phase(p) for p in fmt["phases"]]
    fmt["teacher_steps"] = [_parse_pair(s) for s in fmt["teacher_steps"]]
    fmt["desk_tools_by_phase"] = dict(
        (p["name"], p["text"]) for p in (_parse_pair(d) for d in fmt["desk_tools"]))
    fmt["body"] = "\n".join(lines[body_start:]).strip()
    if not fmt["phases"]:
        common.fail("format %s: no phases declared" % fmt["name"])
    return fmt


def _positive_int(value, what):
    try:
        number = int(str(value).strip())
    except ValueError:
        common.fail("format: %s must be an integer, got %r" % (what, value))
    if number <= 0:
        common.fail("format: %s must be positive" % what)
    return number


def _parse_phase(item):
    """`warm-up 10` or `work 50%` -> {name, minutes|percent}."""
    parts = item.rsplit(None, 1)
    if len(parts) != 2:
        common.fail("format: phase %r needs a name and minutes-or-percent" % item)
    name, spec = parts[0].strip(), parts[1].strip()
    if spec.endswith("%"):
        return {"name": name, "percent": float(_positive_int(spec[:-1], "phase percent"))}
    return {"name": name, "minutes": _positive_int(spec, "phase minutes")}


def _parse_pair(item):
    """`prepare: content ready` -> {name, text}; a bare item keeps an empty text."""
    if ":" in item:
        name, text = item.split(":", 1)
        return {"name": name.strip(), "text": text.strip()}
    return {"name": item.strip(), "text": ""}


def load_format(spec):
    """Resolve a format by name (formats/<name>.md) or by path."""
    spec = spec or DEFAULT_FORMAT
    candidates = [Path(spec), FORMATS / spec, FORMATS / ("%s.md" % spec)]
    for path in candidates:
        if path.is_file():
            return parse_format(path.read_text())
    known = sorted(p.stem for p in FORMATS.glob("*.md")) if FORMATS.is_dir() else []
    common.fail("unknown format %r (known: %s)" % (spec, ", ".join(known) or "none"))


# --------------------------------------------------------------------------- #
# the template split
# --------------------------------------------------------------------------- #

def scale_phases(fmt, minutes):
    """Scale the format's phases to `minutes`, summing exactly, dropping zeros."""
    shares = []
    for phase in fmt["phases"]:
        if "percent" in phase:
            shares.append(phase["percent"] / 100.0)
        else:
            shares.append(float(phase["minutes"]) / fmt["minutes"])
    total = sum(shares) or 1.0
    exact = [minutes * s / total for s in shares]
    out = _largest_remainder(exact, minutes)
    scaled = [{"name": p["name"], "minutes": m}
              for p, m in zip(fmt["phases"], out) if m > 0]
    if not scaled:
        common.fail("minutes too small for format %s" % fmt["name"])
    return scaled


def _largest_remainder(exact, total):
    """Round `exact` to ints that sum to `total`, biggest fraction wins."""
    floors = [int(v) for v in exact]
    short = total - sum(floors)
    order = sorted(range(len(exact)), key=lambda i: exact[i] - floors[i], reverse=True)
    for i in order[:max(0, short)]:
        floors[i] += 1
    return floors


def _split(total, parts):
    """Split `total` minutes into `parts` near-equal positive chunks."""
    parts = max(1, min(parts, total))
    base, extra = divmod(total, parts)
    return [base + (1 if i < extra else 0) for i in range(parts)]


def template_steps(fmt, minutes, people, goal):
    """The offline split: student steps per phase, then the teacher's own."""
    short = _short(goal)
    steps = []
    for phase in scale_phases(fmt, minutes):
        # ceil, not round: default_step_minutes is a timebox, so it is a ceiling
        wanted = -(-phase["minutes"] // fmt["default_step_minutes"])
        chunks = _split(phase["minutes"], wanted)
        label = _phase_title(phase["name"])
        for i, chunk in enumerate(chunks, 1):
            title = label if len(chunks) == 1 else "%s %d/%d" % (label, i, len(chunks))
            steps.append({"title": "%s: %s" % (title, short), "owner": "",
                          "minutes": chunk, "done_when": fmt["default_done_when"],
                          "phase": phase["name"], "role": "student"})
    for i, step in enumerate(steps):
        step["owner"] = people[i % len(people)] if people else ""
    for entry in fmt["teacher_steps"]:
        steps.append({"title": "%s: %s" % (_phase_title(entry["name"]), short),
                      "owner": "teacher", "minutes": fmt["teacher_step_minutes"],
                      "done_when": entry["text"] or fmt["default_done_when"],
                      "phase": entry["name"], "role": "teacher"})
    return steps


def _phase_title(name):
    return name[:1].upper() + name[1:]


def _short(text, limit=60):
    text = " ".join(str(text).split())
    return text if len(text) <= limit else text[:limit - 1].rstrip() + "…"


def template_draft(fmt, args, people):
    return {"version": common.DRAFT_VERSION,
            "title": args.title or _short(args.goal, 72),
            "goal": args.goal, "epic": args.epic, "minutes": args.minutes,
            "format": fmt["name"], "people": people, "source": "template",
            "steps": template_steps(fmt, args.minutes, people, args.goal)}


# --------------------------------------------------------------------------- #
# the agent path (one shot through `bashy chat`), with the template as fallback
# --------------------------------------------------------------------------- #

def fill_prompt(fmt, args, people):
    path = PROMPTS / "plan.md"
    if not path.is_file():
        common.fail("missing prompt %s" % path)
    phases = ", ".join("%s %dm" % (p["name"], p["minutes"])
                       for p in scale_phases(fmt, args.minutes))
    values = {
        "goal": args.goal,
        "title": args.title or _short(args.goal, 72),
        "epic": args.epic,
        "minutes": str(args.minutes),
        "people": ", ".join(people) or "(none)",
        "people_json": json.dumps(people),
        "format_name": fmt["name"],
        "format_body": fmt["body"],
        "phases": phases,
        "default_step_minutes": str(fmt["default_step_minutes"]),
        "pairing": fmt["pairing"],
        "tone": fmt["tone"],
        "teacher_steps": "; ".join(e["name"] for e in fmt["teacher_steps"]),
        "nudge_points": ", ".join(fmt["nudge_points"]),
    }
    text = path.read_text()
    for key, value in values.items():
        text = text.replace("{{%s}}" % key, value)
    return text


def extract_json_object(text):
    """First balanced JSON object that looks like a draft; None if there is none."""
    decoder = json.JSONDecoder()
    first = None
    for i, char in enumerate(text or ""):
        if char != "{":
            continue
        try:
            obj, _ = decoder.raw_decode(text[i:])
        except ValueError:
            continue
        if isinstance(obj, dict):
            if "steps" in obj:
                return obj
            if first is None:
                first = obj
    return first


def normalise_agent_draft(draft, fmt, args, people):
    """Make an agent's object answerable by validate_draft without inventing work."""
    out = dict(draft)
    out["version"] = common.DRAFT_VERSION
    out["source"] = "agent"
    out.setdefault("goal", args.goal)
    out.setdefault("title", args.title or _short(args.goal, 72))
    out.setdefault("epic", args.epic)
    out.setdefault("format", fmt["name"])
    out.setdefault("people", people)
    out["minutes"] = _as_int(out.get("minutes"), args.minutes)
    steps = out.get("steps")
    if not isinstance(steps, list):
        return out
    fixed = []
    for step in steps:
        if not isinstance(step, dict):
            return out
        step = dict(step)
        step["minutes"] = _as_int(step.get("minutes"), 0)
        step["owner"] = str(step.get("owner") or "")
        step["role"] = str(step.get("role") or "student")
        step["phase"] = str(step.get("phase") or "main")
        step["done_when"] = str(step.get("done_when") or fmt["default_done_when"])
        step["title"] = str(step.get("title") or "")
        if step["role"] == "teacher" and not step["owner"]:
            step["owner"] = "teacher"
        fixed.append(step)
    out["steps"] = fixed
    return out


def _as_int(value, default):
    try:
        return int(str(value).strip())
    except (TypeError, ValueError):
        return default


def reject_reason(draft):
    """None if the draft validates; the validator's message otherwise."""
    buf = io.StringIO()
    try:
        with contextlib.redirect_stderr(buf):
            common.validate_draft(draft)
    except SystemExit:
        return buf.getvalue().strip() or "invalid draft"
    return None


def agent_draft(agent, fmt, args, people):
    """Ask the agent once; return a valid draft or None (caller falls back)."""
    prompt = fill_prompt(fmt, args, people)
    out = common.bashy("chat", "--agent", agent, "-m", prompt, "--json", check=False)
    envelope = extract_json_object(out)
    if not isinstance(envelope, dict) or "output" not in envelope:
        return _declined(agent, "no bashy-chat-v1 envelope on stdout")
    if _as_int(envelope.get("exit_code"), 0) != 0:
        return _declined(agent, "agent exited %s" % envelope.get("exit_code"))
    obj = extract_json_object(envelope.get("output") or "")
    if obj is None:
        return _declined(agent, "no JSON object in the agent's output")
    draft = normalise_agent_draft(obj, fmt, args, people)
    reason = reject_reason(draft)
    if reason:
        return _declined(agent, reason)
    return draft


def _declined(agent, reason):
    print("tutor: agent %s did not produce a usable draft (%s); using the format template"
          % (agent, reason), file=sys.stderr)
    return None


# --------------------------------------------------------------------------- #
# verbs
# --------------------------------------------------------------------------- #

def people_list(raw):
    seen, out = set(), []
    for chunk in (raw or "").replace(",", " ").split():
        if chunk not in seen:
            seen.add(chunk)
            out.append(chunk)
    return out


def cmd_plan(argv):
    ap = argparse.ArgumentParser(prog="tutor plan", description="print an editable session draft")
    ap.add_argument("goal")
    ap.add_argument("--minutes", type=int, default=0, help="total session minutes")
    ap.add_argument("--people", default="", help="comma or space separated handles")
    ap.add_argument("--format", dest="format", default=DEFAULT_FORMAT, help="format name or path")
    ap.add_argument("--agent", default="", help="planning agent (default $TUTOR_AGENT)")
    ap.add_argument("--title", default="", help="card title (default the goal)")
    ap.add_argument("--epic", default="", help="epic grouping label")
    ap.add_argument("--json", action="store_true", help="machine-readable output")
    args = ap.parse_args(argv)
    if not args.goal.strip():
        common.fail("plan: GOAL is required")
    fmt = load_format(args.format)
    if args.minutes <= 0:
        args.minutes = fmt["minutes"]
    people = people_list(args.people)
    agent = args.agent or os.environ.get("TUTOR_AGENT", "")
    draft = agent_draft(agent, fmt, args, people) if agent.strip() else None
    if draft is None:
        draft = template_draft(fmt, args, people)
    common.validate_draft(draft)
    common.emit(draft, True)
    return 0


def load_draft(source):
    if source == "-":
        text = sys.stdin.read()
    else:
        path = Path(source)
        if not path.is_file():
            common.fail("apply: no such draft %s" % source)
        text = path.read_text()
    try:
        draft = json.loads(text)
    except ValueError as err:
        common.fail("apply: draft is not JSON (%s)" % err)
    return common.validate_draft(draft)


def cmd_apply(argv):
    ap = argparse.ArgumentParser(prog="tutor apply", description="create the card and its steps")
    ap.add_argument("draft", help="draft file, or - for stdin")
    ap.add_argument("--json", action="store_true", help="machine-readable output")
    args = ap.parse_args(argv)
    draft = load_draft(args.draft)  # refuses before anything is created

    sprint_args = ["sprint", "add", draft["title"]]
    if draft.get("epic"):
        sprint_args += ["--epic", draft["epic"]]
    if draft.get("goal"):
        sprint_args += ["--primary-goal", draft["goal"]]
    envelope = common.bashy(*sprint_args, "--json", json_out=True)
    sprint = (envelope.get("result") or {}).get("sprint")
    if not isinstance(sprint, int):
        common.fail("apply: sprint add returned no sprint number")

    for owner in dict.fromkeys(s.get("owner", "") for s in draft["steps"]):
        if owner:
            common.bashy("person", "add", owner, check=False)  # already exists is fine

    created = []
    for index, step in enumerate(draft["steps"], 1):
        labels = common.step_labels(index, step["minutes"],
                                   step.get("role", "student"), step.get("phase", ""))
        todo = ["todo", "add", step["title"], "--sprint", str(sprint)]
        if step.get("owner"):
            todo += ["--owner", step["owner"]]
        for label in labels:
            todo += ["--label", label]
        todo += ["--note", step.get("done_when") or ""]
        result = common.bashy(*todo, "--json", json_out=True)
        created.append({"index": index, "id": result.get("id"),
                        "title": step["title"], "owner": step.get("owner", "")})

    out = {"sprint": sprint, "steps": created}
    text = "sprint %d: %d steps\n%s" % (
        sprint, len(created),
        "\n".join("  %02d %-12s %s" % (s["index"], s["owner"] or "-", s["title"])
                  for s in created))
    common.emit(out, args.json, text)
    return 0


def main(argv):
    verb = argv[0] if argv else ""
    if verb == "plan":
        return cmd_plan(argv[1:])
    if verb == "apply":
        return cmd_apply(argv[1:])
    common.fail("plan: unknown verb %r" % verb, 2)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

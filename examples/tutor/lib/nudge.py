"""tutor nudge — owned by story 23cede21adca (Sprint 547). Contract: lib/common.py."""
import json
import os
import re
import sys
from pathlib import Path

from common import bashy, bashy_bin, emit, fail, metadata

FALLBACK_QUESTIONS = [
    "What have you tried so far for this step, and what happened?",
    "What do you think \"done\" looks like for this step? What's the smallest next move you could try?",
    "What's one question you could ask yourself to check your understanding before moving on?",
    "What would you try next, and why?",
    "What part feels stuck, and what have you already ruled out?",
]


def load_prompt_template():
    """Load prompts/nudge.md; return text and fallback list."""
    candidates = [
        Path(__file__).resolve().parent.parent / "prompts" / "nudge.md",
        Path(__file__).resolve().parent / "prompts" / "nudge.md",
        Path.cwd() / "prompts" / "nudge.md",
        Path.cwd() / "examples" / "tutor" / "prompts" / "nudge.md",
    ]
    tpl = None
    tpl_path = None
    for p in candidates:
        if p.is_file():
            tpl = p.read_text(encoding="utf-8")
            tpl_path = p
            break
    if tpl is None:
        # built-in minimal template
        tpl = (
            "You are a supportive tutor. {{audience}} {{tone}}\n"
            "Give hints and questions only, never the full solution. Keep tone encouraging.\n"
            "Title: {{title}}\nDone when: {{done_when}}\n"
        )
    # parse fallback questions from template if present
    fallbacks = []
    if tpl:
        # find section "## Fallback questions"
        lines = tpl.splitlines()
        in_fallback = False
        for line in lines:
            if "fallback" in line.lower() and line.strip().startswith("#"):
                in_fallback = True
                continue
            if in_fallback:
                # stop at next heading
                if line.strip().startswith("#"):
                    break
                m = re.match(r"\s*-\s+(.*\S)", line)
                if m:
                    q = m.group(1).strip()
                    if q:
                        fallbacks.append(q)
        # also if no heading, just collect bullet lines that look like questions
        # but prefer explicit section
    if not fallbacks:
        fallbacks = FALLBACK_QUESTIONS
    return tpl, fallbacks


def fill_prompt(template, title, done_when, meta=None):
    """Fill placeholders with title/done_when only."""
    meta = meta or {}
    done_when = done_when or ""
    # Replace known placeholders
    replacements = {
        "{{audience}}": ("Audience: " + meta["audience"] + ".") if meta.get("audience") else "",
        "{{tone}}": meta.get("tone", ""),
        "{{title}}": title,
        "{{TITLE}}": title,
        "{{Title}}": title,
        "{{step_title}}": title,
        "{{STEP_TITLE}}": title,
        "{{step-title}}": title,
        "{{done_when}}": done_when,
        "{{DONE_WHEN}}": done_when,
        "{{Done_when}}": done_when,
        "{{done-when}}": done_when,
        "{title}": title,
        "{done_when}": done_when,
    }
    out = template
    for k, v in replacements.items():
        out = out.replace(k, v)
    # If template had no placeholder and still doesn't contain title, append explicit context
    # This ensures the agent sees the step even if template forgot placeholders
    if title not in out:
        out = out.rstrip() + "\n\nTitle: " + title + "\nDone when: " + done_when + "\n"
    elif done_when and done_when not in out and done_when.strip():
        # if done_when not already present, append it (title already present)
        # avoid duplicating template that already has done_when line with empty
        if "Done when" not in out:
            out = out.rstrip() + "\nDone when: " + done_when + "\n"
    return out


def is_deliverable(text):
    """Heuristics for outputs that look like full solutions."""
    if not text or not text.strip():
        return False
    if "```" in text:
        return True
    words = text.split()
    if len(words) > 80:
        return True
    # also treat very long char count as essay
    if len(text) > 800 and len(words) > 60:
        return True
    return False


def parse_doing_nodes(sprint_id):
    """Return list of doing todo nodes for sprint_id, parsed from bashy todo list."""
    raw = bashy("todo", "list", "--user", "--json", "--status", "doing", check=False)
    nodes = []
    for line in raw.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except Exception:
            continue
        if obj.get("t") != "node":
            continue
        attrs = obj.get("attrs") or {}
        # sprint id stored as int
        try:
            s = attrs.get("sprint")
            if s is not None and int(s) != sprint_id:
                continue
        except Exception:
            continue
        labels = attrs.get("labels") or []
        has_step = any(re.match(r"^step-\d+$", lab) for lab in labels)
        if not has_step:
            continue
        if attrs.get("state") != "doing":
            continue
        nodes.append(attrs)
    return nodes


def fetch_body(todo_id):
    """Fetch todo body (done_when) via bashy todo show."""
    raw = bashy("todo", "show", "--user", todo_id, "--json", check=False)
    body = ""
    for line in raw.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except Exception:
            continue
        if obj.get("t") != "node":
            continue
        attrs = obj.get("attrs") or {}
        if attrs.get("id") != todo_id:
            # some implementations omit id check but still node is the todo
            # if only one node, accept it
            pass
        # body may be under "body" or "note" or "description"
        b = attrs.get("body")
        if b is None:
            b = attrs.get("note") or attrs.get("description") or ""
        if isinstance(b, str):
            body = b
            break
    return body.strip()


def main(argv):
    # argv includes "nudge" as first element when called via tutor.bsh
    args = list(argv)
    if args and args[0] == "nudge":
        args = args[1:]

    as_json = False
    person = None
    agent = None
    sprint_raw = None
    # simple parse
    i = 0
    # collect positional
    positionals = []
    while i < len(args):
        a = args[i]
        if a == "--json":
            as_json = True
            i += 1
        elif a == "--person":
            if i + 1 >= len(args):
                fail("tutor nudge: --person requires a value")
            person = args[i + 1]
            i += 2
        elif a.startswith("--person="):
            person = a.split("=", 1)[1]
            i += 1
        elif a == "--agent":
            if i + 1 >= len(args):
                fail("tutor nudge: --agent requires a value")
            agent = args[i + 1]
            i += 2
        elif a.startswith("--agent="):
            agent = a.split("=", 1)[1]
            i += 1
        elif a in ("-h", "--help"):
            print("Usage: tutor nudge N [--person P] [--agent A] [--json]", file=sys.stdout)
            return
        elif a.startswith("-"):
            fail("tutor nudge: unknown option %s" % a)
        else:
            positionals.append(a)
            i += 1

    if len(positionals) < 1:
        fail("tutor nudge: sprint number is required")
    sprint_raw = positionals[0]
    try:
        sprint_id = int(sprint_raw)
    except ValueError:
        fail("tutor nudge: sprint must be an integer, got %r" % sprint_raw)

    # optional extra positionals are error
    if len(positionals) > 1:
        fail("tutor nudge: too many arguments")

    # Agent resolution: --agent → $TUTOR_AGENT → none
    if agent is None:
        env_agent = os.environ.get("TUTOR_AGENT", "")
        if env_agent and env_agent.strip():
            agent = env_agent.strip()
        else:
            agent = None
    else:
        if not agent.strip():
            # empty string means fallback to env then none
            env_agent = os.environ.get("TUTOR_AGENT", "")
            agent = env_agent.strip() if env_agent.strip() else None
        else:
            agent = agent.strip()

    # Find doing steps
    nodes = parse_doing_nodes(sprint_id)
    if not nodes:
        fail("tutor nudge: no doing step for sprint %s" % sprint_id)

    # Filter by person if given
    chosen = None
    if person is not None:
        filtered = [n for n in nodes if (n.get("assignee") == person or n.get("owner") == person)]
        if not filtered:
            fail("tutor nudge: no doing step for sprint %s person %s" % (sprint_id, person))
        # sort by step number and pick first
        def step_key(n):
            for lab in n.get("labels") or []:
                m = re.match(r"^step-(\d+)$", lab)
                if m:
                    return int(m.group(1))
            return 999
        filtered.sort(key=step_key)
        chosen = filtered[0]
    else:
        def step_key(n):
            for lab in n.get("labels") or []:
                m = re.match(r"^step-(\d+)$", lab)
                if m:
                    return int(m.group(1))
            return 999
        nodes.sort(key=step_key)
        chosen = nodes[0]

    title = chosen.get("title") or chosen.get("name") or ""
    todo_id = chosen.get("id")
    done_when = ""
    if todo_id:
        done_when = fetch_body(todo_id)

    template, fallbacks = load_prompt_template()
    prompt = fill_prompt(template, title, done_when, metadata(sprint_id))
    fallback_q = fallbacks[0] if fallbacks else FALLBACK_QUESTIONS[0]

    hint = None
    if agent is None:
        hint = fallback_q
    else:
        # One-shot chat
        raw = bashy("chat", "--agent", agent, "-m", prompt, "--json", check=False)
        # Try to parse envelope
        output_text = None
        # raw may contain JSON object (or lines). Try full parse first.
        raw_stripped = raw.strip()
        if raw_stripped:
            # chat --json returns a single JSON object with .output
            # but broken fake may return multiple lines; try to find object with output
            candidates = []
            # try whole
            try:
                obj = json.loads(raw_stripped)
                candidates.append(obj)
            except Exception:
                # try line by line
                for line in raw_stripped.splitlines():
                    line=line.strip()
                    if not line:
                        continue
                    try:
                        candidates.append(json.loads(line))
                    except Exception:
                        continue
            for obj in candidates:
                if isinstance(obj, dict):
                    if "output" in obj and isinstance(obj["output"], str):
                        output_text = obj["output"]
                        break
                    # sometimes nested under result
                    if "result" in obj and isinstance(obj["result"], dict) and "output" in obj["result"]:
                        output_text = obj["result"]["output"]
                        break
                # also check if entire output is string
            if output_text is None:
                # last resort: treat raw as plain text (maybe no json)
                # but we already tried, so fallback
                pass
        if output_text is None or not output_text.strip():
            # agent failure → fallback
            hint = fallback_q
        else:
            # post-filter
            if is_deliverable(output_text):
                hint = fallback_q
            else:
                hint = output_text.strip()

    if hint is None:
        hint = fallback_q

    # Ensure hint is not empty
    hint = hint.strip() if isinstance(hint, str) else str(hint)
    if not hint:
        hint = fallback_q

    # Determine person for output (if not explicitly given, use assignee of chosen step)
    out_person = person if person is not None else (chosen.get("assignee") or chosen.get("owner") or "")

    if as_json:
        obj = {
            "hint": hint,
            "person": out_person,
            "sprint": sprint_id,
            "step": title,
        }
        # keep done_when for debugging but not required; include if not empty
        if done_when:
            obj["done_when"] = done_when
        emit(obj, True)
    else:
        # plain text: just the hint
        print(hint)


if __name__ == "__main__":
    main(sys.argv[1:])

"""tutor prep — owned by story f8feda454ff8 (Sprint 547). Contract: lib/common.py."""
import argparse
import json
import os
import re
import shutil
import sys
from pathlib import Path

import common
import plan
import render


class PlanArgs:
    def __init__(self, goal, minutes=0, people="", fmt="class", agent="", title="", epic=""):
        self.goal = goal
        self.minutes = minutes
        self.people = people
        self.format = fmt
        self.agent = agent
        self.title = title
        self.epic = epic


def make_slug(text, max_len=40):
    s = re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")
    s = re.sub(r"-+", "-", s)
    if len(s) > max_len:
        s = s[:max_len].rstrip("-")
    return s or "step"


def sort_key(path):
    m = re.match(r"^(\d+)", path.stem)
    if m:
        return (int(m.group(1)), path.name)
    return (9999, path.name)


def parse_content_file(path, fmt):
    raw_text = path.read_text(encoding="utf-8")
    fm_lines, body_text = render.extract_front_matter_and_body(raw_text)
    fm = {}
    if fm_lines:
        fm = render.parse_simple_yaml("\n".join(fm_lines)) or {}

    title = str(fm.get("title") or "").strip()
    if not title:
        m = re.search(r"^#\s+(.+)$", body_text, re.MULTILINE)
        if m:
            title = m.group(1).strip()
        else:
            stem = re.sub(r"^\d+[-_]?", "", path.stem)
            title = stem.replace("-", " ").replace("_", " ").title() or path.stem

    minutes = None
    if "timebox" in fm:
        tb = str(fm["timebox"]).strip()
        m = re.match(r"^(\d+)\s*m?$", tb)
        if m:
            minutes = int(m.group(1))
    if minutes is None and "minutes" in fm:
        try:
            minutes = int(fm["minutes"])
        except (ValueError, TypeError):
            pass
    if minutes is None or minutes <= 0:
        minutes = fmt.get("default_step_minutes", 20)

    done_when = str(fm.get("done_when") or "").strip()
    if not done_when:
        done_when = fmt.get("default_done_when", plan.DEFAULT_DONE_WHEN)

    role = str(fm.get("role") or "").strip().lower()
    if role not in ("student", "teacher"):
        role = "student"

    phase = str(fm.get("phase") or "").strip().lower()
    if not phase:
        for p in fmt.get("phases", []):
            p_name = p["name"].lower()
            if p_name in path.stem.lower() or p_name in title.lower():
                phase = p_name
                break
        if not phase:
            phase = "work"

    owner = str(fm.get("owner") or "").strip()
    if role == "teacher" and not owner:
        owner = "teacher"

    return {
        "title": title,
        "owner": owner,
        "minutes": minutes,
        "done_when": done_when,
        "phase": phase,
        "role": role,
        "content": f"content/{path.name}",
    }


def make_step_content(step):
    return (
        f"---\n"
        f"title: {step['title']}\n"
        f"timebox: {step['minutes']}m\n"
        f"role: {step.get('role', 'student')}\n"
        f"phase: {step.get('phase', '')}\n"
        f"done_when: {step.get('done_when', '')}\n"
        f"---\n"
        f"# {step['title']}\n\n"
        f"~~~teacher\n"
        f"~~~\n"
    )


def ensure_teacher_steps(steps, fmt, goal):
    short = plan._short(goal)
    existing_teacher_phases = {
        s["phase"] for s in steps if s.get("role") == "teacher"
    }
    for entry in fmt.get("teacher_steps", []):
        if entry["name"] not in existing_teacher_phases:
            steps.append({
                "title": "%s: %s" % (plan._phase_title(entry["name"]), short),
                "owner": "teacher",
                "minutes": fmt["teacher_step_minutes"],
                "done_when": entry["text"] or fmt["default_done_when"],
                "phase": entry["name"],
                "role": "teacher"
            })


def cmd_prep(argv):
    ap = argparse.ArgumentParser(prog="tutor prep", description="prepare a class content directory")
    ap.add_argument("dir", help="target directory to prepare")
    ap.add_argument("--from", dest="from_path", default="", help="existing content dir to copy and modify")
    ap.add_argument("--goal", "-g", default="", help="session goal")
    ap.add_argument("--minutes", "-m", type=int, default=0, help="total session minutes")
    ap.add_argument("--agent", "-a", default="", help="planning agent (default $TUTOR_AGENT)")
    ap.add_argument("--people", default="", help="comma or space separated handles")
    ap.add_argument("--title", default="", help="session title")
    ap.add_argument("--epic", default="", help="epic grouping label")
    ap.add_argument("--format", dest="format", default=plan.DEFAULT_FORMAT, help="format name or path")
    ap.add_argument("--json", action="store_true", help="machine-readable output")
    args = ap.parse_args(argv)

    if args.minutes < 0:
        common.fail("prep: minutes must be a positive integer")

    target_dir = Path(args.dir)
    if target_dir.exists() and not target_dir.is_dir():
        common.fail("prep: %s is not a directory" % target_dir)

    is_copy = bool(args.from_path)
    if is_copy:
        src_path = Path(args.from_path)
        if not src_path.exists():
            common.fail("prep: source directory not found: %s" % src_path)
        if not src_path.is_dir():
            common.fail("prep: source %s is not a directory" % src_path)
        target_dir.mkdir(parents=True, exist_ok=True)
        shutil.copytree(src_path, target_dir, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns(".git", "__pycache__", "*.pyc"))

    fmt_file = target_dir / "format.md"
    if fmt_file.is_file():
        fmt = plan.parse_format(fmt_file.read_text(encoding="utf-8"))
    else:
        fmt = plan.load_format(args.format)
        src_fmt = (
            Path(args.format)
            if Path(args.format).is_file()
            else (
                plan.FORMATS / f"{args.format}.md"
                if (plan.FORMATS / f"{args.format}.md").is_file()
                else plan.FORMATS / "class.md"
            )
        )
        target_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy(src_fmt, fmt_file)

    content_dir = target_dir / "content"
    draft_file = target_dir / "draft.json"

    # From scratch if no --from was specified and content dir doesn't already have markdown files
    has_existing_content = content_dir.is_dir() and any(content_dir.glob("*.md"))
    from_scratch = not is_copy and not has_existing_content

    if from_scratch:
        content_dir.mkdir(parents=True, exist_ok=True)
        total_minutes = args.minutes if args.minutes > 0 else fmt["minutes"]
        goal = args.goal.strip() or f"Class session: {target_dir.name}"
        title = args.title.strip() or plan._short(goal, 72)
        people = plan.people_list(args.people)
        agent = args.agent or os.environ.get("TUTOR_AGENT", "")

        plan_args = PlanArgs(
            goal=goal,
            minutes=total_minutes,
            people=args.people,
            fmt=args.format,
            agent=agent,
            title=title,
            epic=args.epic,
        )

        draft = plan.agent_draft(agent, fmt, plan_args, people) if agent.strip() else None
        if draft is None:
            draft = plan.template_draft(fmt, plan_args, people)

        student_steps = [s for s in draft["steps"] if s.get("role", "student") == "student"]
        for idx, s in enumerate(student_steps, 1):
            slug = make_slug(s["title"])
            filename = f"{idx:02d}-{slug}.md"
            file_path = content_dir / filename
            file_path.write_text(make_step_content(s), encoding="utf-8")
            s["content"] = f"content/{filename}"

        ensure_teacher_steps(draft["steps"], fmt, goal)
        common.validate_draft(draft)
        draft_file.write_text(json.dumps(draft, indent=2) + "\n", encoding="utf-8")
    else:
        content_dir.mkdir(parents=True, exist_ok=True)
        old_draft = {}
        if draft_file.is_file():
            try:
                old_draft = json.loads(draft_file.read_text(encoding="utf-8"))
            except Exception:
                old_draft = {}

        goal = args.goal.strip() or old_draft.get("goal") or target_dir.name.replace("-", " ").title()
        title = (
            args.title.strip()
            or (plan._short(goal, 72) if args.goal.strip() else old_draft.get("title"))
            or plan._short(goal, 72)
        )
        epic = args.epic.strip() or old_draft.get("epic", "")
        people = plan.people_list(args.people) if args.people else old_draft.get("people", [])

        md_files = sorted(content_dir.glob("*.md"), key=sort_key)
        steps = []
        for p in md_files:
            step = parse_content_file(p, fmt)
            steps.append(step)

        if people:
            student_steps = [s for s in steps if s.get("role") == "student"]
            for i, s in enumerate(student_steps):
                if not s.get("owner"):
                    s["owner"] = people[i % len(people)]

        ensure_teacher_steps(steps, fmt, goal)

        if args.minutes > 0:
            total_minutes = args.minutes
        elif old_draft.get("minutes"):
            total_minutes = old_draft["minutes"]
        else:
            student_mins = sum(s["minutes"] for s in steps if s.get("role") == "student")
            total_minutes = student_mins if student_mins > 0 else fmt["minutes"]

        draft = {
            "version": common.DRAFT_VERSION,
            "title": title,
            "goal": goal,
            "epic": epic,
            "minutes": total_minutes,
            "format": fmt["name"],
            "people": people,
            "source": "prep",
            "steps": steps,
        }

        common.validate_draft(draft)
        draft_file.write_text(json.dumps(draft, indent=2) + "\n", encoding="utf-8")

    student_count = sum(1 for s in draft["steps"] if s.get("role") == "student")
    teacher_count = sum(1 for s in draft["steps"] if s.get("role") == "teacher")
    content_files = sorted(content_dir.glob("*.md"), key=sort_key)
    out = {
        "dir": str(target_dir),
        "draft": str(draft_file),
        "format": str(fmt_file),
        "content_dir": str(content_dir),
        "steps": draft["steps"],
        "content_files": [f"content/{p.name}" for p in content_files],
        "goal": draft["goal"],
        "title": draft["title"],
        "minutes": draft["minutes"],
    }
    text = (
        f"prepared {target_dir}: {len(draft['steps'])} steps ({student_count} student, {teacher_count} teacher)\n"
        f"  format:  {fmt_file}\n"
        f"  draft:   {draft_file}\n"
        f"  content: {content_dir} ({len(content_files)} files)"
    )
    common.emit(out, args.json, text)
    return 0


def main(argv):
    verb = argv[0] if argv else ""
    if verb == "prep":
        return cmd_prep(argv[1:])
    return cmd_prep(argv)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

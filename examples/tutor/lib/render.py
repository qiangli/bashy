"""tutor render — owned by story 9d0a95cf258d (Sprint 547). Contract: lib/common.py."""
import argparse
import json
import re
import sys
from pathlib import Path

from common import emit, fail

HERE = Path(__file__).resolve().parent


def resolve_step_file(target):
    """Resolve FILE or STEP name to an existing markdown Path."""
    p = Path(target)
    if p.is_file():
        return p.resolve()

    # Search in possible directories
    candidates_dirs = [
        Path.cwd() / "content",
        Path.cwd(),
        HERE.parent / "content",
        HERE.parent,
    ]

    for d in candidates_dirs:
        if not d.is_dir():
            continue
        # Direct name or with .md
        for name in [target, f"{target}.md", f"step-{target}.md"]:
            candidate = d / name
            if candidate.is_file():
                return candidate.resolve()

        if target.isdigit():
            num_cand = d / f"step-{int(target):02d}.md"
            if num_cand.is_file():
                return num_cand.resolve()

        # Glob match
        matches = list(d.glob(f"*{target}*.md"))
        if matches:
            return matches[0].resolve()

    fail(f"step or file not found: {target}", 2)


def parse_scalar(val_str):
    """Parse a scalar string into bool, int, float, or stripped string."""
    val_str = val_str.strip()
    if not val_str:
        return None
    low = val_str.lower()
    if low in ("true", "yes"):
        return True
    if low in ("false", "no"):
        return False
    if low in ("null", "~"):
        return None
    if (val_str.startswith('"') and val_str.endswith('"')) or (val_str.startswith("'") and val_str.endswith("'")):
        return val_str[1:-1]
    if val_str.startswith("[") and val_str.endswith("]"):
        inner = val_str[1:-1].strip()
        if not inner:
            return []
        return [parse_scalar(item.strip()) for item in inner.split(",") if item.strip()]
    try:
        if "." in val_str:
            return float(val_str)
        return int(val_str)
    except ValueError:
        return val_str


def parse_simple_yaml(text):
    """Parse a subset of YAML suitable for front-matter (mappings, lists, scalars)."""
    lines = text.splitlines()
    root = {}
    i = 0
    while i < len(lines):
        line = lines[i]
        if not line.strip() or line.strip().startswith("#"):
            i += 1
            continue
        m = re.match(r"^([a-zA-Z0-9_-]+)\s*:\s*(.*)$", line)
        if m:
            key = m.group(1)
            raw_val = m.group(2).strip()
            if raw_val in ("|", "|-"):
                block_lines = []
                i += 1
                base_indent = None
                while i < len(lines):
                    bline = lines[i]
                    if not bline.strip():
                        block_lines.append("")
                        i += 1
                        continue
                    indent = len(bline) - len(bline.lstrip())
                    if base_indent is None:
                        base_indent = indent
                    if indent < base_indent:
                        break
                    block_lines.append(bline[base_indent:])
                    i += 1
                root[key] = "\n".join(block_lines).strip()
                continue
            elif not raw_val:
                i += 1
                nested_items = []
                current_item = None
                nested_key = None
                while i < len(lines):
                    nline = lines[i]
                    if not nline.strip() or nline.strip().startswith("#"):
                        i += 1
                        continue
                    n_indent = len(nline) - len(nline.lstrip())
                    if n_indent == 0:
                        break
                    m_item = re.match(r"^\s*-\s+(.*)$", nline)
                    if m_item:
                        item_content = m_item.group(1).strip()
                        if ":" in item_content:
                            ik, iv = item_content.split(":", 1)
                            current_item = {ik.strip(): parse_scalar(iv)}
                            nested_items.append(current_item)
                            nested_key = None
                        else:
                            if nested_key and isinstance(current_item, dict):
                                current_item[nested_key].append(parse_scalar(item_content))
                            else:
                                current_item = parse_scalar(item_content)
                                nested_items.append(current_item)
                        i += 1
                    else:
                        m_prop = re.match(r"^\s*([a-zA-Z0-9_-]+)\s*:\s*(.*)$", nline)
                        if m_prop and isinstance(current_item, dict):
                            pk, pv = m_prop.group(1), m_prop.group(2).strip()
                            if not pv:
                                nested_key = pk
                                current_item[pk] = []
                            else:
                                nested_key = None
                                current_item[pk] = parse_scalar(pv)
                            i += 1
                        else:
                            i += 1
                root[key] = nested_items
                continue
            else:
                root[key] = parse_scalar(raw_val)
                i += 1
        else:
            i += 1
    return root


def strip_answers_from_yaml_lines(lines):
    """Remove fields whose key starts with 'answer' from front-matter lines."""
    out = []
    skip_indent = None
    for line in lines:
        stripped = line.strip()
        if not stripped:
            if skip_indent is None:
                out.append(line)
            continue
        indent = len(line) - len(line.lstrip())
        if skip_indent is not None:
            if indent > skip_indent:
                continue
            else:
                skip_indent = None

        m_list = re.match(r"^(\s*-\s+)(answer\w*)\s*:(.*)$", line, re.IGNORECASE)
        m_map = re.match(r"^(\s*)(answer\w*)\s*:(.*)$", line, re.IGNORECASE)
        if m_list:
            skip_indent = len(m_list.group(1))
            continue
        elif m_map:
            skip_indent = len(m_map.group(1))
            continue

        out.append(line)
    return out


def filter_answers_from_obj(obj):
    """Recursively remove dictionary keys starting with 'answer'."""
    if isinstance(obj, dict):
        return {
            k: filter_answers_from_obj(v)
            for k, v in obj.items()
            if not k.lower().startswith("answer")
        }
    elif isinstance(obj, list):
        return [filter_answers_from_obj(item) for item in obj]
    return obj


def process_body_blocks(body_text, edition, owner_role="owner"):
    """Private blocks are visible to the owner and explicitly named roles."""
    out = []
    visible = None
    for line in body_text.splitlines():
        if visible is None:
            match = re.fullmatch(r"\s*~~~private(?:\s+(.*))?\s*", line)
            if match:
                allowed = (match[1] or "").split()
                visible = edition in ("owner", owner_role) or (edition != "shared" and edition in allowed)
                if visible:
                    out.append("> Private:")
            else:
                out.append(line)
        elif line.strip() == "~~~":
            visible = None
        elif visible:
            out.append("> " + line if line else ">")
    return re.sub(r"\n{3,}", "\n\n", "\n".join(out))


def extract_front_matter_and_body(raw_text):
    """Extract front-matter lines and body from markdown content."""
    lines = raw_text.replace("\r\n", "\n").splitlines()
    if lines and lines[0].strip() == "---":
        fm_lines = []
        body_start = None
        for idx in range(1, len(lines)):
            if lines[idx].strip() == "---":
                body_start = idx + 1
                break
            fm_lines.append(lines[idx])
        if body_start is not None:
            body_text = "\n".join(lines[body_start:])
            return fm_lines, body_text

    return None, raw_text


def questions_to_form_fields(questions):
    """Convert parsed question definitions to bashy form definition fields."""
    fields = []
    for idx, q in enumerate(questions, 1):
        if isinstance(q, str):
            fields.append({
                "id": f"q{idx}",
                "label": q,
                "kind": "text",
            })
            continue

        if not isinstance(q, dict):
            continue

        fid = str(q.get("id") or f"q{idx}")
        label = str(q.get("label") or q.get("question") or q.get("prompt") or q.get("title") or f"Question {idx}")
        kind = str(q.get("kind") or "text").lower()
        if kind not in ("text", "textarea", "choice", "multi", "yes-no", "file"):
            kind = "text"

        field = {
            "id": fid,
            "label": label,
            "kind": kind,
        }
        if q.get("required"):
            field["required"] = True
        if kind in ("choice", "multi"):
            opts = [str(o) for o in q.get("options", [])]
            if not opts:
                opts = ["Option 1"]
            field["options"] = opts
            rec = q.get("recommended")
            if rec and str(rec) in opts:
                field["recommended"] = str(rec)
        if q.get("help"):
            field["help"] = str(q["help"])

        fields.append(field)
    return fields


def render_content(file_path, edition, fmt="md", as_json=False):
    """Render a step file for participant or owner edition."""
    raw_text = file_path.read_text(encoding="utf-8")
    fm_lines, body_text = extract_front_matter_and_body(raw_text)

    # Parse front-matter if present
    parsed_fm = parse_simple_yaml("\n".join(fm_lines)) if fm_lines is not None else {}

    import plan
    local_formats = [file_path.parent / "format.md", file_path.parent.parent / "format.md"]
    spec = parsed_fm.get("format") or next((str(p) for p in local_formats if p.is_file()), "session")
    session_format = plan.load_format(spec)
    roles = session_format["roles"]
    if edition not in roles + ["owner", "shared"]:
        fail("edition must be a format role, owner or shared", 2)
    is_owner = edition in ("owner", roles[0])

    # Determine title
    title = parsed_fm.get("title")
    if not title:
        m_title = re.search(r"^#\s+(.+)$", body_text, re.MULTILINE)
        if m_title:
            title = m_title.group(1).strip()
        else:
            title = file_path.stem

    done_when = parsed_fm.get("done_when")
    raw_questions = parsed_fm.get("questions") or []

    # Format: form
    if fmt == "form":
        form_fields = questions_to_form_fields(raw_questions)
        form_def = {
            "title": title,
            "fields": form_fields,
        }
        out_str = json.dumps(form_def, indent=2)
        emit(form_def, as_json=as_json, text=out_str)
        return

    # Process body blocks
    rendered_body = process_body_blocks(body_text, edition, roles[0])

    # Process front-matter for Markdown output
    if fm_lines is not None:
        if not is_owner:
            cleaned_fm_lines = strip_answers_from_yaml_lines(fm_lines)
            fm_part = "\n".join(cleaned_fm_lines).strip()
            rendered_md = f"---\n{fm_part}\n---\n{rendered_body}" if fm_part else rendered_body
        else:
            fm_part = "\n".join(fm_lines).strip()
            rendered_md = f"---\n{fm_part}\n---\n{rendered_body}" if fm_part else rendered_body
    else:
        rendered_md = rendered_body

    # Prepare structured JSON object
    filtered_fm = filter_answers_from_obj(parsed_fm) if not is_owner else parsed_fm
    structured_questions = filtered_fm.get("questions") or []

    obj = {
        "title": title,
        "edition": edition,
        "content": rendered_body,
    }
    if done_when:
        obj["done_when"] = done_when
    if structured_questions:
        obj["questions"] = structured_questions

    for k, v in filtered_fm.items():
        if k not in obj and k != "questions":
            obj[k] = v

    if fmt == "json" or as_json:
        emit(obj, as_json=True)
    else:
        emit(obj, as_json=False, text=rendered_md)


def main(argv=None):
    if argv is None:
        argv = sys.argv[1:]
    if argv and argv[0] == "render":
        argv = argv[1:]

    parser = argparse.ArgumentParser(
        prog="tutor render",
        description="Render a lesson or step for participant or owner edition."
    )
    parser.add_argument("target", metavar="FILE|STEP", help="Step name or markdown file path")
    parser.add_argument("--edition", required=True,
                        help="Format role, owner or shared")
    parser.add_argument("--format", choices=["md", "json", "form"], default="md",
                        help="Output format (md, json, or form)")
    parser.add_argument("--json", action="store_true",
                        help="Emit JSON output envelope")

    args = parser.parse_args(argv)

    step_path = resolve_step_file(args.target)
    render_content(step_path, args.edition, fmt=args.format, as_json=args.json)


if __name__ == "__main__":
    main(sys.argv[1:])

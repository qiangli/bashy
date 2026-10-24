# examples/record — tutorial: a skill-tier command in one Bash# file

`record` asks the person at this computer for a photo, a video or a voice
take, and prints the path of the take. It is also a worked example of how an
agent adds a command to bashy **without touching the core**: one Bash# file
that is both a skill and a program, registered once, then usable by every
agent as a command, a skill or an MCP tool.

## The cycle

```bash
bashy dag examples/record test       # 1. check it (no camera needed)
bashy dag examples/record register   # 2. register it as the command `record`
bashy dag examples/record show       # 3. see what agents see (catalog, skill, MCP schema)
bashy dag examples/record use        # 4. take a photo (the person presses the buttons)
```

After `register`:

```bash
record photo --prompt "Headshot for the founder page"
record video --script pitch.txt --max-seconds 75 --out founder.mp4
ffprobe "$(record audio)"            # composes: only the path is printed
```

## Anatomy of `record.bsh`

1. **`~~~skill as s`**: the skill, in the format agents read: a name, a
   description that says *when* to use it, and how. `record --skill` prints
   it, and `register` stores it as the command's help, so `bashy commands
   record` shows it to any agent.
2. **`~~~go as go`**: the one part that needs Go: a loopback HTTP server that
   serves `page.html` once, accepts one upload, and writes it mode 0600.
   Bash# fences hold imports and functions only.
3. **The shell part**: argument parsing, then `path := go.Record(...)`, which
   calls the fence with bare shell variables.

`page.html` is the person's side: a small camera preview, a large
teleprompter (speed slider, up/down keys, "Full script" to practise),
Start / Stop / Retake / "Use this".

## Why this shape

- **Core untouched.** The certified core stays fixed; a skill-tier command is
  a file plus a registry entry, editable and replaceable without a release.
- **One job, composable.** `record` gets a take and prints a path, nothing
  else. Composition (write the script, record, assemble with
  `examples/video`) is another Bash# script.
- **The human never types a command.** The agent runs `record`; the person
  allows the camera once and presses buttons. The same flow works from a
  terminal, a web chat or a messenger.
- **Typed arguments.** `register` declares the positional `kind`
  (photo|video|audio) and the flags, so bashy validates calls and publishes
  them as the MCP tool schema.

## Lessons from building it

- A Go fence cannot hold a top-level `const`; use a function.
- Call a fence with assignment syntax (`x := go.F(a, b)`), not `$(go.F …)`.
- A failed fence call leaves the result empty; check it and exit non-zero.
- The page must fit one screen: the buttons were first hidden below a 16:9
  preview. The teleprompter is the main area; the preview is small.

---
title: Introduction to Bashy Agentic Shell
done_when: All questions answered and exercise completed
timebox: 15m
role: participant
questions:
  - id: q1
    label: What bashy command shows your unified inbox?
    kind: text
    required: true
    answer: bashy inbox
    answer_rubric: Exactly "bashy inbox" or "bashy inbox --json"
  - id: q2
    label: Which edition of tutor content hides owner answers and notes?
    kind: choice
    options:
      - shared
      - owner
    recommended: shared
    answer: shared
  - id: q3
    label: Explain how bashy tutor composes with desk tools without coupling.
    kind: textarea
    help: One or two sentences describing shell pipes and JSON interfaces.
    answer_rubric: Mentions piping rendered form definitions to bashy form add, or reading answers back via bashy form show.
answer_notes:
  Ensure participants understand that tutor never imports form code directly; composition is via standard pipes and JSON.
---
# Introduction to Bashy Agentic Shell

Welcome to today's interactive session. In this step, you will explore the
basics of working with the bashy agentic shell.

### Goals
1. Check your unified inbox to see outstanding requests and peer updates.
2. Verify that your environment is properly seated with your agent identity.
3. Answer the check questions below.

~~~private
Key answers for facilitation:
1. `bashy inbox` (or `bashy inbox --peek` / `bashy inbox --json`)
2. `shared` edition removes ~~~private blocks and front-matter answer* fields.
3. tutor render outputs form JSON piped to `bashy form add --def -`; answers read via `bashy form show --json`.
~~~

### Hands-on Exercise
Run the following command in your terminal:

```bash
bashy inspect context --json
```

Observe your session mode and available tools. When ready, complete the questions.

~~~private
- If participants see an unseated session warning on `bashy inbox`, guide them to set `export BASHY_AGENT=<name>` or use `--as <name>`.
- Allow approximately 5 minutes for reading and 10 minutes for hands-on exploration.
~~~

### Next Steps
Once you have submitted your answers, notify your instructor or run the next step in your sprint card.

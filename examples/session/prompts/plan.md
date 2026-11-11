You are planning one timeboxed working session and returning it as JSON.

GOAL: {{goal}}
TITLE: {{title}}
EPIC: {{epic}}
TOTAL MINUTES: {{minutes}}
PEOPLE: {{people}}
FORMAT: {{format_name}}
ROLES (owner first): {{roles_json}}
AUDIENCE: {{audience}}
PHASES (name and minutes, already scaled to the total): {{phases}}
DEFAULT STEP MINUTES: {{default_step_minutes}}
PAIRING: {{pairing}}
TONE: {{tone}}
OWNER STEPS: {{owner_steps}}
NUDGE POINTS: {{nudge_points}}

Format guidance:
{{format_body}}

Reply with ONE JSON object and nothing else — no prose, no markdown fence:

{"version": 1, "title": "<short card title>", "goal": "{{goal}}",
 "epic": "{{epic}}", "minutes": {{minutes}}, "format": "{{format_name}}",
 "people": {{people_json}},
 "steps": [{"title": "<what the person does>", "owner": "<person or empty>",
            "minutes": <positive int>, "done_when": "<checkable evidence>",
            "phase": "<phase name>", "role": "{{participant_role}}"}]}

Rules:
- Keep the phases and their minutes. Participant step minutes MUST sum to {{minutes}}.
- Give every participant step an owner from PEOPLE, round-robin, so nobody idles.
  With no people, use "" for every participant owner.
- End with the owner's steps: role "{{owner_role}}", owner "{{owner_role}}", one per
  OWNER STEPS entry, in that order.
- `done_when` is evidence another person can check, not an intention.
- A step title names an action, not a topic.

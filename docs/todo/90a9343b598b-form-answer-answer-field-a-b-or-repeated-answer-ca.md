---
id: 90a9343b598b
kind: bug
title: 'form answer: --answer FIELD=a,b (or repeated --answer) cannot set a multi field'
seq: 3320
status: todo
priority: p2
created: 2026-11-25T20:05:05.175918Z
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0.1 layout, lifecycle, one-step launch'
---

Found while laying out form as shapeware (Sprint 721 P1, recorded as KI-1 in examples/desk/form/SKILL.md). Symptom: form answer ID --answer topics=news,events fails 'is not one of its options'; repeated --answer topics=news --answer topics=events keeps only the last value. Cause (proven by the P1 worker): --answer binds one string per field id; multi answers need a JSON array. Workaround (KI-1): pass --answers - with {"topics":["news","events"]}. Root cause first before a fix (decide comma split vs repeated-flag accumulation for multi kinds only); then retire KI-1.

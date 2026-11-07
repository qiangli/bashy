---
id: d969a9b56758
kind: feature
title: 'Student-owned pairing on a satellite: members mint their own pairing codes and share what they choose'
seq: 3175
status: todo
priority: p2
labels:
    - satellite
created: 2026-11-07T21:11:18.920651Z
---

Follow-up from Sprint 587: today a host paired through the satellite owner's invite is owned by the satellite owner in the satellite record, and only the satellite owner can mint codes for the satellite, so a student cannot create satellite-level shares from their own host. Smallest change is in the hosted satellite (private repository): let a member who holds a satellite login mint a pairing code for their own account, and have the master accept that member for the assignment. Keep scope empty by default. Until then use the C2 opt-out procedure.

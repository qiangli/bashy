# Explicit ycode YOLO selection

Sprint 415, story 12a192fc8b78. Select Genie's shipped agent-yolo.yaml when
the operator passes bashy ycode --yolo. Pass the selected filename to the
existing model-profile recipe; offline utilities use the same embedded source.
Consume the selector before recipe dispatch, preserve literal text and flag
values, reject competing custom configuration, and scope environment changes
to this invocation. Test parser behavior, file selection, conflict handling
and the actual recipe handoff. YAML owns every permission decision.

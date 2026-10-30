# C1 delivery — Sprint 537, Story 2935

Implemented entirely under `examples/desk/chart`: desk lifecycle server and
CLI, Bash# entry, registration/launcher recipe, shapable page template, bounded
JSON/CSV/NDJSON/OTLP parsing, ten SVG widget selections, snapshots and opt-in
file/loopback refresh, and browser create/view/share/revoke/close.

Validation on Dragon:

- Initial per-widget/OTLP golden tests failed with missing baselines; reviewed
  geometry and values, then stored ten widget and five OTLP SVG baselines.
- Counter rates asserted independently: cumulative deltas, delta intervals,
  resets with known start time, unknown resets and duplicate timestamps.
- Histogram p50/p95 values and open-tail bounds asserted independently.
- An extreme finite-value regression failed with non-finite SVG geometry;
  normalize-before-padding fixed it and the test passed.
- `bashy dag gate`: focused `GOWORK=off go test -count=1 -race .` passes,
  including `contracttest.Run`, SVG goldens, stream separation, latest histogram
  selection, strict decoding, escaping, secret rejection, CLI/API parity,
  participant sharing/revocation, persistence failure and refresh behavior.
- `browser-test.mjs` written; browser and cross-OS round trips are intentionally
  left for the designated test hosts. No browser installation or podman run.

Kit requests (no kit edits made):

1. `DecodeAction`: require the second Decode to return io.EOF. K0 currently
   accepts a second valid JSON value. Chart has a local strict decoder plus a
   regression test.
2. Fixture sharing/join: deep-copy Members and Invites before modifying them,
   so a failed atomic save cannot mutate cached state. Chart does this locally
   and tests a failed invitation save.

Known scope: OTLP legacy Summary is refused; numbers render through float64,
so integer values above 2^53 may round. Histogram percentiles are bucket
estimates. Refresh is demand-driven; closed items retain the final snapshot.
Shaped templates are owner-supplied trusted HTML, while data strings are escaped.

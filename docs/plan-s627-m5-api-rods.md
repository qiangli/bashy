# Sprint 627 M5 — API rod implementation plan and receipt

1. Extend typed `mode: http` generically for HTTPS, JSON file arguments, and
   escaped path arguments. Keep bearer resolution in the existing vault ref
   path, and verify the transport with an in-process fixture.
2. Add one Replicate tool declaration for song, image generation and edit,
   animation, and prediction reads. Register it and three `api` rods from the
   default source pin. Declare `spend` on creation and on the rods.
3. Map `bashy-svc-request-v1` fields to the provider model inputs, poll
   boundedly, download and decode the outputs, and emit a typed rod result.
   Refuse fields the selected models cannot honor.
4. Verify the no-consent refusal, cost-class dry run, request mapping,
   prediction lifecycle, and artifact handoff with fixtures. Leave any live
   paid smoke to explicit operator approval.

The [rod README](../examples/api-rods/README.md) names the model schemas and
operational limits. This plan changes no model or browser venue on the dev
host.

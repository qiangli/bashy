# Release signing plan (Sprint 407, story 3414b9da74b0)

1. Sign and notarize native Darwin executables before archive creation.
2. Sign Windows executables with Azure Artifact Signing on a Windows runner,
   then assemble and upload the final signed archives and raw assets.
3. Recompute sidecars and the complete checksum manifest from final bytes.
4. Carry explicit per-platform signing booleans through candidate and stable
   release notes. Verify downloaded published bytes on native QA runners.
5. Keep credential-free development tags publishable with one UNSIGNED notice
   for each affected platform, and preserve the current asset names.

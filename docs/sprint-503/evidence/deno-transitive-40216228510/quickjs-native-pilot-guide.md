# QuickJS-NG native pilot: execution and acceptance boundary

The [pilot workflow](../../../../.github/workflows/s503-quickjs-native-pilot.yml)
is an **S503.6 research gate**, not a product release or installation path.
It runs only when the reviewed workflow or pilot script changes on the
`s503-contract-integration` branch. `workflow_dispatch` additionally requires
registration on the repository default branch. Two jobs, `ubuntu-24.04` and
`windows-11-arm`, each have a 30-minute timeout and retain evidence even on
failure. A passing CI status remains a research result until the retained
native artifacts receive independent review.

The [pilot script](../../../../.github/scripts/s503_quickjs_native_pilot.py)
downloads the exact QuickJS-NG `v0.17.0` commit archive
`6d46d07d04041b40f4f49eaa7fdebe44c314c699` from GitHub codeload.
It checks size **905,853 bytes** and SHA-256
`a62cf1ff7d6d2f82b90a2d247a57e9eb56b81c03feb1f372a53923426e358cb0`,
then checks the MIT `LICENSE` and CMake recipe bytes before building. It
rejects tar path escapes and links. CMake builds only native `qjs`, with
mimalloc and shared-library options off. It records the binary hash and ELF
or PE machine, source notice candidates, all produced object/archive hashes,
native linker map, direct imports and logs. The binary itself is not uploaded
as an artifact, so this run cannot become a release by retagging its result.

The EJS smoke downloads the pinned yt-dlp `2026.10.21` zipapp (SHA-256
`b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14`),
checks bundled EJS `0.8.0` and both solver-file hashes, verifies yt-dlp accepts
`--no-remote-components --no-js-runtimes --js-runtimes quickjs:<qjs>`, then
runs the **actual bundled EJS code** in native `qjs` with deterministic
prepared `sig` and `n` challenge functions. It expects `abc → cba` and
`xyz → xyzn`. No remote EJS package, live YouTube player, player extraction,
media download, transcription, or inference is exercised. The output says so
explicitly; do not count this as S503.5 operator URL acceptance.

`result.json` can report `research_result: PASS` only after source, native
build, map/import, pinned zipapp and controlled EJS checks pass. It **always**
reports `product_admitted: false` and `native_link_notice_closure: BLOCKED`.
The retained map and notices require an independent component-level review:
classify every loaded object/archive and system provider, verify source and
runtime license terms, retain applicable notice text, and bind them to each
final executable hash. A Windows ARM64 CMake/SDK/import-reader gap fails the
job and remains a reported blocker, never a substitute with x64 emulation.

After a successful pilot, the remaining route is six native builds and
independent notice reviews, a security review of yt-dlp's QuickJS temporary
script files, a real pinned EJS player-extraction challenge, current operator
canonical and shortened URL acquisition, and the six-target transcript/export
acceptance gates. The [decision memo](runtime-build-decision.md) compares
that route with the more expensive Deno/V8 first-party build.

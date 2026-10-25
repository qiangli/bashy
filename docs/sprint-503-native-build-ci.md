# S503 native whisper.cpp build gate

`.github/workflows/s503-native-build.yml` runs on integration-branch changes to
the build recipe and also supports manual `workflow_dispatch` once the file is
on the default branch. It gates the six public runner labels established by the S503.8 identity
preflight: Linux amd64/arm64, macOS amd64/arm64, and Windows amd64/arm64.
It builds the pinned whisper.cpp v1.9.5 source with
`examples/transcribe/dependencies/build_whisper.py` on each native runner.
The Python process and underlying OS must both match the matrix target;
macOS translation is refused.

Each job uploads its identity JSON, source/build/dependency report, binary,
and binary SHA-256. The build recipe checks the source tarball's pinned
size and SHA-256 before unpacking, configures a CPU-only build without
OpenMP, audits dynamic libraries, and places the CLI in its canonical
coordination cache. The workflow then copies that binary into the artifact
and checks its SHA-256 and byte count against the report. GitHub's
`actions/attest` records provenance for each successful binary. Failed jobs
still upload any evidence they produced.

This gate installs Python 3.12 through `actions/setup-python`, since runner
toolchains differ. It expects CMake and a native C/C++ compiler on each
runner; Windows selects Ninja and the detected native compiler pair because
the standard ARM runner has no Visual Studio instance. It does not build Bashy,
fetch a model, or run inference. A green
job establishes one native dependency build only. Six green jobs and their
attestations are required before claiming six-target build admission;
native transcription remains a separate gate.

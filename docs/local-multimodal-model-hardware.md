# Local multimodal model hardware baseline (Sprint 382, Story 356)

Story `df42d8bc7b65`: reproducible local hardware baseline for running the
Bashy/MCP agent alongside creative tools (Kdenlive, GIMP, Audacity, Ardour,
Natron, Blender, Story Architect, Kitsu, and OpenToonz). Models inspect or
generate creative assets; agent tools apply edits directly to native projects.

## Evidence capture and methodology

Host evidence was captured directly on claimed host `novidesign.local` (epoch43,
macOS 26.3, Darwin 25.3.0 arm64, Apple M4 Max 14-core, 36.0 GiB unified memory,
Metal 4) via SSH, prior to verified host cleanup. This corrects the previous
draft assumption in run219 that remote temporary directories had already been
reclaimed before inspection: the raw evidence tree at `/tmp/df42-xkz0Ue` (9.6 GiB)
and pointer `/tmp/df42-active-dir` were present and intact, containing full Ollama
server logs, peak RSS accounting files, model weight blobs, downloaded
checkpoints, virtual environments, and disk preflight logs.

**Evidence grades:**

- `measured`: Live telemetry, elapsed request times, process RSS files, model
  allocations, package versions, and disk preflight captured directly from
  `novidesign.local`.
- `admission`: Measured admission / preflight constraint failure on
  `novidesign.local` (e.g. disk capacity preflight halting model weight
  acquisition) or architecture negative admission (e.g. CUDA-dependent recipes
  on no-CUDA platforms).
- `published`: Upstream vendor or project engineering specification with canonical link.

> [!IMPORTANT]
> **Strict evaluation rule:** Installation and import evidence are never promoted
> to inference success. Where an environment was provisioned and dependencies
> or checkpoints were staged without an actual completed inference run, the result
> is recorded as environment/checkpoint evidence only. Throughput is never
> invented; only measured elapsed request times are reported.

## Acceptance matrix

| Model / host case | Result | Grade |
|---|---|---|
| **Qwen3-VL 8B vision request** (Ollama 0.23.4) | `7.457908791 s` elapsed (`POST /api/generate` with `vision-test.png`); Metal allocation 5.4 GiB weights + 4.5 GiB KV cache + 633.2 MiB compute graph; CPU 333.8 MiB weights + 63.3 MiB compute graph; log memory allocation `10.9 GiB`; peak RSS `11,621,408 KiB` (~11.08 GiB) | `measured` |
| **Qwen3-VL 8B text request** (Ollama 0.23.4) | `46.406641625 s` elapsed (`POST /api/generate`); peak RSS `11,656,592 KiB` (~11.12 GiB) | `measured` |
| **Qwen3-VL 8B weights / disk** | Ollama blob `sha256-ed12a4674d72...` measured at `6,140,392,576 bytes` (~6.14 GB), matching published 6.1 GB Q4_K_M footprint | `measured` + `published` ([ollama](https://ollama.com/library/qwen3-vl:8b)) |
| **FLUX.2 Klein 4B** | Download halted by preflight guard: host data volume `/System/Volumes/Data` was 97% full with only 16 GiB free (`/dev/disk3s5 460Gi total, 420Gi used, 16Gi avail`). Partial download stopped at 985 MiB in HuggingFace cache. Apache-2.0; publisher reports fitting ~8 GB GPU VRAM. 9B variant excluded (noncommercial license violates sprint open-source rule) | `admission` (measured storage failure) + `published` ([flux2](https://github.com/black-forest-labs/flux2)) |
| **ACE-Step 1.5** | Local HTTP API; fits from 6 GB GPU VRAM up to 12–24 GB+ for full models; no live run admitted due to 16 GiB free disk preflight boundary. MIT license | `admission` + `published` ([repo](https://github.com/ace-step/ACE-Step-1.5), [API](https://github.com/ace-step/ACE-Step-1.5/blob/main/docs/en/API.md)) |
| **Wan2.2 TI2V-5B** | 720p Text-Image-to-Video; requires ≥24 GB NVIDIA GPU with offloading, generating 5 s clips in minutes (suitable for background/batch generation, not interactive editor latency); no live run admitted due to storage preflight boundary. Apache-2.0 license | `admission` + `published` ([repo](https://github.com/Wan-Video/Wan2.2), [weights](https://huggingface.co/Wan-AI/Wan2.2-TI2V-5B)) |
| **Whisper** (`openai-whisper`) | Provisioned Python 3.14.5 venv (`openai-whisper==20250625`, `torch==2.14.1`), test audio `jfk.wav` (352 KB), and downloaded checkpoint `tiny.pt` (`75,572,083 bytes` / ~72.1 MiB); peak RSS file `635,280 KiB` (~620.4 MiB). No transcription output file present; not promoted to inference success. Transcription memory is not the capacity driver | `measured` (env/checkpoint/RSS) + `published` ([whisper](https://github.com/openai/whisper), [whisper.cpp](https://github.com/ggml-org/whisper.cpp)) |
| **SAM2** (`facebookresearch/sam2`) | Provisioned Python 3.14.5 venv (`sam_2==1.0`, `torch==2.14.1`, `hydra-core==1.3.7`) and downloaded checkpoint `sam2.1_hiera_tiny.pt` (`156,008,466 bytes` / ~148.8 MiB); peak RSS file `844,784 KiB` (~825.0 MiB). No mask propagation output file present; not promoted to inference success. Upstream recipe is CUDA-centered; Apple Silicon MPS path is preliminary and substantially slower | `measured` (env/checkpoint/RSS) + `published` ([sam2](https://github.com/facebookresearch/sam2), [demo](https://github.com/facebookresearch/sam2/blob/main/demo/README.md)) |
| **noviwin AMD / no-CUDA host** | Negative admission: CUDA-centered execution recipes (SAM2 full pipeline, Wan2.2 ≥24 GB NVIDIA offloading) cannot be admitted on AMD or non-CUDA Windows/Linux environments without significant upstream acceleration rewrites | `admission` + `published` |

## Host telemetry and verified cleanup

Telemetry measured on `novidesign.local` (epoch43):

- **Hardware:** Apple M4 Max, 14 cores (14 physical, 14 logical), 36.0 GiB unified memory (`hw.memsize: 38654705664`).
- **Accelerator / OS:** Metal 4 (`MTLGPUFamilyMetal4`, `MTLGPUFamilyApple9`), 28.1 GiB Metal compute memory available out of 36.0 GiB total; macOS 26.3 (Build 25D125), Darwin 25.3.0 arm64.
- **Disk preflight:** APFS volume `/System/Volumes/Data` measured at 460 GiB total, 420 GiB used, 16 GiB available (97% full). This empirical storage ceiling caused immediate admission failure for multi-gigabyte diffusion and video weights.
- **Reclamation:** Following evidence capture, `/tmp/df42-xkz0Ue` (9.6 GiB) and `/tmp/df42-active-dir` were removed over SSH. Post-cleanup verification showed disk usage dropped to 410 GiB used and available disk increased to 26 GiB (10 GiB reclaimed, capacity dropped to 95%).
- **Process isolation:** Verified zero Sprint382 processes running; all five preexisting baseline `ycode-runner --ollama-engine` runners (PIDs 40808, 40771, 40573, 40311, 40229) predating epoch43 remained running and untouched.

## Editor coexistence arithmetic

Running the Bashy/MCP agent alongside creative applications (Kdenlive, GIMP,
Audacity, Ardour, Natron, Blender, Story Architect, Kitsu, OpenToonz) requires
budgeting RAM, GPU VRAM, CPU cores, and storage bandwidth.

### Measured anchor

- One resident vision model (Qwen3-VL 8B Q4_K_M) consumes **10.9 GiB allocated memory**
  in Ollama (5.4 GiB Metal weights + 4.5 GiB KV cache + 633 MiB compute graph)
  and peaks at **~11.1 GiB RSS**.
- Auxiliary lightweight models: Whisper tiny requires ~0.6 GiB RSS; SAM2 tiny
  requires ~0.8 GiB RSS.
- A single resident vision model and lightweight audio tool together occupy
  ~11.7–12.5 GiB memory.

### Planning tiers

1. **12–16 GB GPU VRAM + 32 GB RAM (Minimal / Sequential tier):**
   - Sufficient for one resident lightweight multimodal model (Qwen3-VL 8B ~11 GiB)
     and one active creative editor (e.g. GIMP, Audacity, Ardour, or Story Architect).
   - Workloads must run **strictly sequentially**: the user pauses editing while
     an inference step runs, or the model runs in batch mode when the editor is idle.
   - Concurrent heavy 3D rendering (Blender) or multi-track compositing (Natron/Kdenlive)
     will cause memory paging or OOM.
2. **24 GB NVIDIA GPU VRAM + 64 GB RAM + 8–12 core CPU + 2 TB NVMe (Recommended Default):**
   - Accommodates one primary multimodal model (Qwen3-VL 8B) plus auxiliary segmentation
     (SAM2) or audio transcription (Whisper) alongside an active NLE or digital audio workstation.
   - Allows background model inference while maintaining smooth timeline scrub and UI
     responsiveness in Kdenlive, Ardour, or Blender.
   - Generative image synthesis (FLUX.2 Klein 4B) can be scheduled sequentially without
     evicting the primary vision inspection model.
3. **48 GB+ GPU VRAM + 128 GB RAM (High-throughput / Concurrent Studio tier):**
   - Required for concurrent multi-model pipelines: e.g. simultaneous vision inspection,
     SAM2 video object tracking, and FLUX.2 or Wan2.2 video generation alongside multi-track
     4K/8K editing in Natron and Kdenlive.
4. **Apple Silicon Considerations:**
   - 64 GB unified memory is the recommended baseline (96 GB+ preferred) for concurrent
     creative application suites.
   - Because unified memory is shared between system applications and GPU buffers,
     a 36 GB host (like `novidesign.local`) can support Qwen3-VL 8B (11 GiB) and an editor,
     but leaves insufficient headroom for concurrent diffusion or multi-stream video pipelines.
   - MPS acceleration is preliminary for SAM2 and unavailable for Wan2.2 CUDA offloading;
     unified memory capacity is not an architectural substitute for CUDA acceleration.
5. **Storage Reserves:**
   - A dedicated 2 TB+ NVMe drive is essential. Project files, original audio/video media,
     render caches, and model checkpoints compete directly for disk space. As demonstrated
     by the 97% capacity preflight failure on `novidesign.local`, a 16 GiB free headroom
     prevents downloading required model weights.

## Recommendations and fallbacks

### Recommended host specification

- **Hardware:** 24 GB NVIDIA GPU (RTX 3090/4090 or professional equivalent), 64 GB system RAM,
  8–12 core modern CPU, and 2 TB NVMe storage.
- **Workflow:** Sequential model/editor invocation; unload large generative checkpoints
  when returning to timeline rendering.

### Low-memory and no-CUDA fallbacks

- **Model quantization:** Pin quantized checkpoints (e.g. Qwen3-VL 8B Q4_K_M at 6.1 GB)
  rather than 16-bit unquantized variants.
- **Lightweight auxiliary models:** Restrict local speech recognition to Whisper tiny
  (~72 MB weights, ~620 MB RSS) or `whisper.cpp`, and segmentation to SAM2 tiny (~149 MB weights).
- **Sequential eviction:** Ensure Ollama or local inference runners unload idle models
  (`OLLAMA_KEEP_ALIVE=0` or explicit unload) before launching diffusion pipelines.
- **Remote / cloud offloading:** Defer heavy video generation (Wan2.2 TI2V-5B) and large
  image diffusion (FLUX) to remote GPU workers or dedicated API servers when local VRAM
  or disk headroom fails admission.

## License posture

All selected models comply with the sprint open-source and permissive-bridge policy:

- **Apache-2.0:** FLUX.2 Klein 4B, SAM 2 (`facebookresearch/sam2`), Wan2.2 TI2V-5B.
- **MIT:** ACE-Step 1.5, OpenAI Whisper (`openai/whisper` and `whisper.cpp`).
- **Excluded:** FLUX.2 9B is excluded due to its noncommercial license restriction,
  which violates the sprint permissive licensing rule.

## Built-in MCP server independence

> [!NOTE]
> **No model prerequisite for MCP:** The built-in MCP server in Bashy operates
> purely on deterministic native project files, application interchange formats,
> and CLI automation interfaces (e.g. Kdenlive XML, Blender Python APIs, GIMP scripts).
> Local multimodal model inference and generative video are optional companion
> capabilities for automated asset creation; **no model or GPU is required** to run
> or use the built-in MCP server.

---
id: df42d8bc7b65
kind: spike
title: Validate local multimodal model hardware for agent media editing
seq: 356
status: todo
created: 2026-09-30T07:39:48.317848Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Research brief for the agent-only media workflow in sprint #336. Determine a reproducible local hardware baseline for running the Bashy/MCP agent alongside Kdenlive, GIMP, Audacity, Ardour, Natron, Blender, Story Architect, Kitsu, and OpenToonz, with specialized models invoked through local APIs or commands. Keep the model orchestration distinct from editor operations: models inspect or generate assets; agent tools apply edits to native projects.

Published facts to use as baselines (verify current versions and exact checkpoints before implementation):
- Ollama qwen3-vl:8b is a 6.1 GB Q4_K_M download, with text and image input and tool support; the file size is not a complete runtime-memory requirement. https://ollama.com/library/qwen3-vl:8b
- FLUX.2 Klein 4B is Apache-2.0 and its publisher says it fits in about 8 GB of GPU memory for image generation/editing. The 9B variant has a noncommercial license and is outside this sprint's open-source rule. https://github.com/black-forest-labs/flux2
- ACE-Step 1.5 is MIT-licensed, has a local HTTP API, and publishes model choices from 6 GB GPU memory upward; larger music models benefit from 12-24 GB or more. https://github.com/ace-step/ACE-Step-1.5 and https://github.com/ace-step/ACE-Step-1.5/blob/main/docs/en/API.md
- Whisper code and weights are MIT-licensed. whisper.cpp reports roughly 0.3-4 GB model memory across tiny through large; transcription is not the main capacity driver. https://github.com/openai/whisper and https://github.com/ggml-org/whisper.cpp
- SAM 2 is Apache-2.0 and provides video mask propagation. Its published setup centers CUDA; MPS support is preliminary and can be substantially slower, so benchmark the actual host. https://github.com/facebookresearch/sam2 and https://github.com/facebookresearch/sam2/blob/main/demo/README.md
- Wan2.2 TI2V-5B is Apache-2.0, supports text/image-to-video at about 720p, and documents at least 24 GB NVIDIA GPU memory with offloading. Published 5-second generation takes minutes, so 24 GB is feasible rather than interactive. https://github.com/Wan-Video/Wan2.2 and https://huggingface.co/Wan-AI/Wan2.2-TI2V-5B

Planning tiers (engineering estimates, not vendor minimums): 12-16 GB GPU + 32 GB RAM for Qwen/Whisper and sequential light image/music jobs; 24 GB NVIDIA GPU + 64 GB RAM + modern 8-12 core CPU + 2 TB NVMe as the reasonably responsive default with sequential workloads; 48 GB+ GPU + 128 GB RAM for simultaneous editing/inference or frequent generation. On Apple silicon, estimate 64 GB unified memory minimum and 96 GB+ preferable for concurrent creative apps, but do not assume this is equivalent to CUDA VRAM or that Wan's CUDA recipe will work unchanged. Reserve storage for checkpoints, cache, media originals, proxies, renders, and project versions.

Acceptance for this research story: publish a tested per-model/per-editor memory and throughput matrix on representative Linux/NVIDIA and macOS/Apple-silicon hosts where available; distinguish downloaded weight size, peak GPU/unified memory, system RAM, disk footprint, and elapsed time; specify how many models can stay resident beside an editor; record lower-memory fallback choices and the recommended host profile. Do not make model-assisted editing or video generation a prerequisite for the built-in MCP server itself. Keep all selected model weights and integration code under licenses compatible with the sprint's open-source/permissive-bridge rule.

"""Local-only Wan2.2 inference probe for the native Windows spike."""

import runpy
import sys
import threading
import time
import types
from pathlib import Path

import psutil
import torch
import torch.distributed

if not hasattr(torch.distributed, "is_initialized"):
    torch.distributed.is_initialized = lambda: False

root = Path(__file__).resolve().parent
code = root / "Wan2.2"
sys.path.insert(0, str(code))

# AMD's Windows Torch omits FSDP. Wan imports this module at package import
# time even when single-GPU inference disables both FSDP flags.
fsdp_shim = types.ModuleType("wan.distributed.fsdp")


def unavailable_fsdp(*_args, **_kwargs):
    raise RuntimeError("FSDP is unavailable in AMD Windows Torch")


fsdp_shim.shard_model = unavailable_fsdp
sys.modules["wan.distributed.fsdp"] = fsdp_shim

# Wan's package initializer eagerly imports every task, including S2V and
# Animate with optional decord/peft dependencies. Load only TI2V for this run.
wan_package = types.ModuleType("wan")
wan_package.__path__ = [str(code / "wan")]
sys.modules["wan"] = wan_package
from wan.textimage2video import WanTI2V  # noqa: E402

wan_package.WanTI2V = WanTI2V

from wan.modules.vae2_2 import Wan2_2_VAE  # noqa: E402


start = time.monotonic()
peak_rss = 0
stop = False


def sample_memory():
    global peak_rss
    process = psutil.Process()
    while not stop:
        peak_rss = max(peak_rss, process.memory_info().rss)
        time.sleep(0.25)


sampler = threading.Thread(target=sample_memory, daemon=True)
sampler.start()
original_decode = Wan2_2_VAE.decode


def measured_decode(self, latents):
    print(
        "VAE_DECODE_START",
        "device", self.device,
        "parameter_device", next(self.model.parameters()).device,
        "latent_device", latents[0].device,
        flush=True,
    )
    decoded = original_decode(self, latents)
    torch.cuda.synchronize()
    print("VAE_DECODE_END", "shape", decoded[0].shape, flush=True)
    return decoded


Wan2_2_VAE.decode = measured_decode
sys.argv = [
    str(code / "generate.py"),
    "--task", "ti2v-5B",
    "--size", "1280*704",
    "--frame_num", "17",
    "--sample_steps", "4",
    "--ckpt_dir", str(root / "Wan2.2-TI2V-5B"),
    "--offload_model", "True",
    "--convert_model_dtype",
    "--t5_cpu",
    "--save_file", str(root / "ti2v-5b-native.mp4"),
    "--prompt", "A fictional singer turns to camera on a softly lit stage, gentle motion, original character, cinematic",
]
try:
    runpy.run_path(str(code / "generate.py"), run_name="__main__")
finally:
    stop = True
    sampler.join(timeout=2)
    print(
        "SPIKE_METRICS",
        "wall_seconds", round(time.monotonic() - start, 3),
        "peak_rss_bytes", peak_rss,
        "gpu_peak_allocated_bytes", torch.cuda.max_memory_allocated(),
        "gpu_peak_reserved_bytes", torch.cuda.max_memory_reserved(),
        flush=True,
    )

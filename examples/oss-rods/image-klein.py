"""Offline FLUX.2 klein-4B generate/edit driver for image-klein.bsh."""
import argparse
import os
from pathlib import Path

os.environ["HF_HUB_OFFLINE"] = "1"
os.environ["TRANSFORMERS_OFFLINE"] = "1"

ap = argparse.ArgumentParser()
ap.add_argument("--role", required=True)
ap.add_argument("--model", required=True)
ap.add_argument("--prompt", required=True)
ap.add_argument("--width", type=int, required=True)
ap.add_argument("--height", type=int, required=True)
ap.add_argument("--seed", type=int, required=True)
ap.add_argument("--image", default="")
ap.add_argument("--outdir", required=True)
a = ap.parse_args()

import torch
from diffusers import Flux2KleinPipeline
from PIL import Image

if not torch.cuda.is_available():
    raise RuntimeError("FLUX.2 klein-4B draft inference needs a supported GPU")
pipe = Flux2KleinPipeline.from_pretrained(
    a.model, torch_dtype=torch.bfloat16, local_files_only=True
)
pipe.enable_model_cpu_offload()
kwargs = dict(
    prompt=a.prompt,
    width=a.width,
    height=a.height,
    guidance_scale=1.0,
    num_inference_steps=4,
    generator=torch.Generator(device="cpu").manual_seed(a.seed),
)
if a.role == "image.edit":
    kwargs["image"] = Image.open(a.image).convert("RGB")
out = pipe(**kwargs).images[0]
path = Path(a.outdir) / ("edit-01.png" if a.role == "image.edit" else "image-01.png")
out.save(path)
print(path, flush=True)

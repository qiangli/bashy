# animate-echomimic-patch.py SRC DST — the two inference patches proved on
# the AWS A10G (Sprint 635 attempt 12), applied to a COPY of infer_flash.py.
# Run by the rod's staged venv python at --stage time; never at render.
#   1. low_cpu_mem_usage=False: a plain load (the meta-device load leaves
#      tensors empty under diffusers 0.33.1).
#   2. the authors' mmgp offloader (app_mm.py), LowRAM_HighVRAM with an 80 %
#      VRAM budget, in place of the first pipeline.to(device); the second
#      pipeline.to is neutralised (mmgp owns placement).
import sys

src, dst = sys.argv[1], sys.argv[2]
with open(src, encoding="utf-8", newline="") as f:
    s = f.read()
s = s.replace("low_cpu_mem_usage=True if not fsdp_dit else False", "low_cpu_mem_usage=False")
s = s.replace("low_cpu_mem_usage=True,", "low_cpu_mem_usage=False,")
nl = "\r\n" if "\r\n" in s else "\n"
old = "    pipeline.to(device=device)" + nl
i = s.find(old)
if i < 0:
    sys.exit("PATCH_FAILED: pipeline.to(device=device) not found in " + src)
new = nl.join([
    "    from mmgp import offload, profile_type",
    "    budgets = int(torch.cuda.get_device_properties(0).total_memory/1048576 * 0.80)",
    "    print('mmgp offload budget', budgets, 'MB (LowRAM_HighVRAM)')",
    "    offload.profile(pipeline, profile_type.LowRAM_HighVRAM, budgets={'*': budgets}, compile=False)",
]) + nl
s = s[:i] + new + s[i + len(old):]
s = s.replace(old, "    pass  # second pipeline.to removed: mmgp offload owns placement" + nl)
with open(dst, "w", encoding="utf-8", newline="") as f:
    f.write(s)
print("PATCHED_MMGP")

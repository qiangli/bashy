# Sprint 635: EchoMimicV3 Flash natively on the second Windows host (RTX 3070 8 GB), zero cost.
# Isolated under C:\s635; the gate podman machine is never touched. Stage once, then run the 5 s shot-03 proof.
$ErrorActionPreference = 'Continue'
$base = 'C:\s635'
$bashy = "$env:LOCALAPPDATA\outpost\bashy.exe"
$venv = "$base\venv"
$py   = "$venv\Scripts\python.exe"
$src  = "$base\echomimic_v3"
$w    = "$base\weights"
$out  = "$base\out\proof-01"
New-Item -ItemType Directory -Force -Path $base, $w, $out, "$base\in" | Out-Null
Start-Transcript -Path "$base\stage.log" -Append | Out-Null
Write-Output ("START " + (Get-Date -Format o))
nvidia-smi --query-gpu=name,driver_version,memory.total,memory.used --format=csv,noheader
# --- toolchain: uv-managed Python 3.10 in an isolated venv (the Store python is a stub)
if (-not (Test-Path $py)) {
  & $bashy uv python install 3.10 2>&1 | Select-Object -Last 1
  & $bashy uv venv $venv --python 3.10 2>&1 | Select-Object -Last 1
}
& $py --version
# --- ffmpeg: provisioned through bashy's uv as the imageio-ffmpeg package (no winget); exported below for moviepy and the review grids
# --- source at the pinned revision
if (-not (Test-Path "$src\.git")) { git clone -q https://github.com/antgroup/echomimic_v3 $src }
Push-Location $src; git checkout -q 7e89489ca51c0d008fc1963ec6c03fc5bd0b9397; git status --short | Select-Object -First 3; Pop-Location
# --- pinned stack (proved on the AWS A10G): torch 2.6.0 cu124, diffusers 0.33.1, transformers 4.51.3; no TensorFlow / retina-face / gradio
if (-not (Test-Path "$venv\Lib\site-packages\diffusers")) {
  & $bashy uv pip install --python $py torch==2.6.0 torchvision==0.21.0 torchaudio==2.6.0 --index-url https://download.pytorch.org/whl/cu124 2>&1 | Select-Object -Last 1
  & $bashy uv pip install --python $py diffusers==0.33.1 transformers==4.51.3 accelerate==1.6.0 numpy==1.26.4 huggingface_hub==0.30.2 safetensors einops omegaconf decord==0.6.0 moviepy==2.2.1 "imageio[ffmpeg]" librosa scikit-image opencv-python SentencePiece ftfy func_timeout pyloudnorm timm torchdiffeq torchsde beautifulsoup4 tomesd albumentations scenedetect onnxruntime mmgp 2>&1 | Select-Object -Last 1
}
$env:USE_TF = '0'; $env:TRANSFORMERS_NO_TF = '1'; $env:HF_HUB_DISABLE_XET = '1'; $env:HF_HUB_ENABLE_HF_TRANSFER = '0'
$env:PYTORCH_CUDA_ALLOC_CONF = 'expandable_segments:True'
$ff = (& $py -c "import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())" 2>$null | Select-Object -Last 1)
Write-Output ('ffmpeg (via bashy uv / imageio-ffmpeg): ' + $ff)
if ($ff) { $env:IMAGEIO_FFMPEG_EXE = $ff; $env:FFMPEG_BINARY = $ff; $env:Path = (Split-Path $ff) + ';' + $env:Path }
& $py -c "import torch, diffusers, transformers, accelerate, decord, moviepy, librosa, pyloudnorm; from mmgp import offload, profile_type; print('stack ok torch', torch.__version__, 'cuda', torch.cuda.is_available(), torch.cuda.get_device_name(0), 'diffusers', diffusers.__version__, 'transformers', transformers.__version__)"
Push-Location $src
& $py -c "import sys; sys.path.insert(0,'.'); from src.wan_transformer3d_audio_2512 import WanTransformerAudioMask3DModel; from src.pipeline_wan_fun_inpaint_audio_2512 import WanFunInpaintAudioPipeline; from src.wav2vec2 import Wav2Vec2Model; from src.wan_vae import AutoencoderKLWan; from src.wan_text_encoder import WanT5EncoderModel; from src.wan_image_encoder import CLIPModel; from src.fm_solvers_unipc import FlowUniPCMultistepScheduler; print('MODEL_IMPORTS_OK')"
# --- patches proved on AWS: plain load; the authors' mmgp offloader (app_mm.py) in place of BOTH pipeline.to(device) calls
if (-not (Select-String -Path infer_flash.py -Pattern 'offload.profile' -Quiet)) {
  $s = Get-Content infer_flash.py -Raw
  $s = $s -replace 'low_cpu_mem_usage=True if not fsdp_dit else False', 'low_cpu_mem_usage=False' -replace 'low_cpu_mem_usage=True,', 'low_cpu_mem_usage=False,'
  $old = "    pipeline.to(device=device)`n"
  $new = "    from mmgp import offload, profile_type`n    budgets = int(torch.cuda.get_device_properties(0).total_memory/1048576 * 0.80)`n    print('mmgp offload budget', budgets, 'MB (LowRAM_HighVRAM)')`n    offload.profile(pipeline, profile_type.LowRAM_HighVRAM, budgets={'*': budgets}, compile=False)`n"
  $i = $s.IndexOf($old); if ($i -lt 0) { $old = $old -replace "`n", "`r`n"; $new = $new -replace "`n", "`r`n"; $i = $s.IndexOf($old) }
  if ($i -ge 0) { $s = $s.Substring(0, $i) + $new + $s.Substring($i + $old.Length); $s = $s.Replace($old, "    pass  # second pipeline.to removed: mmgp offload owns placement" + $old.Substring($old.Length - $(if ($old.EndsWith("`r`n")) { 2 } else { 1 }))); [IO.File]::WriteAllText("$src\infer_flash.py", $s); Write-Output 'PATCHED_MMGP' } else { Write-Output 'PATCH_FAILED: pipeline.to(device=device) not found' }
}
Select-String -Path infer_flash.py -Pattern 'offload.profile|pipeline.to\(device|low_cpu_mem_usage' | ForEach-Object { $_.LineNumber.ToString() + ': ' + $_.Line.Trim() }
# --- weights (pinned revisions), fetched once (~12 GB)
function hfget { param([string[]]$a) & $py -m huggingface_hub.commands.huggingface_cli download @a 2>&1 | Select-Object -Last 1 }
if (-not (Test-Path "$w\Wan2.1-Fun-V1.1-1.3B-InP\diffusion_pytorch_model.safetensors")) { hfget @('alibaba-pai/Wan2.1-Fun-V1.1-1.3B-InP','--revision','fc913c34361f4ec879e2f9c78b4f11ae50a937d1','--local-dir',"$w\Wan2.1-Fun-V1.1-1.3B-InP") }
if (-not (Test-Path "$w\chinese-wav2vec2-base\pytorch_model.bin")) { hfget @('TencentGameMate/chinese-wav2vec2-base','--revision','3991242c806928916fff4a8c0e4f76acf661b743','--local-dir',"$w\chinese-wav2vec2-base") }
if (-not (Test-Path "$w\transformer\diffusion_pytorch_model.safetensors")) {
  hfget @('BadToBest/EchoMimicV3','echomimicv3-flash-pro/diffusion_pytorch_model.safetensors','echomimicv3-flash-pro/config.json','--revision','311e176905a8c4c24b240b530488fe636ce4d249','--local-dir',"$w\dl")
  New-Item -ItemType Directory -Force -Path "$w\transformer" | Out-Null; Copy-Item "$w\dl\echomimicv3-flash-pro\*" "$w\transformer\"
}
(Get-FileHash "$w\transformer\diffusion_pytorch_model.safetensors" -Algorithm SHA256).Hash
Write-Output ("WEIGHTS_READY " + (Get-Date -Format o) + " free GB " + [math]::Round((Get-PSDrive C).Free/1GB))
Get-FileHash "$base\in\nova.png","$base\in\nova-5s.wav" -Algorithm SHA256 | ForEach-Object { $_.Hash.Substring(0,16) + ' ' + (Split-Path $_.Path -Leaf) }
# --- the 5 s proof: one GPU job, 60-minute watchdog
$t0 = Get-Date
$gargs = @('infer_flash.py','--image_path',"$base\in\nova.png",'--audio_path',"$base\in\nova-5s.wav",
  '--prompt','A young woman with silver hair sings into a vintage microphone on a lantern-lit harbor stage at night, close-up, lips moving with the song, gentle head motion.',
  '--num_inference_steps','8','--config_path','config/config.yaml','--model_name',"$w\Wan2.1-Fun-V1.1-1.3B-InP",'--ckpt_idx','50000',
  '--transformer_path',"$w\transformer\diffusion_pytorch_model.safetensors",'--save_path',$out,'--wav2vec_model_dir',"$w\chinese-wav2vec2-base",
  '--sampler_name','Flow_Unipc','--video_length','125','--guidance_scale','6.0','--audio_guidance_scale','3.0','--audio_scale','1.0',
  '--neg_scale','1.0','--neg_steps','0','--seed','7011','--enable_teacache','--teacache_threshold','0.1','--num_skip_start_steps','5',
  '--riflex_k','6','--ulysses_degree','1','--ring_degree','1','--weight_dtype','bfloat16','--sample_size','512','512','--fps','25',
  '--add_prompt','','--negative_prompt','','--shift','5.0')
$p = Start-Process -FilePath $py -ArgumentList $gargs -WorkingDirectory $src -NoNewWindow -PassThru -RedirectStandardOutput "$out\gen.out.log" -RedirectStandardError "$out\gen.err.log"
if (-not $p.WaitForExit(60*60*1000)) { Write-Output 'WATCHDOG: killing the generation after 60 min'; Stop-Process -Id $p.Id -Force }
Write-Output ("GEN_SECONDS " + [int]((Get-Date) - $t0).TotalSeconds + " exit " + $p.ExitCode)
Pop-Location
Get-ChildItem $out | Select-Object Name,Length | Format-Table -AutoSize | Out-String
Get-Content "$out\gen.err.log" -Tail 8
$clip = Get-ChildItem $out -Filter *.mp4 -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if ($clip) { Write-Output ("CLIP " + $clip.FullName + " " + $clip.Length + " bytes sha256 " + (Get-FileHash $clip.FullName -Algorithm SHA256).Hash) }
Write-Output ("FINISH " + (Get-Date -Format o))
Stop-Transcript | Out-Null

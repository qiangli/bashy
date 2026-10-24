# examples/dks — cluster worker commands as skill-tier Bash# units

`join.bsh` makes the host it runs on a worker of its satellite's DKS cluster
(or removes it with `--leave`) and reports the node it became: OS, GPU kind,
VRAM. Like `examples/record`, it is one Bash# file that carries its own skill
and composes the host's Outpost; bashy's core is untouched.

Register it on the host that should join, then run it there:

```bash
bashy commands add dks-join --set exec.0=bashy --set exec.1=--bashsharp \
  --set exec.2="$PWD/examples/dks/join.bsh" \
  --set synopsis="join this host to its satellite's DKS cluster as a worker; prints the node and GPU labels" \
  --set long="$(bashy --bashsharp examples/dks/join.bsh --skill)" \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
dks-join                 # Windows/macOS: a native node; Linux: vk-podman
dks-join --leave
```

GPUs are vendor-neutral: NVIDIA, AMD, Intel and Apple are labelled by kind
(`outpost.dhnt.io/gpu-kind`) and memory (`dhnt.io/vram`); NVIDIA also gets a
`nvidia.com/gpu` slot. Jobs select by capability, never by host name.

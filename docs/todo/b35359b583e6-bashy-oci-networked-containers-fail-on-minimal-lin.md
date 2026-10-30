---
id: b35359b583e6
kind: bug
title: 'bashy oci: networked containers fail on minimal Linux (netavark needs host ''nft'')'
seq: 2846
status: todo
priority: p2
labels:
    - linux
created: 2026-10-30T00:35:47.758247Z
---

Found 2026-10-29 during S525.A5w-oci2 live proof on elephant WSL Ubuntu (bashy 394dc596): 'bashy oci run --rm ghcr.io/qiangli/bashy-scratch:c119fd68 uname -a' -> 'Error: netavark: nftables error: unable to execute "nft": No such file or directory'. With --network=none it runs (Linux 6.18 WSL2). Shaping islands always use --network=none, so S525 is unaffected; but bashy oci's default bridge networking is not zero-setup on minimal Linux, which the operator rule treats as a bug. Fix candidates: provision nft via bashy (binmgr, permissive licence check: nftables is GPL-2.0 -> download+exec only, never vendored) or configure netavark's iptables/none backend when nft is absent. Not in any sprint yet.

---
id: c732681ad538
kind: bug
title: 'zcode (Z.ai Code) is registered as a fleet tool and in agents (zcode-glm-5.3 etc.) but not installed on the dev host: no zcode binary, no ~/.zcode'
seq: 3283
status: todo
priority: p2
created: 2026-11-21T09:45:04.055265Z
---

2026-11-21: the operator asked to use zcode/glm for a story; 'command -v zcode' is empty and ~/.zcode does not exist, while 'bashy agent list' shows zcode-glm-5.3 (flat), zcode-glm-5.3-flash, zcode-bedrock-glm-5, zcode-deepseek-v4-pro as available. The tool record (bashy tool show zcode) says the source-built installation is intentionally a follow-up, and its launch needs ZCODE_API_KEY, which is not in the vault (GENIE_GLM_API_KEY and a Z.ai credential are). Root cause first: is installation expected through a managed pin (binmgr) and simply never run here, or is there no install path at all? Also: the fleet should not list a tool's agents as available when the binary is absent (fleet --auth reported 'zcode absent'). Fix the install path (operator: 'we may need to fix the installation') and map the key; meanwhile genie-glm-5.3 reaches the same GLM 5.3 key-based subscription.

---
id: 5c2553ca3490
kind: bug
title: 'intl fixture: fr_FR.ISO8859-1 locale not generated on ubuntu CI'
status: closed
stage: code
reporter: qiangli
created: 2026-07-13T11:12:38.811926Z
weave: 20
closed: 2026-07-13T11:22:31.087544Z
resolution: fixed
closed_by: qiangli
---

The bash-5.3 'intl' fixture fails on Linux CI: unicode1.sub prints 'you do not have the fr_FR.ISO8859-1 locale installed'. The CI locale step (.github/workflows/test.yml, 'Generate the locales the bash-5.3 fixtures require') generates de_DE.UTF-8/en_US.UTF-8/ru_RU.CP1251/zh_TW BIG5/zh_HK BIG5-HKSCS but NOT a working fr_FR.ISO8859-1. Two attempts failed: 'fr_FR.ISO8859-1 ISO-8859-1' (wrong name form) and 'fr_FR ISO-8859-1' (still not generated). TASK: make ubuntu-latest's locale-gen actually produce a locale usable as fr_FR.ISO8859-1, then the intl fixture passes. VERIFY self-contained in a container (no full bashy build needed): 'bashy podman run --rm ubuntu:latest bash -c "apt-get update -qq && apt-get install -y -qq locales && <your locale.gen edits> && locale-gen && locale -a | grep -i fr_fr && LC_ALL=fr_FR.ISO8859-1 locale charmap"' must show the fr_FR ISO-8859-1 locale present and selectable. Check whether ubuntu needs the entry in /etc/locale.gen uncommented (it ships commented in /usr/share/i18n/SUPPORTED form), or dpkg-reconfigure, or the ISO-8859-1 charmap package. Then update test.yml to match. GATE: the change is CI-config only (test.yml); do not touch shell code. Keep it brand-neutral. Do NOT claim it works until the container check above passes.

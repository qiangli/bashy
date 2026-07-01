# Koala Oneliners Smoke Result - 2026-07-01

## Scope

- Campaign: standard benchmark Batch 0.5, lightweight real Unix pipeline smoke.
- Benchmark source: Koala at `~/tests/bashy-standard-benchmarks/koala`, commit `5b6caaf6a96511c973f56a2c781535712d0f9337`.
- Suite: `oneliners`, min input, selected scripts only: `sort`, `wf`, `top-n`, `nfa-regex`.
- Input fetched: `https://atlas.cs.brown.edu/data/dummy/1M.txt`.
- Harness: `bashy podman`.
- Bashy image: `bashy-agent-shell:bashy-current`.
- GNU control image: original GNU Bash `5.3.0(2)-release`.

Raw summary:

- `results/standard-benchmarks/koala-oneliners-20260701/summary.csv`
- `results/standard-benchmarks/koala-oneliners-20260701/bashy.tsv`
- `results/standard-benchmarks/koala-oneliners-20260701/gnu.tsv`
- `results/standard-benchmarks/koala-oneliners-20260701/bashy.stderr`
- `results/standard-benchmarks/koala-oneliners-20260701/gnu.stderr`

Large `.out` files were intentionally not committed.

## Commands

Each script was run directly inside the corresponding container image:

```sh
/usr/local/bin/bashy scripts/${script}.sh inputs/1M.txt > /out/bashy-${script}.out
/usr/local/bin/bash  scripts/${script}.sh inputs/1M.txt > /out/gnu-${script}.out
```

## Result

| Shell | Script | RC | Duration |
| --- | --- | ---: | ---: |
| bashy | `sort` | 0 | 0.046360s |
| bashy | `wf` | 0 | 0.225505s |
| bashy | `top-n` | 0 | 0.216754s |
| bashy | `nfa-regex` | 2 | 0.005681s |
| GNU Bash 5.3 | `sort` | 0 | 0.037413s |
| GNU Bash 5.3 | `wf` | 0 | 0.052431s |
| GNU Bash 5.3 | `top-n` | 0 | 0.046606s |
| GNU Bash 5.3 | `nfa-regex` | 0 | 1.151355s |

Output comparison:

- `sort`: bashy output matches GNU output byte-for-byte. Both differ from Koala's min reference hash, likely because the fetched `1M.txt` was not normalized with Koala's `dos2unix` step.
- `wf`: bashy output matches GNU output and Koala min reference.
- `top-n`: bashy output matches GNU output and Koala min reference.
- `nfa-regex`: bashy fails; GNU succeeds.

The `nfa-regex` root cause is the embedded bashy/coreutils `grep` implementation:

```text
grep: back-reference \1 is not supported (RE2 has no back-references)
```

GNU grep accepts the BRE backreference used by Koala:

```sh
grep '\(.\).*\1\(.\).*\2\(.\).*\3\(.\).*\4'
```

## Interpretation

This smoke found a coreutils-conformance gap rather than a Bash parser/runtime gap. Bashy correctly drives the scripts, but its in-process `grep` is not GNU-compatible for BRE backreferences.

The successful `sort`, `wf`, and `top-n` scripts show that common text-pipeline workloads can already run correctly under bashy for simple pipelines. The failing `nfa-regex` gives a precise next improvement target for bashy's coreutils layer.

## Follow-Up

- Add `grep` BRE/ERE backreference support or a documented fallback path for regex features unsupported by RE2.
- Add `bashy commands`/shell-check reporting for `grep` patterns that require unsupported regex features.
- Consider a focused coreutils conformance test: `printf 'aabb\n' | grep '\(.\).*\1'`.

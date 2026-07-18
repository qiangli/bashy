# VSC-PCTS Utility Campaign Feasibility - 2026-07-18

Status: **public-safe Workstream C checkpoint.** This file records what may be
committed publicly about utility-campaign shardability. It intentionally omits
licensed test source, raw journals, private host names, credentials, and withheld
utilities results.

## Boundary

- `cert` is the authoritative certification-shaped profile. It is one declared
  SUT, one worker, one chunk, no shard flags, no cache reuse, no retries, and
  enough repeats to compare stability before dispatch.
- `reference` is the comparison arm. It keeps the same one-worker, one-chunk
  ordering constraints and swaps only the declared reference SUT.
- `campaign` is development feedback only. Its output must say that it is not
  certifiable, even when it uses the same licensed harness on a private host.

Distributed results may help find defects faster, but they do not certify
bashy and they may not be used as an Open Group certification result.

## Public Findings

The public Bashy repository does not contain the licensed VSC-PCTS utility
corpus or the private harness plan. From the public side, Workstream C can
therefore validate the guard contract and document the measurement slots, but
cannot honestly publish utility-unit timings, utility assertion identifiers, or
the private unit graph until the license scope explicitly permits that.

Current public-safe feasibility result:

| Field | Value |
|---|---|
| Total utility campaign time | **unknown publicly** - withheld with the private utilities arm |
| Longest independently invocable utility unit | **unknown publicly** - requires private harness-unit audit |
| Required chunk count for sub-30-minute feedback | **unknown publicly** - depends on the longest indivisible private unit and setup cost |
| Sub-30-minute campaign achievable | **unknown publicly** - not asserted from public evidence |
| Safe public manifest | **not produced yet** - wait for a private audit that reduces units to stable public identifiers or digests only |

## Shared-State Hazards To Measure Privately

The private harness audit must classify each candidate unit for:

- setup and teardown cost;
- dependence on global TET state or generated scenario files;
- use of shared temporary paths, current directory, locale, umask, clock, or
  environment;
- dependence on PATH utility resolution beyond the declared SUT;
- whether reruns mutate journals or harness state read by later units;
- whether the unit can be named publicly as a stable identifier or only by a
  private digest.

If any unit cannot be separated without changing its setup, environment, or
observable verdict, the campaign manifest must record that unit as indivisible.

## Execution Plan

1. Run the `cert` profile only as a serial, exclusive, certification-shaped run.
2. In the private workspace, inspect the utility harness at the coarsest public
   unit boundary that does not expose licensed source.
3. Measure setup time and unit wall time for each candidate boundary under the
   `campaign` profile.
4. Publish only a scrubbed manifest with stable public names or digests, unit
   counts, timing summaries, and infrastructure classifications.
5. If a safe manifest cannot be produced, use public surrogate suites for
   distributed feedback and keep the licensed utilities arm serial/private.

Until step 4 is complete, the public answer is deliberately negative: no
certifiable utility sharding claim exists, and no sub-30-minute campaign claim
is made.

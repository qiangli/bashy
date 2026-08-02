# VSC-PCTS regression baseline — 2026-08-01

Published under Open Group ticket #280298 for **conformance-work purposes**.
No "certified" / "passes the Open Group suite" claim is made; the suite is
never redistributed.

This is the **baseline for regression testing**. It records every test purpose
that fails under the bashy userland but **not** under the GNU baseline arm on
the same host, so it isolates what is ours from what the suite or the
environment fails anyway.


Derived from a **per-assertion** diff of the two arms, not a per-set count. A
test set can fail the same number of times in both arms on *different*
purposes; only a TP-level diff says what is actually ours.

```
bashy failing test purposes : 1602
GNU   failing test purposes : 1381
shared (NOT ours)           : 1311
BASHY-ONLY                  :  291   across 63 test sets
```

`at`, `diff`, `tail` are excluded throughout: they hit the 600 s cap under the
bashy arm only, so their journals are truncated and a "0 failures" there is not
a pass. Anything measured against them is under-measured, not clean.

**This file is the regression baseline.** A later run should reproduce these
291 or explain each difference. A *new* TP appearing here is a regression; one
disappearing is a fix worth naming in a commit.

## How the streams are cut

Partitioned by **code scope**, so agents do not collide on files. One stream is
deliberately not parallelisable (S1) and one is not work at all until triaged
(X). Counts are bashy-only failing assertions.

| stream | scope | TPs | parallel-safe |
|---|---|---|---|
| S1 | Regex engine — SINGLE OWNER, do not parallelise inside | 40 | **NO — single owner** |
| S2 | find — traversal, primaries, and a NO-list decision | 40 | yes |
| S3 | date/time formatting | 18 | yes |
| S4 | awk | 17 | yes |
| S5 | Text transforms — line/field/character | 40 | yes |
| S6 | Listing and dump formatting | 35 | yes |
| S7 | Exec / environment | 26 | yes |
| S8 | File operations and metadata | 44 | yes |
| S9 | Scheduling utilities | 5 | yes |
| S10 | Shell | 2 | yes |
| S11 | Small single-assertion utilities | 10 | yes |
| X | NOT OURS — triage before assigning | 14 | — triage first |

---

## S1 — Regex engine — SINGLE OWNER, do not parallelise inside  (40 assertions)

`coreutils/pkg/bre` + `cmds/sed`. sed and grep share the BRE/ERE engine; splitting them across agents means two agents editing one engine. Take the whole stream or none of it.

### `sed` — 37

- **TP4** — Assertion #4 (A): Test the -e and -f options
- **TP15** — Assertion #15 (A): Test general asseriton 34
- **TP40** — Assertion #40 (A): Test commands with no addresses
- **TP41** — Assertion #41 (A): Test using a decimal address
- **TP50** — Assertion #50 (A): Test general assertion 104 using BRE addressing
- **TP51** — Assertion #51 (B): Test general assertion 105 using BRE addressing
- **TP82** — Assertion #82 (A): Test general assertion 136 using BRE addressing
- **TP83** — Assertion #83 (A): Test general assertion 137 using BRE addressing
- **TP84** — Assertion #84 (A): Test general assertion 138 using BRE addressing
- **TP137** — Assertion #137 (A): Test general assertion 101 using the s command
- **TP138** — Assertion #138 (A): Test general assertion 102 using the s command
- **TP169** — *(no assertion text recorded)*
- **TP170** — Assertion #170 (A): Test general assertion 134 using the s command
- **TP171** — Assertion #171 (A): Test general assertion 135 using the s command
- **TP179** — Assertion #179 (A): Test general assertion 143 using the s command
- **TP188** — Assertion #188 (A): Test the s command and the special replacement string character '&'
- **TP189** — Assertion #189 (A): Test the s command and the escaped special character '&'
- **TP203** — *(no assertion text recorded)*
- **TP209** — Assertion #209 (C): Test general assertion 68 using the w wfile command
- **TP211** — Assertion #211 (A): Test general assertion 49 using the w wfile command
- **TP229** — Assertion #229 (A): LC_CTYPE is set to a valid value
- **TP230** — Assertion #230 (B): LC_MESSAGES is set to a valid value
- **TP232** — Assertion #232 (A): Verify that a semicolon character preceding a sed command has no effect
- **TP233** — Assertion #233 (A): Verify that a blank and semicolon characters preceding a sed command has no effect
- **TP234** — Assertion #234 (A): Verify that the pattern space is empty at the start of each cycle
- **TP238** — Assertion #238 (A): Verify that the hold space can hold at least 8192 bytes
- **TP246** — Assertion #246 (A): Verify that the sed a command writes data to standard output in the absence of N or n commands
- **TP253** — Assertion #253 (A): Verify that the sed command r rfile supplied to sed without the argument -n, writes the contents of rfile to standard output after the contents of the pattern space
- **TP256** — Assertion #256 (A): Verify that sed -w wfile overwrites existing wfile
- **TP257** — Assertion #257 (A): Verify that sed commands preceded by a ! are applied to addresses not selected
- **TP259** — Assertion #259 (A): Verify that sed commands enclosed in {} and the {} are preceded by by blank characters are applied in sequence to the addressed pattern space
- **TP260** — Assertion #260 (A): Verify that sed commands enclosed in {} and the {} are followed by by blank characters are applied in sequence to the addressed pattern space
- **TP261** — Assertion #261 (A): Verify that sed commands preceded by blank characters enclosed in {} are applied in sequence to the addressed pattern space
- **TP269** — Assertion #269 (A): Verify that the sed y commands with an alternate delimiter character is supported
- **TP271** — Assertion #271 (A): Verify that the sed y commands with an alternate delimiter character can use the alternate delimiter in the command
- **TP273** — Assertion #273 (A): Verify that the sed y commands supports use of the backslash character within the second string.
- **TP274** — Assertion #274 (A): Verify a successful call to sed returns 0

### `grep` — 3

- **TP137** — Assertion #137 (C): Test GA68 for input file
- **TP140** — Assertion #140 (A): Input file is standard input.
- **TP141** — Assertion #141 (A): Input file is not standard input.

## S2 — find — traversal, primaries, and a NO-list decision  (40 assertions)

`coreutils/cmds/find`. Self-contained. NOTE: some failures are `-exec`/`-ok`, which are NO-list (they shell out) — those need a DECISION, not a fix. Separate the primary/matching bugs from the NO-list ones before starting.

### `find` — 40

- **TP4** — Assertion #4 (A): Verify output of ga28
- **TP16** — Assertion #16 (A): GA23 with -atime n -ctime n -mtime n
- **TP17** — Assertion #17 (A): GA23 with -links n
- **TP19** — Assertion #19 (A): GA23 with -group gname
- **TP20** — Assertion #20 (A): GA23 with -user uname
- **TP23** — Assertion #23 (A): Test pattern matching against GA200
- **TP34** — Assertion #34 (B): Test pattern matching against GA211
- **TP38** — *(no assertion text recorded)*
- **TP56** — Assertion #56 (B): Primary -xdev
- **TP57** — Assertion #57 (A): Primary -prune
- **TP58** — Assertion #58 (A): Primary -perm mode
- **TP59** — Assertion #59 (A): Primary -perm -mode
- **TP62** — Assertion #62 (C): Primary -type b
- **TP63** — Assertion #63 (C): Primary -type c
- **TP64** — Assertion #64 (A): Primary -type d
- **TP65** — Assertion #65 (A): Primary -type p
- **TP70** — Assertion #70 (A): Primary -group
- **TP71** — Assertion #71 (A): Primary -group <group_number>
- **TP75** — Assertion #75 (A): Primary -mtime
- **TP76** — Assertion #76 (A): Primary -ctime
- **TP77** — Assertion #77 (A): Primary -exec
- **TP80** — Assertion #80 (A): Test GA62 for -exec utility_name
- **TP81** — Assertion #81 (A): Test GA63 for -exec utility_name
- **TP82** — Assertion #82 (A): Test GA64 for -exec utility_name
- **TP85** — Assertion #85 (A): Test GA67 for -exec utility_name
- **TP87** — Assertion #87 (C): Test GA69 for -exec utility_name
- **TP99** — Assertion #99 (C): Test GA69 for -ok utility_name
- **TP109** — Assertion #109 (A): -o operator
- **TP119** — Assertion #119 (A): GA45 when uname is invalid
- **TP120** — Assertion #120 (A): GA53 when <path> contains inaccessible file or directory
- **TP121** — Assertion #121 (C): When resolving a symbolic link generates a pathname longer than PATH_MAX, find may report an error
- **TP122** — Assertion #122 (A):
- **TP123** — Assertion #123 (A): find -H and a file operand is a symbolic link
- **TP124** — Assertion #124 (A): find -H and a file encountered during traversal of a file hierarchy is a symbolic link
- **TP125** — Assertion #125 (A): find -L and a file operand is a symbolic link
- **TP126** — Assertion #126 (A): find -L and a file encountered during traversal of a file hierarchy is a symbolic link
- **TP133** — Assertion #133 (A): Verify output of ga500
- **TP136** — Assertion #136 (A): Verify output of ga503
- **TP139** — Assertion #139 (A): Primary -newer with a symlink
- **TP143** — Assertion #143 (A): find -L and a symbolic link for which the referenced file does not exist

## S3 — date/time formatting  (18 assertions)

`coreutils/cmds/date`, `cmds/touch`. Dominated by the POSIX `%E*`/`%O*` modifiers, which fall back to the unmodified conversion in the C/POSIX locale — a bounded, well-specified change.

### `date` — 17

- **TP7** — Assertion #7 (A): TZ environment variable affects date
- **TP45** — Assertion #45 (A): date +%Ec generates same as date +%c
- **TP49** — Assertion #49 (A): date +%Ey generates same output as date +%y
- **TP51** — Assertion #51 (A): date +%EY generates same output as date +%Y
- **TP53** — Assertion #53 (A): date +%Od generates same output as date +%d
- **TP55** — Assertion #55 (A): date +%Oe generates same output as date +%e
- **TP57** — Assertion #57 (A): date +%OH generates same output as date +%H
- **TP59** — Assertion #59 (A): date +%OI generates same output as date +%I
- **TP61** — Assertion #61 (A): date +%Om generates same output as date +%m
- **TP63** — Assertion #63 (A): date +%OM generates same output as date +%M
- **TP65** — Assertion #65 (A): date +%OS generates same output as date +%S
- **TP67** — Assertion #67 (A): date +%Ou generates same output as date +%u
- **TP69** — Assertion #69 (A): date +%OU generates same output as date +%U
- **TP71** — Assertion #71 (A): date +%Ow generates same output as date +%w
- **TP73** — Assertion #73 (A): date +%OW generates same output as date +%W
- **TP75** — Assertion #75 (A): date +%Oy generates same output as date +%y
- **TP81** — Assertion #81 (A): GA39: Error cause message to standard error and non-zero exit

### `touch` — 1

- **TP62** — Assertion #62 (A): An argument beginning with '-' following an operand

## S4 — awk  (17 assertions)

`coreutils/cmds/awk`. Large and self-contained; no shared engine.

### `awk` — 17

- **TP25** — Assertion #25 (A): GA27: When a required option arg is missing an error occurs
- **TP110** — Assertion #110 (A):
- **TP170** — Assertion #170 (A):
- **TP171** — Assertion #171 (A):
- **TP172** — Assertion #172 (A):
- **TP173** — Assertion #173 (A):
- **TP179** — Assertion #179 (A):
- **TP180** — *(no assertion text recorded)*
- **TP191** — Assertion #191 (A):
- **TP198** — Assertion #198 (A):
- **TP207** — Assertion #207 (A):
- **TP290** — Assertion #290 (A):
- **TP373** — Assertion #373 (A):
- **TP479** — *(no assertion text recorded)*
- **TP480** — *(no assertion text recorded)*
- **TP518** — Assertion #518 (A):
- **TP519** — Assertion #519 (A):

## S5 — Text transforms — line/field/character  (40 assertions)

One file per command under `coreutils/cmds/`. Genuinely parallel — an agent can take any subset. Mostly small deltas (1–4 assertions each).

### `uniq` — 4

- **TP41** — *(no assertion text recorded)*
- **TP55** — Assertion #55 (B): If error occurs after outfile is created, it is not removed.
- **TP56** — *(no assertion text recorded)*
- **TP69** — Assertion #69 (A): An argument beginning with '-' following an operand

### `cut` — 3

- **TP41** — Assertion #41 (A): cut with valid operand and one invalid operand
- **TP44** — Assertion #44 (A): An argument beginning with '-' following an operand
- **TP45** — Assertion #45 (A): An empty input file is accepted as a text file

### `paste` — 3

- **TP40** — Assertion #40 (A): -s not specified when one or more input files cannot be opened
- **TP42** — Assertion #42 (A): An argument beginning with '-' following an operand
- **TP44** — Assertion #44 (A): empty input file with -s

### `tr` — 3

- **TP33** — Assertion #33 (A): GA39: Error causes message to standard error and non-zero exit
- **TP34** — Assertion #34 (A): An argument beginning with '-' following an operand
- **TP35** — *(no assertion text recorded)*

### `expand` — 3

- **TP20** — *(no assertion text recorded)*
- **TP21** — *(no assertion text recorded)*
- **TP32** — *(no assertion text recorded)*

### `head` — 3

- **TP24** — Assertion #24 (A): Test of GA32 - head doesn't read standard input
- **TP38** — *(no assertion text recorded)*
- **TP42** — Assertion #42 (B): LC_* or LANG set to value which begins with a slash

### `cat` — 3

- **TP24** — Assertion #24 (A): Verify output of ga49.
- **TP26** — Assertion #26 (A): An argument beginning with '-' following an operand
- **TP27** — Assertion #27 (A): pathname resolution of an empty symbolic link

### `unexpand` — 2

- **TP21** — *(no assertion text recorded)*
- **TP32** — *(no assertion text recorded)*

### `fold` — 2

- **TP44** — Assertion #44 (A): An argument beginning with '-' following an operand
- **TP47** — *(no assertion text recorded)*

### `comm` — 2

- **TP30** — Assertion #30 (A): GA45 when source_file is inaccessible
- **TP33** — Assertion #33 (A): pathname resolution of an empty symbolic link

### `cmp` — 2

- **TP5** — Assertion #5 (A): Verify that if files are same size but different byte values then command writes out in specified format.
- **TP25** — Assertion #25 (A): Verify contents of EOF message

### `tee` — 2

- **TP8** — Assertion #8 (A): The -i option causes tee to ignore SIGINT.
- **TP26** — Assertion #26 (A): Test of GA11 - the utility recreates files with correct attributes.

### `strings` — 2

- **TP34** — *(no assertion text recorded)*
- **TP36** — *(no assertion text recorded)*

### `csplit` — 1

- **TP7** — *(no assertion text recorded)*

### `split` — 1

- **TP39** — *(no assertion text recorded)*

### `sort` — 1

- **TP93** — *(no assertion text recorded)*

### `join` — 1

- **TP48** — Assertion #48 (A): Empty input file and empty output file

### `tsort` — 1

- **TP10** — *(no assertion text recorded)*

### `wc` — 1

- **TP33** — Assertion #33 (A): An argument beginning with '-' following an operand

## S6 — Listing and dump formatting  (35 assertions)

`cmds/ls`, `cmds/pr`, `cmds/od`. Output-shape work: column layout, long format, TZ rendering in listings.

### `pr` — 15

- **TP4** — Assertion #4 (A): Test pr without options
- **TP6** — Assertion #6 (A): Test pr with a file operand of - and with no file operand
- **TP9** — Assertion #9 (A): Test the -column option
- **TP10** — Assertion #10 (A): Test the -column and -a options
- **TP13** — Assertion #13 (A): Test the -e[gap] option and tabulation characters
- **TP14** — Assertion #14 (A): Test general assertion 23
- **TP15** — Assertion #15 (A): Test the -e[char][gap] option
- **TP17** — Assertion #17 (A): Test the -h option
- **TP23** — Assertion #23 (A): Test the -m option
- **TP24** — Assertion #24 (A): Test the -n[width] option
- **TP26** — Assertion #26 (A): Test the -n and -m option interaction
- **TP27** — Assertion #27 (A): Test the -n[char][width] option
- **TP30** — Assertion #30 (A): Test the -sc and -columns options
- **TP38** — Assertion #38 (A): Test general assertion 29
- **TP57** — Assertion #57 (A): An argument beginning with '+' following an operand

### `ls` — 12

- **TP39** — Assertion #39 (A): Verify output of ga67.
- **TP44** — Assertion #44 (A): Check that non-existing operand causes diagnostic to stderr.
- **TP46** — Assertion #46 (A): Reset the TZ environmental variable; see if conversion changes.
- **TP64** — Assertion #64 (A): long format output of directory gives a total line.
- **TP70** — Assertion #70 (A): Verify that ls -d, -F or -l on a symbolic link, writes the name of the symbolic link
- **TP75** — Assertion #75 (A): Verify that ls -H evaluates symbolic link operands
- **TP81** — Assertion #81 (A): Verify that ls will successfully operate with both -H and -L options specified
- **TP86** — Assertion #86 (A): Verify that when ls is supplied a symbolic link with a circular link in the path prefix, ls fails with non-zero exit value
- **TP93** — Assertion #93 (A): Verify -1 does not disable long format output
- **TP98** — Assertion #98 (A): Verify with -F and -p options that last option controls the output.
- **TP106** — Assertion #106 (A): An argument beginning with '-' following an operand
- **TP115** — *(no assertion text recorded)*

### `od` — 5

- **TP11** — Assertion #11 (A): GA39: Error causes message to standard error and non-zero exit
- **TP20** — Assertion #20 (A): Verify that when 'od' is called with the '-j skip' option and 'skip' is an octal number with a leading zero, then 'od' ignores the first octal 'skip' characters read from the input file before it commences writing the formatted input to standard output.
- **TP21** — Assertion #21 (A): Verify that when 'od' is called with the '-j skip' option and 'skip' consists of the characters 0x or 0X followed by a hexadecimal number, then 'od' ignores the first hexadecimal 'skip' characters read from the input file before it commences writing the formatted input to standard output.
- **TP37** — Assertion #37 (A): Verify that when 'od' is called with '-t specifier' where 'specifier' consists of a series of concatenated valid option-arguments, then 'od' treats the concatenation in same manner as separate '-t' options.
- **TP61** — Assertion #61 (A): An argument beginning with '-' following an operand

### `file` — 1

- **TP7** — Assertion #7 (C): When resolving a symbolic link generates a pathname longer than PATH_MAX, ...

### `cksum_03` — 1

- **TP16** — Assertion #16 (A): GA69

### `echo` — 1

- **TP8** — Assertion #8 (A): GA39: Error causes message to standard error and non-zero exit

## S7 — Exec / environment  (26 assertions)

Commands that exec another utility by argv. `env COMMAND` is NO-list and the suite tests exactly that, so part of this stream is the same decision as S2.

### `env` — 12

- **TP5** — Assertion #5 (A): Zero or more name=value pairs and a utility
- **TP6** — Assertion #6 (A): End-of-options delimiter test
- **TP7** — Assertion #7 (A): The -i option
- **TP9** — Assertion #9 (A): GA60
- **TP10** — Assertion #10 (A): GA61
- **TP11** — *(no assertion text recorded)*
- **TP12** — *(no assertion text recorded)*
- **TP13** — Assertion #13 (A): GA64
- **TP16** — Assertion #16 (A): GA67
- **TP19** — Assertion #19 (B): A standard utility processes its operands in the order specified
- **TP20** — Assertion #20 (A): Test of GA32 - env doesn't read standard input
- **TP26** — Assertion #26 (A): If env does not find utility, exit status 127

### `xargs` — 5

- **TP9** — Assertion #9 (A): GA26
- **TP11** — Assertion #11 (A): End-of-options delimiter test
- **TP29** — *(no assertion text recorded)*
- **TP32** — Assertion #32 (A): Search for an executable file using PATH information
- **TP36** — Assertion #36 (A): Test of GA38 - xargs produces no standard output.

### `nohup` — 3

- **TP4** — Assertion #4 (A): Nohup invokes a utility and ignores SIGHUP
- **TP10** — Assertion #10 (A): GA28: nohup recognizes -- as the end of options.
- **TP14** — *(no assertion text recorded)*

### `nice` — 3

- **TP6** — *(no assertion text recorded)*
- **TP18** — *(no assertion text recorded)*
- **TP32** — *(no assertion text recorded)*

### `time` — 2

- **TP19** — *(no assertion text recorded)*
- **TP20** — *(no assertion text recorded)*

### `kill_NE` — 1

- **TP8** — Assertion #8 (A): kill -l exit_status returns signal name without SIG prefix

## S8 — File operations and metadata  (44 assertions)

One file per command. Parallel-safe, though `cp`/`dd` are the two with real volume and are worth a dedicated agent each.

### `dd` — 10

- **TP5** — Assertion #5 (A): Verify \'dd bs=expr\' output, along with \'sync\', \'noerror\' and \'notrunc\'.
- **TP7** — Assertion #7 (A): Verify dd conv=? with non-sync, non-noerror and non-notrunc.
- **TP21** — Assertion #21 (A): Verify \'dd conv=lcase\' transforms characters to lowercase.
- **TP22** — Assertion #22 (A): Verify \'dd conv=ucase\' transforms characters to uppercase.
- **TP23** — Assertion #23 (A): Verify \'dd conv=swab\' reverses bytes.
- **TP24** — Assertion #24 (A): Verify \'dd conv=swab\' swaps bytes but ignores last byte on odd number of bytes.
- **TP30** — Assertion #30 (A): Verify \'dd (ibs
- **TP31** — Assertion #31 (A): Verify \'dd (ibs
- **TP33** — Assertion #33 (A): Verify \'dd (ibs
- **TP63** — *(no assertion text recorded)*

### `cp` — 8

- **TP17** — Assertion #17 (A): Copy a directory with umask have been preset
- **TP22** — Assertion #22 (A): Copy a reg. file with -i option and not affirm.answer
- **TP24** — Assertion #24 (A): Copy a reg. file with -i option and affirm.answer
- **TP71** — Assertion #71 (A): Test GA66 with a target_directory
- **TP84** — Assertion #84 (A): cp -RL symbolic_link target_file
- **TP86** — Assertion #86 (A): cp -R -P directory target
- **TP95** — Assertion #95 (A): Test GA503 with source_file
- **TP96** — Assertion #96 (A): cp -P without -R does not follow source_file symlinks

### `rm` — 6

- **TP6** — Assertion #6 (A): rm with a pathname whose last component is dot-dot
- **TP7** — Assertion #7 (A): rm with a pathname whose last component is dot
- **TP11** — Assertion #11 (A): Try rm a directory
- **TP14** — Assertion #14 (A): Remove a directory with -i mode
- **TP54** — Assertion #54 (A): Verify that when rm is called with a symbolic link as the target, only the symbolic link is deleted
- **TP57** — Assertion #57 (A): no operands when -f in effect

### `mkfifo` — 5

- **TP18** — Assertion #18 (A): When a single mode clause starts with 'a'
- **TP26** — Assertion #26 (A): When a single mode clause has op '+' and who and param are specified
- **TP29** — Assertion #29 (A): When a single mode clause has op '-' and who and param are specified
- **TP32** — Assertion #32 (A): When a single mode clause has op '=' and who and param are specified
- **TP47** — Assertion #47 (A): Test of GA38 - mkfifo produces no standard output.

### `du` — 4

- **TP9** — Assertion #9 (C): du with -H and -L
- **TP16** — Assertion #16 (A): An argument beginning with '-' following an operand
- **TP21** — *(no assertion text recorded)*
- **TP33** — *(no assertion text recorded)*

### `df` — 3

- **TP4** — *(no assertion text recorded)*
- **TP5** — *(no assertion text recorded)*
- **TP21** — *(no assertion text recorded)*

### `ln` — 2

- **TP35** — Assertion #35 (A): ln 2 source-files when the target is a directory
- **TP58** — Assertion #58 (A): ln without -L or -P on a symlink

### `mv` — 1

- **TP86** — Assertion #86 (A): source and dest are distinct directory entries for the same existing file

### `mkdir` — 1

- **TP59** — Assertion #59 (A): An argument beginning with '-' following an operand

### `rmdir` — 1

- **TP29** — Assertion #29 (A): In case of failure

### `chmod` — 1

- **TP56** — Assertion #56 (A): An argument beginning with '-' following an operand

### `chgrp` — 1

- **TP52** — Assertion #52 (A): An argument beginning with '-' following an operand

### `pathchk` — 1

- **TP26** — Assertion #26 (B): GA48: Full pathname usage in LC_* and LANG variables

## S9 — Scheduling utilities  (5 assertions)

`cmds/at`, `cmds/batch`. Note `at` itself is EXCLUDED from the baseline (it capped under bashy only), so treat this stream as under-measured.

### `batch` — 5

- **TP8** — *(no assertion text recorded)*
- **TP9** — *(no assertion text recorded)*
- **TP10** — *(no assertion text recorded)*
- **TP12** — *(no assertion text recorded)*
- **TP14** — *(no assertion text recorded)*

## S10 — Shell  (2 assertions)

NOT coreutils — `bashy cmd/bash` + the `sh` engine. Different repo scope entirely, so it never collides with a utilities agent.

### `sh_05` — 1

- **TP25** — *(no assertion text recorded)*

### `sh_12` — 1

- **TP15** — *(no assertion text recorded)*

## S11 — Small single-assertion utilities  (10 assertions)

One assertion each. Good warm-up work, and safe to hand to one agent as a batch since the files are disjoint.

### `basename` — 2

- **TP8** — Assertion #8 (A): GA39: Error causes message to standard error and non-zero exit
- **TP9** — Assertion #9 (A): An argument beginning with '-' following an operand

### `dirname` — 1

- **TP7** — Assertion #7 (B): GA39: Error causes message to standard error and non-zero exit

### `expr` — 1

- **TP125** — Assertion #125 (A): GA44

### `id` — 1

- **TP41** — *(no assertion text recorded)*

### `logname` — 1

- **TP7** — Assertion #7 (A): GA39: Error causes message to standard error and non-zero exit

### `pwd` — 1

- **TP6** — Assertion #6 (A): GA39: Error causes message to standard error and non-zero exit

### `test` — 1

- **TP18** — Assertion #18 (A): GA67: A pathname of no more than PATH_MAX bytes can be resolved

### `leftbrack` — 1

- **TP18** — Assertion #18 (A): GA67: A pathname of no more than PATH_MAX bytes can be resolved

### `uname` — 1

- **TP19** — *(no assertion text recorded)*

## X — NOT OURS — triage before assigning  (14 assertions)

**We do not ship `patch`.** The same distro binary ran in BOTH arms, so a bashy-only failure here cannot be a utility defect — only the driving shell differed. Either a shell bug surfacing through the tset, or run nondeterminism. Do NOT assign this as a coreutils fix until triaged.

### `patch` — 14

- **TP1** — Assertion #1 (C):
- **TP2** — Assertion #2 (C):
- **TP3** — *(no assertion text recorded)*
- **TP4** — Assertion #4 (C):
- **TP5** — *(no assertion text recorded)*
- **TP9** — Assertion #9 (C):
- **TP10** — Assertion #10 (C):
- **TP12** — Assertion #12 (C):
- **TP30** — *(no assertion text recorded)*
- **TP31** — Assertion #31 (C):
- **TP34** — Assertion #34 (C):
- **TP42** — *(no assertion text recorded)*
- **TP44** — Assertion #44 (C):
- **TP48** — Assertion #48 (C):


---

Total accounted: 291 assertions.

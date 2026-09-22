# Sprint 253 Windows locale service result

Story: #711 (`f3f0e3ef7aae`)

## Result

The Windows fixture root now has the fail-closed host-provider handoff, but the
complete corpus locale set is **not serviceable** under the no-bundled-data
contract. Story #711 is not complete.

On Windows, `tools/bash53suite` resolves the host's native `locale` executable
before replacing the fixture `PATH` with the private yoke userland. It exports
that absolute path as `BASHY_HOST_LOCALE` and supplies all seven corpus names as
`BASHY_HOST_LOCALE_NAMES`. An explicitly provisioned package path wins, so a
caller can install a stronger POSIX provider in one step without a harness
change. No locale archive or table is copied into Bashy or the fixture root.

The coordinated coreutils#13 candidate (`6fc5b3ef`) consumes that contract. It
accepts a candidate only when the host service:

1. selects the requested locale without silently falling back;
2. answers `locale -k` for `LC_CTYPE`, `LC_NUMERIC`, `LC_TIME`, `LC_COLLATE`,
   `LC_MONETARY`, and `LC_MESSAGES`; and
3. returns a non-empty charmap.

Bashy review found that the candidate does not yet compare that charmap with
the requested codeset. The correction was returned to coreutils#13 and the
sprint manager before integration: the comparison must be case- and
punctuation-insensitive, allow only explicit aliases, and reject Big5 or ASCII
for a Big5-HKSCS request. The candidate is therefore not integration-ready and
this report does not treat its commit as completion evidence.

Thus the bridge makes the six names proved by probe run 35776069474 available
even though Git Bash omits their non-UTF-8 spellings from `locale -a`, while
rejecting its false-success fallback for `zh_HK.big5hkscs`.

## Exact blocker

No evaluated native Windows provider supplies the seventh locale:

- Git for Windows' MSYS service accepts the other six requests, but
  `LC_ALL=zh_HK.big5hkscs locale charmap` falls back to
  `ANSI_X3.4-1968` (run 35776069474).
- Current Cygwin is a real POSIX locale package and provides the required
  `locale -k LC_MESSAGES` values, but its documented supported charset list
  contains Big5 only, not Big5-HKSCS. Cygwin maps those locale charsets to
  Windows code pages; Windows defines code page 950 for Big5 and no
  Big5-HKSCS code-page identifier. Installing the package in a private root
  therefore cannot make `zh_HK.big5hkscs` select that charmap.
- Windows ICU has a Big5-HKSCS converter, but its compiled locale resource
  bundles omit CLDR's POSIX messages section and do not supply the required
  `yesexpr`, `noexpr`, `yesstr`, and `nostr` values. ICU therefore cannot pass
  the all-six-categories invariant by itself.

Aliasing Big5-HKSCS to Big5, accepting the ASCII fallback, fabricating
`LC_MESSAGES`, or downloading raw glibc/CLDR source would each claim data the
host does not service and is forbidden by the story contract. Completion needs
a Windows-installable POSIX locale package/service that exposes both genuine
Big5-HKSCS conversion and all POSIX categories, or an explicit contract change
authorizing locale data. Neither exists in the approved scope today.

## Focused verification

- `go test ./tools/bash53suite -run 'TestConfigureHostLocaleProvider|TestPrepareUserland'`
- `GOOS=windows GOARCH=amd64 go test -c ./tools/bash53suite`
- coreutils#13: focused fake-provider tests exercise selection fallback, every
  category, non-empty charmap, and `TestAllLocales`; exact charmap matching is
  the review correction still required before integration.

Primary package references:

- <https://cygwin.com/cygwin-ug-net/setup-locale.html>
- <https://cygwin.com/cygwin-ug-net/locale.html>
- <https://learn.microsoft.com/windows/win32/intl/code-page-identifiers>
- <https://learn.microsoft.com/windows/win32/intl/international-components-for-unicode--icu->

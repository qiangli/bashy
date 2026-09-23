# Windows WSL locale provider

Sprint: #253

Story: #711 (`f3f0e3ef7aae`)

## Plan and production contract

The Windows conformance job provisions the same seven locales as the Linux job,
but stores the real locale data in Ubuntu/glibc under WSL. One workflow step
runs `scripts/provision-windows-wsl-locales.sh`; that script:

1. installs the official `Ubuntu` WSL distribution when it is not registered;
2. installs `locales`, adds the six `/etc/locale.gen` entries used on Linux,
   runs `locale-gen`, and creates `ja_JP.SJIS` with the same
   `localedef --no-warnings=ascii` exception as Linux;
3. builds a native Windows PE provider which forwards `LC_ALL`, `-a`,
   `charmap`, and `-k` to Ubuntu's `locale` command; and
4. fails before fixture measurement unless every locale returns its expected
   charmap and all six POSIX categories. The fixture runner also checks that
   the built yoke `locale -a` advertises all seven names through coreutils'
   host-service validator.

The fixture runner receives the provider through `BASHY_HOST_LOCALE`, the
Ubuntu selection through `BASHY_WSL_DISTRO`, the native absolute `wsl.exe`
path through `BASHY_WSL_EXE`, and the complete corpus set through
`BASHY_HOST_LOCALE_NAMES`. The absolute path keeps the bridge working when the
fixture root replaces `PATH` with its private userland. Coreutils remains the
authority that filters advertised names using locale selection, every
category, and the requested charmap; `TestAllLocales` remains unchanged.

The install is idempotent: it reuses an existing Ubuntu registration, does not
duplicate `/etc/locale.gen` entries, and safely regenerates the locale archive
and PE wrapper.

## Non-CI prerequisite

An arbitrary Windows machine must have WSL 2 enabled and `wsl.exe` available;
enabling the Windows feature or completing a reboot is an administrator/host
prerequisite and is not disguised as a locale fallback. Run the provisioner
from Git Bash after that prerequisite is satisfied:

```bash
./scripts/provision-windows-wsl-locales.sh
./scripts/ci-bash53-windows.sh
```

The first command writes `bin/windows-wsl-locales.env`; the fixture script
loads it automatically. A provisioning or validation failure exits nonzero with
an actionable `windows-wsl-locales: ERROR:` diagnostic. No synthetic locale
table is used when WSL or Ubuntu is unavailable.

## Focused verification

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/windowswslocale.exe ./tools/windowswslocale
go test ./tools/windowswslocale
go test ./tools/bash53suite -run '^TestConfigureHostLocaleProvider$'
(cd ../coreutils && go test ./cmds/locale -run '^Test(AllLocales|HostLocaleProvider)')
```

Do not run the full Bash 5.3 fixture suite for this implementation review; the
sprint manager owns the subsequent Windows conformance measurement.

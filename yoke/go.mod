module github.com/qiangli/bashy/yoke

go 1.27.0

toolchain go1.27.4

require (
	github.com/qiangli/yoke v0.0.0-20261121110346-da4b02a4d244
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.4.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.2 // indirect
	github.com/go-git/go-git/v5 v5.19.3 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.5.0 // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/nguyenthenguyen/docx v0.0.0-20230621112118-9c8e795a11db // indirect
	github.com/odvcencio/gotreesitter v0.16.0 // indirect
	github.com/pjbgf/sha1cd v0.7.0 // indirect
	github.com/qiangli/coreutils v0.0.0-20261121101804-c5832f99ccf6 // indirect
	github.com/qiangli/gfy v0.0.0-20261013004914-30d38f086034 // indirect
	github.com/richardlehane/mscfb v1.0.6 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/sabhiram/go-gitignore v0.0.0-20210923224102-525f6e181f06 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/skeema/knownhosts v1.3.2 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/tklauser/ps v0.0.5-0.20260804061010-39c4acb07b31 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/excelize/v2 v2.10.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/benhoyt/goawk => github.com/qiangli/coreutils/third_party/goawk v0.0.0-20261121101804-c5832f99ccf6

replace github.com/jaegertracing/jaeger => github.com/qiangli/jaeger v0.0.0-20260426223533-5aaa7eb1f040

replace github.com/perses/perses => github.com/qiangli/perses v0.0.0-20260426190059-de437951b5e6

replace github.com/ergochat/readline => github.com/qiangli/readline v0.1.4-0.20261121101804-6ed12265d8dd

replace github.com/ollama/ollama => github.com/qiangli/ollama v0.0.0-20261016181410-b0d0bbba93ab

replace go.podman.io/podman/v6 => github.com/qiangli/podman/v6 v6.0.0-20261013004941-fa029d9c82fb

replace github.com/gtsteffaniak/filebrowser/backend => github.com/qiangli/filebrowser-quantum/backend v0.0.0-20261115042727-40f3c84d55a9

replace github.com/odvcencio/gotreesitter => github.com/qiangli/gotreesitter v0.0.0-20261121101804-5522e72a297d

replace modernc.org/sqlite => github.com/qiangli/sqlite v1.50.2-0.20261018085128-5fb52bf328cb

replace github.com/ugorji/go v1.1.4 => github.com/ugorji/go v1.2.14

replace github.com/go-git/go-billy/v5 => github.com/qiangli/yoke/external/go-billy v0.0.0-20261030181600-621d3b7f135f

replace github.com/hashicorp/go-multierror => github.com/qiangli/yoke/external/multierror v0.0.0-20261021020216-636e2fd289e9

replace github.com/hashicorp/errwrap => github.com/qiangli/yoke/external/errwrap v0.0.0-20261021020216-636e2fd289e9

replace github.com/hashicorp/yamux => github.com/qiangli/yamux v0.1.0

replace github.com/libp2p/go-yamux/v5 => github.com/qiangli/yamux/v5 v5.0.0

replace github.com/cyphar/filepath-securejoin => github.com/qiangli/yoke/external/securejoin v0.0.0-20261021020216-636e2fd289e9

replace github.com/mattn/go-runewidth => github.com/qiangli/yoke/external/runewidth v0.0.0-20261025041311-99ef13338402

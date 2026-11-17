# founder-bootcamp

A small Bash# app that walks a solo founder through the three things that
are easy to get wrong: forming the company, filing provisional patents, and
applying for funding. It keeps progress and deadlines, and it drives the
real sites through the founder's own browser with bashy — the founder logs
in, an agent fills, the founder uploads, signs, pays and submits.

Not legal or tax advice. General US process as of 2026; verify at the source.

    ./register.sh                                # registers `founder-bootcamp` (command + skill)
    export BOOTCAMP_DIR=~/private/founder        # keep records somewhere private
    export BOOTCAMP_COMPANY="Acme Labs, Inc."    # defaults are fictitious
    export BOOTCAMP_FOUNDER="Alex Founder"

    founder-bootcamp status                      # tracks and deadlines due in 30 days
    founder-bootcamp next patent                 # next open steps, who does each, the real site
    founder-bootcamp done company incorporate --note "certificate on file"
    founder-bootcamp deadline add 2026-12-01 "83(b) reaches the IRS" --track company
    founder-bootcamp takeover                    # check the live browser hub, print who does what
    founder-bootcamp go patent file              # open USPTO Patent Center in the founder's browser
    founder-bootcamp check-site https://example.com   # names in the SERVED HTML (bashy fetch)
    founder-bootcamp preflight spec.pdf          # text extracts, abstract length, SHA-256 (bashy lens)
    founder-bootcamp deck init pitch && founder-bootcamp deck build pitch      # deck.pdf
    founder-bootcamp demo init demo --deck pitch && founder-bootcamp demo build demo --voice Samantha
    founder-bootcamp page report.html            # one-page progress report
    founder-bootcamp guide patent                # the general runbook from bashy kb

Tracks (`tracks.txt`, one step per line: track|step|who|title|guide|url):
company, patent, funding, launch. `who` is founder, agent or both. `url` is
the real site (Stripe Atlas, USPTO Patent Center and Assignment Center, IRS,
Google Patents, program applications). `{company}` and `{founder}` are
filled from the environment.

Every tool is a bashy command: `bashy browser` (live takeover, slide
rendering), `bashy todo` (deadlines are mirrored with label
`founder-bootcamp` when `BOOTCAMP_DIR` is inside a git repository),
`bashy fetch`, `bashy lens`, `bashy kb`, and the sibling `media` example
(PDF and narrated video).

Checks: `founder-bootcamp test` (or `bash checks/bootcamp_test.sh`).

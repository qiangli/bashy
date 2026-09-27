# ephemeral-host — a registered bashy command

An example of a **registered command**: your own program, listed and dispatched
by bashy like a shipped one, without being a bashy builtin. It rents
short-lived cloud hosts (DigitalOcean today) for agents and makes sure they are
given back.

- `create NAME --ttl 3h --cap 1` — refuses without a deadline (`--ttl`) and a
  budget (`--cap`, USD), when price × ttl exceeds the cap, past the policy
  maximums, over a rolling 24h spend cap, for GPU sizes unless allowed, and when
  the token can see a host named on the policy's production tripwire list.
  The host is appended to a ledger and tagged with its deadline at the provider.
- `list` — the ledger joined with what the provider can see; hosts the ledger
  does not know are shown as `untracked`.
- `policy` — the limits in force.

## The boundary is the account, not this program

Provider API tokens are scoped per resource TYPE: a token that can delete one
droplet can delete every droplet in its team. Give this program a token from a
team (or account) that holds nothing but ephemeral hosts. The ledger stops
cooperating agents from destroying each other's boxes or overspending; the
account boundary stops everything else.

## Set up

```sh
# build (a standalone module; nothing here links into bashy)
go build -o ~/.bashy/bin/ephemeral-host .

# the token, entered out of band and kept in the vault
bashy ask --name DO_EPHEMERAL_TOKEN --stdout | bashy secret set DO_EPHEMERAL_TOKEN

# register it
bashy commands add ephemeral-host \
  --set exec.0="$HOME/.bashy/bin/ephemeral-host" \
  --set synopsis="rent a short-lived cloud host with a mandatory deadline and budget; ledger-scoped" \
  --set effects.0=write --set effects.1=net --set effects.2=cred --set effects.3=spend \
  --set caps.0=json --set caps.1=dry-run \
  --set tier=cloud --set group=cluster-cloud

bashy ephemeral-host policy
```

Optional policy: `$BASHY_HOME/ephemeral-host/policy.json` (or
`~/.bashy/ephemeral-host/policy.json`) overrides the defaults, for example

```json
{ "max_ttl": "12h", "max_cap_usd": 10, "daily_cap_usd": 25,
  "production_tripwire": ["the-name-of-a-production-droplet"] }
```

# Run genie in a contained directory

`bashy genie --contained [-m BINDING|BAND] [--dir DIR] "TASK"` runs one task with the host's subscription model door. The directory defaults to the current directory. Choose a binding by name or a band (`L3`, `L4`, or `L5`). The default is `L3`. For a band, genie uses the first binding returned by the session door.

For example, `bashy genie --contained -m L4 --dir ./checkout "Explain the failing check"` prints the answer on stdout. Start the host door with `bashy llm up` if session creation reports it unavailable. A host with an enforcing file sandbox is required. A backend refusal exits with status 125.

The sandbox can write to the selected directory and its private temporary home. Genie reads its bundle and reaches the model through a loopback forwarder to the scoped session socket. Subscription credentials stay on the host. Bashy builds the builtin genie bundle on first use. Each session expires after 30 minutes or 200 requests and is revoked when the run ends, including on interrupt.

A private JSON record under `${BASHY_HOME:-~/.bashy}/genie/runs/<label>.json` contains the requested selector, selected binding, band (when known), allowed models, expiry, request cap, sandbox, directory and process exit code.

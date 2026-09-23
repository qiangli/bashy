package cli

import "os"

// agentos imports cli, so cli's package variables initialize before agentos
// installs its shell capability manifest. Preserve the cold caller's original
// environment, including order and explicitly supplied shell/agent variables.
// Shell startup still uses its normal enriched environment; only GoSource
// dependency processes receive this separate immutable starting snapshot.
var goSourceProcessEnvironment = os.Environ()

// AdoptGoSourceProcessEnvironment records the restored environment of an
// owned child frame. Ordinary starts deliberately retain the package's cold
// caller snapshot: AgentOS initialization may install its own variables
// before Main runs.
func AdoptGoSourceProcessEnvironment() {
	goSourceProcessEnvironment = os.Environ()
}

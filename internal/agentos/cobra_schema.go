package agentos

import (
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/external/act"
	"github.com/qiangli/yoke/external/actrunner"
	"github.com/qiangli/yoke/external/clang"
	"github.com/qiangli/yoke/external/cmake"
	"github.com/qiangli/yoke/external/curlbin"
	"github.com/qiangli/yoke/external/gh"
	"github.com/qiangli/yoke/external/gitscm"
	"github.com/qiangli/yoke/external/gotoolchain"
	"github.com/qiangli/yoke/external/govulncheck"
	"github.com/qiangli/yoke/external/grype"
	"github.com/qiangli/yoke/external/helm"
	"github.com/qiangli/yoke/external/kopia"
	"github.com/qiangli/yoke/external/kubectl"
	"github.com/qiangli/yoke/external/loom"
	"github.com/qiangli/yoke/external/mise"
	"github.com/qiangli/yoke/external/node"
	"github.com/qiangli/yoke/external/osvscanner"
	"github.com/qiangli/yoke/external/pwsh"
	"github.com/qiangli/yoke/external/python"
	"github.com/qiangli/yoke/external/rclone"
	"github.com/qiangli/yoke/external/registry"
	"github.com/qiangli/yoke/external/rust"
	"github.com/qiangli/yoke/external/seaweedfs"
	"github.com/qiangli/yoke/external/sphere"
	"github.com/qiangli/yoke/external/tessaro"
	"github.com/qiangli/yoke/external/zigcc"
	"github.com/qiangli/yoke/external/zot"
	"github.com/qiangli/yoke/pkg/adapt"
	"github.com/qiangli/yoke/pkg/ask"
	"github.com/qiangli/yoke/pkg/board"
	"github.com/qiangli/yoke/pkg/broker"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/capability"
	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/craft"
	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/gate"
	"github.com/qiangli/yoke/pkg/handoff"
	"github.com/qiangli/yoke/pkg/jobs"
	"github.com/qiangli/yoke/pkg/judge"
	"github.com/qiangli/yoke/pkg/kb"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/mirror"
	"github.com/qiangli/yoke/pkg/pair"
	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/resources"
	"github.com/qiangli/yoke/pkg/sdlc"
	"github.com/qiangli/yoke/pkg/search"
	"github.com/qiangli/yoke/pkg/secrets"
	coreskills "github.com/qiangli/yoke/pkg/skills"
	"github.com/qiangli/yoke/pkg/sota"
	"github.com/qiangli/yoke/pkg/stats"
	"github.com/qiangli/yoke/pkg/steward"
	"github.com/qiangli/yoke/pkg/supervise"
	"github.com/qiangli/yoke/pkg/todo"
	"github.com/qiangli/yoke/pkg/toolcmd"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/qiangli/yoke/pkg/webconsole"
	"github.com/qiangli/yoke/pkg/webinspect"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cobraVerb constructs the same command definitions used by dispatch, without
// executing them. Hand-parsed front doors deliberately have no invented schema.
// Build-tagged engine front doors add their constructors here at init time.
var cobraOptionalVerbs = map[string]func() *cobra.Command{}

func cobraVerb(name string) *cobra.Command {
	if build := cobraOptionalVerbs[name]; build != nil {
		return build()
	}
	if cmd, _, ok := newBusFrontDoorCmd(name); ok {
		return cmd
	}
	switch name {
	case "proxy":
		return newProxyCmd()
	case "llm":
		return broker.NewCmd()
	case "adapt":
		return adapt.NewCmd(adapt.PrepareJSONLens)
	case "adapter":
		return fleet.NewAdaptersCmd()
	case "resource", "resources":
		return resources.NewCommand()
	case "lexicon":
		return newLexiconCmd()
	case "define":
		return newDefineCmd()
	case "todo", "issue":
		return todo.NewTodoCmd()
	case "pair":
		return pair.NewPairCmd()
	case "judge":
		return judge.NewJudgeCmd()
	case "gate":
		return gate.NewGateCmd()
	case "chat":
		return chat.NewChatCmd()
	case "coach":
		return chat.NewCoachCmd()
	case "delegate":
		return chat.NewDelegateCmd()
	case "meet":
		return meet.NewMeetCmd()
	case "app", "apps":
		return webconsole.NewAppsCmd()
	case "supervise":
		return supervise.NewSuperviseCmd()
	case "capability":
		return capability.NewCapabilityCmd()
	case "ping":
		return bus.NewPingCmd()
	case "stats":
		return stats.NewCmd()
	case "leaderboard":
		return capability.NewLeaderboardCmd()
	case "sdlc":
		return sdlc.NewSDLCCmd()
	case "web":
		return webinspect.NewWebCmd()
	case "search":
		return search.NewSearchCmd()
	case "sota":
		return sota.NewSotaCmd()
	case "secret", "secrets":
		return secrets.NewSecretsCmd()
	case "bus":
		return bus.NewBusCmd()
	case "ask":
		return ask.NewAskCmd()
	case "approve":
		return ask.NewApproveCmd()
	case "craft":
		return craft.NewCraftCmd(craftOptions()...)
	case "kb":
		return schemaKBCmd()
	case "schedule":
		return schedule.NewScheduleCmd()
	case "conform", "verify":
		return verifyCmd()
	case "self":
		return selfCmd()
	case "bootstrap", "upgrade":
		return selfCmd()
	case "git-scm":
		return gitscm.NewGitSCMCmd()
	case "remote":
		return remoteCmd()
	case "go":
		return gotoolchain.NewGoCmd()
	case "cmake":
		return cmake.NewCmakeCmd()
	case "clang":
		return clang.NewClangCmd()
	case "zig":
		return zigcc.NewZigCmd()
	case "node":
		return node.NewNodeCmd()
	case "npm":
		return node.NewNpmCmd()
	case "npx":
		return node.NewNpxCmd()
	case "pnpm":
		return node.NewPnpmCmd()
	case "yarn":
		return node.NewYarnCmd()
	case "python":
		return python.NewPythonCmd()
	case "pip":
		return python.NewPipCmd()
	case "uv":
		return python.NewUvCmd()
	case "mise":
		return mise.NewMiseCmd()
	case "cargo":
		return rust.NewCargoCmd()
	case "rustc":
		return rust.NewRustcCmd()
	case "rustup":
		return rust.NewRustupCmd()
	case "rust":
		return rust.NewRustCmd()
	case "pwsh":
		return pwsh.NewCmd()
	case "curl":
		return curlbin.NewCurlCmd()
	case "loom":
		return loom.NewLoomCmd()
	case "zot":
		return zot.NewZotCmd()
	case "seaweedfs":
		return seaweedfs.NewSeaweedfsCmd()
	case "kopia":
		return kopia.NewKopiaCmd()
	case "act":
		return act.NewActCmd()
	case "act-runner":
		return actrunner.NewCmd()
	case "gh":
		return gh.NewGhCmd()
	case "rclone":
		return rclone.NewRcloneCmd()
	case "kubectl":
		return kubectl.NewKubectlCmd()
	case "helm":
		return helm.NewHelmCmd()
	case "govulncheck":
		return govulncheck.NewGovulncheckCmd()
	case "osv-scanner", "osvscanner":
		return osvscanner.NewOsvScannerCmd()
	case "grype":
		return grype.NewGrypeCmd()
	case "peer", "sphere":
		return sphere.NewSphereCmd()
	case "tessaro":
		return tessaro.NewTessaroCmd()
	case "login":
		return tessaro.NewLoginCmd()
	case "mirror":
		return mirror.NewMirrorCmd()
	case "jobs":
		return jobs.JobsCommand()
	case "fg":
		return jobs.FgCommand()
	case "bg":
		return jobs.BgCommand()
	case "kill":
		return jobs.KillCommand()
	case "dag":
		return dag.NewDagCmd()
	case "weave":
		return weave.NewWeaveCmd()
	case "sprint":
		return newSprintCmd()
	case "handoff":
		return handoff.NewHandoffCmd()
	case "resume":
		return handoff.NewResumeCmd()
	case "service":
		return newServiceCmd()
	case "release":
		return releaseCmd()
	case "skill", "skills":
		return coreskills.NewSkillsCmd(skillsOptions()...)
	case "model", "models":
		return newModelsResourcesCmd()
	case "agent", "agents":
		return newAgentsRosterCmd()
	case "person", "people":
		return principal.NewPeopleCmd()
	case "whois":
		return principal.NewWhoisCmd()
	case "tool", "tools":
		return schemaToolCmd()
	case "steward":
		return schemaStewardCmd()
	case "conductor":
		return board.NewConductorCommand(nil, nil)
	case "claim":
		return coord.NewClaimCmd(nil)
	}
	if entry, ok := registry.Lookup(name); ok {
		return entry.NewCmd()
	}
	return nil
}

// schemaKBCmd is the one kb tree: the runtime dispatch and the schema
// projection both build it here, so `kb memory` cannot be mounted in one and
// missing from the other.
func schemaKBCmd() *cobra.Command {
	cmd := kb.NewKBCmd()
	cmd.AddCommand(newKBRecallCmd(), newKBContextCmd(), newKBAskCmd(), kbmemory.NewMemoryCmd())
	return cmd
}
func schemaToolCmd() *cobra.Command {
	cmd := fleet.NewToolsCmd()
	cmd.AddCommand(toolcmd.NewCmd())
	return cmd
}
func schemaStewardCmd() *cobra.Command {
	cmd := board.NewStewardCommand(nil, nil)
	for _, sub := range steward.NewStewardCmd().Commands() {
		cmd.AddCommand(sub)
	}
	cmd.AddCommand(newStewardStartCmd(), newStewardStopCmd())
	return cmd
}

// walkCobraSchemas visits every verb, including hidden verbs, without executing
// a command or parsing argv. Local flags shadow inherited persistent flags.
// Paths use canonical names; aliases do not create duplicate schema entries.
func walkCobraSchemas(cmd *cobra.Command, path string, visit func(string, tool.ArgSchema)) {
	if cmd == nil {
		return
	}
	flags := pflag.NewFlagSet(path, pflag.ContinueOnError)
	flags.AddFlagSet(cmd.LocalNonPersistentFlags())
	flags.AddFlagSet(cmd.PersistentFlags())
	flags.AddFlagSet(cmd.InheritedFlags())
	visit(path, tool.SchemaFromFlags(flags))
	for _, child := range cmd.Commands() {
		walkCobraSchemas(child, path+" "+child.Name(), visit)
	}
}

func cobraSchemas(name string) map[string]tool.ArgSchema {
	cmd := cobraVerb(name)
	if cmd == nil {
		return nil
	}
	schemas := make(map[string]tool.ArgSchema)
	walkCobraSchemas(cmd, name, func(path string, schema tool.ArgSchema) { schemas[path] = schema })
	return schemas
}

# Sprint 692 / Story 3302: certified-shell startup budget

Resolve the nested `github.com/bashsharp/bashsharp/cmd/bsh` module at the
root go.mod bashsharp version, then build in that module with GOWORK=off.
A parent-module zip omits nested modules; downloading the parent is insufficient.
Keep scripts/test-startup-linux.sh unchanged: total init <=15 ms, max <=5 ms,
and the existing 100-job limit. The removed profile tags are already absent
from executable build paths; historical evidence retains its original tags.

Validate the exact build command on the dev host. Linux timing in the official
image remains assigned to rc.40 candidate gate 458628c0, per the operator rule.

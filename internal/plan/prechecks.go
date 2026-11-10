package plan

import (
	"context"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Prechecks runs the checks of steps (P1..P8), in the order of the screen (c):
// P6 before P7, P8 and P3, which become with the shell P6 found. It returns the
// results by step id and that shell (Options.ShellExecutable).
func Prechecks(ctx context.Context, s *state.AppState, o Options, run RunFunc, steps []Step) (map[string]CheckResult, string) {
	has := map[string]bool{}
	for _, st := range steps {
		if st.Kind == Check {
			has[st.ID] = true
		}
	}
	r := map[string]CheckResult{"P1": CheckInstalled(o)}
	if has["P4"] {
		r["P4"] = CheckAipubBin(o, s)
	}
	if has["P5"] {
		r["P5"] = CheckCatalog(o)
	}
	r["P2"] = CheckSSH(ctx, o, run)
	var shell string
	r["P6"], shell = CheckBecomeShell(ctx, o, run)
	o.ShellExecutable = shell
	if has["P7"] {
		r["P7"] = CheckDNS(ctx, o, s, run)
	}
	if has["P8"] {
		r["P8"] = CheckStorageClass(ctx, o, s, run)
	}
	if has["P3"] {
		r["P3"] = CheckLegacy(ctx, o, s, run)
	}
	return r, shell
}

// CheckVerdict says whether the checks of steps allow a run: the ones not run
// yet, and the ones that failed (✗). A warning allows it.
func CheckVerdict(steps []Step, results map[string]CheckResult) (notRun, failed []string) {
	for _, st := range steps {
		if st.Kind != Check || st.Skip {
			continue
		}
		r, ok := results[st.ID]
		switch {
		case !ok:
			notRun = append(notRun, st.ID)
		case !r.OK:
			failed = append(failed, st.ID)
		}
	}
	return notRun, failed
}

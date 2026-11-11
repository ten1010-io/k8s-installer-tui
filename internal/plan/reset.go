package plan

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// The words of the node probe of PrepareReset.
const (
	resetNodeBuilt = "ki-built"
	resetNodeClean = "ki-none"
)

// failedStep is the step a run stopped on, "" when none failed.
func failedStep(r *RunState) string {
	for _, st := range r.Steps {
		if st.Status == StatusFailed {
			return st.ID
		}
	}
	return ""
}

// PrepareReset stands between the typed word and reset-cluster.yml, which
// destroys the cluster on every node of the inventory. It refuses unless all of
// this holds at this moment, whatever the screen showed when x was pressed:
//
//   - no other program runs this installation: it takes the lock of the runs
//     (lockFile) and the caller holds it, through release, until the reset has
//     ended. Without it a run started from another terminal could be building
//     the cluster while the reset takes it apart.
//   - state.json still records the run the screen showed (shown), failed in
//     K1..K3 of a new site (ResetHelps): another run may have gone on since.
//   - the site is plainly new on this control node: its record was looked at
//     and is not there (Options.Detect, required).
//   - no node of the inventory holds the record either: a cluster built from
//     another control node leaves it on its nodes and not here, and K3 fails on
//     such a node ("Kubelet already enabled") exactly because that cluster is
//     running. A node that does not answer is not known to be clean.
//
// What it can not see is a cluster that k8s-installer did not build (kubeadm by
// hand): there is no record of it anywhere. The diagnosis of that failure says
// so, and the word has to be typed.
func PrepareReset(ctx context.Context, o Options, shown *RunState, run RunFunc) (release func(), err error) {
	dir := StateDir(o)
	lock, err := lockFile(filepath.Join(dir, "lock"))
	if err != nil {
		return nil, err // LockedError says who holds it
	}
	fail := func(msg string) (func(), error) {
		lock.Close()
		return nil, errors.New(msg)
	}
	now, err := LoadRunState(dir)
	if err != nil {
		return fail(err.Error())
	}
	if now == nil || shown == nil || !now.Started.Equal(shown.Started) || now.Outcome != OutcomeFailed ||
		failedStep(now) != failedStep(shown) || !ResetHelps(now.Existing, failedStep(now)) {
		return fail(i18n.T("The record of the run is not what the screen showed (another run happened since). Go back to the plan screen and look again"))
	}
	if o.Detect == nil {
		return fail(i18n.T("Cannot tell whether this control node has a cluster record"))
	}
	if site := o.Detect(); site.Existing || site.Unknown {
		return fail(i18n.T("Not a new site — %s", site.Why))
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// raw: a node that an earlier install left half built may have no python.
	// As root (sudo -n, as k8s-installer itself reads the record): on a hardened
	// node /etc/k8s-installer is not open to the login user, and a plain test -e
	// that may not look says "not there". A node where sudo is refused prints
	// neither word and is not known to be clean. The clean word needs proof
	// that the record is absent (test ! -e succeeding): a test that sudoers
	// refuses, or that fails for any other reason, reads as built, and the
	// reset is refused. The nodes of broken_node are left out, as
	// reset-cluster.yml leaves them out.
	probe := "sudo -n true 2>/dev/null && { sudo -n test ! -e " + shellQuote(ClusterRecord) +
		" && echo " + resetNodeClean + " || echo " + resetNodeBuilt + "; }"
	res, out, err := adhoc(ctx, o, run, "all:!broken_node", false, "raw", probe)
	if len(res) == 0 || err != nil {
		// ansible ends non-zero when any node failed or could not be reached:
		// then some node is not in what it printed, or is not clean
		return fail(i18n.T("Could not check the nodes: %s", couldNotRun(out, err)))
	}
	var built, unknown []string
	for node, a := range res {
		switch said := lastWord(a.Stdout); {
		case said == resetNodeBuilt:
			built = append(built, node)
		case said != resetNodeClean:
			unknown = append(unknown, node)
		}
	}
	sort.Strings(built)
	sort.Strings(unknown)
	switch {
	case len(built) > 0:
		return fail(i18n.T("Nodes of a cluster k8s-installer built (%s is there): %s. "+
			"It looks like a cluster built from another control node. Not resetting", ClusterRecord, strings.Join(built, " ")))
	case len(unknown) > 0:
		return fail(i18n.T("Nodes not checked: %s. The reset needs every node shown to have no cluster record "+
			"(fix the connection, or put dead nodes in broken_node of the inventory)", strings.Join(unknown, " ")))
	}
	return func() { lock.Close() }, nil
}

// lastWord is the last line of what a probe printed, trimmed: raw goes through
// a terminal, and what the login prints comes before the probe's own word.
func lastWord(stdout string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(stdout, "\r", "\n")), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

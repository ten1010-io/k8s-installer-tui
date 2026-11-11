package plan

import (
	"errors"
	"io/fs"
	"os"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// ClusterRecord is the file that says a cluster was built from this control
// node: k8s-installer writes ki_etc_applied_release_path (ki_etc_root_path is
// the constant /etc/k8s-installer, group_vars/all/constant-vars.yml) on every
// node of the inventory, the control node's own entry included, as the last
// task of setup-cluster, update-cluster, add-node and upgrade-cluster
// (playbooks/tasks/record-applied-state.yml), so only once one of them has
// succeeded; reset-cluster and remove-node take it away again
// (tasks/reset-cluster-record.yml). Its neighbour applied-vars.yml is written
// at the same time but outlives reset-cluster on purpose, so it would call a
// reset site existing. The files of the wizard say nothing: setup.sh lays a
// template vars.yml, and a site saved and opened again before it was installed
// has all of them. A variable so that tests can point it elsewhere.
var ClusterRecord = "/etc/k8s-installer/applied-release"

// Site is what the plan takes the site to be, and why: whether the cluster is
// there (Options.Existing) and AIPub with it (Options.AipubInstalled).
type Site struct {
	Existing       bool
	AipubInstalled bool
	// Unknown: the record could not be looked at, so "not existing" is a plan,
	// not a finding. What destroys a cluster (the reset of the install screen)
	// is not offered on it
	Unknown bool
	// Why is the one line the plan screen and --install show: the decision,
	// what it rests on and how to say otherwise
	Why string
}

// DetectSite decides it from the flags (--existing, --aipub-installed) and the
// record of this control node. Existing: the flag, or the record is there. A
// record that cannot be looked at (permission) is said, not guessed: the site
// is planned new and the line says to give --existing. AIPub is taken from the
// flag only: aipub-installer keeps nothing that is written only once
// setup-aipub has succeeded (aipub_state_path holds values rendered before the
// install and the logs of it), so there is nothing to read it from.
func DetectSite(existing, aipubInstalled bool) Site {
	s := Site{Existing: existing, AipubInstalled: aipubInstalled}
	switch _, err := os.Stat(ClusterRecord); {
	case existing:
		s.Why = i18n.T("Existing site: given with --existing")
	case err == nil:
		s.Existing = true
		s.Why = i18n.T("Existing site: %s is there (the record k8s-installer leaves once it built the cluster, which reset-cluster.yml removes)", ClusterRecord)
	case errors.Is(err, fs.ErrNotExist):
		s.Why = i18n.T("New site: no %s (start with --existing when there is a cluster already)", ClusterRecord)
	default:
		s.Unknown = true
		s.Why = i18n.T("Planned as a new site: %s could not be looked at (%v). Start with --existing when there is a cluster already", ClusterRecord, err)
	}
	if s.Existing {
		if aipubInstalled {
			s.Why += ". " + i18n.T("AIPub: given with --aipub-installed")
		} else {
			s.Why += ". " + i18n.T("AIPub planned as a new install (--aipub-installed when it is installed already: there is no record of it, so only this flag decides)")
		}
	}
	return s
}

// ResumedSite is the line of a run that goes on with the plan of the last one
// (state.json): what that run took the site to be, whatever is decided now.
func ResumedSite(prev *RunState, now string) string {
	kind := i18n.T("New site")
	if prev.Existing {
		kind = i18n.T("Existing site")
		if prev.AipubInstalled {
			kind += " " + i18n.T("(AIPub installed)")
		}
	}
	return i18n.T("%s: as planned by the last run (started %s). To change it, run from the first step (decided now: %s)",
		kind, prev.Started.Format("2006-01-02 15:04"), now)
}

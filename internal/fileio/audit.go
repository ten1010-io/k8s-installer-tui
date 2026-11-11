package fileio

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The audit policy of the audit preset, files/aipub/audit-policy.yaml, which
// k8s_cp_extra_files copies over /etc/kubernetes/audit-policy.yaml of every
// control plane node. The wizard writes its default only where nothing is: a
// file there (the site's, one taken from a node, one it wrote before) is kept as
// it is, and on an existing site the default waits until the policy of its nodes
// was read or the operator said so.

// AuditPolicyMarker starts the first line of the default the wizard writes, so
// that a later version can tell its own file from a site's.
const AuditPolicyMarker = "# k8s-installer-tui: default AIPub audit policy"

// auditPolicyVersion is the version of templates/audit-policy.yaml, named in the
// marker line. Bumped when the template changes; a file written with an older
// one is kept (not upgraded). v2: the comments in English (the rules as in v1),
// as the file goes to nodes that may have no Korean font.
const auditPolicyVersion = "v2"

// AuditPolicy is the default audit policy for kube-apiserver
// (--audit-policy-file), nil when the audit preset is off: the template taken
// from aipub-installer-archive 0bb076d8 (AIP-2611, templates/audit-policy.yaml)
// under a marker line.
func AuditPolicy(s *state.AppState) []byte {
	if !s.AppliedControlPlane().Audit {
		return nil
	}
	header := AuditPolicyMarker + ", template " + auditPolicyVersion + " (aipub-installer-archive 0bb076d8, AIP-2611)\n" +
		"# Written only where no policy was; never written over. k8s_cp_extra_files copies it to every control plane node in the clear.\n"
	return append([]byte(header), auditPolicyTemplate...)
}

// AuditSource is which policy saving leaves at files/aipub/audit-policy.yaml.
type AuditSource int

const (
	AuditOff     AuditSource = iota // the preset is off
	AuditDefault                    // nothing there: the wizard writes its default
	AuditSite                       // the site's file is kept
	AuditWizard                     // a default the wizard wrote before is kept
	AuditNode                       // a file taken from a node (AuditFrom) is kept
	AuditHeld                       // existing site, the nodes' policy unknown: nothing written
	AuditBroken                     // something unusable is there: nothing written
)

// AuditPolicyPlan says what saving does with the audit policy.
type AuditPolicyPlan struct {
	Source AuditSource
	// Data is the policy used: the default to write, or the file kept
	Data []byte
	// Note says which policy is used, or what keeps one from being used
	Note string
}

// Warn reports whether nothing usable will be there.
func (a AuditPolicyPlan) Warn() bool { return a.Source == AuditHeld || a.Source == AuditBroken }

// Short is the plan in a few words, for the row of the preset.
func (a AuditPolicyPlan) Short() string {
	switch a.Source {
	case AuditDefault:
		return i18n.T("policy: the wizard writes the default policy")
	case AuditSite:
		return i18n.T("policy: the site file is kept as it is")
	case AuditWizard:
		return i18n.T("policy: the default policy written before is kept as it is")
	case AuditNode:
		return i18n.T("policy: the one taken from the node is used as it is")
	case AuditHeld:
		return i18n.T("⚠ policy: not written, the policy of the nodes is not read yet")
	case AuditBroken:
		return i18n.T("⚠ policy: the file cannot be used")
	}
	return ""
}

// auditFile reads what is at path: the file when it is a regular file that is
// not empty (a symlink followed to one), else nothing and what is wrong; absent
// says nothing is there at all (not a dangling symlink either).
func auditFile(path string) (data []byte, problem string, absent bool) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, "", true
	}
	st, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, i18n.T("a symlink whose target does not exist"), false
	case err != nil:
		return nil, i18n.T("unreadable (%v)", err), false
	case st.IsDir():
		return nil, i18n.T("a directory"), false
	case !st.Mode().IsRegular():
		return nil, i18n.T("not a regular file"), false
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, i18n.T("unreadable (%v)", err), false
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, i18n.T("empty (the apiserver does not start with a policy of no rules)"), false
	}
	return data, "", false
}

// IsWizardPolicy reports whether a policy is a default the wizard wrote.
func IsWizardPolicy(data []byte) bool {
	return bytes.HasPrefix(data, []byte(AuditPolicyMarker))
}

// SamePolicy reports whether two policies have the same rules: the same lines
// but for comments, blank lines, trailing spaces and line ends.
func SamePolicy(a, b []byte) bool {
	norm := func(d []byte) string {
		var out []string
		for _, l := range strings.Split(strings.ReplaceAll(string(d), "\r\n", "\n"), "\n") {
			l = strings.TrimRight(l, " \t\r")
			if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n")
	}
	return norm(a) == norm(b)
}

// PlanAuditPolicy is what saving does with the audit policy at path
// (Paths.AuditPolicy).
func PlanAuditPolicy(path string, s *state.AppState) AuditPolicyPlan {
	def := AuditPolicy(s)
	if def == nil {
		return AuditPolicyPlan{}
	}
	data, problem, absent := auditFile(path)
	switch {
	case problem != "":
		return AuditPolicyPlan{Source: AuditBroken, Note: i18n.T("%s is not usable: %s. The wizard does not write over it. Fix or remove it, then save again", path, problem)}
	case absent && s.Existing && !s.ControlPlane.AuditDefaultOK:
		return AuditPolicyPlan{Source: AuditHeld, Note: i18n.T("existing site: the audit policy of the nodes is not read yet, so the default policy is not written. " +
			"In 4b take the policy of the nodes with c, put a file there yourself, or confirm writing the default policy with o on the 4b audit log row")}
	case absent:
		return AuditPolicyPlan{Source: AuditDefault, Data: def, Note: i18n.T("the wizard writes the default AIPub audit policy (aipub-installer-archive AIP-2611)")}
	case s.ControlPlane.AuditFrom != "":
		note := i18n.T("the audit policy of node %s is taken and used as it is", s.ControlPlane.AuditFrom)
		if SamePolicy(data, def) {
			note += i18n.T(" (same as the default policy of the wizard)")
		}
		return AuditPolicyPlan{Source: AuditNode, Data: data, Note: note}
	case IsWizardPolicy(data):
		return AuditPolicyPlan{Source: AuditWizard, Data: data, Note: i18n.T("the default policy the wizard wrote before is kept as it is")}
	}
	return AuditPolicyPlan{Source: AuditSite, Data: data, Note: i18n.T("the policy file of the site is kept as it is (the wizard does not write over it)")}
}

// AuditAdoption is what became of the policy a node runs with.
type AuditAdoption struct {
	Note  string // what was found or done, for a person
	Taken bool   // the node's policy was written to files/aipub now
	// Kept: a usable file is at files/aipub that the wizard did not write (the
	// site's, or the node's taken now or before)
	Kept bool
}

// AdoptAuditPolicy keeps the policy a node runs with (node, nil when it could
// not be read): written to path as it is when nothing is there yet, so that
// saving keeps it. A file there is never written over.
func AdoptAuditPolicy(path string, node []byte) (AuditAdoption, error) {
	data, problem, absent := auditFile(path)
	switch {
	case problem != "":
		return AuditAdoption{Note: i18n.T("⚠ %s is not usable: %s. The policy of the node was not written there", path, problem)}, nil
	case absent && node == nil:
		return AuditAdoption{}, nil
	case absent:
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return AuditAdoption{}, err
		}
		if err := os.WriteFile(path, node, 0644); err != nil {
			return AuditAdoption{}, err
		}
		note := i18n.T("the audit policy of the node is taken and used as it is (%s)", path)
		if SamePolicy(node, auditPolicyTemplate) {
			note += i18n.T(", same as the default policy of the wizard")
		}
		return AuditAdoption{Note: note, Taken: true, Kept: true}, nil
	}
	kept := !IsWizardPolicy(data)
	what := i18n.T("the policy file of the site")
	if !kept {
		what = i18n.T("the default policy the wizard wrote")
	}
	switch {
	case node == nil:
		return AuditAdoption{Note: i18n.T("%s has %s, so it is used", path, what), Kept: kept}, nil
	case SamePolicy(data, node):
		return AuditAdoption{Note: i18n.T("%s: %s is the same as the policy of the node, so it is used as it is", path, what), Kept: kept}, nil
	}
	return AuditAdoption{Note: i18n.T("⚠ %s: %s differs from the policy of the node. It is left as it is, and update-cluster changes the policy of the node to it", path, what), Kept: kept}, nil
}

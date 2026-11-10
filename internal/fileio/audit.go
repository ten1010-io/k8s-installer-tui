package fileio

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
// one is kept (not upgraded).
const auditPolicyVersion = "v1"

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
		return "정책: 위저드가 기본 정책을 씀"
	case AuditSite:
		return "정책: 사이트 파일을 그대로 둠"
	case AuditWizard:
		return "정책: 전에 쓴 기본 정책을 그대로 둠"
	case AuditNode:
		return "정책: 노드에서 가져온 것을 그대로 씀"
	case AuditHeld:
		return "⚠ 정책: 노드의 정책을 읽기 전이라 쓰지 않음"
	case AuditBroken:
		return "⚠ 정책 파일을 쓸 수 없음"
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
		return nil, "대상이 없는 심볼릭 링크입니다", false
	case err != nil:
		return nil, "읽을 수 없습니다 (" + err.Error() + ")", false
	case st.IsDir():
		return nil, "디렉터리입니다", false
	case !st.Mode().IsRegular():
		return nil, "일반 파일이 아닙니다", false
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, "읽을 수 없습니다 (" + err.Error() + ")", false
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, "비어 있습니다 (apiserver 가 규칙 0개 정책으로 뜨지 않음)", false
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
		return AuditPolicyPlan{Source: AuditBroken, Note: path + " 이(가) " + problem + ": 위저드가 덮어쓰지 않습니다. 고치거나 지운 뒤 다시 저장하세요"}
	case absent && s.Existing && !s.ControlPlane.AuditDefaultOK:
		return AuditPolicyPlan{Source: AuditHeld, Note: "기존 사이트: 노드의 감사 정책을 아직 읽지 않아 기본 정책을 쓰지 않습니다. " +
			"4b 에서 c 로 노드의 정책을 가져오거나, 파일을 직접 두거나, 4b 감사 로그 줄에서 o 로 기본 정책 쓰기를 확인하세요"}
	case absent:
		return AuditPolicyPlan{Source: AuditDefault, Data: def, Note: "위저드가 기본 AIPub 감사 정책을 씁니다 (aipub-installer-archive AIP-2611)"}
	case s.ControlPlane.AuditFrom != "":
		note := "노드 " + s.ControlPlane.AuditFrom + " 의 감사 정책을 가져와 그대로 씁니다"
		if SamePolicy(data, def) {
			note += " (위저드 기본 정책과 같음)"
		}
		return AuditPolicyPlan{Source: AuditNode, Data: data, Note: note}
	case IsWizardPolicy(data):
		return AuditPolicyPlan{Source: AuditWizard, Data: data, Note: "위저드가 전에 쓴 기본 정책을 그대로 둡니다"}
	}
	return AuditPolicyPlan{Source: AuditSite, Data: data, Note: "사이트의 정책 파일을 그대로 둡니다 (위저드가 덮어쓰지 않음)"}
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
		return AuditAdoption{Note: "⚠ " + path + " 이(가) " + problem + ": 노드의 정책을 그 자리에 쓰지 않았습니다"}, nil
	case absent && node == nil:
		return AuditAdoption{}, nil
	case absent:
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return AuditAdoption{}, err
		}
		if err := os.WriteFile(path, node, 0644); err != nil {
			return AuditAdoption{}, err
		}
		note := "노드의 감사 정책을 가져와 그대로 씁니다 (" + path + ")"
		if SamePolicy(node, auditPolicyTemplate) {
			note += ", 위저드 기본 정책과 같음"
		}
		return AuditAdoption{Note: note, Taken: true, Kept: true}, nil
	}
	kept := !IsWizardPolicy(data)
	what := "사이트의 정책 파일"
	if !kept {
		what = "위저드가 쓴 기본 정책"
	}
	switch {
	case node == nil:
		return AuditAdoption{Note: path + " 에 " + what + "이 있어 그것을 씁니다", Kept: kept}, nil
	case SamePolicy(data, node):
		return AuditAdoption{Note: path + " 의 " + what + "이 노드의 정책과 같습니다: 그대로 씁니다", Kept: kept}, nil
	}
	return AuditAdoption{Note: "⚠ " + path + " 의 " + what + "이 노드의 정책과 다릅니다: 그대로 두고, update-cluster 가 노드의 정책을 이것으로 바꿉니다", Kept: kept}, nil
}

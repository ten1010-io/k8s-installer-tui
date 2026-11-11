package fileio

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// auditState is sampleState with the audit preset on and nothing else.
func auditState() *state.AppState {
	s := sampleState()
	s.ControlPlane = state.ControlPlane{Audit: true}
	return s
}

// A new site: saving writes the default under its marker line, and saving again
// keeps it (no .bak, the same bytes).
func TestAuditPolicyDefaultWritten(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	s := auditState()
	must(t, WriteAll(p, s))
	data, err := os.ReadFile(p.AuditPolicy())
	must(t, err)
	first := strings.SplitN(string(data), "\n", 2)[0]
	if !strings.HasPrefix(first, AuditPolicyMarker) || !strings.Contains(first, "template "+auditPolicyVersion) ||
		!strings.Contains(first, "aipub-installer-archive 0bb076d8") {
		t.Errorf("marker line: %q", first)
	}
	if !bytes.HasSuffix(data, auditPolicyTemplate) || !IsWizardPolicy(data) {
		t.Errorf("not the template under the marker:\n%s", data)
	}
	if st, _ := os.Stat(p.AuditPolicy()); st.Mode().Perm() != 0644 {
		t.Errorf("mode %v", st.Mode())
	}
	must(t, WriteAll(p, s))
	if _, err := os.Stat(p.AuditPolicy() + ".bak"); err == nil {
		t.Error("the policy written before was backed up and written again")
	}
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditWizard || !strings.Contains(a.Note, "전에 쓴 기본 정책") {
		t.Errorf("plan after saving: %+v", a)
	}
}

// A policy the site has is never written over: the bytes stay, no .bak.
func TestAuditPolicySiteFileKept(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	s := auditState()
	site := []byte("apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: Metadata\n")
	must(t, os.MkdirAll(filepath.Dir(p.AuditPolicy()), 0755))
	must(t, os.WriteFile(p.AuditPolicy(), site, 0600))
	must(t, WriteAll(p, s))
	got, err := os.ReadFile(p.AuditPolicy())
	must(t, err)
	if !bytes.Equal(got, site) {
		t.Errorf("the site's policy changed:\n%s", got)
	}
	if _, err := os.Stat(p.AuditPolicy() + ".bak"); err == nil {
		t.Error("the site's policy was backed up")
	}
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditSite || !bytes.Equal(a.Data, site) || !strings.Contains(a.Note, "사이트의 정책 파일을 그대로") {
		t.Errorf("plan: %+v", a)
	}
	// taken from a node: said so
	s.ControlPlane.AuditFrom = "node1"
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditNode || a.Note != "노드 node1 의 감사 정책을 가져와 그대로 씁니다" {
		t.Errorf("plan of a node's policy: %+v", a)
	}
}

// The preset off writes no policy.
func TestAuditPolicyPresetOffWritesNothing(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	s := auditState()
	s.ControlPlane.Audit = false
	must(t, WriteAll(p, s))
	if _, err := os.Lstat(p.AuditPolicy()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a policy was written with the preset off: %v", err)
	}
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditOff || a.Note != "" {
		t.Errorf("plan: %+v", a)
	}
}

// An empty file, a directory, a dangling symlink at the path: not written over,
// and said what is wrong; the installer would not find a usable file either.
func TestAuditPolicyUnusable(t *testing.T) {
	cases := []struct {
		name  string
		place func(path string)
		want  string
	}{
		{"empty", func(path string) { must(t, os.WriteFile(path, nil, 0644)) }, "비어 있습니다"},
		{"directory", func(path string) { must(t, os.Mkdir(path, 0755)) }, "디렉터리입니다"},
		{"dangling symlink", func(path string) { must(t, os.Symlink(filepath.Join(filepath.Dir(path), "gone.yaml"), path)) },
			"대상이 없는 심볼릭 링크입니다"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := DefaultPaths(t.TempDir())
			s := auditState()
			must(t, os.MkdirAll(filepath.Dir(p.AuditPolicy()), 0755))
			c.place(p.AuditPolicy())
			before, _ := os.Lstat(p.AuditPolicy())
			must(t, WriteAll(p, s))
			after, err := os.Lstat(p.AuditPolicy())
			if err != nil || after.Mode() != before.Mode() || after.Size() != before.Size() {
				t.Errorf("written over: %v %v -> %v", err, before.Mode(), after)
			}
			if _, err := os.Lstat(filepath.Join(filepath.Dir(p.AuditPolicy()), "gone.yaml")); err == nil {
				t.Error("written through the symlink")
			}
			a := PlanAuditPolicy(p.AuditPolicy(), s)
			if a.Source != AuditBroken || !a.Warn() || !strings.Contains(a.Note, c.want) || !strings.Contains(a.Note, "덮어쓰지 않습니다") {
				t.Errorf("plan: %+v", a)
			}
			if got := MissingCpFiles(s, p.FilesDir()); len(got) != 1 || got[0].Src != state.AuditPolicyFile {
				t.Errorf("missing: %v", got)
			}
		})
	}
	// a symlink to a policy is the policy
	p := DefaultPaths(t.TempDir())
	s := auditState()
	must(t, os.MkdirAll(filepath.Dir(p.AuditPolicy()), 0755))
	target := filepath.Join(t.TempDir(), "policy.yaml")
	must(t, os.WriteFile(target, []byte("apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: None\n"), 0644))
	must(t, os.Symlink(target, p.AuditPolicy()))
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditSite {
		t.Errorf("symlink to a policy: %+v", a)
	}
}

// An existing site whose nodes' policy was not read gets no default, until the
// operator says so.
func TestAuditPolicyHeldOnExistingSite(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	s := auditState()
	s.Existing = true
	must(t, WriteAll(p, s))
	if _, err := os.Lstat(p.AuditPolicy()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the default was written on an existing site: %v", err)
	}
	if a := PlanAuditPolicy(p.AuditPolicy(), s); a.Source != AuditHeld || !strings.Contains(a.Note, "노드의 감사 정책을 아직 읽지 않아") {
		t.Errorf("plan: %+v", a)
	}
	if got := MissingCpFiles(s, p.FilesDir()); len(got) != 1 || got[0].Src != state.AuditPolicyFile {
		t.Errorf("held policy not reported missing: %v", got)
	}
	s.ControlPlane.AuditDefaultOK = true
	must(t, WriteAll(p, s))
	if data, err := os.ReadFile(p.AuditPolicy()); err != nil || !IsWizardPolicy(data) {
		t.Errorf("default not written once confirmed: %v", err)
	}
}

// A node's policy goes where nothing is, as it is; a file there is kept.
func TestAdoptAuditPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aipub", "audit-policy.yaml")
	node := []byte("apiVersion: audit.k8s.io/v1\r\nkind: Policy\r\nrules:\r\n- level: Metadata\r\n")
	a, err := AdoptAuditPolicy(path, node)
	must(t, err)
	got, _ := os.ReadFile(path)
	if !a.Taken || !a.Kept || !bytes.Equal(got, node) || !strings.HasPrefix(a.Note, "노드의 감사 정책을 가져와 그대로 씁니다") ||
		strings.Contains(a.Note, "기본 정책과 같음") {
		t.Errorf("%+v\n%q", a, got)
	}
	// again, with another policy on the node: the file stays
	a, err = AdoptAuditPolicy(path, []byte("rules:\n- level: None\n"))
	must(t, err)
	got, _ = os.ReadFile(path)
	if a.Taken || !a.Kept || !bytes.Equal(got, node) || !strings.Contains(a.Note, "노드의 정책과 다릅니다") {
		t.Errorf("%+v", a)
	}
	// the default on the node, with its own comments and CRLF: said so
	other := filepath.Join(t.TempDir(), "audit-policy.yaml")
	crlf := bytes.ReplaceAll(append([]byte("# AIP-2611\n"), auditPolicyTemplate...), []byte("\n"), []byte("\r\n"))
	if a, _ := AdoptAuditPolicy(other, crlf); !strings.HasSuffix(a.Note, "위저드 기본 정책과 같음") {
		t.Errorf("the default not recognised: %+v", a)
	}
	// not read: nothing written
	none := filepath.Join(t.TempDir(), "audit-policy.yaml")
	if a, _ := AdoptAuditPolicy(none, nil); a.Taken || a.Kept || a.Note != "" {
		t.Errorf("%+v", a)
	}
	if _, err := os.Lstat(none); err == nil {
		t.Error("written without a policy")
	}
}

package screens

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// A note of the 확인할 점 tab is broken at its spaces under its mark, Korean
// counted two columns; a word wider than the line is cut.
func TestWrapNote(t *testing.T) {
	got := wrapNote("  • /opt/k8s-installer/ansible/files/aipub/audit-policy.yaml (AIPub 감사 로그 정책: AIPub 모니터링/python 팀 제공)", 40)
	want := []string{
		"  • /opt/k8s-installer/ansible/files/aip", // the path is wider than the line: cut at 40 columns
		"    ub/audit-policy.yaml (AIPub 감사",
		"    로그 정책: AIPub 모니터링/python 팀",
		"    제공)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, l := range got {
		if lipgloss.Width(l) > 40 {
			t.Errorf("%q wider than 40", l)
		}
	}
	for _, c := range []struct {
		text  string
		width int
		want  []string
	}{
		// a word of wide runes cut between them, under the mark
		{"⚠ 확인할점", 5, []string{"⚠ 확", "  인", "  할", "  점"}},
		// too narrow for the indent and a wide rune: left whole, and the loop ends
		{"⚠ 확인", 3, []string{"⚠ 확인"}},
		// a run of spaces is kept; at a break it goes
		{"  • a  b", 40, []string{"  • a  b"}},
		{"aaa  bbb", 4, []string{"aaa", "bbb"}},
		{"", 40, []string{""}},
	} {
		if got := wrapNote(c.text, c.width); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapNote(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

// The audit policy has a tab, and the saved files name it: written when it is
// the default, kept (and said so) when the site has one; with nothing usable the
// tab says why and the file is reported missing.
func TestPreviewAuditPolicy(t *testing.T) {
	paths := fileio.DefaultPaths(t.TempDir())
	st := state.DefaultState()
	st.ControlPlane.Audit = true
	s := NewS7Preview(st, paths, t.TempDir())
	s.SetSize(200, 60)
	s.SyncFromState(st)
	tab := func() *previewTab {
		for i := range s.tabs {
			if s.tabs[i].name == state.AuditPolicyFile {
				return &s.tabs[i]
			}
		}
		return nil
	}
	if a := tab(); a == nil || a.path != paths.AuditPolicy() || a.kept != "" || !fileio.IsWizardPolicy([]byte(a.content)) {
		t.Fatalf("default: %+v", a)
	}
	for _, m := range s.missing {
		if m.Src == state.AuditPolicyFile {
			t.Errorf("reported missing although saving writes it: %v", s.missing)
		}
	}
	s.errors = nil // the nodes are not this test's
	s.doSave()
	notes := s4bANSI.ReplaceAllString(s.notesText(), "")
	if !strings.Contains(notes, "  "+paths.AuditPolicy()) || strings.Contains(notes, "쓰지 않음") {
		t.Errorf("saved files:\n%s", notes)
	}

	// saved again: the file is kept, the list says so
	s.SyncFromState(st)
	if a := tab(); a == nil || a.kept != "위저드가 전에 쓴 기본 정책을 그대로 둡니다" {
		t.Fatalf("kept: %+v", a)
	}
	s.errors = nil
	s.doSave()
	if notes := s4bANSI.ReplaceAllString(s.notesText(), ""); !strings.Contains(notes, paths.AuditPolicy()+" — 쓰지 않음: 위저드가 전에 쓴 기본 정책을 그대로 둡니다") {
		t.Errorf("saved files:\n%s", notes)
	}

	// an empty file: no path to save, the tab and the missing file say why
	if err := os.WriteFile(paths.AuditPolicy(), nil, 0644); err != nil {
		t.Fatal(err)
	}
	s.SyncFromState(st)
	if a := tab(); a == nil || a.path != "" || !strings.Contains(a.content, "비어 있습니다") {
		t.Fatalf("empty: %+v", a)
	}
	if len(s.missing) != 1 || !strings.Contains(schema.FileSource(&st.ControlPlane, s.missing[0]), "비어 있습니다") {
		t.Errorf("missing: %v", s.missing)
	}
}

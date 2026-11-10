package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

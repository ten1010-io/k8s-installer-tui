package screens

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Wrap breaks English at its spaces, never inside a word, and no line is wider
// than asked: lipgloss's own wrap gave "... there is a clu" / "ster" /
// "already)". Korean, indents, blank lines and a word too long are kept as
// they were.
func TestWrapBreaksAtSpaces(t *testing.T) {
	site := "Site: New site: no /etc/k8s-installer/applied-release (start with --existing when there is a cluster already)"
	got := strings.Split(Wrap(site, 96), "\n")
	if len(got) != 2 || !strings.HasSuffix(got[0], " a") || got[1] != "cluster already)" {
		t.Errorf("%q", got)
	}
	ko := "사이트: 새 사이트: /etc/k8s-installer/applied-release 가 없음 (클러스터가 이미 있으면 --existing 으로 시작하세요)"
	text := "Checks\n\n        looks at: " + strings.Repeat("word ", 30) + "\n" + ko + "\n" + strings.Repeat("x", 120)
	for _, l := range strings.Split(Wrap(text, 40), "\n") {
		if lipgloss.Width(l) > 40 {
			t.Errorf("wider than 40: %q", l)
		}
	}
	lines := strings.Split(Wrap(text, 40), "\n")
	if lines[0] != "Checks" || lines[1] != "" || !strings.HasPrefix(lines[2], "        looks at: word") {
		t.Errorf("first lines: %q", lines[:3])
	}
	for _, l := range lines {
		if strings.Contains(l, "wor\n") || strings.HasPrefix(l, "d ") || strings.HasPrefix(l, "rd ") {
			t.Errorf("a word broken: %q", l)
		}
	}
	if Wrap("as it is", 0) != "as it is" {
		t.Error("no width: changed")
	}
}

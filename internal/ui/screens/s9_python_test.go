package screens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
)

// withPython puts setup-python.yml in the install root of the screen, as an
// installer with the python module has it, and builds the plan again.
func withPython(t *testing.T, s9 *S9Install) {
	t.Helper()
	path := filepath.Join(s9.opts.AnsibleDir(), filepath.FromSlash(plan.PythonPlaybook))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# setup-python.yml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s9.rebuild()
}

// Beside an installer with the python module the install screen offers the
// python services, on by default, and the plan ends with A6 and A7. Turned off
// there, the two steps stay in the plan as skipped.
func TestInstallScreenOffersPython(t *testing.T) {
	s9, _ := newRunScreen(t)
	if v := plainView(s9); strings.Contains(v, "Python 서비스") || strings.Contains(v, "setup-python") {
		t.Fatalf("an installer without the python module is offered python:\n%s", v)
	}

	withPython(t, s9)
	v := plainView(s9)
	for _, want := range []string{"Python 서비스", "A6", "setup-python", "A7", "setup-aipub (다시)"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the screen lacks %q:\n%s", want, v)
		}
	}
	if !s9.opts.Python {
		t.Fatal("python should be on by default")
	}

	// archive path → monitoring → python services, then Space
	p := newPump(s9)
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	p.key(tea.KeyMsg{Type: tea.KeyTab})
	if s9.focusIdx != s9FocusPython {
		t.Fatalf("two Tabs from the archive path should reach the python services, focus %d", s9.focusIdx)
	}
	p.key(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if s9.opts.Python {
		t.Fatal("Space on the python services should turn them off")
	}
	if v := plainView(s9); !strings.Contains(v, "python 서비스 설치 안 함") {
		t.Errorf("the plan does not say the python steps are skipped:\n%s", v)
	}
	var skipped []string
	for _, st := range s9.steps {
		if st.Skip && (st.ID == "A6" || st.ID == "A7") {
			skipped = append(skipped, st.ID)
		}
	}
	if strings.Join(skipped, " ") != "A6 A7" {
		t.Errorf("skipped %v, want A6 and A7", skipped)
	}
}

// With the python services offered the focus still walks every input and back.
func TestInstallScreenFocusWithPython(t *testing.T) {
	s9, _ := newRunScreen(t)
	withPython(t, s9)
	p := newPump(s9)
	seen := map[int]bool{s9.focusIdx: true}
	for i := 0; i < 6; i++ {
		p.key(tea.KeyMsg{Type: tea.KeyTab})
		seen[s9.focusIdx] = true
	}
	for _, f := range []int{s9FocusArchive, s9FocusMonitoring, s9FocusPython, s9FocusNav} {
		if !seen[f] {
			t.Errorf("Tab never reached focus %d (saw %v)", f, seen)
		}
	}
	for i := 0; i < 6; i++ {
		p.key(tea.KeyMsg{Type: tea.KeyUp})
	}
	if s9.focusIdx != s9FocusArchive {
		t.Errorf("↑ back from the buttons ends on focus %d, want the archive path", s9.focusIdx)
	}
}

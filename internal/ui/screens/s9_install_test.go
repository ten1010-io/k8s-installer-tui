package screens

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The plan is a stop of the focus only while it is taller than its room: once
// it fits (here a larger screen), the focus on it goes on to the buttons.
func TestS9PlanStopLeftWhenItFits(t *testing.T) {
	st := state.DefaultState()
	st.Nodes = []state.NodeConfig{{Name: "node1", AnsibleHost: "192.168.0.11"}}
	st.KiCpNodes = []string{"node1"}
	st.K8sNodes = []string{"node1"}
	st.K8sCpNodes = []string{"node1"}
	s := NewS9Install(st, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions())
	s.SetSize(96, 20)
	s.SyncFromState(st)
	s.Update(tea.KeyMsg{Type: tea.KeyDown}) // off the archive path; AIPub off, so no 모니터링
	if s.focusIdx != s9FocusPlan {
		t.Fatalf("focus %d on a screen of 20, want the plan", s.focusIdx)
	}
	s.SetSize(96, 200)
	if s.focusIdx != s9FocusNav {
		t.Errorf("focus %d once the plan fits, want the buttons", s.focusIdx)
	}
}

package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func pythonScreen(t *testing.T) (*S5cPython, *state.AppState) {
	t.Helper()
	st := state.DefaultState()
	st.AipubEnabled = true
	st.K8sNodes = []string{"node1", "node2", "node3"}
	s := NewS5cPython()
	s.SetSize(100, 40)
	s.SyncFromState(st)
	return s, st
}

func pressKeys(s *S5cPython, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		s.Update(msg)
	}
}

// The python screen: the name is typed, GPU nodes are checked one by one, the
// deploy tool moves to its next choice and the network operator is a toggle;
// all four go to the state.
func TestPythonScreenEdits(t *testing.T) {
	s, st := pythonScreen(t)
	if v := s.View(); !strings.Contains(v, "cluster1") || !strings.Contains(v, "node3") {
		t.Fatalf("the screen does not start from the state:\n%s", v)
	}

	pressKeys(s, "ctrl+u", "seoul-a")
	// GPU nodes: node1 stays off, node2 and node3 are checked
	pressKeys(s, "down", "down", "space", "down", "space")
	// deploy tool: installer → dag
	pressKeys(s, "down", "enter")
	// network operator on
	pressKeys(s, "down", "space")
	if msgs := s.Validate(); len(msgs) > 0 {
		t.Fatalf("refused: %v", msgs)
	}
	s.SyncToState(st)
	if st.PythonCoasterClusterName != "seoul-a" || strings.Join(st.PythonCoasterGpuNodes, ",") != "node2,node3" ||
		st.PythonDeployTool != "dag" || !st.PythonNetworkOperatorEnabled {
		t.Errorf("state: %q %v %q %v", st.PythonCoasterClusterName, st.PythonCoasterGpuNodes, st.PythonDeployTool, st.PythonNetworkOperatorEnabled)
	}
	if v := s.View(); !strings.Contains(v, "aipub_python_network_operator_ib_nodes") {
		t.Errorf("with the operator on the screen should say where the InfiniBand nodes are set:\n%s", v)
	}
}

func TestPythonScreenRefusals(t *testing.T) {
	// no cluster name: check-python.yml of aipub-installer refuses it too
	s, _ := pythonScreen(t)
	pressKeys(s, "ctrl+u")
	if msgs := strings.Join(s.Validate(), "\n"); !strings.Contains(msgs, "aipub_python_coaster_cluster_name") {
		t.Errorf("an empty cluster name passed: %q", msgs)
	}

	// a GPU node the site file names that is no node of the cluster: shown, so
	// that it can be unchecked, and refused until it is
	st := state.DefaultState()
	st.K8sNodes = []string{"node1"}
	st.PythonCoasterGpuNodes = []string{"node9"}
	s = NewS5cPython()
	s.SetSize(100, 40)
	s.SyncFromState(st)
	if !strings.Contains(s.View(), "node9") {
		t.Fatalf("the node of the site file is not shown:\n%s", s.View())
	}
	if msgs := strings.Join(s.Validate(), "\n"); !strings.Contains(msgs, "node9") {
		t.Errorf("a GPU node outside the cluster passed: %q", msgs)
	}
	pressKeys(s, "down", "down", "space") // node1, then node9: unchecked
	if msgs := s.Validate(); len(msgs) > 0 {
		t.Errorf("after unchecking it: %v", msgs)
	}
}

// The letters h, j, k and l are text in the name and movement elsewhere.
func TestPythonScreenLettersInTheName(t *testing.T) {
	s, st := pythonScreen(t)
	pressKeys(s, "ctrl+u", "h", "j", "k", "l")
	s.SyncToState(st)
	if st.PythonCoasterClusterName != "hjkl" {
		t.Errorf("name %q, want hjkl", st.PythonCoasterClusterName)
	}
	if s.focusIdx != s5cFocusClusterName {
		t.Errorf("typing moved the focus to %d", s.focusIdx)
	}
}

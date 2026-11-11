package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func s5bState() *state.AppState {
	st := state.DefaultState()
	st.AipubEnabled = true
	st.K8sNodes = []string{"node1", "node2", "node3"}
	st.K8sCpNodes = []string{"node1", "node2"}
	return st
}

func send(s *S5bMonitoring, msgs ...tea.KeyMsg) {
	for _, m := range msgs {
		s.Update(m)
	}
}

func runes(text string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)} }

// The defaults come through untouched; what is typed, toggled and picked goes to
// the state; the nodes offered are the k8s control plane nodes.
func TestS5bMonitoring(t *testing.T) {
	st := s5bState()
	s := NewS5bMonitoring()
	s.SetSize(100, 60)
	s.SyncFromState(st)
	if msgs := s.Validate(); len(msgs) != 0 {
		t.Fatalf("defaults refused: %v", msgs)
	}
	back := s5bState()
	s.SyncToState(back)
	if back.MonitoringOpensearchStorageSize != "3Ti" || back.MonitoringApiReplicas != 2 || !back.MonitoringRightsizingEnabled ||
		len(back.MonitoringCpNodes) != 0 {
		t.Errorf("defaults changed on the way: %+v", back)
	}
	view := s.View()
	for _, want := range []string{"node1", "node2", "라이트사이징", "API 레플리카 수"} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen lacks %q", want)
		}
	}
	if strings.Contains(view, "node3") {
		t.Error("a worker offered as an OpenSearch node")
	}

	send(s, tea.KeyMsg{Type: tea.KeyCtrlU}, runes("500Gi")) // OpenSearch size
	s.focusIdx = s5bFocusRightsizing
	send(s, tea.KeyMsg{Type: tea.KeyEnter}) // off
	send(s, tea.KeyMsg{Type: tea.KeyDown})
	send(s, tea.KeyMsg{Type: tea.KeyCtrlU}, runes("1")) // api replicas
	send(s, tea.KeyMsg{Type: tea.KeyDown})              // nodes
	send(s, tea.KeyMsg{Type: tea.KeyDown})              // node2
	send(s, tea.KeyMsg{Type: tea.KeyEnter})
	s.SyncToState(back)
	if back.MonitoringOpensearchStorageSize != "500Gi" || back.MonitoringRightsizingEnabled || back.MonitoringApiReplicas != 1 ||
		strings.Join(back.MonitoringCpNodes, ",") != "node2" {
		t.Errorf("state: %+v", back)
	}

	// a word where a count goes
	s.focusIdx = s5bFocusApiReplicas
	s.syncFocus()
	send(s, tea.KeyMsg{Type: tea.KeyCtrlU}, runes("two"))
	if msgs := s.Validate(); len(msgs) != 1 || !strings.Contains(msgs[0], "aipub_monitoring_api_replicas") {
		t.Errorf("a word as replicas: %v", msgs)
	}
}

// A node of the state that is not a control plane node is kept on the screen, so
// that the check names it.
func TestS5bKeepsUnknownNode(t *testing.T) {
	st := s5bState()
	st.MonitoringCpNodes = []string{"node3"}
	s := NewS5bMonitoring()
	s.SyncFromState(st)
	if !strings.Contains(s.View(), "node3") {
		t.Error("the node was dropped")
	}
	if msgs := s.Validate(); len(msgs) != 1 || !strings.Contains(msgs[0], "node3") {
		t.Errorf("the check: %v", msgs)
	}
}

package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func s5State() *state.AppState {
	st := state.DefaultState()
	st.AipubEnabled = true
	st.K8sCpNodes = []string{"node1"}
	st.K8sNodes = []string{"node1", "node2"}
	st.AipubCpNodes = []string{"node1"}
	return st
}

func press(s *S5Aipub, key tea.KeyType) {
	s.Update(tea.KeyMsg{Type: key})
}

// Enter on a deploy tool or the edition picks the next value of the schema's
// enum; the paths of the binaries are on the screen only while helmfile is
// chosen, and the arrow keys skip them otherwise.
func TestS5DeployChoices(t *testing.T) {
	st := s5State()
	s := NewS5Aipub()
	s.SetSize(100, 60)
	s.SyncFromState(st)

	if strings.Contains(s.View(), "helmfile 경로") {
		t.Error("the helmfile path is shown with the installer as the tool")
	}
	s.focusIdx = s5FocusEdition
	press(s, tea.KeyEnter)
	if s.edition != "lite" {
		t.Errorf("edition = %q after one Enter", s.edition)
	}
	press(s, tea.KeyEnter)
	if s.edition != "enterprise" {
		t.Errorf("edition = %q after two", s.edition)
	}

	// down from the edition goes to the tool, then past the hidden path to the monitoring tool
	press(s, tea.KeyDown)
	if s.focusIdx != s5FocusAppTool {
		t.Fatalf("focus %d, want the AIPub tool", s.focusIdx)
	}
	press(s, tea.KeyDown)
	if s.focusIdx != s5FocusMonTool {
		t.Fatalf("focus %d, want the monitoring tool (the helmfile path is hidden)", s.focusIdx)
	}

	press(s, tea.KeyUp)
	press(s, tea.KeyEnter) // AIPub: helmfile
	if s.appTool != "helmfile" || !strings.Contains(s.View(), "helmfile 경로") {
		t.Fatalf("helmfile chosen but its path is not shown (tool %q)", s.appTool)
	}
	press(s, tea.KeyDown)
	if s.focusIdx != s5FocusAppBin || !s.inputFocused() {
		t.Fatalf("focus %d, want the helmfile path", s.focusIdx)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/opt/bin/helmfile")})
	press(s, tea.KeyDown)
	s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // monitoring: helmfile
	if s.focusIdx != s5FocusMonTool || s.monTool != "helmfile" {
		t.Fatalf("focus %d, monitoring tool %q", s.focusIdx, s.monTool)
	}
	for _, want := range []string{"kustomize 경로", "helm 경로", "dry-run"} {
		if !strings.Contains(s.View(), want) {
			t.Errorf("monitoring helmfile rows lack %q", want)
		}
	}
	press(s, tea.KeyDown)
	press(s, tea.KeyDown)
	press(s, tea.KeyDown)
	press(s, tea.KeyDown)
	if s.focusIdx != s5FocusMonDryRun {
		t.Fatalf("focus %d, want dry-run", s.focusIdx)
	}
	press(s, tea.KeyEnter)

	st2 := s5State()
	s.SyncToState(st2)
	if st2.AipubAppDeployTool != "helmfile" || st2.AipubMonitoringDeployTool != "helmfile" ||
		st2.AipubAppHelmfileBin != "/opt/bin/helmfile" || !st2.AipubMonitoringHelmfileDryRun || st2.AipubAppEdition != "enterprise" {
		t.Errorf("state not filled: %+v", st2)
	}
	if st2.AipubMonitoringHelmBin != "helm" {
		t.Errorf("helm = %q", st2.AipubMonitoringHelmBin)
	}
}

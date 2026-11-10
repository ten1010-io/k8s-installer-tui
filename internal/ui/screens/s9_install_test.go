package screens

import (
	"context"
	"errors"
	"strings"
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

// When ssh-copy-id gives the terminal back (tea.ExecProcess calls back with
// sendDoneMsg), P2 runs again, whether it ended well or was interrupted.
func TestS9RunsP2AfterTheTerminal(t *testing.T) {
	for _, err := range []error{nil, errors.New("signal: interrupt")} {
		st := state.DefaultState()
		s := NewS9Install(st, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions())
		s.SyncFromState(st)
		var p2 int
		s.SetRunner(func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
			if strings.Contains(args[len(args)-1], "'sudo -n true'") {
				p2++
				return []byte("node1 | CHANGED | rc=0 | (stdout) \n"), nil
			}
			return nil, nil
		})
		_, cmd := s.Update(sendDoneMsg{err: err})
		if cmd == nil {
			t.Fatal("P2 does not run again")
		}
		s.Update(cmd())
		if r := s.checks["P2"]; p2 != 1 || !r.OK || s.checking {
			t.Errorf("err %v: P2 ran %d times: %+v", err, p2, r)
		}
		if err != nil && !strings.Contains(s.message, "끝나지 않았습니다 (signal: interrupt)") {
			t.Errorf("message %q", s.message)
		}
	}
}

// A load balancer VIP outside the internal subnets is shown on the preview as a
// warning, and saving does not wait on it.
func TestPreviewWarnsAVIPOutsideTheInternalSubnets(t *testing.T) {
	st := state.DefaultState()
	st.Nodes = []state.NodeConfig{{Name: "node1", AnsibleHost: "192.168.0.11"}}
	st.KiCpNodes, st.K8sNodes, st.K8sCpNodes, st.AipubCpNodes = []string{"node1"}, []string{"node1"}, []string{"node1"}, []string{"node1"}
	st.KiCpHaMode = false
	st.AipubEnabled = true
	st.AipubHaModeStorageClass = "nfs-client"
	st.K8sIngressVIP = "10.1.0.5"
	s := NewS7Preview(st, fileio.DefaultPaths(t.TempDir()), t.TempDir())
	s.SetSize(110, 60)
	s.SyncFromState(st)
	if v := s.View(); !strings.Contains(v, "k8s_ingress_vip: VIP 10.1.0.5가 내부망 대역 (192.168.0.0/24) 밖 — 의도한 구성인지 확인하세요") {
		t.Errorf("no warning:\n%s", v)
	}
	if len(s.errors) != 0 {
		t.Errorf("refused: %v", s.errors)
	}
}

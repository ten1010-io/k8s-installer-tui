package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

// While the checks run the wizard does not leave the install screen: their
// result comes to the screen shown when they end, and on another screen it
// would be lost, leaving 9.설치 at "확인 중" until the program is started again.
func TestPlanScreenHoldsTheWizardWhileChecking(t *testing.T) {
	app, s9, _ := newSSHApp(t)
	at := app.current
	_, cmd := s9.Update(key("c"))
	if cmd == nil || !s9.Busy() {
		t.Fatalf("c started no checks: cmd %v busy %v", cmd != nil, s9.Busy())
	}
	for _, k := range navKeys {
		app.Update(k)
		app.Update(screens.NavigateNextMsg{})
		app.Update(screens.NavigatePrevMsg{})
		if app.current != at || !strings.Contains(strings.Join(app.screenErrors, " "), "사전 확인 중") {
			t.Fatalf("%v left the screen while checking: %d %q", k, app.current, app.screenErrors)
		}
	}
	drain(t, s9, cmd)
	if s9.Busy() || strings.Contains(view(s9), "확인 중...") {
		t.Fatalf("the checks ended, still busy %v:\n%s", s9.Busy(), view(s9))
	}
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if app.current == at {
		t.Error("the wizard is held after the checks")
	}
}

package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
)

// fitSizes are terminals an operator has: a classic 80x24 and a shorter one, a
// small one, and a normal one of two heights.
var fitSizes = [][2]int{{80, 18}, {80, 24}, {100, 30}, {135, 45}, {135, 60}}

// fits checks that the whole wizard is in sight on a terminal of height h.
// App.View cuts what is taller than the terminal, so what is checked is that
// nothing was cut: the outer box opens under the top margin and closes with
// only blank lines under it, the screen's panel closes inside it, and the
// buttons of the screen (prev and right) and the key hints are there.
func (d *driver) fits(h int, right string) {
	d.t.Helper()
	view := ansi.ReplaceAllString(d.app.View(), "")
	lines := strings.Split(view, "\n")
	step := i18n.T(stepTitles[d.app.current])
	bottom := -1
	for i, l := range lines {
		if strings.Contains(l, "╰") {
			bottom = i
		}
	}
	opened := len(lines) > 1 && strings.Contains(lines[1], "╭")
	closed := bottom >= 0
	for _, l := range lines[bottom+1:] {
		closed = closed && strings.TrimSpace(l) == ""
	}
	buttons, hints, panel := false, false, false
	for _, l := range lines {
		// in the language of the run: the Korean of the tests, English where a test sets it
		buttons = buttons || strings.Contains(l, "< "+i18n.T("Back")+" >") && strings.Contains(l, "< "+right+" >")
		hints = hints || strings.Contains(l, i18n.T("Ctrl+N next")+" · "+i18n.T("Ctrl+P back"))
		panel = panel || strings.Contains(l, "┗")
	}
	if len(lines) != h || !opened || !closed || !panel || !buttons || !hints {
		d.t.Errorf("%s at %d lines: %d lines, box opened %v, closed %v, panel closed %v, buttons %v, key hints %v:\n%s",
			step, h, len(lines), opened, closed, panel, buttons, hints, view)
	}
}

// Every screen of the wizard, driven as in TestWizardAipubWithoutHA with the
// audit preset on (its policy file is not there) and JWKS waiting for its CA:
// the preview before and after saving, and refusing to go on before.
func TestScreensFitTerminal(t *testing.T) {
	for _, size := range fitSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			h := size[1]
			d, paths := newDriver(t)
			d.send(tea.WindowSizeMsg{Width: size[0], Height: h})
			for _, step := range []string{"1.노드", "2.그룹", "3.네트워크", "4.K8s"} {
				d.at(step)
				d.fits(h, "다음")
				d.keys("ctrl+n")
			}
			d.at("4b.컨트롤플레인")
			d.keys(" ") // AIPub 감사 로그
			d.fits(h, "다음")
			d.keys("ctrl+n")

			d.at("5.AIPub")
			d.keys("enter", "down", "ctrl+u", "aipub.lkjh.local", "down", "enter", "down", "node1")
			d.fits(h, "다음")
			d.keys("ctrl+n")
			d.at("5b.모니터링")
			d.fits(h, "다음")
			d.keys("ctrl+n")
			d.at("6.AIPub LB")
			d.keys("down", "ctrl+u", "lb-hjkl", "down", "192.168.0.100")
			d.fits(h, "다음")
			d.keys("ctrl+n")
			d.at("7.인증서")
			d.fits(h, "다음")
			d.keys("ctrl+n")

			d.at("8.저장")
			// on a low screen the viewport has a line or two, cut to fit
			if h >= 30 && !strings.Contains(d.screen(), "[ 확인할 점 ]") {
				t.Errorf("the preview does not open on what it reports:\n%s", d.screen())
			}
			d.fits(h, "저장 (s)")
			d.keys("ctrl+n") // refused: not saved; the reason under the screen
			d.at("8.저장")
			d.fits(h, "저장 (s)")
			d.keys("s")
			if _, err := os.Stat(paths.Vars); err != nil {
				t.Fatalf("not saved: %v", err)
			}
			d.fits(h, "다음")
			d.keys("tab", "right", "left") // to the files and back: the tabs as they were
			d.fits(h, "다음")
			d.keys("ctrl+n")
			d.at("9.설치")
			d.fits(h, "실행")
		})
	}
}

// An existing site where much of what is saved can not be changed any more:
// the preview keeps its buttons and the key hints in sight.
func TestPreviewManyWarningsFits(t *testing.T) {
	for _, size := range fitSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			d0, paths := newDriver(t)
			s := d0.app.appState
			if err := fileio.WriteAll(paths, s); err != nil {
				t.Fatal(err)
			}
			s.InternalNetworkSubnets = []string{"192.168.0.0/23"}
			s.K8sPodSubnet = "10.233.0.0/16"
			s.K8sServiceSubnet = "10.97.0.0/16"
			s.ControlPlane.Audit = true
			s.AipubEnabled = true // kube-proxy goes off
			s.AipubIngressZone = "lkjh.local"
			s.AipubHaMode = false
			s.AipubHarborLocalPVNode = "node1"
			s.AipubCpNodes = []string{"node1", "node2"}
			s.K8sIngressVIP = "192.168.0.100"
			existing := plan.DefaultOptions()
			existing.Existing = true
			d := &driver{t: t, app: NewApp(s, paths, existing, "test")}
			d.send(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for i := 0; i < 9 && d.app.current < 9; i++ {
				d.keys("ctrl+n")
			}
			d.at("8.저장")
			if size[1] >= 24 && (!strings.Contains(d.screen(), "[ 확인할 점 ]") || !strings.Contains(d.screen(), "⚠ 확인할 점 ")) {
				t.Errorf("the warnings are not reported:\n%s", d.screen())
			}
			d.fits(size[1], "저장 (s)")
			d.keys("s")
			if _, err := os.Stat(paths.AipubVars); err != nil {
				t.Fatalf("not saved: %v", err)
			}
			d.fits(size[1], "다음")
		})
	}
}

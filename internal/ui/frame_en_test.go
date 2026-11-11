package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

// The app frame in English (the default): the header, the footer and what the
// wizard says under a screen have no Hangul, every screen still fits the
// terminals of fitSizes, and the install screens say no Hangul at all, what
// they show of the other packages (the plan, the schema, the validator)
// included.

func frameHangulRuns(s string) []string {
	var runs []string
	var cur []rune
	for _, r := range s + " " {
		if unicode.Is(unicode.Hangul, r) {
			cur = append(cur, r)
			continue
		}
		if len(cur) > 0 {
			runs = append(runs, string(cur))
			cur = nil
		}
	}
	return runs
}

// frameNoHangul fails on any Hangul in s: text that is the frame's alone.
func frameNoHangul(t *testing.T, what string, s ...string) {
	t.Helper()
	for _, l := range s {
		if len(frameHangulRuns(l)) > 0 {
			t.Errorf("%s says Korean in English: %q", what, ansi.ReplaceAllString(l, ""))
		}
	}
}

// english checks the frame of the step the wizard is on, and its screen when it
// is one of the install screens (whole, what they show of other packages
// included).
func (d *driver) english(own bool) {
	d.t.Helper()
	a := d.app
	outerBoxW, _, _ := a.dims()
	w := outerBoxW - 6 // the text width of the outer box, as View gives them
	frameNoHangul(d.t, "the header", a.renderHeader(w))
	frameNoHangul(d.t, "the footer", a.keyHints(w))
	frameNoHangul(d.t, "the errors under the screen", a.errorLines())
	if !own {
		return
	}
	view := ansi.ReplaceAllString(a.View(), "")
	if runs := frameHangulRuns(view); len(runs) > 0 {
		d.t.Errorf("%s says Korean in English (%s):\n%s", i18n.T(stepTitles[a.current]), strings.Join(runs, " "), view)
	}
}

// frameFits is fits without the buttons, for the wizard's screens before the
// install screens, whose buttons are made English apart: the box opens under
// the top margin and closes, the screen's panel closes in it, and the key
// hints are in sight.
func (d *driver) frameFits(h int) {
	d.t.Helper()
	view := ansi.ReplaceAllString(d.app.View(), "")
	lines := strings.Split(view, "\n")
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
	hints, panel := false, false
	for _, l := range lines {
		hints = hints || strings.Contains(l, "Ctrl+N next · Ctrl+P back")
		panel = panel || strings.Contains(l, "┗")
	}
	if len(lines) != h || !opened || !closed || !panel || !hints {
		d.t.Errorf("%s at %d lines: %d lines, box opened %v, closed %v, panel closed %v, key hints %v:\n%s",
			i18n.T(stepTitles[d.app.current]), h, len(lines), opened, closed, panel, hints, view)
	}
}

// The wizard of TestScreensFitTerminal in English: every screen fits, the
// install screens with their buttons, the key hints in sight, and the frame and
// the install screens say no Korean.
func TestScreensFitTerminalInEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)
	for _, size := range fitSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			h := size[1]
			d, paths := newDriver(t)
			d.send(tea.WindowSizeMsg{Width: size[0], Height: h})
			for _, step := range []string{"1.Nodes", "2.Groups", "3.Network", "4.K8s"} {
				d.at(step)
				d.frameFits(h)
				d.english(false)
				d.keys("ctrl+n")
			}
			d.at("4b.Control plane")
			d.keys(" ")
			d.frameFits(h)
			d.english(false)
			d.keys("ctrl+n")

			d.at("5.AIPub")
			d.keys("enter", "down", "ctrl+u", "aipub.lkjh.local", "down", "enter", "down", "node1")
			d.frameFits(h)
			d.english(false)
			d.keys("ctrl+n")
			d.at("5b.Monitoring")
			d.frameFits(h)
			d.english(false)
			d.keys("ctrl+n")
			d.at("6.AIPub LB")
			d.english(true)
			d.keys("down", "ctrl+u", "lb-hjkl", "down", "192.168.0.100")
			d.fits(h, "Next")
			d.english(true)
			d.keys("ctrl+n")
			d.at("7.Certs")
			d.fits(h, "Next")
			d.english(true)
			d.keys("ctrl+n")

			d.at("8.Save")
			if h >= 30 && !strings.Contains(d.screen(), "[ Notes ]") {
				t.Errorf("the preview does not open on what it reports:\n%s", d.screen())
			}
			d.fits(h, "Save (s)")
			d.english(true)
			d.keys("ctrl+n") // refused: not saved; the reason under the screen
			d.at("8.Save")
			if len(d.app.screenErrors) != 1 || d.app.screenErrors[0] != "Save (s) before going on to the install plan" {
				t.Errorf("refused with %q", d.app.screenErrors)
			}
			d.fits(h, "Save (s)")
			d.english(true)
			d.keys("s")
			if _, err := os.Stat(paths.Vars); err != nil {
				t.Fatalf("not saved: %v", err)
			}
			d.fits(h, "Next")
			d.english(true)
			d.keys("tab", "right", "left")
			d.fits(h, "Next")
			d.keys("ctrl+n")
			d.at("9.Install")
			d.fits(h, "Run")
			d.english(true)
		})
	}
}

// busyScreen is a screen with a sub-edit open that does not say why.
type busyScreen struct{ screens.Screen }

func (busyScreen) Busy() bool { return true }

// The frame's own words: loading, the footer of each screen and while
// quitting, and the reason the wizard stays on a busy screen.
func TestFrameSaysEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)

	s := state.DefaultState()
	a := NewApp(s, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions(), "test")
	if v := a.View(); v != "Loading..." {
		t.Errorf("before the size: %q", v)
	}

	d, _ := newDriver(t)
	for i := range d.app.screenList {
		d.app.current = i
		for _, w := range []int{46, 70, 108} {
			frameNoHangul(t, "the header", d.app.renderHeader(w))
			frameNoHangul(t, "the footer", d.app.keyHints(w))
		}
	}
	d.app.current = 9 // the preview (8.Save) leads with its save key
	if h := d.app.keyHints(200); h != "  Ctrl+S save · Ctrl+N next · Ctrl+P back · ↑↓ move · Enter select · Ctrl+C quit" {
		t.Errorf("footer of the preview: %q", h)
	}
	d.app.current = 0
	d.app.quitting = true
	if h := d.app.keyHints(200); !strings.HasPrefix(h, "  Cleaning up") {
		t.Errorf("footer while quitting: %q", h)
	}
	frameNoHangul(t, "the footer while quitting", d.app.keyHints(200))
	d.app.quitting = false

	d.app.screenList[0] = busyScreen{d.app.screenList[0]}
	d.keys("ctrl+n")
	if len(d.app.screenErrors) != 1 || d.app.screenErrors[0] != "Save or cancel the item being edited before moving on" {
		t.Errorf("busy: %q", d.app.screenErrors)
	}
	frameNoHangul(t, "the errors under the screen", d.app.errorLines())
}

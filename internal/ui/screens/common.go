package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Screen is the interface every wizard step must implement.
type Screen interface {
	tea.Model
	Title() string
	SyncFromState(s *state.AppState)
	SyncToState(s *state.AppState)
	Validate() []string
	SetSize(w, h int)
}

// Busy is a screen that may have a sub-edit open (a form, an item being typed
// in): the wizard does not leave it until that is saved or cancelled.
type Busy interface {
	Busy() bool
}

// Revealer is a screen that brings what its errors name into sight when the
// wizard refuses to leave it (Validate itself changes nothing).
type Revealer interface {
	RevealErrors()
}

// short is the one-line description the schema gives a variable, for a help
// string next to the field that edits it.
func short(key string) string {
	v, _ := schema.Get(key)
	return v.Short
}

type NavigateNextMsg struct{}
type NavigatePrevMsg struct{}
type SaveMsg struct{}

func Next() tea.Cmd { return func() tea.Msg { return NavigateNextMsg{} } }
func Prev() tea.Cmd { return func() tea.Msg { return NavigatePrevMsg{} } }
func Save() tea.Cmd { return func() tea.Msg { return SaveMsg{} } }

// focusedStyle returns a style that applies the focus background/foreground.
// Using CompleteColor ensures the highlight works in TrueColor, ANSI256, and ANSI-16.
// focusedStyle uses ANSI-16 color 12 (bright blue, \033[104m) for background.
// ANSI-16 colors 0-15 work regardless of the terminal's color profile.
func focusedStyle(width int) lipgloss.Style {
	s := lipgloss.NewStyle().
		Background(lipgloss.Color("12")).
		Foreground(lipgloss.Color("15")).
		Bold(true)
	if width > 0 {
		s = s.Width(width)
	}
	return s
}

// RenderButton renders an nmtui-style < label > button.
func RenderButton(label string, focused bool) string {
	text := "< " + label + " >"
	if focused {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("12")).
			Foreground(lipgloss.Color("15")).
			Bold(true).
			Padding(0, 1).
			Render(text)
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Padding(0, 1).
		Render(text)
}

// RenderRow renders a full-width row with highlight when focused.
// content must be PLAIN (unrendered) text; pre-rendered ANSI strings break background.
func RenderRow(content string, focused bool, width int) string {
	if focused {
		return focusedStyle(width).Render(content)
	}
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	return content
}

// RenderNavButtons renders the ← 이전 → ← 다음 → bar centered in width.
// ▶ marks the focused button. ←/→ arrows switch between them within the nav area.
func RenderNavButtons(prevLabel, nextLabel string, prevFocused, nextFocused bool, width int) string {
	var prevDisplay, nextDisplay string
	switch {
	case prevFocused:
		prevDisplay = "▶ " + RenderButton(prevLabel, true)
		nextDisplay = "  " + RenderButton(nextLabel, false)
	case nextFocused:
		prevDisplay = "  " + RenderButton(prevLabel, false)
		nextDisplay = "▶ " + RenderButton(nextLabel, true)
	default:
		prevDisplay = "  " + RenderButton(prevLabel, false)
		nextDisplay = "  " + RenderButton(nextLabel, false)
	}
	buttons := prevDisplay + "   " + nextDisplay
	btnWidth := lipgloss.Width(buttons)
	pad := (width - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + buttons
}

// focusSpan finds the lines of the focused part of a view while it is written:
// mark is called before each part with whether it is the focused one, which
// then runs to the next part.
type focusSpan struct {
	b          *strings.Builder
	start, end int
	open       bool
}

func (f *focusSpan) mark(focused bool) {
	n := strings.Count(f.b.String(), "\n")
	if f.open {
		f.end, f.open = n, false
	}
	if focused {
		f.start, f.end, f.open = n, n+1, true
	}
}

// fit is the view written, cut to height by FitView around the focused part.
func (f *focusSpan) fit(width, height int) string {
	if f.open {
		f.end = strings.Count(f.b.String(), "\n") + 1
	}
	return FitView(f.b.String(), f.start, f.end, width, height)
}

// FitView cuts a view taller than height to the lines around the focused row:
// the row, its status and help (the lines from focusLine up to end) and a few
// lines under it, and says that there is more. Help taller than the room keeps
// the row at the top; a height too low for the marks has none. The focused row
// is never cut. A line wider than width counts the lines it wraps to. The last
// line of the view, the buttons, stays at the bottom whatever is focused (the
// blank line over it when there is room), unless the screen is too low for the
// focused row and the buttons both.
func FitView(view string, focusLine, end, width, height int) string {
	logical := strings.Split(view, "\n")
	// each line as wrapped at width; first[i] is where line i starts
	var lines []string
	first := make([]int, len(logical)+1)
	for i, l := range logical {
		first[i] = len(lines)
		if width > 0 {
			lines = append(lines, strings.Split(lipgloss.NewStyle().Width(width).Render(l), "\n")...)
		} else {
			lines = append(lines, l)
		}
	}
	first[len(logical)] = len(lines)
	h := height
	if h <= 0 || len(lines) <= h {
		return view
	}
	at := func(i int) int { return first[max(0, min(i, len(logical)))] }
	focusLine, end = at(focusLine), at(end)

	// the buttons stay at the bottom whatever is focused, the focused row and as
	// much of its help as fits over them; only a screen too low for both keeps
	// the row alone
	var pinned []string
	if nb := first[len(logical)-1]; h > len(lines)-nb {
		k := nb
		blank := k > 0 && strings.TrimSpace(lines[k-1]) == ""
		if blank {
			k--
		}
		blankLine := lines[k]
		pinned, lines = lines[nb:], lines[:k]
		h -= len(pinned)
		if focusLine >= k { // on the buttons: the end of the rest
			focusLine, end = k-1, k
		}
		end = min(end, k)
		// the blank line over the buttons when the focused part and its marks
		// have room with it
		if blank && end-focusLine+2 <= h-1 {
			pinned = append([]string{blankLine}, pinned...)
			h--
		}
	}

	marks := h >= 3
	room := h // the lines of the view shown whatever the marks
	if marks {
		room = h - 2
	}
	last := focusLine + min(4, h/2)
	if end-1 > last {
		last = end - 1
	}
	start := last - room + 1
	if start > focusLine {
		start = focusLine
	}
	if start > len(lines)-room {
		start = len(lines) - room
	}
	if start < 0 {
		start = 0
	}
	// a mark only where there is more; a line not taken by one shows the view
	top := marks && start > 0
	stop := start + h
	if top {
		stop--
	}
	if stop > len(lines) {
		stop = len(lines)
	}
	bottom := marks && stop < len(lines)
	if bottom {
		stop--
	}
	var win []string
	if top {
		win = append(win, styles.StyleMuted.Render("  ↑ 위에 더 있음"))
	}
	win = append(win, lines[start:stop]...)
	if bottom {
		win = append(win, styles.StyleMuted.Render("  ↓ 아래에 더 있음"))
	}
	return strings.Join(append(win, pinned...), "\n")
}

// RenderSectionHeader renders a section label, highlighted when focused.
func RenderSectionHeader(label string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().
			Foreground(styles.ColorPrimary).
			Bold(true).
			Width(28).
			Render("▶ " + label)
	}
	return styles.StyleLabel.Render("  " + label)
}

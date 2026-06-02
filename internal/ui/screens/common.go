package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

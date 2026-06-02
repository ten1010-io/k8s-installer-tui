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

// RenderButton renders an nmtui-style button.
// Focused buttons are shown with inverse color.
func RenderButton(label string, focused bool) string {
	text := "< " + label + " >"
	if focused {
		return lipgloss.NewStyle().Reverse(true).Bold(true).Padding(0, 1).Render(text)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Padding(0, 1).Render(text)
}

// RenderRow renders a list row. Focused rows use full-width inverse highlight.
func RenderRow(content string, focused bool, width int) string {
	s := lipgloss.NewStyle().Width(width)
	if focused {
		return s.Reverse(true).Render(content)
	}
	return s.Render(content)
}

// RenderNavButtons renders the < 이전 > < 다음 > (or custom label) row
// centered within the given width.
func RenderNavButtons(prevLabel, nextLabel string, prevFocused, nextFocused bool, width int) string {
	prev := RenderButton(prevLabel, prevFocused)
	next := RenderButton(nextLabel, nextFocused)
	buttons := prev + "  " + next
	btnWidth := lipgloss.Width(buttons)
	pad := (width - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + buttons
}

// RenderSectionHeader renders a section title, highlighted when focused.
func RenderSectionHeader(label string, focused bool) string {
	if focused {
		return styles.StyleLabelFocused.Render("▶ " + label)
	}
	return styles.StyleLabel.Render("  " + label)
}

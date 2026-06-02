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
// Focused buttons are shown with blue background.
func RenderButton(label string, focused bool) string {
	text := "< " + label + " >"
	if focused {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("69")).
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

// RenderRow renders a list row. Focused rows use full-width blue highlight.
func RenderRow(content string, focused bool, width int) string {
	if focused {
		s := lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("15")).
			Bold(true)
		if width > 0 {
			s = s.Width(width)
		}
		return s.Render(content)
	}
	if width > 0 {
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	return content
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
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("69")).
			Bold(true).
			Width(28).
			Render("▶ " + label)
	}
	return styles.StyleLabel.Render("  " + label)
}

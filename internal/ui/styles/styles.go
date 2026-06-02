package styles

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary = lipgloss.Color("69")  // blue
	colorSuccess = lipgloss.Color("42")  // green
	colorError   = lipgloss.Color("196") // red
	colorMuted   = lipgloss.Color("240") // gray
	ColorBorder  = lipgloss.Color("238")

	StyleHeader = lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(lipgloss.Color("15")).
			Padding(0, 2).
			Bold(true)

	StyleStep = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleStepActive = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleStepDone = lipgloss.NewStyle().
			Foreground(colorSuccess)

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	StyleLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(28)

	StyleLabelFocused = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Width(28)

	StyleError = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	StyleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	StylePrimary = lipgloss.NewStyle().Foreground(ColorPrimary)

	CheckOn  = StyleSuccess.Render("✓")
	CheckOff = StyleMuted.Render("✗")
	RadioOn  = StylePrimary.Render("●")
	RadioOff = StyleMuted.Render("○")
)

package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary  = lipgloss.Color("69")  // blue
	colorSuccess  = lipgloss.Color("42")  // green
	colorWarning  = lipgloss.Color("214") // orange
	colorError    = lipgloss.Color("196") // red
	colorMuted    = lipgloss.Color("240") // gray
	colorSelected = lipgloss.Color("212") // pink
	colorBorder   = lipgloss.Color("238")

	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			PaddingBottom(1)

	StyleHeader = lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(lipgloss.Color("15")).
			Padding(0, 2).
			Bold(true)

	StyleStep = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	StyleStepActive = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Padding(0, 1)

	StyleStepDone = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Padding(0, 1)

	StyleLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(32)

	StyleLabelFocused = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				Width(32)

	StyleError = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	StyleWarning = lipgloss.NewStyle().
			Foreground(colorWarning)

	StyleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleSelected = lipgloss.NewStyle().
			Foreground(colorSelected).
			Bold(true)

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	StyleFooter = lipgloss.NewStyle().
			Foreground(colorMuted).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	StyleTableHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorBorder)

	StyleTableRow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	StyleTableRowSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("15")).
				Bold(true)

	CheckOn  = StyleSuccess.Render("✓")
	CheckOff = StyleMuted.Render("✗")
	RadioOn  = StylePrimary.Render("●")
	RadioOff = StyleMuted.Render("○")

	StylePrimary = lipgloss.NewStyle().Foreground(colorPrimary)
)

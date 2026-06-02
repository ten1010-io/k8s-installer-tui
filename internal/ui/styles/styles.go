package styles

import "github.com/charmbracelet/lipgloss"

// FocusBg/FocusFg provide background highlighting that works across all
// terminal color profiles (TrueColor → ANSI256 → ANSI-16 fallback).
var (
	FocusBg = lipgloss.CompleteColor{TrueColor: "#0000af", ANSI256: "19", ANSI: "4"}
	FocusFg = lipgloss.CompleteColor{TrueColor: "#ffffff", ANSI256: "15", ANSI: "15"}
	// Accent used for the active column in group table
	AccentBg = lipgloss.CompleteColor{TrueColor: "#005fd7", ANSI256: "26", ANSI: "12"}

	ColorPrimary = lipgloss.Color("69")
	ColorBorder  = lipgloss.Color("238")
	colorSuccess = lipgloss.Color("42")
	colorError   = lipgloss.Color("196")
	colorMuted   = lipgloss.Color("240")

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

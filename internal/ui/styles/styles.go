package styles

import "github.com/charmbracelet/lipgloss"

// FocusBg/FocusFg provide background highlighting that works across all
// terminal color profiles (TrueColor → ANSI256 → ANSI-16 fallback).
var (
	// FocusBg: bright blue visible on any dark terminal.
	// ANSI256 "27" = #005fff, ANSI "12" = bright-blue background (\033[104m).
	FocusBg = lipgloss.CompleteColor{TrueColor: "#005fd7", ANSI256: "27", ANSI: "12"}
	FocusFg = lipgloss.CompleteColor{TrueColor: "#ffffff", ANSI256: "231", ANSI: "15"}
	// AccentBg: slightly brighter blue for the active cell within a focused row.
	AccentBg = lipgloss.CompleteColor{TrueColor: "#0087ff", ANSI256: "33", ANSI: "12"}

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

)

// CheckOn/Off and RadioOn/Off are functions (not vars) so they call Render()
// at display time, after main() has set COLORTERM — ensuring the lipgloss
// renderer is initialized with the correct color profile.
func CheckOn() string  { return StyleSuccess.Render("✓") }
func CheckOff() string { return StyleMuted.Render("✗") }
func RadioOn() string  { return StylePrimary.Render("●") }
func RadioOff() string { return StyleMuted.Render("○") }

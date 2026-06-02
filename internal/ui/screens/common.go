package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Screen is the interface every wizard step must implement.
type Screen interface {
	tea.Model
	Title() string
	// SyncFromState loads the screen's form fields from AppState.
	// Called when the user navigates to this screen.
	SyncFromState(s *state.AppState)
	// SyncToState writes the screen's form values back into AppState.
	// Called when the user successfully navigates away with Next.
	SyncToState(s *state.AppState)
	// Validate returns per-screen validation errors before SyncToState.
	Validate() []string
	// FooterHelp returns a short key-hint string shown in the footer.
	FooterHelp() string
	// SetSize is called on window resize.
	SetSize(w, h int)
}

// NavigateNextMsg signals the App to move to the next screen.
type NavigateNextMsg struct{}

// NavigatePrevMsg signals the App to move to the previous screen.
type NavigatePrevMsg struct{}

// SaveMsg signals the App to write files and exit.
type SaveMsg struct{}

func Next() tea.Cmd { return func() tea.Msg { return NavigateNextMsg{} } }
func Prev() tea.Cmd { return func() tea.Msg { return NavigatePrevMsg{} } }
func Save() tea.Cmd { return func() tea.Msg { return SaveMsg{} } }

package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

type App struct {
	appState      *state.AppState
	screenList    []screens.Screen
	current       int
	width         int
	height        int
	screenErrors  []string // validation errors from current screen's Next attempt
	inventoryPath string
	varsPath      string
}

func NewApp(s *state.AppState, inventoryPath, varsPath string) *App {
	s1 := screens.NewS1Nodes()
	s2 := screens.NewS2Groups()
	s3 := screens.NewS3Network()
	s4 := screens.NewS4Kubernetes()
	s5 := screens.NewS5Aipub()
	s6 := screens.NewS6CertMode()
	s7 := screens.NewS7Preview(s, inventoryPath, varsPath)

	// Pre-load first screen
	s1.SyncFromState(s)

	return &App{
		appState:      s,
		screenList:    []screens.Screen{s1, s2, s3, s4, s5, s6, s7},
		inventoryPath: inventoryPath,
		varsPath:      varsPath,
	}
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		for _, sc := range a.screenList {
			sc.SetSize(msg.Width, msg.Height-4)
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

	case screens.NavigateNextMsg:
		errs := a.screenList[a.current].Validate()
		if len(errs) > 0 {
			a.screenErrors = errs
			return a, nil
		}
		a.screenErrors = nil
		a.screenList[a.current].SyncToState(a.appState)
		if a.current < len(a.screenList)-1 {
			a.current++
			a.screenList[a.current].SyncFromState(a.appState)
		}
		return a, nil

	case screens.NavigatePrevMsg:
		a.screenErrors = nil
		if a.current > 0 {
			a.screenList[a.current].SyncToState(a.appState)
			a.current--
			a.screenList[a.current].SyncFromState(a.appState)
		}
		return a, nil
	}

	// Delegate to current screen
	newModel, cmd := a.screenList[a.current].Update(msg)
	a.screenList[a.current] = newModel.(screens.Screen)
	return a, cmd
}

func (a *App) View() string {
	header := a.renderHeader()
	body := a.screenList[a.current].View()
	footer := a.renderFooter()

	// Trim body to avoid overflow
	bodyLines := strings.Split(body, "\n")
	maxBodyLines := a.height - 6
	if maxBodyLines < 1 {
		maxBodyLines = 1
	}
	if len(bodyLines) > maxBodyLines {
		bodyLines = bodyLines[:maxBodyLines]
	}
	body = strings.Join(bodyLines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (a *App) renderHeader() string {
	steps := make([]string, len(a.screenList))
	titles := []string{"1.노드", "2.그룹", "3.네트워크", "4.K8s", "5.AIPub", "6.인증서", "7.저장"}
	for i := range a.screenList {
		t := titles[i]
		switch {
		case i == a.current:
			steps[i] = StyleStepActive.Render("[ " + t + " ]")
		case i < a.current:
			steps[i] = StyleStepDone.Render("✓ " + t)
		default:
			steps[i] = StyleStep.Render(t)
		}
	}
	bar := strings.Join(steps, StyleMuted.Render(" › "))
	return StyleHeader.Width(a.width).Render("k8s-installer-tui") + "\n" +
		StyleMuted.Render(bar) + "\n"
}

func (a *App) renderFooter() string {
	hint := a.screenList[a.current].FooterHelp()

	errs := ""
	if len(a.screenErrors) > 0 {
		msgs := make([]string, len(a.screenErrors))
		for i, e := range a.screenErrors {
			msgs[i] = "✗ " + e
		}
		errs = "\n" + StyleError.Render(strings.Join(msgs, "\n"))
	}

	nav := fmt.Sprintf("  %s | %s | %s",
		StyleMuted.Render("ctrl+n: 다음"),
		StyleMuted.Render("ctrl+p: 이전"),
		StyleMuted.Render("ctrl+c: 종료"),
	)

	return StyleFooter.Width(a.width).Render(hint+nav+errs)
}

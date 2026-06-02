package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

type App struct {
	appState      *state.AppState
	screenList    []screens.Screen
	current       int
	width         int
	height        int
	screenErrors  []string
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
		_, contentW, contentH := a.panelDims()
		for _, sc := range a.screenList {
			sc.SetSize(contentW, contentH)
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

	newModel, cmd := a.screenList[a.current].Update(msg)
	a.screenList[a.current] = newModel.(screens.Screen)
	return a, cmd
}

func (a *App) View() string {
	if a.width == 0 {
		return "로딩 중..."
	}

	panelW, _, _ := a.panelDims()

	// Screen content
	body := a.screenList[a.current].View()

	// Validation errors shown inside the panel
	if len(a.screenErrors) > 0 {
		msgs := make([]string, len(a.screenErrors))
		for i, e := range a.screenErrors {
			msgs[i] = "✗ " + e
		}
		body += "\n" + styles.StyleError.Render(strings.Join(msgs, "\n"))
	}

	// Bordered center panel
	innerW := panelW - 4 // 2 border + 2 padding
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorder).
		Padding(0, 1).
		Width(innerW).
		Render(body)

	// Center horizontally
	panelRenderedW := lipgloss.Width(panel)
	leftPad := (a.width - panelRenderedW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	pad := strings.Repeat(" ", leftPad)
	panelLines := strings.Split(panel, "\n")
	for i, l := range panelLines {
		panelLines[i] = pad + l
	}
	centeredPanel := strings.Join(panelLines, "\n")

	// Header
	header := a.renderHeader(panelW, leftPad)

	// Footer hint
	footer := styles.StyleMuted.Render("  ↑/↓: 이동   Enter: 선택   Ctrl+C: 종료")

	return header + "\n" + centeredPanel + "\n" + footer
}

func (a *App) renderHeader(panelW, leftPad int) string {
	pad := strings.Repeat(" ", leftPad)

	title := pad + styles.StyleHeader.Render(" k8s-installer-tui ")

	stepTitles := []string{"1.노드", "2.그룹", "3.네트워크", "4.K8s", "5.AIPub", "6.인증서", "7.저장"}
	parts := make([]string, len(a.screenList))
	for i, t := range stepTitles {
		switch {
		case i == a.current:
			parts[i] = styles.StyleStepActive.Render("[ " + t + " ]")
		case i < a.current:
			parts[i] = styles.StyleStepDone.Render("✓ " + t)
		default:
			parts[i] = styles.StyleStep.Render(t)
		}
	}
	stepBar := pad + " " + strings.Join(parts, styles.StyleMuted.Render(" › "))

	return title + "\n" + stepBar
}

// panelDims returns (panelWidth, contentWidth, contentHeight).
func (a *App) panelDims() (int, int, int) {
	panelW := a.width - 4
	if panelW > 84 {
		panelW = 84
	}
	if panelW < 44 {
		panelW = 44
	}
	contentW := panelW - 4 // border(2) + padding(2)
	contentH := a.height - 5 // header(2) + panel border(2) + footer(1)
	if contentH < 10 {
		contentH = 10
	}
	return panelW, contentW, contentH
}

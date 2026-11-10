package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

type App struct {
	appState     *state.AppState
	screenList   []screens.Screen
	current      int
	width        int
	height       int
	screenErrors []string
	heldBusy     bool // screenErrors is the message of busy: a sub-edit held the move
	version      string
}

func NewApp(s *state.AppState, paths fileio.Paths, opts plan.Options, version string) *App {
	// An existing site (its files were there before anything is saved): the
	// cluster is there, and maybe AIPub. The control plane screen offers to read
	// what its apiserver was given by hand, and the plan applies the control
	// plane in one update-cluster.yml once nothing waits for a later step
	if loaded, _ := fileio.LoadSite(paths); loaded != nil {
		opts.Existing = true
		_, opts.AipubInstalled = loaded.Values[schema.FileAipub]
	}
	s1 := screens.NewS1Nodes()
	s2 := screens.NewS2Groups()
	s3 := screens.NewS3Network()
	s4 := screens.NewS4Kubernetes()
	s4b := screens.NewS4bControlPlane()
	s4b.SetSite(opts)
	s5 := screens.NewS5Aipub()
	s5b := screens.NewS5bMonitoring()
	s6 := screens.NewS6AipubLB()
	s7 := screens.NewS6CertMode()
	s8 := screens.NewS7Preview(s, paths, opts.AnsibleDir()+"/files")
	s8.SetInstaller(opts)
	s9 := screens.NewS9Install(s, paths, opts)
	s1.SyncFromState(s)
	return &App{
		appState:   s,
		screenList: []screens.Screen{s1, s2, s3, s4, s4b, s5, s5b, s6, s7, s8, s9},
		version:    version,
	}
}

// The steps of the header, in the order of screenList.
var stepTitles = []string{"1.노드", "2.그룹", "3.네트워크", "4.K8s", "4b.컨트롤플레인", "5.AIPub", "5b.모니터링", "6.AIPub LB", "7.인증서", "8.저장", "9.설치"}

// aipubOnlyScreens are skipped when AIPub is not installed: what they ask goes to
// files/aipub/vars.yml, which is then not written.
var aipubOnlyScreens = map[int]bool{6: true, 7: true, 8: true}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.update(msg)
	a.resize()
	return m, cmd
}

// resize gives the screens the room of the panel, after any message: the
// header wraps by the step the wizard is on, and errors may be shown under the
// screen.
func (a *App) resize() {
	if a.width == 0 {
		return
	}
	_, contentW, contentH := a.dims()
	for _, sc := range a.screenList {
		sc.SetSize(contentW, contentH)
	}
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		// control keys, so no text input takes them as text: they work on every screen
		case "pgup", "ctrl+p":
			return a.goPrev()
		case "pgdown", "ctrl+n":
			return a.goNext()
		case "ctrl+s":
			// only the preview saves; elsewhere the key does nothing
			if !a.onPreview() {
				return a, nil
			}
		}

	case screens.NavigateNextMsg:
		return a.goNext()

	case screens.NavigatePrevMsg:
		return a.goPrev()
	}

	newModel, cmd := a.screenList[a.current].Update(msg)
	a.screenList[a.current] = newModel.(screens.Screen)
	a.dropSettledErrors()
	return a, cmd
}

// dropSettledErrors clears the errors a refused move left under the screen once
// the screen would let the move through, e.g. after the preview is saved.
func (a *App) dropSettledErrors() {
	if len(a.screenErrors) == 0 {
		return
	}
	sc := a.screenList[a.current]
	if b, ok := sc.(screens.Busy); ok && b.Busy() {
		return
	}
	// the sub-edit that held the move is closed: what the screen would still
	// refuse is said on the next move
	if a.heldBusy {
		a.screenErrors, a.heldBusy = nil, false
		return
	}
	if len(sc.Validate()) == 0 {
		a.screenErrors = nil
	}
}

func (a *App) onPreview() bool {
	_, ok := a.screenList[a.current].(*screens.S7Preview)
	return ok
}

// keyHints is the footer line of every screen, at most width wide: the hints in
// order of importance (the preview leads with its save key), as many as fit.
func (a *App) keyHints(width int) string {
	hints := []string{"Ctrl+N 다음", "Ctrl+P 이전", "↑↓ 이동", "Enter 선택", "Ctrl+C 종료"}
	if a.onPreview() {
		hints = append([]string{"Ctrl+S 저장"}, hints...)
	}
	for n := len(hints); n > 1; n-- {
		if line := "  " + strings.Join(hints[:n], " · "); lipgloss.Width(line) <= width {
			return line
		}
	}
	return "  " + hints[0]
}

// skipScreen returns true for screens that should be skipped in navigation.
func (a *App) skipScreen(idx int) bool {
	return aipubOnlyScreens[idx] && !a.appState.AipubEnabled
}

// busy holds the wizard on a screen with a sub-edit open, and says why.
func (a *App) busy() bool {
	if b, ok := a.screenList[a.current].(screens.Busy); ok && b.Busy() {
		a.screenErrors = []string{"입력 중인 항목을 저장하거나 취소한 뒤 이동하세요"}
		a.heldBusy = true
		return true
	}
	return false
}

func (a *App) goNext() (tea.Model, tea.Cmd) {
	if a.busy() {
		return a, nil
	}
	a.heldBusy = false
	errs := a.screenList[a.current].Validate()
	if len(errs) > 0 {
		a.screenErrors = errs
		// refused: the screen shows what the errors name
		if r, ok := a.screenList[a.current].(screens.Revealer); ok {
			r.RevealErrors()
		}
		return a, nil
	}
	a.screenErrors = nil
	a.screenList[a.current].SyncToState(a.appState)
	next := a.current + 1
	for next < len(a.screenList) && a.skipScreen(next) {
		next++
	}
	if next < len(a.screenList) {
		a.current = next
		a.screenList[a.current].SyncFromState(a.appState)
	}
	return a, nil
}

func (a *App) goPrev() (tea.Model, tea.Cmd) {
	if a.busy() {
		return a, nil
	}
	a.screenErrors, a.heldBusy = nil, false
	if a.current > 0 {
		a.screenList[a.current].SyncToState(a.appState)
		prev := a.current - 1
		for prev > 0 && a.skipScreen(prev) {
			prev--
		}
		a.current = prev
		a.screenList[a.current].SyncFromState(a.appState)
	}
	return a, nil
}

func (a *App) View() string {
	if a.width == 0 {
		return "로딩 중..."
	}

	outerBoxW, contentW, _ := a.dims()
	outerContentW := outerBoxW - 4 // border(2) + padding(2)

	// Screen body
	body := a.screenList[a.current].View() + a.errorLines()

	// Inner panel (thick blue border — the actual screen content box); its
	// Width takes in its padding
	innerPanel := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(styles.ColorPrimary).
		Padding(0, 1).
		Width(contentW + 2).
		Render(body)

	// Outer box content: top space + header + step bar + inner panel + label
	var ob strings.Builder
	ob.WriteString("\n") // breathing room above header
	// at the text width of the outer box, as the footer
	ob.WriteString(a.renderHeader(outerContentW - 2))
	ob.WriteString("\n\n")
	ob.WriteString(innerPanel)
	ob.WriteString("\n\n")
	// one line, as dims counts it: the text width of the outer box is its Width less its padding
	footerW := outerContentW - 2
	ob.WriteString(styles.StyleMuted.Copy().MaxWidth(footerW).Render(a.keyHints(footerW)))
	ob.WriteString("\n")

	// Outer box (thin rounded border — wraps everything)
	outerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("250")).
		Padding(0, 1).
		Width(outerContentW).
		Render(ob.String())

	// Center outer box horizontally
	outerRenderedW := lipgloss.Width(outerBox)
	leftMargin := (a.width - outerRenderedW) / 2
	if leftMargin < 0 {
		leftMargin = 0
	}

	// Fill terminal with dark blue background; outer box floats in the center
	darkBg := lipgloss.NewStyle().Background(lipgloss.Color("4"))
	outerLines := strings.Split(outerBox, "\n")

	result := make([]string, a.height)
	for i := range result {
		// Line 0 is empty dark-blue (top margin)
		lineIdx := i - 1
		var mid string
		if lineIdx >= 0 && lineIdx < len(outerLines) {
			mid = outerLines[lineIdx]
		}
		midW := lipgloss.Width(mid)
		leftW := leftMargin
		rightW := a.width - leftW - midW
		if rightW < 0 {
			rightW = 0
		}
		result[i] = darkBg.Width(leftW).Render("") + mid + darkBg.Width(rightW).Render("")
	}

	return strings.Join(result, "\n")
}

func (a *App) renderHeader(width int) string {
	title := styles.StyleHeader.Copy().Width(width).Render("  k8s-installer-tui " + a.version + "  ")

	parts := make([]string, len(a.screenList))
	for i, t := range stepTitles {
		switch {
		case a.skipScreen(i):
			parts[i] = styles.StyleMuted.Render("(" + t + ")")
		case i == a.current:
			parts[i] = styles.StyleStepActive.Render("[ " + t + " ]")
		case i < a.current:
			parts[i] = styles.StyleStepDone.Render("✓ " + t)
		default:
			parts[i] = styles.StyleStep.Render(t)
		}
	}
	stepBar := " " + strings.Join(parts, styles.StyleMuted.Render(" › "))
	return title + "\n" + stepBar
}

// errorLines are the errors of the screen, under it in the panel.
func (a *App) errorLines() string {
	if len(a.screenErrors) == 0 {
		return ""
	}
	msgs := make([]string, len(a.screenErrors))
	for i, e := range a.screenErrors {
		msgs[i] = "✗ " + e
	}
	return "\n" + styles.StyleError.Render(strings.Join(msgs, "\n"))
}

// dims returns (outerBoxTotalWidth, screenContentWidth, screenContentHeight).
func (a *App) dims() (int, int, int) {
	outerBoxW := a.width - 4 // 2-char dark-blue margin each side
	if outerBoxW > 114 {
		outerBoxW = 114
	}
	if outerBoxW < 52 {
		outerBoxW = 52
	}
	// outerContentW = outerBoxW - border(2) - padding(2) = outerBoxW - 4, the
	// Width of the outer box, which takes in its padding: its text is outerBoxW - 6
	// wide, the inner panel with its border. The text of the inner panel, what a
	// screen has, is that less innerBorder(2) and innerPadding(2) = outerBoxW - 10
	outerContentW := outerBoxW - 4
	contentW := outerBoxW - 10
	if contentW < 32 {
		contentW = 32
	}
	// the header as the outer box shows it: the title, and the step bar, which
	// wraps at its width
	headerH := lipgloss.Height(lipgloss.NewStyle().Padding(0, 1).Width(outerContentW).Render(a.renderHeader(outerContentW - 2)))
	errorsH := 0
	if e := a.errorLines(); e != "" {
		errorsH = lipgloss.Height(lipgloss.NewStyle().Width(contentW).Render(e))
	}
	// outer border(2) + top space(1) + header + spacing(2) + inner border(2) +
	// footer(1) + the line under it(1) + margin above the box(1)
	contentH := a.height - 10 - headerH - errorsH
	if contentH < 1 { // a screen fits what it can in the room there is
		contentH = 1
	}
	return outerBoxW, contentW, contentH
}

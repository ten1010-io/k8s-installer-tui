package screens

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The installation plan of the saved settings (internal/plan): the checks to make
// and the playbooks to run, in order. This version shows the plan, its commands
// and a script of them, and runs the checks; it runs no playbook.
//
// Focus layout:
//   0  : 아카이브 경로 (K4)
//   1  : 모니터링 설치 (AIPub only)
//   2  : the plan, when it is taller than the room it has (↑/↓ scroll; ↓ at
//        its bottom goes on to the buttons)
//   3  : nav slot (←/→: 이전/실행)
//
// Off the text input: c 사전 확인, d 명령 보기, w 스크립트 저장

const (
	s9FocusArchive = iota
	s9FocusMonitoring
	s9FocusPlan
	s9FocusNav
)

// checkDoneMsg carries the results of the checks that run ansible and take a
// while: P2, P6, and P3 on an existing site, by step id. shell is what P6 found
// for plan.Options.ShellExecutable (empty: none needed); set only when P6 ran
type checkDoneMsg struct {
	results  map[string]plan.CheckResult
	shell    string
	shellSet bool
}

type S9Install struct {
	appState *state.AppState
	paths    fileio.Paths
	opts     plan.Options
	run      plan.RunFunc

	archive  textinput.Model
	steps    []plan.Step
	checks   map[string]plan.CheckResult
	checking bool
	message  string

	showScript bool
	vp         viewport.Model
	planVP     viewport.Model // the plan, cut to the room under the inputs

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS9Install(s *state.AppState, paths fileio.Paths, opts plan.Options) *S9Install {
	a := textinput.New()
	a.Placeholder = "예: /media/usb/ki-cp-user-registry-artifacts.tgz (이미 넣었으면 비움)"
	a.CharLimit = 256
	return &S9Install{
		appState: s, paths: paths, opts: opts, run: plan.ExecRun,
		archive: a, checks: map[string]plan.CheckResult{}, vp: viewport.New(80, 20), planVP: viewport.New(80, 20),
	}
}

// SetRunner replaces how the checks run commands (tests).
func (s *S9Install) SetRunner(r plan.RunFunc) { s.run = r }

func (s *S9Install) Title() string { return "설치" }
func (s *S9Install) SetSize(w, h int) {
	s.width, s.height = w, h
	s.vp.Width = w
	s.vp.Height = max(5, h-8)
	s.layout()
}

// layout gives the plan the room left by what is above and below it, as
// rendered at the width of the screen; the whole plan when it fits.
func (s *S9Install) layout() {
	measure := lipgloss.NewStyle().Width(max(1, s.width))
	// wrapped here, so that the viewport counts the lines it shows
	content := measure.Render(s.planText())
	s.planVP.Width = s.width
	s.planVP.SetContent(content)
	room := s.height - lipgloss.Height(measure.Render(s.top())) - lipgloss.Height(measure.Render(s.bottom()))
	if n := lipgloss.Height(content); n <= room {
		s.planVP.Height = n
	} else {
		// and a line that says where in the plan it is; on a low screen as little
		// as a line of the plan, the buttons kept in sight
		s.planVP.Height = max(1, room-1)
	}
	s.planVP.SetYOffset(s.planVP.YOffset)
	// the plan fits now (a larger screen, shorter results): no stop there any more
	if s.focusIdx == s9FocusPlan && s.planFits() {
		s.focusIdx = s9FocusNav
	}
}

// planFits is whether the whole plan is in sight; if not, it is a stop of the
// focus, where ↑/↓ scroll it.
func (s *S9Install) planFits() bool {
	return s.planVP.TotalLineCount() <= s.planVP.Height
}

func (s *S9Install) SyncFromState(st *state.AppState) {
	s.archive.SetValue(s.opts.ArchivePath)
	s.focusIdx, s.navIdx = s9FocusArchive, 0
	s.showScript = false
	s.message = ""
	s.rebuild()
	s.syncFocus()
	s.planVP.GotoTop()
}

func (s *S9Install) SyncToState(_ *state.AppState) {}
func (s *S9Install) Validate() []string            { return nil }
func (s *S9Install) Init() tea.Cmd                 { return nil }

func (s *S9Install) rebuild() {
	s.opts.ArchivePath = strings.TrimSpace(s.archive.Value())
	s.opts.SiteDir = s.savedIn()
	// the plan of what was saved: whether the authentication config of JWKS went
	// out with its CA
	s.opts.JWKSCAReady = fileio.LookupCA(s.appState).PEM != ""
	s.steps = plan.Build(s.appState, s.opts)
	// wrapped here, so that the viewport counts the lines it shows and scrolls to
	// the last of them (it would wrap a long line itself and cut what overflows)
	s.vp.SetContent(lipgloss.NewStyle().Width(max(1, s.vp.Width)).Render(s.script()))
}

// savedIn is the directory the settings were saved in, the ansible directory when
// the tool runs where k8s-installer reads them.
func (s *S9Install) savedIn() string { return filepath.Dir(s.paths.Inventory) }

func (s *S9Install) script() string { return plan.Script(s.steps, s.opts, s.savedIn()) }

func (s *S9Install) syncFocus() {
	if s.focusIdx == s9FocusArchive && !s.showScript {
		s.archive.Focus()
	} else {
		s.archive.Blur()
	}
}

func (s *S9Install) next(i int) int {
	i++
	if i == s9FocusMonitoring && !s.appState.AipubEnabled {
		i++
	}
	if i == s9FocusPlan && s.planFits() {
		i++
	}
	return min(i, s9FocusNav)
}

func (s *S9Install) prev(i int) int {
	i--
	if i == s9FocusPlan && s.planFits() {
		i--
	}
	if i == s9FocusMonitoring && !s.appState.AipubEnabled {
		i--
	}
	return max(i, 0)
}

func (s *S9Install) runChecks() tea.Cmd {
	s.checks["P1"] = plan.CheckInstalled(s.opts)
	if s.appState.AipubEnabled {
		// local reads, nothing to wait for
		if plan.HelmfileChosen(s.appState) {
			s.checks["P4"] = plan.CheckAipubBin(s.opts, s.appState)
		}
		s.checks["P5"] = plan.CheckCatalog(s.opts)
	}
	s.checks["P2"] = plan.CheckResult{Lines: []string{"확인 중... (ansible all -m command -a 'sudo -n true')"}}
	s.checking = true
	if s.appState.AipubEnabled {
		s.checks["P7"] = plan.CheckResult{Lines: []string{"확인 중... (getent hosts " + strings.Join(plan.AccessHosts(s.appState), " ") + ")"}}
	}
	if s.appState.AipubEnabled {
		s.checks["P8"] = plan.CheckResult{Lines: []string{"확인 중... (kubectl get storageclass " + s.appState.AipubHaModeStorageClass + ")"}}
	}
	s.checks["P6"] = plan.CheckResult{Lines: []string{"확인 중... (ansible all -m command -a 'sudo -n /bin/sh -c true')"}}
	opts, run, st := s.opts, s.run, s.appState
	if opts.Existing {
		// read only: the apiserver manifest of the first control plane node
		s.checks["P3"] = plan.CheckResult{Lines: []string{"확인 중... (ansible -m slurp " + plan.APIServerManifest + ")"}}
	}
	return func() tea.Msg {
		r := map[string]plan.CheckResult{"P2": plan.CheckSSH(context.Background(), opts, run)}
		var shell string
		r["P6"], shell = plan.CheckBecomeShell(context.Background(), opts, run)
		if st.AipubEnabled {
			r["P7"] = plan.CheckDNS(context.Background(), opts, st, run)
			opts.ShellExecutable = shell // the kubectl of P8 is a become
			r["P8"] = plan.CheckStorageClass(context.Background(), opts, st, run)
		}
		if opts.Existing {
			// after P6: the slurp is a become, too
			opts.ShellExecutable = shell
			r["P3"] = plan.CheckLegacy(context.Background(), opts, st, run)
		}
		return checkDoneMsg{results: r, shell: shell, shellSet: true}
	}
}

func (s *S9Install) writeScript() {
	path := filepath.Join(s.savedIn(), "install-plan.sh")
	if err := os.WriteFile(path, []byte(s.script()), 0755); err != nil {
		s.message = "스크립트 저장 실패: " + err.Error()
		return
	}
	s.message = "스크립트 저장: " + path
}

func (s *S9Install) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.layout() // the plan as it is shown: whether it fits, its bottom
	switch msg := msg.(type) {
	case checkDoneMsg:
		for id, r := range msg.results {
			s.checks[id] = r
		}
		s.checking = false
		if msg.shellSet {
			s.opts.ShellExecutable = msg.shell
			s.rebuild()
		}
		return s, nil

	case tea.KeyMsg:
		typing := s.focusIdx == s9FocusArchive && !s.showScript && msg.Type == tea.KeyRunes
		if s.showScript {
			switch msg.String() {
			case "d", "esc", "q":
				s.showScript = false
				s.syncFocus()
				return s, nil
			}
			var cmd tea.Cmd
			s.vp, cmd = s.vp.Update(msg)
			return s, cmd
		}
		if !typing {
			switch msg.String() {
			case "d":
				s.rebuild()
				s.vp.GotoTop()
				s.showScript = true
				s.syncFocus()
				return s, nil
			case "w":
				s.rebuild()
				s.writeScript()
				return s, nil
			case "c":
				if !s.checking {
					return s, s.runChecks()
				}
				return s, nil
			}
		}
		switch msg.String() {
		case "up":
			if s.focusIdx == s9FocusPlan && !s.planVP.AtTop() {
				s.planVP.LineUp(1)
				return s, nil
			}
			s.focusIdx = s.prev(s.focusIdx)
			s.syncFocus()
			return s, nil
		case "down", "tab":
			if msg.String() == "down" && s.focusIdx == s9FocusPlan && !s.planVP.AtBottom() {
				s.planVP.LineDown(1)
				return s, nil
			}
			s.rebuild()
			s.focusIdx = s.next(s.focusIdx)
			s.syncFocus()
			return s, nil
		case "left":
			if s.focusIdx == s9FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right":
			if s.focusIdx == s9FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s9FocusArchive:
				if msg.String() == "enter" {
					s.rebuild()
					s.focusIdx = s.next(s.focusIdx)
					s.syncFocus()
					return s, nil
				}
			case s9FocusMonitoring:
				s.opts.Monitoring = !s.opts.Monitoring
				s.rebuild()
				return s, nil
			case s9FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				s.message = "실행은 다음 버전(R2)에서 지원합니다. 지금은 w로 스크립트를 저장하거나 d로 명령을 보고 실행하세요"
				return s, nil
			}
		}
	}
	var cmd tea.Cmd
	if s.focusIdx == s9FocusArchive && !s.showScript {
		s.archive, cmd = s.archive.Update(msg)
		s.rebuild()
	}
	return s, cmd
}

// preflightOf is the variable of the schema named key that carries a Preflight
// hint (ansible_user is in two files, only one has it).
func preflightOf(key string) (schema.Var, bool) {
	for _, v := range schema.All() {
		if v.Key == key && v.Preflight != "" {
			return v, true
		}
	}
	return schema.Var{}, false
}

func (s *S9Install) View() string {
	var b strings.Builder
	if s.showScript {
		b.WriteString(styles.StyleTitle.Render("설치 명령") + styles.StyleMuted.Render("   ↑/↓ 스크롤   d·Esc 닫기") + "\n\n")
		b.WriteString(s.vp.View() + "\n")
		return b.String()
	}

	s.layout()
	top := s.top()
	b.WriteString(top + "\n" + s.planVP.View() + "\n")
	if !s.planFits() {
		// where in the plan the viewport is
		bar := fmt.Sprintf("── 계획 %d–%d / %d줄", s.planVP.YOffset+1, min(s.planVP.YOffset+s.planVP.Height, s.planVP.TotalLineCount()),
			s.planVP.TotalLineCount())
		if s.focusIdx == s9FocusPlan {
			b.WriteString(styles.StylePrimary.Render("▶ "+bar+" ── ↑/↓ 스크롤, 끝에서 ↓ 버튼으로") + "\n")
		} else {
			b.WriteString(styles.StyleMuted.Render("  "+bar+" ── Tab·↓ 로 와서 ↑/↓ 스크롤") + "\n")
		}
	}
	b.WriteString(s.bottom())

	// a screen too low even for a line of the plan: the buttons stay, the
	// focused part in sight over them. top ends on a blank line after the inputs
	view := b.String()
	n := strings.Count(top, "\n")
	start, end := n-1, n // 모니터링
	switch s.focusIdx {
	case s9FocusArchive:
		start, end = n-1, n
		if s.appState.AipubEnabled {
			start, end = n-2, n-1
		}
	case s9FocusPlan:
		start, end = n+1, n+2+s.planVP.Height // the plan and the line under it
	case s9FocusNav:
		start = strings.Count(view, "\n")
		end = start + 1
	}
	return FitView(view, start, end, s.width, s.height)
}

// top is what is above the plan: the title, where the settings were saved and
// the inputs.
func (s *S9Install) top() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("설치 계획") + "\n")
	b.WriteString(styles.StyleMuted.Render("저장한 설정으로 실행할 순서입니다. 이 버전은 계획과 명령만 보여 주고, 실행은 하지 않습니다.") + "\n\n")

	if copies := plan.CopyCommands(s.opts, s.savedIn()); len(copies) > 0 {
		b.WriteString(styles.StyleWarning.Render("! 설정을 "+s.savedIn()+" 에 저장했습니다. 설치 전에 "+
			s.opts.AnsibleDir()+" 로 복사해야 합니다 (스크립트에 포함)") + "\n\n")
	}

	f := s.focusIdx
	b.WriteString(RenderSectionHeader("아카이브 경로", f == s9FocusArchive) + "  " + s.archive.View() + "\n")
	if s.appState.AipubEnabled {
		mon := styles.RadioOff() + " 설치 안 함"
		if s.opts.Monitoring {
			mon = styles.RadioOn() + " 설치"
		}
		b.WriteString(RenderSectionHeader("모니터링", f == s9FocusMonitoring) + "  " + mon + "\n")
	}
	return b.String() // its last line is blank
}

// planText is the plan: the checks and the steps.
func (s *S9Install) planText() string {
	var b strings.Builder
	b.WriteString(styles.StyleMuted.Render("  ✓ 통과   ! 경고 (계속 가능)   ✗ 실패   - 건너뜀   · 아직 안 함") + "\n\n")
	started, inChecks := false, false
	for _, st := range s.steps {
		// the checks first and the steps that run after them, each under a heading
		if isCheck := st.Kind == plan.Check; !started || isCheck != inChecks {
			if isCheck {
				b.WriteString(styles.StyleTitle.Render("사전 확인") + styles.StyleMuted.Render("  (읽기만 합니다. c 로 실행)") + "\n")
			} else {
				b.WriteString("\n" + styles.StyleTitle.Render("실행 순서") + "\n")
			}
			started, inChecks = true, isCheck
		}
		icon := "·"
		res, checked := s.checks[st.ID]
		switch {
		case st.Skip:
			icon = "-"
		case checked && res.Skipped:
			icon = "-"
		case checked && res.OK && res.Warn:
			icon = "!"
		case checked && res.OK:
			icon = "✓"
		case checked && !res.OK && !((st.ID == "P2" || st.ID == "P3" || st.ID == "P6" || st.ID == "P7" || st.ID == "P8") && s.checking):
			icon = "✗"
		}
		line := fmt.Sprintf("  %s %-3s %-30s", icon, st.ID, st.Title)
		switch {
		case st.Skip:
			b.WriteString(styles.StyleMuted.Render(line+"건너뜀: "+st.SkipWhy) + "\n")
		case icon == "✗":
			b.WriteString(styles.StyleError.Render(line) + "\n")
		case icon == "!":
			b.WriteString(styles.StyleWarning.Render(line) + "\n")
		case icon == "✓":
			b.WriteString(styles.StyleSuccess.Render(line) + "\n")
		default:
			b.WriteString(line + "\n")
		}
		switch {
		case st.Skip:
		case st.Kind == plan.Check && st.Sees != "":
			b.WriteString(styles.StyleMuted.Render("        보는 것: "+st.Sees) + "\n")
			if v, ok := preflightOf(st.Var); ok {
				b.WriteString(styles.StyleMuted.Render("        힌트 ("+v.Key+"): "+v.Preflight) + "\n")
			}
		default:
			b.WriteString(styles.StyleMuted.Render("        "+st.Note) + "\n")
		}
		if checked {
			// what it found, in the colour of the verdict
			style := lipgloss.NewStyle()
			switch icon {
			case "✗":
				style = styles.StyleError
			case "!":
				style = styles.StyleWarning
			case "-":
				style = styles.StyleMuted
			}
			for i, l := range res.Lines {
				if i == 0 && res.Skipped && !strings.HasPrefix(l, "건너뜀") {
					l = "건너뜀: " + l
				}
				b.WriteString(style.Render("        "+l) + "\n")
			}
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// bottom is what is under the plan: a message, the keys and the buttons.
func (s *S9Install) bottom() string {
	var b strings.Builder
	if s.message != "" {
		b.WriteString("\n" + styles.StylePrimary.Render(s.message) + "\n")
	}
	b.WriteString("\n" + styles.StyleMuted.Render("  c 사전 확인   d 명령 보기   w 스크립트 저장 (install-plan.sh)   — 입력 칸 밖에서") + "\n")

	prevFocused := s.focusIdx == s9FocusNav && s.navIdx == 0
	runFocused := s.focusIdx == s9FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "실행 (준비 중)", prevFocused, runFocused, s.width))
	return b.String()
}

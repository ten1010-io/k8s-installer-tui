package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// Focus layout:
//   0  : content (viewport — ↑/↓ scroll, ←/→ 탭 전환)
//   1  : nav slot (←/→: 이전/저장, 이전/다음 once saved; reached by Tab or ↓ at the bottom)
//
// s and Ctrl+S save from anywhere.

const (
	s7FocusContent = iota
	s7FocusNav
	s7FocusMax = s7FocusNav
)

// previewTab is one file of the preview. A file AIPub does not need is not shown.
// The last tab is the site-values view, which has no path: it is not saved.
// The first tab is 확인할 점 when there is anything to report; no path either.
// kept says why a file with a path is not written (it is kept as it is).
type previewTab struct {
	name    string
	path    string
	content string
	kept    string
}

// notesTab is the name of the tab of what the preview reports: validation
// errors, warnings and notes, and the files once saved.
const notesTab = "확인할 점"

// note is one line of the 확인할 점 tab, wrapped to the width of the viewport
// in its style.
type note struct {
	style lipgloss.Style
	text  string
}

type S7Preview struct {
	tabs      []previewTab
	vp        viewport.Model
	activeTab int
	notes     []note // the 확인할 점 tab, the first; none: no such tab
	notesW    int    // the width the notes tab was wrapped at; -1: to wrap again
	noTitle   bool   // the screen is too low for the title as well (layout)
	errors    []validator.Error
	warnings  []schema.Change     // values an existing cluster can not take, or takes by rebuilding nodes
	loaded    *schema.Loaded      // what the site's files held when the wizard started; nil for a new site
	missing   []state.CpExtraFile // files of k8s_cp_extra_files not under ansible/files
	caNote    []string            // the authentication config of JWKS waiting for its CA
	binNote   []string            // helmfile chosen and the package carries no helmfile/kustomize
	vipNote   []string            // validator.Warnings: worth a look, saving does not wait on it
	catalog   string              // one line on the catalog of the package, when it has one
	catalogOK bool
	installer plan.Options // where the installer is, for what the preview reads of the package
	// installerFiles is ansible/files of the installer (setup.sh), where
	// k8s_cp_extra_files are read from when the settings are saved elsewhere
	installerFiles string
	saved          bool
	saveErr        string
	appState       *state.AppState
	paths          fileio.Paths

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS7Preview(s *state.AppState, paths fileio.Paths, installerFiles string) *S7Preview {
	// Read now, before anything is saved: the warnings compare against the site
	// as it was. A file that can not be read leaves nothing to compare against
	loaded, _ := fileio.LoadSite(paths)
	return &S7Preview{
		vp:       viewport.New(80, 20),
		appState: s,
		paths:    paths,
		loaded:   loaded,

		installerFiles: installerFiles,
	}
}

// SetInstaller tells the preview where the installer is (its package record is
// read for the warnings about it).
func (s *S7Preview) SetInstaller(o plan.Options) { s.installer = o }

func (s *S7Preview) Title() string { return "미리보기 & 저장" }
func (s *S7Preview) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.layout()
}

// layout fits the viewport to the room left by what is above and below it, as
// rendered at the width of the screen (a line that wraps counts as two), and
// wraps the 확인할 점 tab to that width.
func (s *S7Preview) layout() {
	s.vp.Width = s.width
	if len(s.notes) > 0 && s.notesW != s.width {
		s.notesW = s.width
		s.tabs[0].content = s.notesText()
		if s.activeTab == 0 {
			s.vp.SetContent(s.tabs[0].content)
		}
	}
	measure := lipgloss.NewStyle().Width(s.width)
	room := func() int {
		return s.height - lipgloss.Height(measure.Render(s.above())) - lipgloss.Height(measure.Render(s.below())) - 2 // the lines of the scroll bar
	}
	// on a low screen the title goes first (the step bar names the screen)
	s.noTitle = false
	vpH := room()
	if vpH < 3 {
		s.noTitle = true
		vpH = room()
	}
	if vpH < 1 {
		vpH = 1 // lower still: View cuts the rest around it
	}
	s.vp.Height = vpH
	s.vp.SetYOffset(s.vp.YOffset) // within the content at the new height
}

func (s *S7Preview) notesText() string {
	var lines []string
	for _, n := range s.notes {
		for _, l := range wrapNote(n.text, s.width) {
			lines = append(lines, n.style.Render(l))
		}
	}
	return strings.Join(lines, "\n")
}

// wrapNote breaks a note into lines at most width wide at its spaces, the lines
// after the first indented under the text after its indent and mark ("  • ",
// "⚠ "); a word wider than a line is cut.
func wrapNote(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	rest := strings.TrimLeft(text, " ")
	head := text[:len(text)-len(rest)]
	for _, p := range []string{"• ", "⚠ ", "✓ ", "✗ "} {
		if strings.HasPrefix(rest, p) {
			head += p
			break
		}
	}
	pad := strings.Repeat(" ", lipgloss.Width(head))
	var lines []string
	line, empty := head, true
	// split at each space, so that a run of spaces in the note is kept (as empty
	// words) except where the line breaks
	for _, word := range strings.Split(strings.TrimPrefix(text, head), " ") {
		if empty && word == "" {
			continue
		}
		if !empty && lipgloss.Width(line+" "+word) > width {
			lines = append(lines, strings.TrimRight(line, " "))
			line, empty = pad, true
			if word == "" {
				continue
			}
		}
		if !empty {
			line += " "
		}
		line, empty = line+word, false
		for width >= len(pad)+2 && lipgloss.Width(line) > width { // room for a wide rune after the indent
			cut, w := 0, 0
			for i, r := range line {
				if w += lipgloss.Width(string(r)); w > width {
					cut = i
					break
				}
			}
			lines = append(lines, line[:cut])
			line = pad + line[cut:]
		}
	}
	return append(lines, line)
}

// buildNotes is what the preview reports, for the 확인할 점 tab.
func (s *S7Preview) buildNotes(st *state.AppState) {
	s.notes = nil
	add := func(style lipgloss.Style, text string) { s.notes = append(s.notes, note{style, text}) }
	if len(s.errors) > 0 {
		add(styles.StyleError, fmt.Sprintf("✗ 검증 오류 %d건:", len(s.errors)))
		for _, e := range s.errors {
			add(styles.StyleError, "  • "+e.Error())
		}
	}

	// not blocking: the operator may mean it, but update-cluster.yml will refuse
	// or rebuild nodes for it
	if len(s.warnings) > 0 {
		add(styles.StyleWarning, fmt.Sprintf("⚠ 기존 클러스터에서 바꾸기 어려운 값 %d건:", len(s.warnings)))
		for _, w := range s.warnings {
			add(styles.StyleWarning, "  • "+w.Message())
		}
	}

	// not blocking either: the site puts these files there, maybe after saving
	if len(s.missing) > 0 {
		add(styles.StyleWarning, fmt.Sprintf("⚠ 컨트롤플레인 파일 %d건이 ansible/files 에 없습니다 (setup-cluster 전에 두세요):", len(s.missing)))
		for _, f := range s.missing {
			add(styles.StyleWarning, "  • "+s.installerFiles+"/"+f.Src+schema.FileSource(&st.ControlPlane, f))
		}
	}

	for _, n := range s.caNote {
		add(styles.StyleWarning, "⚠ "+n)
	}
	for _, n := range s.vipNote {
		add(styles.StyleWarning, "⚠ "+n)
	}
	for _, n := range s.binNote {
		add(styles.StyleWarning, "⚠ "+n)
	}
	if s.catalog != "" {
		if s.catalogOK {
			add(styles.StyleMuted, "✓ "+s.catalog)
		} else {
			add(styles.StyleWarning, "⚠ "+s.catalog)
		}
	}
}

// warningCount is the number of warnings and notes of the 확인할 점 tab (the
// errors are counted apart).
func (s *S7Preview) warningCount() int {
	n := len(s.warnings) + len(s.missing) + len(s.caNote) + len(s.vipNote) + len(s.binNote)
	if s.catalog != "" && !s.catalogOK {
		n++
	}
	return n
}

// notesFirst puts the 확인할 점 tab first, adding it when it is not there yet,
// and returns whether it added it.
func (s *S7Preview) notesFirst() bool {
	s.notesW = -1 // wrapped again by layout
	if len(s.notes) == 0 || (len(s.tabs) > 0 && s.tabs[0].name == notesTab) {
		return false
	}
	s.tabs = append([]previewTab{{name: notesTab}}, s.tabs...)
	return true
}

func (s *S7Preview) SyncFromState(st *state.AppState) {
	s.errors = validator.Validate(st)
	s.warnings = schema.FixedChanges(s.loaded, st)
	// which audit policy is used, for the warnings and the site-values view
	st.ControlPlane.AuditNote = fileio.PlanAuditPolicy(s.paths.AuditPolicy(), st).Note
	s.missing = fileio.MissingCpFiles(st, s.paths.FilesDir(), s.installerFiles)
	s.saved = false
	s.saveErr = ""

	s.vipNote = validator.Warnings(st)
	s.binNote = nil
	if r := plan.CheckAipubBin(s.installer, st); r.Warn {
		s.binNote = r.Lines
	}
	s.catalog = ""
	if st.AipubEnabled {
		if line, ok, found := plan.CatalogSummary(s.installer); found {
			s.catalog, s.catalogOK = line, ok
		}
	}

	s.tabs = nil
	r, err := fileio.Render(s.paths, st)
	if err != nil {
		s.tabs = []previewTab{{name: "오류", content: fmt.Sprintf("렌더링 오류: %v", err)}}
	} else {
		s.caNote = nil
		if r.JWKSPending(st) {
			s.caNote = append(s.caNote, "AIPub 인증 (JWKS): CA 대기 ("+r.CA.Path+" 없음). "+state.JWKSConfigFile+" 는 쓰지 않고, "+
				"설치 계획의 A5.1 (--fill-auth-config) 이 setup-cert 뒤에 만듭니다",
				"그 전에 update-cluster/노드 추가를 돌리면 설치기가 '소스 파일 없음'으로 거부합니다 (노드는 그대로)")
		}
		if r.CA.Err != nil && st.AppliedControlPlane().JWKS {
			s.caNote = append(s.caNote, "JWKS CA 파일: "+r.CA.Err.Error())
		}
		s.tabs = []previewTab{
			{name: "inventory.yml", path: s.paths.Inventory, content: string(r.Inventory)},
			{name: "vars.yml", path: s.paths.Vars, content: string(r.Vars)},
		}
		if r.AipubVars != nil {
			s.tabs = append(s.tabs, previewTab{name: "aipub/vars.yml", path: s.paths.AipubVars, content: string(r.AipubVars)})
		}
		// the audit policy: the default written, the file kept, or why none is
		switch a := r.Audit; {
		case a.Source == fileio.AuditDefault:
			s.tabs = append(s.tabs, previewTab{name: state.AuditPolicyFile, path: s.paths.AuditPolicy(), content: string(a.Data)})
		case a.Warn():
			s.tabs = append(s.tabs, previewTab{name: state.AuditPolicyFile, content: "# " + a.Note + "\n"})
		case a.Source != fileio.AuditOff:
			s.tabs = append(s.tabs, previewTab{name: state.AuditPolicyFile, path: s.paths.AuditPolicy(), content: string(a.Data), kept: a.Note})
		}
		// all of the above in one annotated view; not a file that is saved
		s.tabs = append(s.tabs, previewTab{name: schema.SiteValuesFile, content: string(schema.SiteValues(st))})
	}
	s.buildNotes(st)
	s.notesFirst()

	// on 확인할 점 when there is anything to report: the first thing seen
	s.activeTab = 0
	s.focusIdx = 0
	s.navIdx = 0
	s.refreshViewport()
}

func (s *S7Preview) SyncToState(_ *state.AppState) {}

func (s *S7Preview) Validate() []string {
	errs := validator.Validate(s.appState)
	if len(errs) == 0 {
		if !s.saved {
			return []string{"저장(s)한 뒤에 설치 계획으로 넘어갈 수 있습니다"}
		}
		return nil
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return msgs
}

func (s *S7Preview) Init() tea.Cmd { return nil }

func (s *S7Preview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.layout() // ↓ at the bottom of the viewport as it is shown
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx == s7FocusContent {
				s.vp.LineUp(1)
			} else {
				s.focusIdx = s7FocusContent
			}
			return s, nil
		case "down", "j":
			if s.focusIdx == s7FocusContent {
				if s.vp.AtBottom() {
					s.focusIdx = s7FocusNav
					s.navIdx = 1 // 저장, then 다음
				} else {
					s.vp.LineDown(1)
				}
			}
			return s, nil
		case "left", "h":
			switch s.focusIdx {
			case s7FocusContent:
				if s.activeTab > 0 {
					s.activeTab--
					s.refreshViewport()
				}
			case s7FocusNav:
				if s.navIdx > 0 {
					s.navIdx--
				}
			}
		case "right", "l":
			switch s.focusIdx {
			case s7FocusContent:
				if s.activeTab < len(s.tabs)-1 {
					s.activeTab++
					s.refreshViewport()
				}
			case s7FocusNav:
				if s.navIdx < 1 {
					s.navIdx++
				}
			}
		case "tab", "shift+tab":
			// files and their content ↔ the buttons, which open on 저장 (다음 once saved)
			if s.focusIdx == s7FocusNav {
				s.focusIdx = s7FocusContent
			} else {
				s.focusIdx = s7FocusNav
				s.navIdx = 1
			}
			return s, nil
		case "enter", " ":
			if s.focusIdx == s7FocusNav && s.navIdx == 0 {
				return s, Prev()
			}
			if s.saved {
				return s, Next()
			}
			if s.focusIdx == s7FocusNav {
				return s.doSave()
			}
		case "s", "ctrl+s":
			return s.doSave()
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s *S7Preview) doSave() (tea.Model, tea.Cmd) {
	// saved already: writing again would put the files just saved into .bak,
	// over the ones that were there before
	if s.saved {
		return s, nil
	}
	if len(s.errors) > 0 {
		s.saveErr = "검증 오류가 있어 저장할 수 없습니다"
		return s, nil
	}
	if err := fileio.WriteAll(s.paths, s.appState); err != nil {
		s.saveErr = fmt.Sprintf("저장 실패: %v", err)
		return s, nil
	}
	s.saved = true
	s.saveErr = ""
	// on the button that now reads 다음: Enter goes on to the plan
	s.focusIdx, s.navIdx = s7FocusNav, 1

	// the files written, in 확인할 점 (added if it was not there: the tab shown stays)
	if len(s.notes) > 0 {
		s.notes = append(s.notes, note{styles.StyleMuted, ""})
	}
	s.notes = append(s.notes, note{styles.StyleSuccess, "✓ 저장한 파일 (기존 파일은 .bak 으로 백업):"})
	for _, t := range s.tabs {
		if t.path != "" && t.kept == "" {
			s.notes = append(s.notes, note{styles.StyleMuted, "  " + t.path})
		}
	}
	for _, t := range s.tabs {
		if t.kept != "" {
			s.notes = append(s.notes, note{styles.StyleMuted, "  " + t.path + " — 쓰지 않음: " + t.kept})
		}
	}
	if s.notesFirst() {
		s.activeTab++
	}
	s.layout()
	return s, nil
}

func (s *S7Preview) refreshViewport() {
	s.layout() // the 확인할 점 tab wrapped
	if s.activeTab < len(s.tabs) {
		s.vp.SetContent(s.tabs[s.activeTab].content)
	}
	s.vp.GotoTop()
}

func (s *S7Preview) scrollBar() string {
	hasAbove := s.vp.YOffset > 0
	hasBelow := !s.vp.AtBottom()

	lineLen := s.width
	if lineLen < 4 {
		lineLen = 4
	}

	topLine := strings.Repeat("─", lineLen)
	botLine := strings.Repeat("─", lineLen)

	if hasAbove {
		indicator := " ↑ "
		topLine = indicator + strings.Repeat("─", lineLen-len(indicator))
	}
	if hasBelow {
		indicator := " ↓ "
		botLine = strings.Repeat("─", lineLen-len(indicator)) + indicator
	}

	return topLine + "\n" + s.vp.View() + "\n" + botLine
}

func (s *S7Preview) View() string {
	s.layout()
	above := s.above()
	view := above + "\n" + s.scrollBar() + "\n" + s.below()
	// a screen too low even for the smallest viewport: the buttons stay, and the
	// viewport (or the buttons) in sight over them
	start := strings.Count(above, "\n") + 1
	end := start + s.vp.Height + 2
	if s.focusIdx == s7FocusNav {
		start = strings.Count(view, "\n")
		end = start + 1
	}
	return FitView(view, start, end, s.width, s.height)
}

// above is what is above the viewport: the title and the tabs.
func (s *S7Preview) above() string {
	// Tabs — ←/→ 전환
	contentFocused := s.focusIdx == s7FocusContent
	parts := make([]string, len(s.tabs))
	for i, t := range s.tabs {
		if i == s.activeTab {
			parts[i] = styles.StylePrimary.Render("[ " + t.name + " ]")
		} else {
			parts[i] = styles.StyleMuted.Render("  " + t.name + "  ")
		}
	}
	tabLine := strings.Join(parts, " ")
	if contentFocused && len(s.tabs) > 1 {
		tabLine += styles.StyleMuted.Render("   ←/→ 탭 전환")
	}
	if s.noTitle {
		return tabLine
	}
	return styles.StyleTitle.Render("미리보기 & 저장") + "\n\n" + tabLine
}

// below is what is under the viewport, a few lines whatever there is to
// report (that is in the 확인할 점 tab), and the buttons.
func (s *S7Preview) below() string {
	var lines []string
	if len(s.errors) == 0 {
		lines = append(lines, styles.StyleSuccess.Render("✓ 검증 통과 — 저장 가능합니다"))
	} else {
		lines = append(lines, styles.StyleError.Render(fmt.Sprintf("✗ 검증 오류 %d건 — '%s' 탭", len(s.errors), notesTab)))
	}
	if n := s.warningCount(); n > 0 {
		lines = append(lines, styles.StyleWarning.Render(fmt.Sprintf("⚠ 확인할 점 %d건 — '%s' 탭", n, notesTab)))
	}
	if s.saveErr != "" {
		lines = append(lines, styles.StyleError.Render("✗ "+s.saveErr))
	}
	if s.saved {
		lines = append(lines, styles.StyleSuccess.Render("✓ 저장 완료 — 기존 파일은 .bak 으로 백업")+
			styles.StyleMuted.Render(" · 저장한 파일은 '"+notesTab+"' 탭"))
	}

	prevFocused := s.focusIdx == s7FocusNav && s.navIdx == 0
	saveFocused := s.focusIdx == s7FocusNav && s.navIdx == 1
	right := "저장 (s)"
	if s.saved {
		right = "다음"
	}
	return strings.Join(lines, "\n") + "\n\n" + RenderNavButtons("이전", right, prevFocused, saveFocused, s.width)
}

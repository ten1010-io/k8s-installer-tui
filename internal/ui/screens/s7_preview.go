package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// Focus layout:
//   0  : content (viewport — ↑/↓ scroll, ←/→ 탭 전환)
//   1  : nav slot (←/→: 이전/저장)

const (
	s7FocusContent = iota
	s7FocusNav
	s7FocusMax = s7FocusNav
)

// previewTab is one file of the preview. A file AIPub does not need is not shown.
// The last tab is the site-values view, which has no path: it is not saved.
type previewTab struct {
	name    string
	path    string
	content string
}

type S7Preview struct {
	tabs      []previewTab
	vp        viewport.Model
	activeTab int
	errors    []validator.Error
	warnings  []schema.Change // values an existing cluster can not take, or takes by rebuilding nodes
	loaded    *schema.Loaded  // what the site's files held when the wizard started; nil for a new site
	saved     bool
	saveErr   string
	appState  *state.AppState
	paths     fileio.Paths

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS7Preview(s *state.AppState, paths fileio.Paths) *S7Preview {
	// Read now, before anything is saved: the warnings compare against the site
	// as it was. A file that can not be read leaves nothing to compare against
	loaded, _ := fileio.LoadSite(paths)
	return &S7Preview{
		vp:       viewport.New(80, 20),
		appState: s,
		paths:    paths,
		loaded:   loaded,
	}
}

func (s *S7Preview) Title() string { return "미리보기 & 저장" }
func (s *S7Preview) SetSize(w, h int) {
	s.width = w
	s.height = h
	vpH := h - 14
	if vpH < 5 {
		vpH = 5
	}
	s.vp.Width = w
	s.vp.Height = vpH
}

func (s *S7Preview) SyncFromState(st *state.AppState) {
	s.errors = validator.Validate(st)
	s.warnings = schema.FixedChanges(s.loaded, st)
	s.saved = false
	s.saveErr = ""

	s.tabs = nil
	r, err := fileio.Render(s.paths, st)
	if err != nil {
		s.tabs = []previewTab{{name: "오류", content: fmt.Sprintf("렌더링 오류: %v", err)}}
	} else {
		s.tabs = []previewTab{
			{"inventory.yml", s.paths.Inventory, string(r.Inventory)},
			{"vars.yml", s.paths.Vars, string(r.Vars)},
		}
		if r.AipubVars != nil {
			s.tabs = append(s.tabs, previewTab{"aipub/vars.yml", s.paths.AipubVars, string(r.AipubVars)})
		}
		// all of the above in one annotated view; not a file that is saved
		s.tabs = append(s.tabs, previewTab{schema.SiteValuesFile, "", string(schema.SiteValues(st))})
	}

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
		case "tab":
			s.focusIdx = s7FocusNav
		case "enter", " ":
			if s.saved {
				return s, Next()
			}
			if s.focusIdx == s7FocusNav {
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s.doSave()
			}
		case "s":
			return s.doSave()
		case "ctrl+n":
			// the app checks Validate, which asks for the save first
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s *S7Preview) doSave() (tea.Model, tea.Cmd) {
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
	return s, nil
}

func (s *S7Preview) refreshViewport() {
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
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("미리보기 & 저장") + "\n\n")

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
		tabLine += styles.StyleMuted.Render("   ←/→ 파일 전환")
	}
	b.WriteString(tabLine + "\n")

	b.WriteString(s.scrollBar() + "\n")

	if len(s.errors) == 0 {
		b.WriteString(styles.StyleSuccess.Render("✓ 검증 통과 — 저장 가능합니다") + "\n")
	} else {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("✗ 검증 오류 %d건:", len(s.errors))) + "\n")
		for _, e := range s.errors {
			b.WriteString(styles.StyleError.Render("  • "+e.Error()) + "\n")
		}
	}

	// not blocking: the operator may mean it, but update-cluster.yml will refuse
	// or rebuild nodes for it
	if len(s.warnings) > 0 {
		b.WriteString(styles.StyleWarning.Render(fmt.Sprintf("⚠ 기존 클러스터에서 바꾸기 어려운 값 %d건:", len(s.warnings))) + "\n")
		for _, w := range s.warnings {
			b.WriteString(styles.StyleWarning.Render("  • "+w.Message()) + "\n")
		}
	}

	if s.saveErr != "" {
		b.WriteString(styles.StyleError.Render("✗ "+s.saveErr) + "\n")
	}
	if s.saved {
		b.WriteString("\n" + styles.StyleSuccess.Render("✓ 저장 완료!") + "\n")
		for _, t := range s.tabs {
			if t.path != "" {
				b.WriteString(styles.StyleMuted.Render("  "+t.path) + "\n")
			}
		}
		b.WriteString(styles.StyleMuted.Render("  기존 파일은 .bak으로 백업되었습니다") + "\n")
		b.WriteString("\n" + styles.StyleTitle.Render("설치 계획을 보시겠습니까?") + "  " +
			styles.StyleMuted.Render("Enter: 9.설치로   Ctrl+C: 종료") + "\n")
	}

	prevFocused := s.focusIdx == s7FocusNav && s.navIdx == 0
	saveFocused := s.focusIdx == s7FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "저장 (s)", prevFocused, saveFocused, s.width))

	return b.String()
}

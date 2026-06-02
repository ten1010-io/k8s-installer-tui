package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// Focus layout:
//   0  : inventory.yml tab
//   1  : vars.yml tab
//   2  : viewport (scrollable)
//   3  : nav slot (←/→: 이전/저장)

const (
	s7FocusTabInv = iota
	s7FocusTabVars
	s7FocusViewport
	s7FocusNav
	s7FocusMax = s7FocusNav
)

type S7Preview struct {
	inventoryContent string
	varsContent      string
	vp               viewport.Model
	errors           []validator.Error
	saved            bool
	saveErr          string
	appState         *state.AppState
	inventoryPath    string
	varsPath         string

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS7Preview(s *state.AppState, inventoryPath, varsPath string) *S7Preview {
	return &S7Preview{
		vp:            viewport.New(80, 20),
		appState:      s,
		inventoryPath: inventoryPath,
		varsPath:      varsPath,
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

func (s *S7Preview) activeTab() int {
	if s.focusIdx == s7FocusTabVars {
		return 1
	}
	return 0
}

func (s *S7Preview) SyncFromState(st *state.AppState) {
	s.errors = validator.Validate(st)
	s.saved = false
	s.saveErr = ""

	inv, err := fileio.RenderInventoryString(st)
	if err != nil {
		s.inventoryContent = fmt.Sprintf("렌더링 오류: %v", err)
	} else {
		s.inventoryContent = inv
	}

	vars, err := fileio.RenderVarsString(st)
	if err != nil {
		s.varsContent = fmt.Sprintf("렌더링 오류: %v", err)
	} else {
		s.varsContent = vars
	}

	s.focusIdx = 0
	s.navIdx = 0
	s.refreshViewport()
}

func (s *S7Preview) SyncToState(_ *state.AppState) {}

func (s *S7Preview) Validate() []string {
	errs := validator.Validate(s.appState)
	if len(errs) == 0 {
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
			if s.focusIdx == s7FocusViewport {
				if s.vp.YOffset == 0 {
					// 뷰포트 맨 위 → 탭으로 포커스 이동
					s.focusIdx--
					s.refreshViewport()
				} else {
					s.vp.LineUp(1)
				}
				return s, nil
			}
			if s.focusIdx > 0 {
				s.focusIdx--
				s.refreshViewport()
			}
		case "down", "j":
			if s.focusIdx == s7FocusViewport {
				if s.vp.AtBottom() {
					// 뷰포트 맨 아래 → nav로 포커스 이동
					s.focusIdx = s7FocusNav
				} else {
					s.vp.LineDown(1)
				}
				return s, nil
			}
			if s.focusIdx < s7FocusMax {
				s.focusIdx++
				s.refreshViewport()
			}
		case "tab":
			// 뷰포트 스크롤 중에도 Tab으로 바로 nav 이동
			s.focusIdx = s7FocusNav
		case "left", "h":
			if s.focusIdx == s7FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s7FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s7FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s.doSave()
			case s7FocusTabInv:
				s.refreshViewport()
			case s7FocusTabVars:
				s.refreshViewport()
			}
		case "s":
			return s.doSave()
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
	if err := fileio.WriteInventory(s.inventoryPath, s.appState); err != nil {
		s.saveErr = fmt.Sprintf("inventory.yml 저장 실패: %v", err)
		return s, nil
	}
	if err := fileio.WriteVars(s.varsPath, s.appState); err != nil {
		s.saveErr = fmt.Sprintf("vars.yml 저장 실패: %v", err)
		return s, nil
	}
	s.saved = true
	s.saveErr = ""
	return s, nil
}

func (s *S7Preview) refreshViewport() {
	if s.activeTab() == 0 {
		s.vp.SetContent(s.inventoryContent)
	} else {
		s.vp.SetContent(s.varsContent)
	}
	s.vp.GotoTop()
}

func (s *S7Preview) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("미리보기 & 저장") + "\n\n")

	// Tabs
	var tab0, tab1 string
	switch {
	case s.focusIdx == s7FocusTabInv:
		tab0 = RenderButton("inventory.yml", true)
	case s.activeTab() == 0:
		tab0 = styles.StylePrimary.Render("[ inventory.yml ]")
	default:
		tab0 = styles.StyleMuted.Render("  inventory.yml  ")
	}
	switch {
	case s.focusIdx == s7FocusTabVars:
		tab1 = RenderButton("vars.yml", true)
	case s.activeTab() == 1:
		tab1 = styles.StylePrimary.Render("[ vars.yml ]")
	default:
		tab1 = styles.StyleMuted.Render("  vars.yml  ")
	}
	b.WriteString(tab0 + "  " + tab1 + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	b.WriteString(s.vp.View() + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	if len(s.errors) == 0 {
		b.WriteString(styles.StyleSuccess.Render("✓ 검증 통과 — 저장 가능합니다") + "\n")
	} else {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("✗ 검증 오류 %d건:", len(s.errors))) + "\n")
		for _, e := range s.errors {
			b.WriteString(styles.StyleError.Render("  • "+e.Error()) + "\n")
		}
	}

	if s.saveErr != "" {
		b.WriteString(styles.StyleError.Render("✗ "+s.saveErr) + "\n")
	}
	if s.saved {
		b.WriteString("\n" + styles.StyleSuccess.Render("✓ 저장 완료!") + "\n")
		b.WriteString(styles.StyleMuted.Render(fmt.Sprintf("  %s\n  %s", s.inventoryPath, s.varsPath)) + "\n")
		b.WriteString(styles.StyleMuted.Render("  기존 파일은 .bak으로 백업되었습니다") + "\n")
	}

	prevFocused := s.focusIdx == s7FocusNav && s.navIdx == 0
	saveFocused := s.focusIdx == s7FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "저장 (s)", prevFocused, saveFocused, s.width))

	return b.String()
}

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

type S7Preview struct {
	inventoryContent string
	varsContent      string
	activeTab        int // 0=inventory, 1=vars
	vp               viewport.Model
	errors           []validator.Error
	saved            bool
	saveErr          string
	appState         *state.AppState

	inventoryPath string
	varsPath      string

	width  int
	height int
}

func NewS7Preview(s *state.AppState, inventoryPath, varsPath string) *S7Preview {
	vp := viewport.New(80, 20)
	return &S7Preview{
		vp:            vp,
		appState:      s,
		inventoryPath: inventoryPath,
		varsPath:      varsPath,
	}
}

func (s *S7Preview) Title() string      { return "미리보기 & 저장" }
func (s *S7Preview) SetSize(w, h int)   { s.width = w; s.height = h; s.vp.Width = w; s.vp.Height = h - 14 }
func (s *S7Preview) FooterHelp() string {
	if s.saved {
		return "저장 완료! ctrl+c 로 종료"
	}
	return "tab: 탭 전환  ↑/↓: 스크롤  s: 저장  ctrl+p: 이전"
}

func (s *S7Preview) SyncFromState(st *state.AppState) {
	s.errors = validator.Validate(st)

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
		case "tab":
			s.activeTab = (s.activeTab + 1) % 2
			s.refreshViewport()
		case "s":
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
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	s.vp, cmd = s.vp.Update(msg)
	return s, cmd
}

func (s *S7Preview) refreshViewport() {
	if s.activeTab == 0 {
		s.vp.SetContent(s.inventoryContent)
	} else {
		s.vp.SetContent(s.varsContent)
	}
	s.vp.GotoTop()
}

func (s *S7Preview) View() string {
	var b strings.Builder

	b.WriteString(styles.StyleTitle.Render("미리보기 & 저장") + "\n")

	// Tabs
	tab0 := "  inventory.yml  "
	tab1 := "  vars.yml  "
	if s.activeTab == 0 {
		tab0 = styles.StyleSelected.Render("[ inventory.yml ]")
		tab1 = styles.StyleMuted.Render("[ vars.yml ]")
	} else {
		tab0 = styles.StyleMuted.Render("[ inventory.yml ]")
		tab1 = styles.StyleSelected.Render("[ vars.yml ]")
	}
	b.WriteString(tab0 + "  " + tab1 + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	// Viewport
	b.WriteString(s.vp.View() + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	// Validation status
	if len(s.errors) == 0 {
		b.WriteString(styles.StyleSuccess.Render("✓ 검증 통과 — 저장 가능합니다") + "\n")
	} else {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("✗ 검증 오류 %d건:", len(s.errors))) + "\n")
		for _, e := range s.errors {
			b.WriteString(styles.StyleError.Render("  • " + e.Error()) + "\n")
		}
	}

	if s.saveErr != "" {
		b.WriteString(styles.StyleError.Render("✗ " + s.saveErr) + "\n")
	}
	if s.saved {
		b.WriteString("\n" + styles.StyleSuccess.Render("✓ 저장 완료!") + "\n")
		b.WriteString(styles.StyleMuted.Render(fmt.Sprintf("  %s\n  %s", s.inventoryPath, s.varsPath)) + "\n")
		b.WriteString(styles.StyleMuted.Render("  기존 파일은 .bak으로 백업되었습니다") + "\n")
	}

	return b.String()
}

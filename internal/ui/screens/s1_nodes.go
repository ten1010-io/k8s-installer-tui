package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
)

const (
	s1FieldName = iota
	s1FieldHost
	s1FieldPort
	s1FieldUser
	s1FieldCount
)

// S1Nodes is the first wizard screen: define nodes.
type S1Nodes struct {
	nodes   []state.NodeConfig
	cursor  int
	editing bool
	editIdx int // -1 = new node

	fields    [s1FieldCount]textinput.Model
	focusField int
	formErr   string

	width  int
	height int
	err    string
}

func NewS1Nodes() *S1Nodes {
	s := &S1Nodes{editIdx: -1}
	for i := range s.fields {
		t := textinput.New()
		t.CharLimit = 128
		s.fields[i] = t
	}
	s.fields[s1FieldName].Placeholder = "node1"
	s.fields[s1FieldHost].Placeholder = "192.168.0.1"
	s.fields[s1FieldPort].Placeholder = "22 (기본값)"
	s.fields[s1FieldUser].Placeholder = "root (기본값)"
	return s
}

func (s *S1Nodes) Title() string      { return "노드 정의" }
func (s *S1Nodes) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S1Nodes) FooterHelp() string {
	if s.editing {
		return "tab: 다음 필드  shift+tab: 이전 필드  enter: 저장  esc: 취소"
	}
	return "a: 추가  e/enter: 편집  d: 삭제  ↑/↓: 이동  ctrl+n: 다음"
}

func (s *S1Nodes) SyncFromState(st *state.AppState) {
	s.nodes = make([]state.NodeConfig, len(st.Nodes))
	copy(s.nodes, st.Nodes)
}

func (s *S1Nodes) SyncToState(st *state.AppState) {
	st.Nodes = make([]state.NodeConfig, len(s.nodes))
	copy(st.Nodes, s.nodes)
}

func (s *S1Nodes) Validate() []string {
	if len(s.nodes) == 0 {
		return []string{"노드를 최소 1개 이상 추가해주세요"}
	}
	seen := map[string]bool{}
	for _, n := range s.nodes {
		if n.Name == "" {
			return []string{"이름이 비어있는 노드가 있습니다"}
		}
		if n.AnsibleHost == "" {
			return []string{fmt.Sprintf("'%s'의 ansible_host가 비어있습니다", n.Name)}
		}
		if seen[n.Name] {
			return []string{fmt.Sprintf("노드 이름 '%s'가 중복됩니다", n.Name)}
		}
		seen[n.Name] = true
	}
	return nil
}

func (s *S1Nodes) Init() tea.Cmd { return nil }

func (s *S1Nodes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.editing {
		return s.updateForm(msg)
	}
	return s.updateList(msg)
}

func (s *S1Nodes) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.nodes)-1 {
				s.cursor++
			}
		case "a":
			s.openForm(-1, state.NodeConfig{})
		case "e", "enter":
			if len(s.nodes) > 0 {
				s.openForm(s.cursor, s.nodes[s.cursor])
			}
		case "d", "delete":
			if len(s.nodes) > 0 {
				s.nodes = append(s.nodes[:s.cursor], s.nodes[s.cursor+1:]...)
				if s.cursor >= len(s.nodes) && s.cursor > 0 {
					s.cursor--
				}
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

func (s *S1Nodes) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editing = false
			s.formErr = ""
			return s, nil
		case "tab":
			s.fields[s.focusField].Blur()
			s.focusField = (s.focusField + 1) % s1FieldCount
			s.fields[s.focusField].Focus()
			return s, textinput.Blink
		case "shift+tab":
			s.fields[s.focusField].Blur()
			s.focusField = (s.focusField - 1 + s1FieldCount) % s1FieldCount
			s.fields[s.focusField].Focus()
			return s, textinput.Blink
		case "enter":
			if err := s.saveForm(); err != "" {
				s.formErr = err
				return s, nil
			}
			s.editing = false
			s.formErr = ""
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.fields[s.focusField], cmd = s.fields[s.focusField].Update(msg)
	return s, cmd
}

func (s *S1Nodes) openForm(idx int, n state.NodeConfig) {
	s.editIdx = idx
	s.editing = true
	s.focusField = s1FieldName
	s.formErr = ""
	s.fields[s1FieldName].SetValue(n.Name)
	s.fields[s1FieldHost].SetValue(n.AnsibleHost)
	s.fields[s1FieldPort].SetValue(n.AnsiblePort)
	s.fields[s1FieldUser].SetValue(n.AnsibleUser)
	for i := range s.fields {
		s.fields[i].Blur()
	}
	s.fields[s.focusField].Focus()
}

func (s *S1Nodes) saveForm() string {
	name := strings.TrimSpace(s.fields[s1FieldName].Value())
	host := strings.TrimSpace(s.fields[s1FieldHost].Value())
	if name == "" {
		return "이름을 입력해주세요"
	}
	if host == "" {
		return "ansible_host를 입력해주세요"
	}
	// Check duplicate (exclude self when editing)
	for i, n := range s.nodes {
		if n.Name == name && i != s.editIdx {
			return fmt.Sprintf("노드 이름 '%s'가 이미 존재합니다", name)
		}
	}
	nc := state.NodeConfig{
		Name:        name,
		AnsibleHost: host,
		AnsiblePort: strings.TrimSpace(s.fields[s1FieldPort].Value()),
		AnsibleUser: strings.TrimSpace(s.fields[s1FieldUser].Value()),
	}
	if s.editIdx == -1 {
		s.nodes = append(s.nodes, nc)
		s.cursor = len(s.nodes) - 1
	} else {
		s.nodes[s.editIdx] = nc
	}
	return ""
}

func (s *S1Nodes) View() string {
	var b strings.Builder

	b.WriteString(ui.StyleTitle.Render("노드 정의") + "\n")
	b.WriteString(ui.StyleMuted.Render("inventory.yml의 all.hosts 섹션을 구성합니다.") + "\n\n")

	// Table header
	colW := []int{14, 18, 8, 14}
	headers := []string{"이름", "ansible_host", "포트", "SSH 유저"}
	header := ""
	for i, h := range headers {
		header += lipgloss.NewStyle().Width(colW[i]).Bold(true).Foreground(lipgloss.Color("69")).Render(h)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", 56) + "\n")

	if len(s.nodes) == 0 {
		b.WriteString(ui.StyleMuted.Render("  노드가 없습니다. [a]를 눌러 추가하세요.") + "\n")
	}
	for i, n := range s.nodes {
		port := n.AnsiblePort
		if port == "" {
			port = ui.StyleMuted.Render("22")
		}
		user := n.AnsibleUser
		if user == "" {
			user = ui.StyleMuted.Render("root")
		}
		row := ""
		row += lipgloss.NewStyle().Width(colW[0]).Render(n.Name)
		row += lipgloss.NewStyle().Width(colW[1]).Render(n.AnsibleHost)
		row += lipgloss.NewStyle().Width(colW[2]).Render(port)
		row += lipgloss.NewStyle().Width(colW[3]).Render(user)
		if i == s.cursor && !s.editing {
			b.WriteString(ui.StyleTableRowSelected.Render(row) + "\n")
		} else {
			b.WriteString(row + "\n")
		}
	}
	b.WriteString("\n")

	if s.err != "" {
		b.WriteString(ui.StyleError.Render("✗ "+s.err) + "\n\n")
	}

	if s.editing {
		b.WriteString(s.viewForm())
	}

	return b.String()
}

func (s *S1Nodes) viewForm() string {
	title := "새 노드 추가"
	if s.editIdx >= 0 {
		title = "노드 편집"
	}
	var b strings.Builder
	b.WriteString(ui.StyleBox.Render(
		ui.StyleSelected.Render(title) + "\n\n" +
			renderField("이름", s.fields[s1FieldName].View(), s.focusField == s1FieldName) +
			renderField("ansible_host", s.fields[s1FieldHost].View(), s.focusField == s1FieldHost) +
			renderField("ansible_port", s.fields[s1FieldPort].View(), s.focusField == s1FieldPort) +
			renderField("ansible_ssh_user", s.fields[s1FieldUser].View(), s.focusField == s1FieldUser),
	))
	if s.formErr != "" {
		b.WriteString("\n" + ui.StyleError.Render("✗ "+s.formErr))
	}
	return b.String()
}

func renderField(label, input string, focused bool) string {
	lStyle := ui.StyleLabel
	if focused {
		lStyle = ui.StyleLabelFocused
	}
	return lStyle.Render(label+":") + "  " + input + "\n"
}

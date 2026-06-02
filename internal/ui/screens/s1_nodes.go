package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// List mode focus layout:
//   0..N-1  : node rows
//   N       : < 추가 >
//   N+1     : < 이전 >
//   N+2     : < 다음 >
//
// Form mode focus layout:
//   0  : name field
//   1  : host field
//   2  : port field
//   3  : user field
//   4  : < 저장 >
//   5  : < 취소 >

const (
	s1FieldName = iota
	s1FieldHost
	s1FieldPort
	s1FieldUser
	s1FieldCount

	s1FormSave   = s1FieldCount
	s1FormCancel = s1FieldCount + 1
	s1FormMax    = s1FormCancel
)

type S1Nodes struct {
	nodes    []state.NodeConfig
	focusIdx int // list mode focus
	editing  bool
	editIdx  int // -1 = new node
	formFocus int
	fields   [s1FieldCount]textinput.Model
	formErr  string
	width    int
	height   int
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

func (s *S1Nodes) Title() string { return "노드 정의" }
func (s *S1Nodes) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S1Nodes) SyncFromState(st *state.AppState) {
	s.nodes = make([]state.NodeConfig, len(st.Nodes))
	copy(s.nodes, st.Nodes)
	s.clampFocus()
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

func (s *S1Nodes) listMax() int { return len(s.nodes) + 2 }

func (s *S1Nodes) clampFocus() {
	max := s.listMax()
	if s.focusIdx > max {
		s.focusIdx = max
	}
	if s.focusIdx < 0 {
		s.focusIdx = 0
	}
}

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
			if s.focusIdx > 0 {
				s.focusIdx--
			}
		case "down", "j":
			if s.focusIdx < s.listMax() {
				s.focusIdx++
			}
		case "enter", " ":
			return s.activateList()
		case "a":
			s.openForm(-1, state.NodeConfig{})
		case "d", "delete":
			if s.focusIdx < len(s.nodes) && len(s.nodes) > 0 {
				s.nodes = append(s.nodes[:s.focusIdx], s.nodes[s.focusIdx+1:]...)
				s.clampFocus()
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

func (s *S1Nodes) activateList() (tea.Model, tea.Cmd) {
	max := s.listMax()
	switch s.focusIdx {
	case max - 1: // < 이전 >
		return s, Prev()
	case max: // < 다음 >
		return s, Next()
	case len(s.nodes): // < 추가 >
		s.openForm(-1, state.NodeConfig{})
	default:
		if s.focusIdx < len(s.nodes) {
			s.openForm(s.focusIdx, s.nodes[s.focusIdx])
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
		case "up":
			if s.formFocus > 0 {
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Blur()
				}
				s.formFocus--
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Focus()
					return s, textinput.Blink
				}
			}
		case "down", "tab":
			if s.formFocus < s1FormMax {
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Blur()
				}
				s.formFocus++
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Focus()
					return s, textinput.Blink
				}
			}
		case "shift+tab":
			if s.formFocus > 0 {
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Blur()
				}
				s.formFocus--
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Focus()
					return s, textinput.Blink
				}
			}
		case "enter", " ":
			switch s.formFocus {
			case s1FormSave:
				if err := s.saveForm(); err != "" {
					s.formErr = err
					return s, nil
				}
				s.editing = false
				s.formErr = ""
			case s1FormCancel:
				s.editing = false
				s.formErr = ""
			default:
				// Enter on field: move to next
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Blur()
				}
				s.formFocus++
				if s.formFocus < s1FieldCount {
					s.fields[s.formFocus].Focus()
					return s, textinput.Blink
				}
			}
			return s, nil
		}
	}
	if s.formFocus < s1FieldCount {
		var cmd tea.Cmd
		s.fields[s.formFocus], cmd = s.fields[s.formFocus].Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *S1Nodes) openForm(idx int, n state.NodeConfig) {
	s.editIdx = idx
	s.editing = true
	s.formFocus = s1FieldName
	s.formErr = ""
	s.fields[s1FieldName].SetValue(n.Name)
	s.fields[s1FieldHost].SetValue(n.AnsibleHost)
	s.fields[s1FieldPort].SetValue(n.AnsiblePort)
	s.fields[s1FieldUser].SetValue(n.AnsibleUser)
	for i := range s.fields {
		s.fields[i].Blur()
	}
	s.fields[s.formFocus].Focus()
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
		s.focusIdx = len(s.nodes) - 1
	} else {
		s.nodes[s.editIdx] = nc
	}
	return ""
}

func (s *S1Nodes) View() string {
	if s.editing {
		return s.viewForm()
	}
	return s.viewList()
}

func (s *S1Nodes) viewList() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("노드 정의") + "\n")
	b.WriteString(styles.StyleMuted.Render("inventory.yml › all.hosts") + "\n\n")

	colW := []int{14, 18, 8, 12}
	header := ""
	for i, h := range []string{"이름", "ansible_host", "포트", "SSH 유저"} {
		header += lipgloss.NewStyle().Width(colW[i]).Bold(true).Foreground(styles.ColorPrimary).Render(h)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	if len(s.nodes) == 0 {
		b.WriteString(styles.StyleMuted.Render("  (노드 없음)") + "\n")
	}
	for i, n := range s.nodes {
		port := n.AnsiblePort
		if port == "" {
			port = "22"
		}
		user := n.AnsibleUser
		if user == "" {
			user = "root"
		}
		row := lipgloss.NewStyle().Width(colW[0]).Render(n.Name) +
			lipgloss.NewStyle().Width(colW[1]).Render(n.AnsibleHost) +
			lipgloss.NewStyle().Width(colW[2]).Render(port) +
			lipgloss.NewStyle().Width(colW[3]).Render(user)
		b.WriteString(RenderRow(row, s.focusIdx == i, s.width) + "\n")
	}

	b.WriteString("\n")
	addFocused := s.focusIdx == len(s.nodes)
	b.WriteString("  " + RenderButton("추가", addFocused) +
		styles.StyleMuted.Render("  (a: 추가  d: 삭제  Enter: 편집)") + "\n")

	prevFocused := s.focusIdx == s.listMax()-1
	nextFocused := s.focusIdx == s.listMax()
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

func (s *S1Nodes) viewForm() string {
	title := "새 노드 추가"
	if s.editIdx >= 0 {
		title = "노드 편집"
	}
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(title) + "\n\n")

	labels := []string{"이름", "ansible_host", "ansible_port", "ansible_ssh_user"}
	for i, label := range labels {
		focused := s.formFocus == i
		lStyle := styles.StyleLabel
		if focused {
			lStyle = styles.StyleLabelFocused
		}
		b.WriteString(lStyle.Render(label+":") + "  " + s.fields[i].View() + "\n")
	}

	if s.formErr != "" {
		b.WriteString("\n" + styles.StyleError.Render("✗ "+s.formErr) + "\n")
	}

	b.WriteString("\n")
	saveFocused := s.formFocus == s1FormSave
	cancelFocused := s.formFocus == s1FormCancel
	b.WriteString("  " + RenderButton("저장", saveFocused) + "  " + RenderButton("취소", cancelFocused))
	b.WriteString(styles.StyleMuted.Render("  (Esc: 취소)"))

	return b.String()
}


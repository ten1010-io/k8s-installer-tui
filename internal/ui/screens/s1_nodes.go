package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// List mode focus:
//   0..N-1  : node rows
//   N       : < Add >
//   N+1     : nav area  (←/→ switches navIdx: 0=back, 1=next)
//
// Form mode focus:
//   0..3    : fields (name, host, port, user)
//   4       : < Save >
//   5       : < Cancel >

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
	nodes     []state.NodeConfig
	focusIdx  int
	navIdx    int // 0=back, 1=next
	editing   bool
	editIdx   int
	formFocus int
	fields    [s1FieldCount]textinput.Model
	formErr   string
	width     int
	height    int
}

func NewS1Nodes() *S1Nodes {
	s := &S1Nodes{editIdx: -1}
	for i := range s.fields {
		t := textinput.New()
		t.CharLimit = 128
		s.fields[i] = t
	}
	s.fields[s1FieldName].Placeholder = "node1"
	s.fields[s1FieldHost].Placeholder = "x.x.x.x"
	return s
}

func (s *S1Nodes) Title() string { return i18n.T("Nodes") }
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
		return []string{i18n.T("Add at least one node")}
	}
	seen := map[string]bool{}
	for _, n := range s.nodes {
		if n.Name == "" {
			return []string{i18n.T("A node has no name")}
		}
		if n.AnsibleHost == "" {
			return []string{i18n.T("ansible_host of '%s' is empty", n.Name)}
		}
		if seen[n.Name] {
			return []string{i18n.T("Node name '%s' is duplicated", n.Name)}
		}
		seen[n.Name] = true
	}
	return nil
}

func (s *S1Nodes) Init() tea.Cmd { return nil }

// Busy: the node form is open.
func (s *S1Nodes) Busy() bool { return s.editing }

// navFocus is the focusIdx value for the nav area.
func (s *S1Nodes) navFocus() int { return len(s.nodes) + 1 }

func (s *S1Nodes) clampFocus() {
	max := s.navFocus()
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
			if s.focusIdx < s.navFocus() {
				s.focusIdx++
				if s.focusIdx == s.navFocus() {
					s.navIdx = 1 // next
				}
			}
		case "left", "h":
			if s.focusIdx == s.navFocus() && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s.navFocus() && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			return s.activateList()
		case "a":
			s.openForm(-1, state.NodeConfig{})
			return s, textinput.Blink
		case "d", "delete":
			if s.focusIdx < len(s.nodes) && len(s.nodes) > 0 {
				s.nodes = append(s.nodes[:s.focusIdx], s.nodes[s.focusIdx+1:]...)
				s.clampFocus()
			}
		}
	}
	return s, nil
}

func (s *S1Nodes) activateList() (tea.Model, tea.Cmd) {
	switch {
	case s.focusIdx == s.navFocus(): // nav area
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	case s.focusIdx == len(s.nodes): // add
		s.openForm(-1, state.NodeConfig{})
		return s, textinput.Blink
	default: // node row
		if s.focusIdx < len(s.nodes) {
			s.openForm(s.focusIdx, s.nodes[s.focusIdx])
			return s, textinput.Blink
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
		case "up", "shift+tab":
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
		return i18n.T("Enter a name")
	}
	if host == "" {
		return i18n.T("Enter ansible_host")
	}
	for i, n := range s.nodes {
		if n.Name == name && i != s.editIdx {
			return i18n.T("Node name '%s' already exists", name)
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
	b.WriteString(styles.StyleTitle.Render(i18n.T("Nodes")) + "\n")
	b.WriteString(styles.StyleMuted.Render("inventory.yml › all.hosts") + "\n\n")

	colW := []int{14, 18, 8, 12}
	header := ""
	for i, h := range []string{i18n.T("Name"), "ansible_host", i18n.T("Port"), i18n.T("SSH user")} {
		header += lipgloss.NewStyle().Width(colW[i]).Bold(true).Foreground(styles.ColorPrimary).Render(h)
	}
	b.WriteString("  " + header + "\n")
	b.WriteString("  " + strings.Repeat("─", 54) + "\n")

	if len(s.nodes) == 0 {
		b.WriteString(styles.StyleMuted.Render("  "+i18n.T("(no nodes)")) + "\n")
	}
	// the focused row stays in sight on a low screen
	fs := focusSpan{b: &b}
	for i, n := range s.nodes {
		fs.mark(s.focusIdx == i)
		port := n.AnsiblePort
		if port == "" {
			port = "22"
		}
		user := n.AnsibleUser
		if user == "" {
			user = "root"
		}
		b.WriteString(renderNodeRow(n.Name, n.AnsibleHost, port, user, colW, s.focusIdx == i) + "\n")
	}

	fs.mark(false)
	b.WriteString("\n")
	addFocused := s.focusIdx == len(s.nodes)
	fs.mark(addFocused)
	b.WriteString("  " + RenderButton(i18n.T("Add"), addFocused) +
		styles.StyleMuted.Render("  "+i18n.T("(a: add  d: delete  Enter: edit)")) + "\n")

	prevFocused := s.focusIdx == s.navFocus() && s.navIdx == 0
	nextFocused := s.focusIdx == s.navFocus() && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(s.focusIdx == s.navFocus())
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))

	return fs.fit(s.width, s.height)
}

func (s *S1Nodes) viewForm() string {
	title := i18n.T("Add node")
	if s.editIdx >= 0 {
		title = i18n.T("Edit node")
	}
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(title) + "\n\n")

	s.fields[s1FieldPort].Placeholder = i18n.T("22 (default)")
	s.fields[s1FieldUser].Placeholder = i18n.T("root (default)")
	labels := []string{i18n.T("Name"), "ansible_host", "ansible_port", "ansible_ssh_user"}
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
	b.WriteString("  " + RenderButton(i18n.T("Save"), saveFocused) + "  " + RenderButton(i18n.T("Cancel"), cancelFocused))
	b.WriteString(styles.StyleMuted.Render("  " + i18n.T("(Esc: cancel)")))

	return b.String()
}

// renderNodeRow renders a node row with per-cell focus background so that
// ANSI reset codes between cells do not break the highlight.
func renderNodeRow(name, host, port, user string, colW []int, focused bool) string {
	if focused {
		cs := focusedStyle(0)
		return cs.Width(2).Render("▶") +
			cs.Width(colW[0]).Render(name) +
			cs.Width(colW[1]).Render(host) +
			cs.Width(colW[2]).Render(port) +
			cs.Width(colW[3]).Render(user)
	}
	return "  " +
		lipgloss.NewStyle().Width(colW[0]).Render(name) +
		lipgloss.NewStyle().Width(colW[1]).Foreground(styles.ColorPrimary).Render(host) +
		lipgloss.NewStyle().Width(colW[2]).Render(port) +
		lipgloss.NewStyle().Width(colW[3]).Render(user)
}

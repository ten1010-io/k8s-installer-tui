package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

const (
	colKiCp = iota
	colK8s
	colK8sCp
	colGPU
	colCount
)

type groupRow struct {
	nodeName string
	checks   [colCount]bool
}

// S2Groups assigns nodes to groups via a checkbox table.
type S2Groups struct {
	rows   []groupRow
	curRow int
	curCol int
	width  int
	height int
}

func NewS2Groups() *S2Groups { return &S2Groups{} }

func (s *S2Groups) Title() string      { return "그룹 할당" }
func (s *S2Groups) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S2Groups) FooterHelp() string {
	return "↑/↓: 행 이동  ←/→: 열 이동  space: 토글  ctrl+n: 다음  ctrl+p: 이전"
}

func (s *S2Groups) SyncFromState(st *state.AppState) {
	s.rows = make([]groupRow, len(st.Nodes))
	for i, n := range st.Nodes {
		s.rows[i] = groupRow{
			nodeName: n.Name,
			checks: [colCount]bool{
				colKiCp: st.IsKiCpNode(n.Name),
				colK8s:  st.IsK8sNode(n.Name),
				colK8sCp: st.IsK8sCpNode(n.Name),
				colGPU:  st.IsNvidiaGPUNode(n.Name),
			},
		}
	}
}

func (s *S2Groups) SyncToState(st *state.AppState) {
	st.KiCpNodes = nil
	st.K8sNodes = nil
	st.K8sCpNodes = nil
	st.NvidiaGPUNodes = nil
	for _, r := range s.rows {
		if r.checks[colKiCp] {
			st.KiCpNodes = append(st.KiCpNodes, r.nodeName)
		}
		if r.checks[colK8s] {
			st.K8sNodes = append(st.K8sNodes, r.nodeName)
		}
		if r.checks[colK8sCp] {
			st.K8sCpNodes = append(st.K8sCpNodes, r.nodeName)
		}
		if r.checks[colGPU] {
			st.NvidiaGPUNodes = append(st.NvidiaGPUNodes, r.nodeName)
		}
	}
}

func (s *S2Groups) Validate() []string {
	hasK8s := false
	for _, r := range s.rows {
		if r.checks[colK8s] {
			hasK8s = true
			break
		}
	}
	if !hasK8s {
		return []string{"k8s_node 그룹에 최소 1개 이상의 노드를 할당해주세요"}
	}
	// k8s_cp, nvidia_gpu는 k8s_node가 체크된 노드만 유효
	for _, r := range s.rows {
		if !r.checks[colK8s] && (r.checks[colK8sCp] || r.checks[colGPU]) {
			return []string{"k8s_cp 또는 nvidia_gpu는 k8s_node로 지정된 노드에만 설정할 수 있습니다"}
		}
	}
	return nil
}

func (s *S2Groups) Init() tea.Cmd { return nil }

func (s *S2Groups) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.curRow > 0 {
				s.curRow--
			}
		case "down", "j":
			if s.curRow < len(s.rows)-1 {
				s.curRow++
			}
		case "left", "h":
			if s.curCol > 0 {
				s.curCol--
			}
		case "right", "l":
			if s.curCol < colCount-1 {
				s.curCol++
			}
		case " ":
			if len(s.rows) > 0 {
				s.toggle(s.curRow, s.curCol)
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

func (s *S2Groups) toggle(row, col int) {
	r := &s.rows[row]
	// k8s_cp and GPU require k8s_node to be checked first
	if (col == colK8sCp || col == colGPU) && !r.checks[colK8s] {
		return
	}
	// Unchecking k8s_node also unchecks k8s_cp and GPU
	if col == colK8s && r.checks[colK8s] {
		r.checks[colK8sCp] = false
		r.checks[colGPU] = false
	}
	r.checks[col] = !r.checks[col]
}

func (s *S2Groups) View() string {
	var b strings.Builder

	b.WriteString(styles.StyleTitle.Render("그룹 할당") + "\n")
	b.WriteString(styles.StyleMuted.Render("각 노드의 역할을 선택합니다.") + "\n\n")

	colLabels := []string{"ki_cp_node", "k8s_node", "k8s_cp", "nvidia_gpu"}
	colW := []int{14, 12, 12, 12, 12}

	// Header
	header := lipgloss.NewStyle().Width(colW[0]).Bold(true).Render("노드")
	for i, l := range colLabels {
		style := lipgloss.NewStyle().Width(colW[i+1]).Bold(true).Foreground(lipgloss.Color("69"))
		header += style.Render(l)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", 62) + "\n")

	for rowIdx, r := range s.rows {
		isSelectedRow := rowIdx == s.curRow
		nameStyle := lipgloss.NewStyle().Width(colW[0])
		if isSelectedRow {
			nameStyle = nameStyle.Foreground(lipgloss.Color("212")).Bold(true)
		}
		row := nameStyle.Render(r.nodeName)
		for colIdx, checked := range r.checks {
			mark := styles.CheckOff
			if checked {
				mark = styles.CheckOn
			}
			cellStyle := lipgloss.NewStyle().Width(colW[colIdx+1]).Align(lipgloss.Center)
			if isSelectedRow && colIdx == s.curCol {
				cellStyle = cellStyle.Background(lipgloss.Color("62")).Bold(true)
				mark = " " + mark + " "
			}
			// Dim k8s_cp/GPU cells when k8s_node not checked
			if (colIdx == colK8sCp || colIdx == colGPU) && !r.checks[colK8s] {
				mark = styles.StyleMuted.Render("-")
			}
			row += cellStyle.Render(mark)
		}
		b.WriteString(row + "\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.StyleMuted.Render("참고: k8s_cp, nvidia_gpu는 k8s_node가 체크된 노드에서만 활성화됩니다") + "\n")

	return b.String()
}

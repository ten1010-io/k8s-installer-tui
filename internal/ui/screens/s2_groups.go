package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0..N-1  : table rows (←/→ for columns within a row)
//   N       : < 이전 >
//   N+1     : < 다음 >

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

type S2Groups struct {
	rows     []groupRow
	curRow   int
	curCol   int
	focusNav bool // true = nav buttons focused
	navIdx   int  // 0=이전, 1=다음
	width    int
	height   int
}

func NewS2Groups() *S2Groups { return &S2Groups{} }

func (s *S2Groups) Title() string { return "그룹 할당" }
func (s *S2Groups) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S2Groups) SyncFromState(st *state.AppState) {
	s.rows = make([]groupRow, len(st.Nodes))
	for i, n := range st.Nodes {
		s.rows[i] = groupRow{
			nodeName: n.Name,
			checks: [colCount]bool{
				colKiCp:  st.IsKiCpNode(n.Name),
				colK8s:   st.IsK8sNode(n.Name),
				colK8sCp: st.IsK8sCpNode(n.Name),
				colGPU:   st.IsNvidiaGPUNode(n.Name),
			},
		}
	}
	s.curRow = 0
	s.focusNav = false
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
	for _, r := range s.rows {
		if !r.checks[colK8s] && (r.checks[colK8sCp] || r.checks[colGPU]) {
			return []string{"k8s_cp / nvidia_gpu는 k8s_node로 지정된 노드에만 설정할 수 있습니다"}
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
			if s.focusNav {
				s.focusNav = false
			} else if s.curRow > 0 {
				s.curRow--
			}
		case "down", "j":
			if !s.focusNav {
				if s.curRow < len(s.rows)-1 {
					s.curRow++
				} else {
					s.focusNav = true
					s.navIdx = 0
				}
			}
		case "left", "h":
			if s.focusNav {
				if s.navIdx > 0 {
					s.navIdx--
				}
			} else if s.curCol > 0 {
				s.curCol--
			}
		case "right", "l":
			if s.focusNav {
				if s.navIdx < 1 {
					s.navIdx++
				}
			} else if s.curCol < colCount-1 {
				s.curCol++
			}
		case " ", "enter":
			if s.focusNav {
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			}
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
	if (col == colK8sCp || col == colGPU) && !r.checks[colK8s] {
		return
	}
	if col == colK8s && r.checks[colK8s] {
		r.checks[colK8sCp] = false
		r.checks[colGPU] = false
	}
	r.checks[col] = !r.checks[col]
}

func (s *S2Groups) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("그룹 할당") + "\n")
	b.WriteString(styles.StyleMuted.Render("각 노드의 역할을 선택합니다.  ←/→: 열 이동  Space: 토글") + "\n\n")

	colLabels := []string{"ki_cp_node", "k8s_node", "k8s_cp", "nvidia_gpu"}
	colW := []int{14, 14, 12, 12, 12}

	header := lipgloss.NewStyle().Width(colW[0]).Bold(true).Render("노드")
	for i, l := range colLabels {
		header += lipgloss.NewStyle().Width(colW[i+1]).Bold(true).Foreground(styles.ColorPrimary).Render(l)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	for rowIdx, r := range s.rows {
		isSelectedRow := !s.focusNav && rowIdx == s.curRow
		b.WriteString(renderGroupRow(r, colW, isSelectedRow, s.curCol) + "\n")
	}

	b.WriteString("\n" + styles.StyleMuted.Render("* k8s_cp / nvidia_gpu는 k8s_node 체크 후 활성화") + "\n")

	prevFocused := s.focusNav && s.navIdx == 0
	nextFocused := s.focusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

// renderGroupRow renders one row of the group table with per-cell background
// when selected so ANSI reset codes between cells don't break the highlight.
func renderGroupRow(r groupRow, colW []int, selected bool, curCol int) string {
	if selected {
		rs := lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("15")).Bold(true)
		cs := lipgloss.NewStyle().Background(lipgloss.Color("69")).Foreground(lipgloss.Color("15")).Bold(true)

		row := rs.Width(2).Render("▶") + rs.Width(colW[0]-2).Render(r.nodeName)
		for colIdx, checked := range r.checks {
			mark := styles.CheckOff
			if checked {
				mark = styles.CheckOn
			}
			if (colIdx == colK8sCp || colIdx == colGPU) && !r.checks[colK8s] {
				mark = "-"
			}
			cellSt := rs
			if colIdx == curCol {
				cellSt = cs
			}
			row += cellSt.Width(colW[colIdx+1]).Align(lipgloss.Center).Render(mark)
		}
		return row
	}

	row := "  " + lipgloss.NewStyle().Width(colW[0]-2).Render(r.nodeName)
	for colIdx, checked := range r.checks {
		mark := styles.CheckOff
		if checked {
			mark = styles.CheckOn
		}
		if (colIdx == colK8sCp || colIdx == colGPU) && !r.checks[colK8s] {
			mark = styles.StyleMuted.Render("-")
		}
		row += lipgloss.NewStyle().Width(colW[colIdx+1]).Align(lipgloss.Center).Render(mark)
	}
	return row
}

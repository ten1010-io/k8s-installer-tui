package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0..N-1  : table rows  (←/→: ki_cp(0) / Master(1))
//   N       : nav slot    (←/→: 이전/다음)
//
// Worker column is derived (read-only): a node that is not a Master.
// All nodes are automatically k8s_node — that column is not shown.
//
// There is no GPU column: k8s-installer removed nvidia_gpu and gives containers
// the gpus a node has through CDI, found on the node itself.

const (
	colKiCp   = iota // ki_cp_node
	colMaster        // k8s_cp → "Master Node"
	colCount         // number of interactive columns (Worker is derived)
)

type groupRow struct {
	nodeName string
	checks   [colCount]bool
}

type S2Groups struct {
	rows     []groupRow
	curRow   int
	curCol   int // 0=ki_cp, 1=master, 2=gpu  (CPU Worker col is read-only)
	focusNav bool
	navIdx   int // 0=이전, 1=다음
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
				colKiCp:   st.IsKiCpNode(n.Name),
				colMaster: st.IsK8sCpNode(n.Name),
			},
		}
	}
	s.curRow = 0
	s.focusNav = false
}

func (s *S2Groups) SyncToState(st *state.AppState) {
	// All nodes are k8s_node members automatically.
	st.K8sNodes = make([]string, len(s.rows))
	for i, r := range s.rows {
		st.K8sNodes[i] = r.nodeName
	}
	st.KiCpNodes = nil
	st.K8sCpNodes = nil
	st.NvidiaGPUNodes = nil
	for _, r := range s.rows {
		if r.checks[colKiCp] {
			st.KiCpNodes = append(st.KiCpNodes, r.nodeName)
		}
		if r.checks[colMaster] {
			st.K8sCpNodes = append(st.K8sCpNodes, r.nodeName)
		}
	}
}

func (s *S2Groups) Validate() []string {
	ki, cp := 0, 0
	for _, r := range s.rows {
		if r.checks[colKiCp] {
			ki++
		}
		if r.checks[colMaster] {
			cp++
		}
	}
	if ki == 0 {
		return []string{"ki_cp_node를 1개 이상 선택해주세요 (제어 노드가 그중 첫 번째입니다)"}
	}
	if cp == 0 {
		return []string{"Master Node를 1개 이상 선택해주세요"}
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
					s.navIdx = 1 // 다음
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
		}
	}
	return s, nil
}

func (s *S2Groups) toggle(row, col int) {
	s.rows[row].checks[col] = !s.rows[row].checks[col]
}

func (s *S2Groups) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("그룹 할당") + "\n")
	b.WriteString(styles.StyleMuted.Render("각 노드의 역할을 선택합니다.  ←/→: 열 이동  Space: 선택") + "\n")
	b.WriteString(styles.StyleMuted.Render("GPU는 노드에서 자동으로 인식합니다 (k8s-installer CDI). 따로 표시하지 않습니다.") + "\n\n")

	// 3 display columns: ki_cp | Master | Worker(derived)
	colLabels := []string{"ki_cp_node", "Master Node", "Worker"}
	colW := []int{14, 14, 14, 12}

	header := lipgloss.NewStyle().Width(colW[0]).Bold(true).Render("노드")
	for i, l := range colLabels {
		header += lipgloss.NewStyle().Width(colW[i+1]).Bold(true).Foreground(styles.ColorPrimary).Render(l)
	}
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("─", s.width) + "\n")

	// the focused row stays in sight on a low screen
	fs := focusSpan{b: &b}
	for rowIdx, r := range s.rows {
		isSelectedRow := !s.focusNav && rowIdx == s.curRow
		fs.mark(isSelectedRow)
		b.WriteString(renderGroupRow(r, colW, isSelectedRow, s.curCol) + "\n")
	}

	prevFocused := s.focusNav && s.navIdx == 0
	nextFocused := s.focusNav && s.navIdx == 1
	fs.mark(false)
	b.WriteString("\n")
	fs.mark(s.focusNav)
	b.WriteString(RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return fs.fit(s.width, s.height)
}

func renderGroupRow(r groupRow, colW []int, selected bool, curCol int) string {
	marks := [3]string{styles.CheckOff(), styles.CheckOff(), styles.StyleMuted.Render("-")}
	if r.checks[colKiCp] {
		marks[0] = styles.CheckOn()
	}
	if r.checks[colMaster] {
		marks[1] = styles.CheckOn()
	} else {
		marks[2] = styles.CheckOn()
	}

	if selected {
		rs := focusedStyle(0)
		cs := lipgloss.NewStyle().Background(styles.AccentBg).Foreground(styles.FocusFg).Bold(true)

		row := rs.Width(2).Render("▶") + rs.Width(colW[0]-2).Render(r.nodeName)
		for i := 0; i < len(marks); i++ {
			st := rs
			if i < colCount && i == curCol {
				st = cs
			}
			row += st.Width(colW[i+1]).Align(lipgloss.Center).Render(marks[i])
		}
		return row
	}

	row := "  " + lipgloss.NewStyle().Width(colW[0]-2).Render(r.nodeName)
	for i := 0; i < len(marks); i++ {
		row += lipgloss.NewStyle().Width(colW[i+1]).Align(lipgloss.Center).Render(marks[i])
	}
	return row
}

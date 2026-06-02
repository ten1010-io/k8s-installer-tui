package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout (normal mode):
//   0       : cert validity input
//   1..N    : LB list rows
//   N+1     : < LB 추가 >
//   N+2     : ingress LB selector
//   N+3     : ingress port input
//   N+4     : nav slot (←/→: 이전/다음)

// LB form focus:
//   0: name, 1: vip, 2..M+1: node checkboxes, M+2: < 저장 >, M+3: < 취소 >

type S4Kubernetes struct {
	certValidity textinput.Model
	lbs          []state.LBConfig
	lbCursor     int

	editingLB bool
	lbEditIdx int
	lbName    textinput.Model
	lbVIP     textinput.Model
	lbNodes   []string
	lbChecks  []bool
	lbFormFocus int

	ingressLBIdx int
	ingressPort  textinput.Model

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS4Kubernetes() *S4Kubernetes {
	s := &S4Kubernetes{lbEditIdx: -1, ingressLBIdx: 0}
	s.certValidity = textinput.New()
	s.certValidity.Placeholder = "26280h"
	s.certValidity.CharLimit = 16

	s.lbName = textinput.New()
	s.lbName.Placeholder = "lb1"
	s.lbName.CharLimit = 64

	s.lbVIP = textinput.New()
	s.lbVIP.Placeholder = "192.168.0.200"
	s.lbVIP.CharLimit = 64

	s.ingressPort = textinput.New()
	s.ingressPort.Placeholder = "443"
	s.ingressPort.CharLimit = 8

	return s
}

func (s *S4Kubernetes) Title() string { return "Kubernetes 설정" }
func (s *S4Kubernetes) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S4Kubernetes) lbAddFocus() int   { return len(s.lbs) + 1 }
func (s *S4Kubernetes) ingressFocus() int { return len(s.lbs) + 2 }
func (s *S4Kubernetes) portFocus() int    { return len(s.lbs) + 3 }
func (s *S4Kubernetes) navFocus() int     { return len(s.lbs) + 4 }

func (s *S4Kubernetes) SyncFromState(st *state.AppState) {
	s.certValidity.SetValue(st.K8sCertificateValidityPeriod)
	s.lbs = make([]state.LBConfig, len(st.K8sLoadBalancers))
	for i, lb := range st.K8sLoadBalancers {
		s.lbs[i] = state.LBConfig{Name: lb.Name, VIP: lb.VIP, Nodes: append([]string{}, lb.Nodes...)}
	}
	s.lbNodes = append([]string{}, st.K8sNodes...)
	s.ingressPort.SetValue(strconv.Itoa(st.K8sDefaultIngressClass.Port))
	s.ingressLBIdx = 0
	for i, lb := range s.lbs {
		if lb.Name == st.K8sDefaultIngressClass.LoadBalancer {
			s.ingressLBIdx = i
			break
		}
	}
	s.focusIdx = 0
	s.navIdx = 0
	s.editingLB = false
	s.syncFocusedInputs()
}

func (s *S4Kubernetes) SyncToState(st *state.AppState) {
	st.K8sCertificateValidityPeriod = s.certValidity.Value()
	st.K8sLoadBalancers = make([]state.LBConfig, len(s.lbs))
	for i, lb := range s.lbs {
		st.K8sLoadBalancers[i] = state.LBConfig{Name: lb.Name, VIP: lb.VIP, Nodes: append([]string{}, lb.Nodes...)}
	}
	port, _ := strconv.Atoi(s.ingressPort.Value())
	lbName := ""
	if s.ingressLBIdx >= 0 && s.ingressLBIdx < len(s.lbs) {
		lbName = s.lbs[s.ingressLBIdx].Name
	}
	st.K8sDefaultIngressClass = state.IngressConfig{LoadBalancer: lbName, Port: port}
}

func (s *S4Kubernetes) Validate() []string {
	if s.certValidity.Value() == "" {
		return []string{"인증서 유효기간을 입력해주세요 (예: 26280h)"}
	}
	if len(s.lbs) == 0 {
		return []string{"로드밸런서를 최소 1개 이상 추가해주세요"}
	}
	p, err := strconv.Atoi(s.ingressPort.Value())
	if err != nil || p < 1 || p > 65535 {
		return []string{"인그레스 포트는 1-65535 범위여야 합니다"}
	}
	return nil
}

func (s *S4Kubernetes) Init() tea.Cmd { return nil }

func (s *S4Kubernetes) syncFocusedInputs() {
	s.certValidity.Blur()
	s.ingressPort.Blur()
	if !s.editingLB {
		if s.focusIdx == 0 {
			s.certValidity.Focus()
		} else if s.focusIdx == s.portFocus() {
			s.ingressPort.Focus()
		}
	}
}

func (s *S4Kubernetes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.editingLB {
		return s.updateLBForm(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx > 0 {
				s.focusIdx--
				s.syncFocusedInputs()
			}
		case "down", "j":
			if s.focusIdx < s.navFocus() {
				s.focusIdx++
				s.syncFocusedInputs()
			}
		case "left", "h":
			if s.focusIdx == s.ingressFocus() && s.ingressLBIdx > 0 {
				s.ingressLBIdx--
			} else if s.focusIdx == s.navFocus() && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s.ingressFocus() && s.ingressLBIdx < len(s.lbs)-1 {
				s.ingressLBIdx++
			} else if s.focusIdx == s.navFocus() && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			return s.activate()
		case "d", "delete":
			lbBase := 1
			if s.focusIdx >= lbBase && s.focusIdx < lbBase+len(s.lbs) {
				idx := s.focusIdx - lbBase
				s.lbs = append(s.lbs[:idx], s.lbs[idx+1:]...)
				if s.ingressLBIdx >= len(s.lbs) && s.ingressLBIdx > 0 {
					s.ingressLBIdx--
				}
				s.syncFocusedInputs()
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	if s.focusIdx == 0 {
		s.certValidity, cmd = s.certValidity.Update(msg)
	} else if s.focusIdx == s.portFocus() {
		s.ingressPort, cmd = s.ingressPort.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) activate() (tea.Model, tea.Cmd) {
	switch {
	case s.focusIdx == s.navFocus():
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	case s.focusIdx == s.lbAddFocus():
		s.openLBForm(-1, state.LBConfig{})
	case s.focusIdx >= 1 && s.focusIdx < s.lbAddFocus():
		idx := s.focusIdx - 1
		if idx < len(s.lbs) {
			s.openLBForm(idx, s.lbs[idx])
		}
	}
	return s, nil
}

func (s *S4Kubernetes) openLBForm(idx int, lb state.LBConfig) {
	s.editingLB = true
	s.lbEditIdx = idx
	s.lbFormFocus = 0
	s.lbName.SetValue(lb.Name)
	s.lbVIP.SetValue(lb.VIP)
	s.lbChecks = make([]bool, len(s.lbNodes))
	for i, n := range s.lbNodes {
		for _, lbn := range lb.Nodes {
			if lbn == n {
				s.lbChecks[i] = true
			}
		}
	}
	s.lbName.Focus()
	s.lbVIP.Blur()
}

func (s *S4Kubernetes) lbFormMax() int { return 2 + len(s.lbNodes) + 1 } // name+vip+nodes+save+cancel

func (s *S4Kubernetes) updateLBForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	nodeBase := 2
	saveFocus := nodeBase + len(s.lbNodes)
	cancelFocus := saveFocus + 1

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingLB = false
			return s, nil
		case "up":
			if s.lbFormFocus > 0 {
				s.lbFormFocus--
				s.syncLBFormFocus()
			}
		case "down", "tab":
			if s.lbFormFocus < cancelFocus {
				s.lbFormFocus++
				s.syncLBFormFocus()
			}
		case "shift+tab":
			if s.lbFormFocus > 0 {
				s.lbFormFocus--
				s.syncLBFormFocus()
			}
		case "enter", " ":
			switch {
			case s.lbFormFocus == saveFocus:
				s.saveLBForm()
				s.editingLB = false
			case s.lbFormFocus == cancelFocus:
				s.editingLB = false
			case s.lbFormFocus >= nodeBase && s.lbFormFocus < nodeBase+len(s.lbNodes):
				s.lbChecks[s.lbFormFocus-nodeBase] = !s.lbChecks[s.lbFormFocus-nodeBase]
			default:
				s.lbFormFocus++
				s.syncLBFormFocus()
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	if s.lbFormFocus == 0 {
		s.lbName, cmd = s.lbName.Update(msg)
	} else if s.lbFormFocus == 1 {
		s.lbVIP, cmd = s.lbVIP.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) syncLBFormFocus() {
	s.lbName.Blur()
	s.lbVIP.Blur()
	if s.lbFormFocus == 0 {
		s.lbName.Focus()
	} else if s.lbFormFocus == 1 {
		s.lbVIP.Focus()
	}
}

func (s *S4Kubernetes) saveLBForm() {
	nodes := []string{}
	for i, checked := range s.lbChecks {
		if checked {
			nodes = append(nodes, s.lbNodes[i])
		}
	}
	lb := state.LBConfig{
		Name:  strings.TrimSpace(s.lbName.Value()),
		VIP:   strings.TrimSpace(s.lbVIP.Value()),
		Nodes: nodes,
	}
	if s.lbEditIdx == -1 {
		s.lbs = append(s.lbs, lb)
		s.lbCursor = len(s.lbs) - 1
	} else {
		s.lbs[s.lbEditIdx] = lb
	}
}

func (s *S4Kubernetes) View() string {
	if s.editingLB {
		return s.viewLBForm()
	}
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("Kubernetes 설정") + "\n\n")

	certFocused := s.focusIdx == 0
	b.WriteString(RenderSectionHeader("인증서 유효기간", certFocused) + "  " +
		s.certValidity.View() + styles.StyleMuted.Render("  (예: 26280h = 3년)") + "\n\n")

	b.WriteString(RenderSectionHeader("로드밸런서 목록", s.focusIdx == 1 && len(s.lbs) > 0) + "\n")
	for i, lb := range s.lbs {
		rowFocused := s.focusIdx == i+1
		nodes := strings.Join(lb.Nodes, ", ")
		if nodes == "" {
			nodes = styles.StyleMuted.Render("(노드 없음)")
		}
		row := fmt.Sprintf("  %s  vip: %s  nodes: %s",
			styles.StylePrimary.Render(lb.Name), lb.VIP, nodes)
		b.WriteString(RenderRow(row, rowFocused, s.width) + "\n")
	}
	addFocused := s.focusIdx == s.lbAddFocus()
	b.WriteString("  " + RenderButton("LB 추가", addFocused) +
		styles.StyleMuted.Render("  (Enter: 편집  d: 삭제)") + "\n\n")

	ingressFocused := s.focusIdx == s.ingressFocus()
	lbName := styles.StyleMuted.Render("(없음)")
	if s.ingressLBIdx >= 0 && s.ingressLBIdx < len(s.lbs) {
		lbName = styles.StylePrimary.Render(s.lbs[s.ingressLBIdx].Name)
	}
	b.WriteString(RenderSectionHeader("기본 인그레스 LB", ingressFocused) +
		"  " + lbName + styles.StyleMuted.Render("  (←/→ 선택)") + "\n")

	portFocused := s.focusIdx == s.portFocus()
	b.WriteString(RenderSectionHeader("인그레스 포트", portFocused) + "  " + s.ingressPort.View() + "\n")

	prevFocused := s.focusIdx == s.navFocus() && s.navIdx == 0
	nextFocused := s.focusIdx == s.navFocus() && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

func (s *S4Kubernetes) viewLBForm() string {
	nodeBase := 2
	saveFocus := nodeBase + len(s.lbNodes)
	cancelFocus := saveFocus + 1

	title := "새 로드밸런서"
	if s.lbEditIdx >= 0 {
		title = "로드밸런서 편집"
	}
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(title) + "\n\n")

	f0 := s.lbFormFocus == 0
	f1 := s.lbFormFocus == 1
	b.WriteString(renderFormField("이름", s.lbName.View(), f0) + "\n")
	b.WriteString(renderFormField("VIP", s.lbVIP.View(), f1) + "\n\n")

	b.WriteString(styles.StyleLabel.Render("노드 (Space/Enter: 토글):") + "\n")
	for i, n := range s.lbNodes {
		focused := s.lbFormFocus == nodeBase+i
		mark := styles.CheckOff
		if s.lbChecks[i] {
			mark = styles.CheckOn
		}
		row := "  " + mark + " " + n
		b.WriteString(RenderRow(row, focused, s.width) + "\n")
	}

	b.WriteString("\n")
	saveFocused := s.lbFormFocus == saveFocus
	cancelFocused := s.lbFormFocus == cancelFocus
	b.WriteString("  " + RenderButton("저장", saveFocused) + "  " + RenderButton("취소", cancelFocused) +
		styles.StyleMuted.Render("  (Esc: 취소)"))

	return b.String()
}

func renderFormField(label, input string, focused bool) string {
	lStyle := styles.StyleLabel
	if focused {
		lStyle = styles.StyleLabelFocused
	}
	return lStyle.Render(label+":") + "  " + input
}

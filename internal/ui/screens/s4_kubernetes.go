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

type S4Kubernetes struct {
	certValidity textinput.Model
	lbs          []state.LBConfig
	lbCursor     int

	// LB edit form
	editingLB bool
	lbEditIdx int
	lbName    textinput.Model
	lbVIP     textinput.Model
	lbNodes   []string // all k8s node names
	lbChecks  []bool   // which nodes are in this LB

	// Default ingress
	ingressLBIdx int // index into lbs (-1 = none)
	ingressPort  textinput.Model

	// focus: 0=certValidity, 1=LB list, 2=ingress
	focus  int
	lbFormField int // 0=name, 1=vip, 2=nodes

	width  int
	height int
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

func (s *S4Kubernetes) Title() string      { return "Kubernetes 설정" }
func (s *S4Kubernetes) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S4Kubernetes) FooterHelp() string {
	if s.editingLB {
		return "tab: 필드 이동  space: 노드 토글  enter: 저장  esc: 취소"
	}
	return "tab: 섹션 이동  a: LB 추가  e: LB 편집  d: LB 삭제  ←/→: ingress LB 선택  ctrl+n: 다음"
}

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

func (s *S4Kubernetes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.editingLB {
		return s.updateLBForm(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.focus = (s.focus + 1) % 3
			s.syncFocus()
		case "shift+tab":
			s.focus = (s.focus - 1 + 3) % 3
			s.syncFocus()
		case "up", "k":
			if s.focus == 1 && s.lbCursor > 0 {
				s.lbCursor--
			}
		case "down", "j":
			if s.focus == 1 && s.lbCursor < len(s.lbs)-1 {
				s.lbCursor++
			}
		case "left", "h":
			if s.focus == 2 && s.ingressLBIdx > 0 {
				s.ingressLBIdx--
			}
		case "right", "l":
			if s.focus == 2 && s.ingressLBIdx < len(s.lbs)-1 {
				s.ingressLBIdx++
			}
		case "a":
			if s.focus == 1 {
				s.openLBForm(-1, state.LBConfig{})
			}
		case "e", "enter":
			if s.focus == 1 && len(s.lbs) > 0 {
				s.openLBForm(s.lbCursor, s.lbs[s.lbCursor])
			}
		case "d":
			if s.focus == 1 && len(s.lbs) > 0 {
				s.lbs = append(s.lbs[:s.lbCursor], s.lbs[s.lbCursor+1:]...)
				if s.lbCursor >= len(s.lbs) && s.lbCursor > 0 {
					s.lbCursor--
				}
				if s.ingressLBIdx >= len(s.lbs) {
					s.ingressLBIdx = len(s.lbs) - 1
				}
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	// Update focused text inputs
	var cmd tea.Cmd
	if s.focus == 0 {
		s.certValidity, cmd = s.certValidity.Update(msg)
	}
	if s.focus == 2 {
		s.ingressPort, cmd = s.ingressPort.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) syncFocus() {
	s.certValidity.Blur()
	s.ingressPort.Blur()
	switch s.focus {
	case 0:
		s.certValidity.Focus()
	case 2:
		s.ingressPort.Focus()
	}
}

func (s *S4Kubernetes) openLBForm(idx int, lb state.LBConfig) {
	s.editingLB = true
	s.lbEditIdx = idx
	s.lbFormField = 0
	s.lbName.SetValue(lb.Name)
	s.lbVIP.SetValue(lb.VIP)
	// Build checks
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

func (s *S4Kubernetes) updateLBForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingLB = false
			return s, nil
		case "tab":
			s.lbFormField = (s.lbFormField + 1) % 3
			s.lbName.Blur()
			s.lbVIP.Blur()
			if s.lbFormField == 0 {
				s.lbName.Focus()
			} else if s.lbFormField == 1 {
				s.lbVIP.Focus()
			}
			return s, textinput.Blink
		case " ":
			if s.lbFormField == 2 {
				// no cursor tracking in this simple impl; toggle is handled by node index
			}
		case "enter":
			if s.lbFormField == 2 {
				// save
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
				s.editingLB = false
			}
		}
		// Toggle node checks when on nodes field
		if s.lbFormField == 2 && len(msg.String()) == 1 {
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(s.lbChecks) {
				s.lbChecks[idx] = !s.lbChecks[idx]
			}
		}
	}
	var cmd tea.Cmd
	if s.lbFormField == 0 {
		s.lbName, cmd = s.lbName.Update(msg)
	} else if s.lbFormField == 1 {
		s.lbVIP, cmd = s.lbVIP.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("Kubernetes 설정") + "\n\n")

	// Cert validity
	b.WriteString(s.sectionHeader("인증서 유효기간", s.focus == 0))
	b.WriteString("  " + s.certValidity.View() + styles.StyleMuted.Render("  (예: 26280h = 3년)") + "\n\n")

	// Load balancers
	b.WriteString(s.sectionHeader("로드밸런서 목록", s.focus == 1))
	if len(s.lbs) == 0 {
		b.WriteString(styles.StyleMuted.Render("  없음 — [a]를 눌러 추가") + "\n")
	}
	for i, lb := range s.lbs {
		mark := "  "
		if i == s.lbCursor && s.focus == 1 {
			mark = styles.StyleSelected.Render("▶ ")
		}
		nodes := strings.Join(lb.Nodes, ", ")
		if nodes == "" {
			nodes = styles.StyleMuted.Render("(노드 없음)")
		}
		b.WriteString(fmt.Sprintf("%s%s  vip: %s  nodes: %s\n", mark, styles.StylePrimary.Render(lb.Name), lb.VIP, nodes))
	}
	if s.editingLB {
		b.WriteString("\n" + s.viewLBForm())
	}
	b.WriteString("\n")

	// Default ingress
	b.WriteString(s.sectionHeader("기본 인그레스 클래스", s.focus == 2))
	lbName := styles.StyleMuted.Render("(로드밸런서 없음)")
	if s.ingressLBIdx >= 0 && s.ingressLBIdx < len(s.lbs) {
		lbName = styles.StylePrimary.Render(s.lbs[s.ingressLBIdx].Name)
	}
	b.WriteString("  로드밸런서: " + lbName + styles.StyleMuted.Render("  (←/→ 로 선택)") + "\n")
	b.WriteString("  호스트 포트: " + s.ingressPort.View() + "\n")

	return b.String()
}

func (s *S4Kubernetes) viewLBForm() string {
	var b strings.Builder
	title := "새 로드밸런서"
	if s.lbEditIdx >= 0 {
		title = "로드밸런서 편집"
	}
	b.WriteString(styles.StyleBox.Render(
		styles.StyleSelected.Render(title) + "\n\n" +
			renderField("이름", s.lbName.View(), s.lbFormField == 0) +
			renderField("VIP", s.lbVIP.View(), s.lbFormField == 1) +
			s.renderNodeChecks(),
	))
	return b.String()
}

func (s *S4Kubernetes) renderNodeChecks() string {
	var b strings.Builder
	label := styles.StyleLabel.Render("노드:")
	if s.lbFormField == 2 {
		label = styles.StyleLabelFocused.Render("노드:")
	}
	b.WriteString(label + "\n")
	for i, n := range s.lbNodes {
		mark := styles.CheckOff
		if s.lbChecks[i] {
			mark = styles.CheckOn
		}
		b.WriteString(fmt.Sprintf("  [%d] %s %s\n", i+1, mark, n))
	}
	b.WriteString(styles.StyleMuted.Render("  숫자 키(1-9)로 토글, enter로 저장") + "\n")
	return b.String()
}

func (s *S4Kubernetes) sectionHeader(label string, focused bool) string {
	if focused {
		return styles.StyleLabelFocused.Render("▶ " + label) + "\n"
	}
	return styles.StyleLabel.Render("  " + label) + "\n"
}

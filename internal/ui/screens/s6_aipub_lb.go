package screens

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The load balancers of AIPub: aipub_load_balancers and aipub_default_ingress_class
// of files/aipub/vars.yml, which cilium announces over L2 (network module of
// aipub-installer). They were k8s_load_balancers of k8s-installer before it left
// the network to aipub-installer. Shown only when AIPub is installed.
//
// Focus layout:
//   0  : 인그레스 모드 (1=별도IP / 2=공유IP)
//   1  : Ingress LB명
//   2  : Ingress VIP
//   3  : Gateway LB명 — mode 1 전용
//   4  : Gateway VIP  — mode 1 전용
//   5  : Host Network Port
//   6  : nav slot (←/→: 이전/다음)

const (
	s6FocusMode = iota
	s6FocusIngressLBName
	s6FocusIngressVIP
	s6FocusGatewayLBName
	s6FocusGatewayVIP
	s6FocusHostPort
	s6FocusNav
	s6FocusMax = s6FocusNav
)

type S6AipubLB struct {
	ingressLBName textinput.Model
	ingressVIP    textinput.Model
	gatewayLBName textinput.Model
	gatewayVIP    textinput.Model
	hostPort      textinput.Model
	lbNodes       []string // the k8s control plane nodes, which answer ARP for the VIPs

	mode     int // 1 or 2
	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS6AipubLB() *S6AipubLB {
	newInput := func(placeholder string, limit int) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = limit
		return t
	}
	return &S6AipubLB{
		ingressLBName: newInput("예: lb1", 64),
		ingressVIP:    newInput("x.x.x.x", 64),
		gatewayLBName: newInput("예: lb2", 64),
		gatewayVIP:    newInput("x.x.x.x", 64),
		hostPort:      newInput("443", 8),
		mode:          2,
	}
}

func (s *S6AipubLB) Title() string { return "AIPub 로드밸런서" }
func (s *S6AipubLB) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S6AipubLB) SyncFromState(st *state.AppState) {
	s.mode = st.K8sIngressMode
	if s.mode != 1 && s.mode != 2 {
		s.mode = 2
	}
	s.ingressLBName.SetValue(st.K8sIngressLBName)
	s.ingressVIP.SetValue(st.K8sIngressVIP)
	s.gatewayLBName.SetValue(st.K8sGatewayLBName)
	s.gatewayVIP.SetValue(st.K8sGatewayVIP)
	if st.K8sIngressHostNetworkPort > 0 {
		s.hostPort.SetValue(strconv.Itoa(st.K8sIngressHostNetworkPort))
	}
	s.lbNodes = append([]string{}, st.K8sCpNodes...)
	s.focusIdx = 0
	s.navIdx = 0
	s.syncFocus()
}

func (s *S6AipubLB) SyncToState(st *state.AppState) {
	st.K8sIngressMode = s.mode
	st.K8sIngressLBName = strings.TrimSpace(s.ingressLBName.Value())
	st.K8sIngressVIP = strings.TrimSpace(s.ingressVIP.Value())
	if s.mode == 1 {
		st.K8sGatewayLBName = strings.TrimSpace(s.gatewayLBName.Value())
		st.K8sGatewayVIP = strings.TrimSpace(s.gatewayVIP.Value())
	} else {
		st.K8sGatewayLBName = ""
		st.K8sGatewayVIP = ""
	}
	if p, err := strconv.Atoi(s.hostPort.Value()); err == nil {
		st.K8sIngressHostNetworkPort = p
	}
}

func (s *S6AipubLB) Validate() []string {
	if strings.TrimSpace(s.ingressLBName.Value()) == "" {
		return []string{"Ingress LB명을 입력해주세요"}
	}
	if strings.TrimSpace(s.ingressVIP.Value()) == "" {
		return []string{"Ingress VIP를 입력해주세요"}
	}
	if s.mode == 1 {
		if strings.TrimSpace(s.gatewayLBName.Value()) == "" {
			return []string{"별도 IP 모드에서는 Gateway LB명을 입력해주세요"}
		}
		if strings.TrimSpace(s.gatewayLBName.Value()) == strings.TrimSpace(s.ingressLBName.Value()) {
			return []string{"별도 IP 모드에서는 Ingress LB명과 Gateway LB명이 달라야 합니다"}
		}
		gw := strings.TrimSpace(s.gatewayVIP.Value())
		if gw == "" {
			return []string{"별도 IP 모드에서는 Gateway VIP를 입력해주세요"}
		}
		if gw == strings.TrimSpace(s.ingressVIP.Value()) {
			return []string{"별도 IP 모드에서는 Ingress VIP와 Gateway VIP가 달라야 합니다"}
		}
	}
	p, err := strconv.Atoi(s.hostPort.Value())
	if err != nil || p < 1 || p > 65535 {
		return []string{"Host Network Port는 1-65535 범위여야 합니다"}
	}
	return nil
}

func (s *S6AipubLB) Init() tea.Cmd { return nil }

func (s *S6AipubLB) syncFocus() {
	s.ingressLBName.Blur()
	s.ingressVIP.Blur()
	s.gatewayLBName.Blur()
	s.gatewayVIP.Blur()
	s.hostPort.Blur()
	switch s.focusIdx {
	case s6FocusIngressLBName:
		s.ingressLBName.Focus()
	case s6FocusIngressVIP:
		s.ingressVIP.Focus()
	case s6FocusGatewayLBName:
		s.gatewayLBName.Focus()
	case s6FocusGatewayVIP:
		s.gatewayVIP.Focus()
	case s6FocusHostPort:
		s.hostPort.Focus()
	}
}

func (s *S6AipubLB) nextFocus() int {
	next := s.focusIdx + 1
	if s.mode == 2 && (next == s6FocusGatewayLBName || next == s6FocusGatewayVIP) {
		next = s6FocusHostPort
	}
	return next
}

func (s *S6AipubLB) prevFocus() int {
	prev := s.focusIdx - 1
	if s.mode == 2 && (prev == s6FocusGatewayVIP || prev == s6FocusGatewayLBName) {
		prev = s6FocusIngressVIP
	}
	return prev
}

func (s *S6AipubLB) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Letters typed into a text input are text: h/j/k/l move only elsewhere
		if msg.Type == tea.KeyRunes && s.focusIdx != s6FocusMode && s.focusIdx != s6FocusNav {
			break
		}
		switch msg.String() {
		case "up", "k":
			if prev := s.prevFocus(); prev >= 0 {
				s.focusIdx = prev
				s.syncFocus()
			}
		case "down", "j":
			if next := s.nextFocus(); next <= s6FocusMax {
				if next == s6FocusNav {
					s.navIdx = 1 // 다음
				}
				s.focusIdx = next
				s.syncFocus()
			}
		case "left", "h":
			switch s.focusIdx {
			case s6FocusMode:
				if s.mode > 1 {
					s.mode--
				}
			case s6FocusNav:
				if s.navIdx > 0 {
					s.navIdx--
				}
			}
		case "right", "l":
			switch s.focusIdx {
			case s6FocusMode:
				if s.mode < 2 {
					s.mode++
				}
			case s6FocusNav:
				if s.navIdx < 1 {
					s.navIdx++
				}
			}
		case "enter", " ":
			switch s.focusIdx {
			case s6FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s6FocusMode:
				if s.mode == 1 {
					s.mode = 2
				} else {
					s.mode = 1
				}
			}
		}
	}
	var cmd tea.Cmd
	switch s.focusIdx {
	case s6FocusIngressLBName:
		s.ingressLBName, cmd = s.ingressLBName.Update(msg)
	case s6FocusIngressVIP:
		s.ingressVIP, cmd = s.ingressVIP.Update(msg)
	case s6FocusGatewayLBName:
		s.gatewayLBName, cmd = s.gatewayLBName.Update(msg)
	case s6FocusGatewayVIP:
		s.gatewayVIP, cmd = s.gatewayVIP.Update(msg)
	case s6FocusHostPort:
		s.hostPort, cmd = s.hostPort.Update(msg)
	}
	return s, cmd
}

func (s *S6AipubLB) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("AIPub 로드밸런서") + "\n")
	b.WriteString(styles.StyleMuted.Render("Cilium이 L2로 알리는 VIP입니다 (files/aipub/vars.yml → aipub_load_balancers).") + "\n")
	nodes := "(2단계에서 Master Node를 지정하세요)"
	if len(s.lbNodes) > 0 {
		nodes = strings.Join(s.lbNodes, ", ")
	}
	b.WriteString(styles.StyleMuted.Render("VIP에 응답하는 노드: "+nodes+" (Master Node)") + "\n\n")

	f := s.focusIdx

	mode1Radio := styles.RadioOff() + " 1. 별도 IP"
	mode2Radio := styles.RadioOff() + " 2. 공유 IP"
	if s.mode == 1 {
		mode1Radio = styles.RadioOn() + " 1. 별도 IP"
	} else {
		mode2Radio = styles.RadioOn() + " 2. 공유 IP"
	}
	modeHint := ""
	if f == s6FocusMode {
		modeHint = styles.StyleMuted.Render("  ← → 또는 Enter")
	}
	b.WriteString(RenderSectionHeader("인그레스 모드", f == s6FocusMode) + modeHint + "\n")
	b.WriteString("    " + mode1Radio + styles.StyleMuted.Render("  — Ingress/Gateway 개별 VIP") + "\n")
	b.WriteString("    " + mode2Radio + styles.StyleMuted.Render("  — Ingress/Gateway 동일 VIP") + "\n\n")

	b.WriteString(RenderSectionHeader("Ingress LB명", f == s6FocusIngressLBName) + "\n")
	b.WriteString("    " + s.ingressLBName.View() + "\n\n")

	if s.mode == 2 {
		b.WriteString(RenderSectionHeader("Ingress/Gateway 공유 VIP", f == s6FocusIngressVIP) + "\n")
	} else {
		b.WriteString(RenderSectionHeader("Ingress VIP", f == s6FocusIngressVIP) + "\n")
	}
	b.WriteString("    " + s.ingressVIP.View() + "\n\n")

	if s.mode == 1 {
		b.WriteString(RenderSectionHeader("Gateway LB명", f == s6FocusGatewayLBName) + "\n")
		b.WriteString("    " + s.gatewayLBName.View() + "\n\n")

		b.WriteString(RenderSectionHeader("Gateway VIP", f == s6FocusGatewayVIP) + "\n")
		b.WriteString("    " + s.gatewayVIP.View() + "\n\n")
	}

	b.WriteString(RenderSectionHeader("Host Network Port", f == s6FocusHostPort) +
		"  " + s.hostPort.View() + "\n\n")

	prevFocused := f == s6FocusNav && s.navIdx == 0
	nextFocused := f == s6FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

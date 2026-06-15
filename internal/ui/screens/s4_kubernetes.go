package screens

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : 인증서 유효기간
//   1  : 인그레스 모드 (1=별도IP / 2=공유IP)
//   2  : Ingress LB명
//   3  : Ingress VIP
//   4  : Gateway LB명 — mode 1 전용
//   5  : Gateway VIP  — mode 1 전용
//   6  : Host Network Port
//   7  : nav slot (←/→: 이전/다음)

const (
	s4FocusCert = iota
	s4FocusMode
	s4FocusIngressLBName
	s4FocusIngressVIP
	s4FocusGatewayLBName
	s4FocusGatewayVIP
	s4FocusHostPort
	s4FocusNav
	s4FocusMax = s4FocusNav
)

type S4Kubernetes struct {
	certValidity  textinput.Model
	ingressLBName textinput.Model
	ingressVIP    textinput.Model
	gatewayLBName textinput.Model
	gatewayVIP    textinput.Model
	hostPort      textinput.Model

	mode     int // 1 or 2
	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS4Kubernetes() *S4Kubernetes {
	newInput := func(placeholder string, limit int) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = limit
		return t
	}
	return &S4Kubernetes{
		certValidity:  newInput("26280h", 16),
		ingressLBName: newInput("예: lb1", 64),
		ingressVIP:    newInput("x.x.x.x", 64),
		gatewayLBName: newInput("예: lb2", 64),
		gatewayVIP:    newInput("x.x.x.x", 64),
		hostPort:      newInput("443", 8),
		mode:          2,
	}
}

func (s *S4Kubernetes) Title() string { return "Kubernetes 설정" }
func (s *S4Kubernetes) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S4Kubernetes) SyncFromState(st *state.AppState) {
	s.certValidity.SetValue(st.K8sCertificateValidityPeriod)
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
	s.focusIdx = 0
	s.navIdx = 0
	s.syncFocus()
}

func (s *S4Kubernetes) SyncToState(st *state.AppState) {
	st.K8sCertificateValidityPeriod = s.certValidity.Value()
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

func (s *S4Kubernetes) Validate() []string {
	if s.certValidity.Value() == "" {
		return []string{"인증서 유효기간을 입력해주세요 (예: 26280h)"}
	}
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

func (s *S4Kubernetes) Init() tea.Cmd { return nil }

func (s *S4Kubernetes) syncFocus() {
	s.certValidity.Blur()
	s.ingressLBName.Blur()
	s.ingressVIP.Blur()
	s.gatewayLBName.Blur()
	s.gatewayVIP.Blur()
	s.hostPort.Blur()
	switch s.focusIdx {
	case s4FocusCert:
		s.certValidity.Focus()
	case s4FocusIngressLBName:
		s.ingressLBName.Focus()
	case s4FocusIngressVIP:
		s.ingressVIP.Focus()
	case s4FocusGatewayLBName:
		s.gatewayLBName.Focus()
	case s4FocusGatewayVIP:
		s.gatewayVIP.Focus()
	case s4FocusHostPort:
		s.hostPort.Focus()
	}
}

func (s *S4Kubernetes) nextFocus() int {
	next := s.focusIdx + 1
	if s.mode == 2 && (next == s4FocusGatewayLBName || next == s4FocusGatewayVIP) {
		next = s4FocusHostPort
	}
	return next
}

func (s *S4Kubernetes) prevFocus() int {
	prev := s.focusIdx - 1
	if s.mode == 2 && (prev == s4FocusGatewayVIP || prev == s4FocusGatewayLBName) {
		prev = s4FocusIngressVIP
	}
	return prev
}

func (s *S4Kubernetes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if prev := s.prevFocus(); prev >= 0 {
				s.focusIdx = prev
				s.syncFocus()
			}
		case "down", "j":
			if next := s.nextFocus(); next <= s4FocusMax {
				s.focusIdx = next
				s.syncFocus()
			}
		case "left", "h":
			switch s.focusIdx {
			case s4FocusMode:
				if s.mode > 1 {
					s.mode--
				}
			case s4FocusNav:
				if s.navIdx > 0 {
					s.navIdx--
				}
			}
		case "right", "l":
			switch s.focusIdx {
			case s4FocusMode:
				if s.mode < 2 {
					s.mode++
				}
			case s4FocusNav:
				if s.navIdx < 1 {
					s.navIdx++
				}
			}
		case "enter", " ":
			switch s.focusIdx {
			case s4FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s4FocusMode:
				if s.mode == 1 {
					s.mode = 2
				} else {
					s.mode = 1
				}
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	switch s.focusIdx {
	case s4FocusCert:
		s.certValidity, cmd = s.certValidity.Update(msg)
	case s4FocusIngressLBName:
		s.ingressLBName, cmd = s.ingressLBName.Update(msg)
	case s4FocusIngressVIP:
		s.ingressVIP, cmd = s.ingressVIP.Update(msg)
	case s4FocusGatewayLBName:
		s.gatewayLBName, cmd = s.gatewayLBName.Update(msg)
	case s4FocusGatewayVIP:
		s.gatewayVIP, cmd = s.gatewayVIP.Update(msg)
	case s4FocusHostPort:
		s.hostPort, cmd = s.hostPort.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("Kubernetes 설정") + "\n\n")

	f := s.focusIdx

	// 인증서 유효기간
	b.WriteString(RenderSectionHeader("인증서 유효기간", f == s4FocusCert) +
		"  " + s.certValidity.View() + styles.StyleMuted.Render("  (예: 26280h = 3년)") + "\n\n")

	// 인그레스 모드 선택
	mode1Radio := styles.RadioOff() + " 1. 별도 IP"
	mode2Radio := styles.RadioOff() + " 2. 공유 IP"
	if s.mode == 1 {
		mode1Radio = styles.RadioOn() + " 1. 별도 IP"
	} else {
		mode2Radio = styles.RadioOn() + " 2. 공유 IP"
	}
	modeHint := ""
	if f == s4FocusMode {
		modeHint = styles.StyleMuted.Render("  ← → 또는 Enter")
	}
	b.WriteString(RenderSectionHeader("인그레스 모드", f == s4FocusMode) + modeHint + "\n")
	b.WriteString("    " + mode1Radio + styles.StyleMuted.Render("  — Ingress/Gateway 개별 VIP") + "\n")
	b.WriteString("    " + mode2Radio + styles.StyleMuted.Render("  — Ingress/Gateway 동일 VIP") + "\n\n")

	// Ingress LB명
	b.WriteString(RenderSectionHeader("Ingress LB명", f == s4FocusIngressLBName) + "\n")
	b.WriteString("    " + s.ingressLBName.View() + "\n\n")

	// Ingress VIP
	if s.mode == 2 {
		b.WriteString(RenderSectionHeader("Ingress/Gateway 공유 VIP", f == s4FocusIngressVIP) + "\n")
	} else {
		b.WriteString(RenderSectionHeader("Ingress VIP", f == s4FocusIngressVIP) + "\n")
	}
	b.WriteString("    " + s.ingressVIP.View() + "\n\n")

	// Gateway LB명 / VIP — mode 1 전용
	if s.mode == 1 {
		b.WriteString(RenderSectionHeader("Gateway LB명", f == s4FocusGatewayLBName) + "\n")
		b.WriteString("    " + s.gatewayLBName.View() + "\n\n")

		b.WriteString(RenderSectionHeader("Gateway VIP", f == s4FocusGatewayVIP) + "\n")
		b.WriteString("    " + s.gatewayVIP.View() + "\n\n")
	}

	// Host Network Port
	b.WriteString(RenderSectionHeader("Host Network Port", f == s4FocusHostPort) +
		"  " + s.hostPort.View() + "\n\n")

	prevFocused := f == s4FocusNav && s.navIdx == 0
	nextFocused := f == s4FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

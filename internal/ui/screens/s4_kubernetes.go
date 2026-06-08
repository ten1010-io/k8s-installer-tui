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
//   1  : ingress class 이름
//   2  : HA 모드 토글
//   3  : VIP 입력
//   4  : HTTP 포트
//   5  : HTTPS 포트
//   6  : nav slot (←/→: 이전/다음)
//
// controller_nodes는 ki_cp_node 선택을 자동 사용 (UI 없음).

const (
	s4FocusCert = iota
	s4FocusName
	s4FocusHaMode
	s4FocusVIP
	s4FocusHTTP
	s4FocusHTTPS
	s4FocusNav
	s4FocusMax = s4FocusNav
)

type S4Kubernetes struct {
	certValidity textinput.Model
	ingressName  textinput.Model
	haMode       bool
	vip          textinput.Model
	httpPort     textinput.Model
	httpsPort    textinput.Model

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
		certValidity: newInput("26280h", 16),
		ingressName:  newInput("lb1", 64),
		vip:          newInput("예: 192.168.0.100", 64),
		httpPort:     newInput("80", 8),
		httpsPort:    newInput("443", 8),
	}
}

func (s *S4Kubernetes) Title() string { return "Kubernetes 설정" }
func (s *S4Kubernetes) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S4Kubernetes) SyncFromState(st *state.AppState) {
	s.certValidity.SetValue(st.K8sCertificateValidityPeriod)
	s.ingressName.SetValue(st.K8sIngressClassName)
	s.haMode = st.K8sIngressHaMode
	s.vip.SetValue(st.K8sIngressHaModeVIP)
	if st.K8sIngressHttpPort > 0 {
		s.httpPort.SetValue(strconv.Itoa(st.K8sIngressHttpPort))
	}
	if st.K8sIngressHttpsPort > 0 {
		s.httpsPort.SetValue(strconv.Itoa(st.K8sIngressHttpsPort))
	}
	s.focusIdx = 0
	s.navIdx = 0
	s.syncFocus()
}

func (s *S4Kubernetes) SyncToState(st *state.AppState) {
	st.K8sCertificateValidityPeriod = s.certValidity.Value()
	st.K8sIngressClassName = strings.TrimSpace(s.ingressName.Value())
	st.K8sIngressHaMode = s.haMode
	st.K8sIngressHaModeVIP = strings.TrimSpace(s.vip.Value())
	if p, err := strconv.Atoi(s.httpPort.Value()); err == nil {
		st.K8sIngressHttpPort = p
	}
	if p, err := strconv.Atoi(s.httpsPort.Value()); err == nil {
		st.K8sIngressHttpsPort = p
	}
}

func (s *S4Kubernetes) Validate() []string {
	if s.certValidity.Value() == "" {
		return []string{"인증서 유효기간을 입력해주세요 (예: 26280h)"}
	}
	if strings.TrimSpace(s.ingressName.Value()) == "" {
		return []string{"Ingress class 이름을 입력해주세요"}
	}
	if s.haMode && strings.TrimSpace(s.vip.Value()) == "" {
		return []string{"HA 모드 활성화 시 VIP를 입력해주세요"}
	}
	httpP, err1 := strconv.Atoi(s.httpPort.Value())
	httpsP, err2 := strconv.Atoi(s.httpsPort.Value())
	if err1 != nil || httpP < 1 || httpP > 65535 {
		return []string{"HTTP 포트는 1-65535 범위여야 합니다"}
	}
	if err2 != nil || httpsP < 1 || httpsP > 65535 {
		return []string{"HTTPS 포트는 1-65535 범위여야 합니다"}
	}
	return nil
}

func (s *S4Kubernetes) Init() tea.Cmd { return nil }

func (s *S4Kubernetes) syncFocus() {
	s.certValidity.Blur()
	s.ingressName.Blur()
	s.vip.Blur()
	s.httpPort.Blur()
	s.httpsPort.Blur()
	switch s.focusIdx {
	case s4FocusCert:
		s.certValidity.Focus()
	case s4FocusName:
		s.ingressName.Focus()
	case s4FocusVIP:
		s.vip.Focus()
	case s4FocusHTTP:
		s.httpPort.Focus()
	case s4FocusHTTPS:
		s.httpsPort.Focus()
	}
}

func (s *S4Kubernetes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx > 0 {
				s.focusIdx--
				s.syncFocus()
			}
		case "down", "j":
			if s.focusIdx < s4FocusMax {
				s.focusIdx++
				s.syncFocus()
			}
		case "left", "h":
			if s.focusIdx == s4FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s4FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s4FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s4FocusHaMode:
				s.haMode = !s.haMode
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
	case s4FocusName:
		s.ingressName, cmd = s.ingressName.Update(msg)
	case s4FocusVIP:
		s.vip, cmd = s.vip.Update(msg)
	case s4FocusHTTP:
		s.httpPort, cmd = s.httpPort.Update(msg)
	case s4FocusHTTPS:
		s.httpsPort, cmd = s.httpsPort.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("Kubernetes 설정") + "\n\n")

	f := s.focusIdx

	b.WriteString(RenderSectionHeader("인증서 유효기간", f == s4FocusCert) +
		"  " + s.certValidity.View() + styles.StyleMuted.Render("  (예: 26280h = 3년)") + "\n\n")

	b.WriteString(RenderSectionHeader("Ingress Class 이름", f == s4FocusName) +
		"  " + s.ingressName.View() + "\n\n")

	haStr := styles.RadioOff() + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn() + " 활성"
	}
	b.WriteString(RenderSectionHeader("HA 모드", f == s4FocusHaMode) + "  " + haStr + "\n\n")

	b.WriteString(RenderSectionHeader("HA VIP", f == s4FocusVIP) + "\n")
	b.WriteString("    " + s.vip.View() + "\n")
	if !s.haMode {
		b.WriteString("    " + styles.StyleMuted.Render("HA 활성 시 필수 — 예: 192.168.0.100") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(RenderSectionHeader("HTTP 포트", f == s4FocusHTTP) +
		"  " + s.httpPort.View() + "\n\n")

	b.WriteString(RenderSectionHeader("HTTPS 포트", f == s4FocusHTTPS) +
		"  " + s.httpsPort.View() + "\n\n")

	b.WriteString(styles.StyleMuted.Render("  ※ controller_nodes는 2단계 ki_cp_node 선택과 동일하게 적용됩니다") + "\n")

	prevFocused := f == s4FocusNav && s.navIdx == 0
	nextFocused := f == s4FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

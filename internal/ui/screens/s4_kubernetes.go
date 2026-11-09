package screens

import (
	"net"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The cluster as k8s-installer builds it (group_vars/all/vars.yml). The load
// balancers that were here belong to AIPub now (S6AipubLB).
//
// Focus layout:
//   0  : k8s 마이너 버전
//   1  : 파드 서브넷
//   2  : 서비스 서브넷
//   3  : 인증서 유효기간
//   4  : nav slot (←/→: 이전/다음)

const (
	s4FocusMinor = iota
	s4FocusPodSubnet
	s4FocusServiceSubnet
	s4FocusCert
	s4FocusNav
	s4FocusMax = s4FocusNav
)

var reMinorVersion = regexp.MustCompile(`^1\.[0-9]+$`)

type S4Kubernetes struct {
	minorVersion  textinput.Model
	podSubnet     textinput.Model
	serviceSubnet textinput.Model
	certValidity  textinput.Model

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
		minorVersion:  newInput("1.36", 8),
		podSubnet:     newInput("10.244.0.0/16", 32),
		serviceSubnet: newInput("10.96.0.0/12", 32),
		certValidity:  newInput("26280h", 16),
	}
}

func (s *S4Kubernetes) Title() string { return "Kubernetes 설정" }
func (s *S4Kubernetes) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S4Kubernetes) inputs() []*textinput.Model {
	return []*textinput.Model{&s.minorVersion, &s.podSubnet, &s.serviceSubnet, &s.certValidity}
}

func (s *S4Kubernetes) SyncFromState(st *state.AppState) {
	s.minorVersion.SetValue(st.K8sMinorVersion)
	s.podSubnet.SetValue(st.K8sPodSubnet)
	s.serviceSubnet.SetValue(st.K8sServiceSubnet)
	s.certValidity.SetValue(st.K8sCertificateValidityPeriod)
	s.focusIdx = 0
	s.navIdx = 0
	s.syncFocus()
}

func (s *S4Kubernetes) SyncToState(st *state.AppState) {
	st.K8sMinorVersion = strings.TrimSpace(s.minorVersion.Value())
	st.K8sPodSubnet = strings.TrimSpace(s.podSubnet.Value())
	st.K8sServiceSubnet = strings.TrimSpace(s.serviceSubnet.Value())
	st.K8sCertificateValidityPeriod = strings.TrimSpace(s.certValidity.Value())
}

func (s *S4Kubernetes) Validate() []string {
	if !reMinorVersion.MatchString(strings.TrimSpace(s.minorVersion.Value())) {
		return []string{"쿠버네티스 마이너 버전 형식이 올바르지 않습니다 (예: 1.36)"}
	}
	for _, f := range []struct{ name, val string }{
		{"파드 서브넷", s.podSubnet.Value()},
		{"서비스 서브넷", s.serviceSubnet.Value()},
	} {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(f.val)); err != nil {
			return []string{f.name + "이 CIDR 형식이 아닙니다 (예: 10.244.0.0/16)"}
		}
	}
	if strings.TrimSpace(s.certValidity.Value()) == "" {
		return []string{"인증서 유효기간을 입력해주세요 (예: 26280h)"}
	}
	return nil
}

func (s *S4Kubernetes) Init() tea.Cmd { return nil }

func (s *S4Kubernetes) syncFocus() {
	for i, inp := range s.inputs() {
		if i == s.focusIdx {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (s *S4Kubernetes) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if s.focusIdx > 0 {
				s.focusIdx--
				s.syncFocus()
			}
		case "down":
			if s.focusIdx < s4FocusMax {
				s.focusIdx++
				s.syncFocus()
			}
		case "left":
			if s.focusIdx == s4FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right":
			if s.focusIdx == s4FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter":
			if s.focusIdx == s4FocusNav {
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			}
			if s.focusIdx < s4FocusNav {
				s.focusIdx++
				s.syncFocus()
			}
			return s, nil
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	if s.focusIdx < len(s.inputs()) {
		inp := s.inputs()[s.focusIdx]
		*inp, cmd = inp.Update(msg)
	}
	return s, cmd
}

func (s *S4Kubernetes) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("Kubernetes 설정") + "\n")
	b.WriteString(styles.StyleMuted.Render("k8s-installer가 클러스터를 만들 때 쓰는 값입니다 (group_vars/all/vars.yml).") + "\n\n")

	f := s.focusIdx
	b.WriteString(RenderSectionHeader("마이너 버전", f == s4FocusMinor) + "  " + s.minorVersion.View() +
		styles.StyleMuted.Render("  "+short("k8s_minor_version")) + "\n\n")
	b.WriteString(RenderSectionHeader("파드 서브넷", f == s4FocusPodSubnet) + "  " + s.podSubnet.View() + "\n")
	b.WriteString(RenderSectionHeader("서비스 서브넷", f == s4FocusServiceSubnet) + "  " + s.serviceSubnet.View() + "\n")
	b.WriteString(styles.StyleMuted.Render("    노드·서비스가 쓰는 망과 겹치면 안 됩니다. 클러스터를 만든 뒤에는 바꿀 수 없습니다") + "\n\n")
	b.WriteString(RenderSectionHeader("인증서 유효기간", f == s4FocusCert) + "  " + s.certValidity.View() +
		styles.StyleMuted.Render("  (예: 26280h = 3년)") + "\n\n")

	b.WriteString(styles.StyleMuted.Render(short("k8s_kube_proxy_enabled")+". 5단계에서 정해집니다") + "\n")

	prevFocused := f == s4FocusNav && s.navIdx == 0
	nextFocused := f == s4FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))
	return b.String()
}

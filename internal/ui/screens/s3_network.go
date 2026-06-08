package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : subnet list
//   1  : HA mode toggle
//   2  : VIP input
//   3  : DNSSEC toggle
//   4  : DNS servers list
//   5  : NTP servers list
//   6  : extra zone input
//   7  : nav slot (←/→: 이전/다음)

const (
	s3FocusSubnet = iota
	s3FocusHaMode
	s3FocusVIP
	s3FocusDNSSEC
	s3FocusDNS
	s3FocusNTP
	s3FocusExtraZone
	s3FocusNav
	s3FocusMax = s3FocusNav
)

type editKind int

const (
	editNone   editKind = iota
	editSubnet          // adding to s.subnets
	editDNS             // adding to s.dnsServers
	editNTP             // adding to s.ntpServers
)

type S3Network struct {
	subnets    []string
	haMode     bool
	vip        textinput.Model
	dnssec     bool
	dnsServers []string
	ntpServers []string
	extraZone  textinput.Model

	// shared inline input for subnet / DNS / NTP entry
	editMode  editKind
	listInput textinput.Model

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS3Network() *S3Network {
	s := &S3Network{}

	s.vip = textinput.New()
	s.vip.Placeholder = "예: 192.168.0.100"
	s.vip.CharLimit = 64

	s.extraZone = textinput.New()
	s.extraZone.Placeholder = "example.com (선택사항)"
	s.extraZone.CharLimit = 253

	s.listInput = textinput.New()
	s.listInput.CharLimit = 253

	return s
}

func (s *S3Network) Title() string { return "네트워크 설정" }
func (s *S3Network) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S3Network) SyncFromState(st *state.AppState) {
	s.subnets = append([]string{}, st.InternalNetworkSubnets...)
	s.haMode = st.KiCpHaMode
	s.vip.SetValue(st.KiCpHaModeVIP)
	s.dnssec = st.KiCpDnsDnssecValidation
	s.dnsServers = append([]string{}, st.KiCpDnsUpstreamServers...)
	s.ntpServers = append([]string{}, st.KiCpNtpUpstreamServers...)
	s.extraZone.SetValue(st.InternalNetworkExtraZone)
	s.focusIdx = 0
	s.navIdx = 0
	s.editMode = editNone
	s.syncFocusedInputs()
}

func (s *S3Network) SyncToState(st *state.AppState) {
	st.InternalNetworkSubnets = append([]string{}, s.subnets...)
	st.KiCpHaMode = s.haMode
	st.KiCpHaModeVIP = s.vip.Value()
	st.KiCpDnsDnssecValidation = s.dnssec
	st.KiCpDnsUpstreamServers = append([]string{}, s.dnsServers...)
	st.KiCpNtpUpstreamServers = append([]string{}, s.ntpServers...)
	st.InternalNetworkExtraZone = s.extraZone.Value()
}

func (s *S3Network) Validate() []string {
	if len(s.subnets) == 0 {
		return []string{"내부 서브넷을 최소 1개 이상 입력해주세요"}
	}
	if s.haMode && strings.TrimSpace(s.vip.Value()) == "" {
		return []string{"HA 모드 활성화 시 VIP를 입력해주세요"}
	}
	if len(s.dnsServers) == 0 {
		return []string{"DNS upstream 서버를 최소 1개 입력해주세요"}
	}
	if len(s.ntpServers) == 0 {
		return []string{"NTP upstream 서버를 최소 1개 입력해주세요"}
	}
	return nil
}

func (s *S3Network) Init() tea.Cmd { return nil }

func (s *S3Network) syncFocusedInputs() {
	s.vip.Blur()
	s.extraZone.Blur()
	if s.editMode != editNone {
		return
	}
	switch s.focusIdx {
	case s3FocusVIP:
		s.vip.Focus()
	case s3FocusExtraZone:
		s.extraZone.Focus()
	}
}

func (s *S3Network) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.editMode != editNone {
		return s.updateEdit(msg)
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
			if s.focusIdx < s3FocusMax {
				s.focusIdx++
				s.syncFocusedInputs()
			}
		case "left", "h":
			if s.focusIdx == s3FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s3FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			return s.activate()
		case "d", "delete":
			s.deleteLastItem()
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	switch s.focusIdx {
	case s3FocusVIP:
		s.vip, cmd = s.vip.Update(msg)
	case s3FocusExtraZone:
		s.extraZone, cmd = s.extraZone.Update(msg)
	}
	return s, cmd
}

func (s *S3Network) activate() (tea.Model, tea.Cmd) {
	switch s.focusIdx {
	case s3FocusNav:
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	case s3FocusSubnet:
		s.startEdit(editSubnet, "예: 192.168.0.0/24")
		return s, textinput.Blink
	case s3FocusHaMode:
		s.haMode = !s.haMode
	case s3FocusDNSSEC:
		s.dnssec = !s.dnssec
	case s3FocusDNS:
		s.startEdit(editDNS, "8.8.8.8")
		return s, textinput.Blink
	case s3FocusNTP:
		s.startEdit(editNTP, "time1.google.com")
		return s, textinput.Blink
	}
	return s, nil
}

func (s *S3Network) startEdit(mode editKind, placeholder string) {
	s.editMode = mode
	s.listInput.SetValue("")
	s.listInput.Placeholder = placeholder
	s.listInput.Focus()
}

func (s *S3Network) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editMode = editNone
			s.listInput.Blur()
			s.syncFocusedInputs()
			return s, nil
		case "enter":
			if v := strings.TrimSpace(s.listInput.Value()); v != "" {
				switch s.editMode {
				case editSubnet:
					s.subnets = append(s.subnets, v)
				case editDNS:
					s.dnsServers = append(s.dnsServers, v)
				case editNTP:
					s.ntpServers = append(s.ntpServers, v)
				}
			}
			s.editMode = editNone
			s.listInput.Blur()
			s.syncFocusedInputs()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.listInput, cmd = s.listInput.Update(msg)
	return s, cmd
}

func (s *S3Network) deleteLastItem() {
	switch s.focusIdx {
	case s3FocusSubnet:
		if len(s.subnets) > 0 {
			s.subnets = s.subnets[:len(s.subnets)-1]
		}
	case s3FocusDNS:
		if len(s.dnsServers) > 0 {
			s.dnsServers = s.dnsServers[:len(s.dnsServers)-1]
		}
	case s3FocusNTP:
		if len(s.ntpServers) > 0 {
			s.ntpServers = s.ntpServers[:len(s.ntpServers)-1]
		}
	}
}

func (s *S3Network) renderListSection(b *strings.Builder, label string, items []string, focused bool, mode editKind) {
	b.WriteString(RenderSectionHeader(label, focused) + "\n")
	for _, item := range items {
		b.WriteString("    " + styles.StylePrimary.Render("•") + " " + item + "\n")
	}
	if s.editMode == mode {
		b.WriteString("    " + s.listInput.View() + "\n")
	} else if focused {
		b.WriteString(styles.StyleMuted.Render("    Enter: 추가  d: 마지막 삭제") + "\n")
	}
	b.WriteString("\n")
}

func (s *S3Network) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("네트워크 설정") + "\n\n")

	s.renderListSection(&b, "내부 서브넷 (CIDR)", s.subnets, s.focusIdx == s3FocusSubnet, editSubnet)

	haStr := styles.RadioOff() + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn() + " 활성"
	}
	b.WriteString(RenderSectionHeader("Control Plane HA 모드", s.focusIdx == s3FocusHaMode) + "  " + haStr + "\n\n")

	b.WriteString(RenderSectionHeader("HA VIP", s.focusIdx == s3FocusVIP) + "\n")
	b.WriteString("    " + s.vip.View() + "\n")
	if !s.haMode {
		b.WriteString("    " + styles.StyleMuted.Render("HA 활성 시 필수 — 예: 192.168.0.100") + "\n")
	}
	b.WriteString("\n")

	dnssecStr := styles.RadioOff() + " 비활성"
	if s.dnssec {
		dnssecStr = styles.RadioOn() + " 활성"
	}
	b.WriteString(RenderSectionHeader("DNSSEC 검증", s.focusIdx == s3FocusDNSSEC) + "  " + dnssecStr + "\n\n")

	s.renderListSection(&b, "DNS upstream 서버", s.dnsServers, s.focusIdx == s3FocusDNS, editDNS)
	s.renderListSection(&b, "NTP upstream 서버", s.ntpServers, s.focusIdx == s3FocusNTP, editNTP)

	b.WriteString(RenderSectionHeader("추가 DNS 존 (선택)", s.focusIdx == s3FocusExtraZone) + "  " + s.extraZone.View() + "\n")

	prevFocused := s.focusIdx == s3FocusNav && s.navIdx == 0
	nextFocused := s.focusIdx == s3FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

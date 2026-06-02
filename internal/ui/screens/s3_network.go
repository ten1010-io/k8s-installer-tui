package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : subnets section
//   1  : HA mode toggle
//   2  : VIP input
//   3  : DNSSEC toggle
//   4  : DNS servers section
//   5  : NTP servers section
//   6  : extra zone input
//   7  : nav slot (←/→: 이전/다음)

const (
	s3FocusSubnets = iota
	s3FocusHaMode
	s3FocusVIP
	s3FocusDNSSEC
	s3FocusDNS
	s3FocusNTP
	s3FocusExtraZone
	s3FocusNav
	s3FocusMax = s3FocusNav
)

type S3Network struct {
	subnets    []string
	haMode     bool
	vip        textinput.Model
	dnssec     bool
	dnsServers []string
	ntpServers []string
	extraZone  textinput.Model

	// inline editing state
	editingList  bool   // editing a list (subnet/dns/ntp)
	editListFor  int    // which section: s3FocusSubnets, s3FocusDNS, s3FocusNTP
	listInput    textinput.Model

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS3Network() *S3Network {
	s := &S3Network{dnssec: true}

	s.vip = textinput.New()
	s.vip.Placeholder = "192.168.0.100"
	s.vip.CharLimit = 64

	s.extraZone = textinput.New()
	s.extraZone.Placeholder = "example.com (선택사항)"
	s.extraZone.CharLimit = 253

	s.listInput = textinput.New()
	s.listInput.CharLimit = 253

	s.syncFocusedInputs()
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
	s.editingList = false
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
	if s.haMode && s.vip.Value() == "" {
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
	if !s.editingList {
		if s.focusIdx == s3FocusVIP {
			s.vip.Focus()
		} else if s.focusIdx == s3FocusExtraZone {
			s.extraZone.Focus()
		}
	}
}

func (s *S3Network) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if s.editingList {
		return s.updateListEdit(msg)
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
			return s.activate(msg.String())
		case "d", "delete":
			s.deleteLastItem(s.focusIdx)
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	var cmd tea.Cmd
	if s.focusIdx == s3FocusVIP {
		s.vip, cmd = s.vip.Update(msg)
	} else if s.focusIdx == s3FocusExtraZone {
		s.extraZone, cmd = s.extraZone.Update(msg)
	}
	return s, cmd
}

func (s *S3Network) activate(key string) (tea.Model, tea.Cmd) {
	switch s.focusIdx {
	case s3FocusNav:
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	case s3FocusHaMode:
		s.haMode = !s.haMode
	case s3FocusDNSSEC:
		s.dnssec = !s.dnssec
	case s3FocusSubnets, s3FocusDNS, s3FocusNTP:
		s.editingList = true
		s.editListFor = s.focusIdx
		s.listInput.SetValue("")
		switch s.focusIdx {
		case s3FocusSubnets:
			s.listInput.Placeholder = "192.168.0.0/24"
		case s3FocusDNS:
			s.listInput.Placeholder = "8.8.8.8"
		case s3FocusNTP:
			s.listInput.Placeholder = "time1.google.com"
		}
		s.listInput.Focus()
		return s, textinput.Blink
	}
	return s, nil
}

func (s *S3Network) updateListEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingList = false
			s.listInput.Blur()
			return s, nil
		case "enter":
			v := strings.TrimSpace(s.listInput.Value())
			if v != "" {
				switch s.editListFor {
				case s3FocusSubnets:
					s.subnets = append(s.subnets, v)
				case s3FocusDNS:
					s.dnsServers = append(s.dnsServers, v)
				case s3FocusNTP:
					s.ntpServers = append(s.ntpServers, v)
				}
			}
			s.editingList = false
			s.listInput.Blur()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.listInput, cmd = s.listInput.Update(msg)
	return s, cmd
}

func (s *S3Network) deleteLastItem(section int) {
	switch section {
	case s3FocusSubnets:
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

func (s *S3Network) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("네트워크 설정") + "\n\n")

	// Subnets
	subFocused := s.focusIdx == s3FocusSubnets
	b.WriteString(RenderSectionHeader("내부 서브넷 (CIDR)", subFocused) + "\n")
	for _, sub := range s.subnets {
		b.WriteString("    " + styles.StylePrimary.Render("•") + " " + sub + "\n")
	}
	if s.editingList && s.editListFor == s3FocusSubnets {
		b.WriteString("    " + s.listInput.View() + "\n")
	} else if subFocused {
		b.WriteString(styles.StyleMuted.Render("    Enter: 추가  d: 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	// HA mode
	haFocused := s.focusIdx == s3FocusHaMode
	haStr := styles.RadioOff + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn + " 활성"
	}
	b.WriteString(RenderSectionHeader("Control Plane HA 모드", haFocused) + "  " + haStr + "\n\n")

	// VIP
	vipFocused := s.focusIdx == s3FocusVIP
	vipView := s.vip.View()
	if !s.haMode {
		vipView = styles.StyleMuted.Render("(HA 비활성 — 불필요)")
	}
	b.WriteString(RenderSectionHeader("HA VIP", vipFocused) + "  " + vipView + "\n\n")

	// DNSSEC
	dnsFocused := s.focusIdx == s3FocusDNSSEC
	dnssecStr := styles.RadioOff + " 비활성"
	if s.dnssec {
		dnssecStr = styles.RadioOn + " 활성"
	}
	b.WriteString(RenderSectionHeader("DNSSEC 검증", dnsFocused) + "  " + dnssecStr + "\n\n")

	// DNS upstream
	dnsSecFocused := s.focusIdx == s3FocusDNS
	b.WriteString(RenderSectionHeader("DNS upstream 서버", dnsSecFocused) + "\n")
	for _, srv := range s.dnsServers {
		b.WriteString("    " + styles.StylePrimary.Render("•") + " " + srv + "\n")
	}
	if s.editingList && s.editListFor == s3FocusDNS {
		b.WriteString("    " + s.listInput.View() + "\n")
	} else if dnsSecFocused {
		b.WriteString(styles.StyleMuted.Render("    Enter: 추가  d: 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	// NTP upstream
	ntpFocused := s.focusIdx == s3FocusNTP
	b.WriteString(RenderSectionHeader("NTP upstream 서버", ntpFocused) + "\n")
	for _, srv := range s.ntpServers {
		b.WriteString("    " + styles.StylePrimary.Render("•") + " " + srv + "\n")
	}
	if s.editingList && s.editListFor == s3FocusNTP {
		b.WriteString("    " + s.listInput.View() + "\n")
	} else if ntpFocused {
		b.WriteString(styles.StyleMuted.Render("    Enter: 추가  d: 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	// Extra zone
	extraFocused := s.focusIdx == s3FocusExtraZone
	b.WriteString(RenderSectionHeader("추가 DNS 존 (선택)", extraFocused) + "  " + s.extraZone.View() + "\n")

	prevFocused := s.focusIdx == s3FocusNav && s.navIdx == 0
	nextFocused := s.focusIdx == s3FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

type S3Network struct {
	subnets    []string
	haMode     bool
	vip        textinput.Model
	dnssec     bool
	dnsServers []string
	ntpServers []string
	extraZone  textinput.Model

	// editing state for list items
	editingSubnet  bool
	subnetInput    textinput.Model
	editingDNS     bool
	dnsInput       textinput.Model
	editingNTP     bool
	ntpInput       textinput.Model

	focus  int // which section is focused: 0=subnets, 1=haMode, 2=vip, 3=dnssec, 4=dns, 5=ntp, 6=extraZone
	width  int
	height int
}

func NewS3Network() *S3Network {
	s := &S3Network{dnssec: true}

	s.vip = textinput.New()
	s.vip.Placeholder = "192.168.0.100"
	s.vip.CharLimit = 64

	s.extraZone = textinput.New()
	s.extraZone.Placeholder = "example.com (선택사항)"
	s.extraZone.CharLimit = 253

	s.subnetInput = textinput.New()
	s.subnetInput.Placeholder = "192.168.0.0/24"
	s.subnetInput.CharLimit = 64

	s.dnsInput = textinput.New()
	s.dnsInput.Placeholder = "8.8.8.8"
	s.dnsInput.CharLimit = 64

	s.ntpInput = textinput.New()
	s.ntpInput.Placeholder = "time1.google.com 또는 IP"
	s.ntpInput.CharLimit = 253

	return s
}

func (s *S3Network) Title() string      { return "네트워크 설정" }
func (s *S3Network) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S3Network) FooterHelp() string {
	return "tab: 섹션 이동  space: 토글  a: 항목 추가  d: 항목 삭제  ctrl+n: 다음"
}

func (s *S3Network) SyncFromState(st *state.AppState) {
	s.subnets = append([]string{}, st.InternalNetworkSubnets...)
	s.haMode = st.KiCpHaMode
	s.vip.SetValue(st.KiCpHaModeVIP)
	s.dnssec = st.KiCpDnsDnssecValidation
	s.dnsServers = append([]string{}, st.KiCpDnsUpstreamServers...)
	s.ntpServers = append([]string{}, st.KiCpNtpUpstreamServers...)
	s.extraZone.SetValue(st.InternalNetworkExtraZone)
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

func (s *S3Network) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle active text inputs first
	if s.editingSubnet {
		return s.updateSubnetEdit(msg)
	}
	if s.editingDNS {
		return s.updateDNSEdit(msg)
	}
	if s.editingNTP {
		return s.updateNTPEdit(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.focus = (s.focus + 1) % 7
			s.syncFocus()
		case "shift+tab":
			s.focus = (s.focus - 1 + 7) % 7
			s.syncFocus()
		case " ":
			switch s.focus {
			case 1:
				s.haMode = !s.haMode
			case 3:
				s.dnssec = !s.dnssec
			}
		case "a":
			switch s.focus {
			case 0:
				s.editingSubnet = true
				s.subnetInput.SetValue("")
				s.subnetInput.Focus()
				return s, textinput.Blink
			case 4:
				s.editingDNS = true
				s.dnsInput.SetValue("")
				s.dnsInput.Focus()
				return s, textinput.Blink
			case 5:
				s.editingNTP = true
				s.ntpInput.SetValue("")
				s.ntpInput.Focus()
				return s, textinput.Blink
			}
		case "d":
			switch s.focus {
			case 0:
				if len(s.subnets) > 0 {
					s.subnets = s.subnets[:len(s.subnets)-1]
				}
			case 4:
				if len(s.dnsServers) > 0 {
					s.dnsServers = s.dnsServers[:len(s.dnsServers)-1]
				}
			case 5:
				if len(s.ntpServers) > 0 {
					s.ntpServers = s.ntpServers[:len(s.ntpServers)-1]
				}
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}

	// Focused textinput updates
	case tea.Msg:
		if s.focus == 2 {
			var cmd tea.Cmd
			s.vip, cmd = s.vip.Update(msg)
			return s, cmd
		}
		if s.focus == 6 {
			var cmd tea.Cmd
			s.extraZone, cmd = s.extraZone.Update(msg)
			return s, cmd
		}
	}
	return s, nil
}

func (s *S3Network) syncFocus() {
	s.vip.Blur()
	s.extraZone.Blur()
	if s.focus == 2 {
		s.vip.Focus()
	}
	if s.focus == 6 {
		s.extraZone.Focus()
	}
}

func (s *S3Network) updateSubnetEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingSubnet = false
			s.subnetInput.Blur()
			return s, nil
		case "enter":
			v := strings.TrimSpace(s.subnetInput.Value())
			if v != "" {
				s.subnets = append(s.subnets, v)
			}
			s.editingSubnet = false
			s.subnetInput.Blur()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.subnetInput, cmd = s.subnetInput.Update(msg)
	return s, cmd
}

func (s *S3Network) updateDNSEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingDNS = false
			s.dnsInput.Blur()
			return s, nil
		case "enter":
			v := strings.TrimSpace(s.dnsInput.Value())
			if v != "" {
				s.dnsServers = append(s.dnsServers, v)
			}
			s.editingDNS = false
			s.dnsInput.Blur()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.dnsInput, cmd = s.dnsInput.Update(msg)
	return s, cmd
}

func (s *S3Network) updateNTPEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			s.editingNTP = false
			s.ntpInput.Blur()
			return s, nil
		case "enter":
			v := strings.TrimSpace(s.ntpInput.Value())
			if v != "" {
				s.ntpServers = append(s.ntpServers, v)
			}
			s.editingNTP = false
			s.ntpInput.Blur()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.ntpInput, cmd = s.ntpInput.Update(msg)
	return s, cmd
}

func (s *S3Network) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("네트워크 설정") + "\n\n")

	b.WriteString(s.sectionHeader("내부 서브넷 (CIDR)", s.focus == 0))
	for _, sub := range s.subnets {
		b.WriteString("  " + styles.StylePrimary.Render("•") + " " + sub + "\n")
	}
	if s.editingSubnet {
		b.WriteString("  " + s.subnetInput.View() + "\n")
	} else if s.focus == 0 {
		b.WriteString(styles.StyleMuted.Render("  [a] 추가  [d] 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(s.sectionHeader("Control Plane HA 모드", s.focus == 1))
	haStr := styles.RadioOff + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn + " 활성"
	}
	b.WriteString("  " + haStr + styles.StyleMuted.Render("  (space로 토글)") + "\n\n")

	b.WriteString(s.sectionHeader("HA VIP", s.focus == 2))
	vipView := s.vip.View()
	if !s.haMode {
		vipView = styles.StyleMuted.Render("(HA 모드 비활성 — 설정 불필요)")
	}
	b.WriteString("  " + vipView + "\n\n")

	b.WriteString(s.sectionHeader("DNSSEC 검증", s.focus == 3))
	dnssecStr := styles.RadioOff + " 비활성"
	if s.dnssec {
		dnssecStr = styles.RadioOn + " 활성"
	}
	b.WriteString("  " + dnssecStr + "\n\n")

	b.WriteString(s.sectionHeader("DNS upstream 서버", s.focus == 4))
	for _, srv := range s.dnsServers {
		b.WriteString("  " + styles.StylePrimary.Render("•") + " " + srv + "\n")
	}
	if s.editingDNS {
		b.WriteString("  " + s.dnsInput.View() + "\n")
	} else if s.focus == 4 {
		b.WriteString(styles.StyleMuted.Render("  [a] 추가  [d] 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(s.sectionHeader("NTP upstream 서버", s.focus == 5))
	for _, srv := range s.ntpServers {
		b.WriteString("  " + styles.StylePrimary.Render("•") + " " + srv + "\n")
	}
	if s.editingNTP {
		b.WriteString("  " + s.ntpInput.View() + "\n")
	} else if s.focus == 5 {
		b.WriteString(styles.StyleMuted.Render("  [a] 추가  [d] 마지막 삭제") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(s.sectionHeader("추가 DNS 존 (선택사항)", s.focus == 6))
	b.WriteString("  " + s.extraZone.View() + "\n")

	return b.String()
}

func (s *S3Network) sectionHeader(label string, focused bool) string {
	if focused {
		return styles.StyleLabelFocused.Render("▶ " + label) + "\n"
	}
	return styles.StyleLabel.Render("  " + label) + "\n"
}

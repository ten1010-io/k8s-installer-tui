package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

var certModes = []struct {
	value       string
	label       string
	description string
}{
	{"self_signed", "self_signed", "CA를 자동 생성하고 TLS 인증서를 발급합니다.\n  추가 파일 제공 없이 설치가 가능합니다."},
	{"ca_provided", "ca_provided", "고객이 ca.crt + ca.key를 제공합니다.\n  TLS 인증서 발급은 설치 도구가 수행합니다."},
	{"tls_provided", "tls_provided", "고객이 tls.crt + tls.key를 모두 제공합니다.\n  인증서 발급 단계를 건너뜁니다."},
}

// Focus layout:
//   0..2  : cert mode options
//   3     : nav slot (←/→: 이전/다음)

type S6CertMode struct {
	selected int
	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS6CertMode() *S6CertMode { return &S6CertMode{} }

func (s *S6CertMode) Title() string { return "인증서 모드" }
func (s *S6CertMode) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S6CertMode) SyncFromState(st *state.AppState) {
	s.selected = 0
	for i, m := range certModes {
		if m.value == st.KiCertMode {
			s.selected = i
			break
		}
	}
	s.focusIdx = s.selected
	s.navIdx = 0
}

func (s *S6CertMode) SyncToState(st *state.AppState) {
	st.KiCertMode = certModes[s.selected].value
}

func (s *S6CertMode) Validate() []string { return nil }

func (s *S6CertMode) Init() tea.Cmd { return nil }

const (
	s6NavFocus = 3 // after 3 options, single nav slot
	s6MaxFocus = s6NavFocus
)

func (s *S6CertMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx > 0 {
				s.focusIdx--
			}
		case "down", "j":
			if s.focusIdx < s6MaxFocus {
				s.focusIdx++
			}
		case "left", "h":
			if s.focusIdx == s6NavFocus && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s6NavFocus && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			return s.activate()
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

func (s *S6CertMode) activate() (tea.Model, tea.Cmd) {
	switch s.focusIdx {
	case s6NavFocus:
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	default: // option selection
		s.selected = s.focusIdx
	}
	return s, nil
}

func (s *S6CertMode) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("인증서 모드 선택") + "\n")
	b.WriteString(styles.StyleMuted.Render("ki_cert_mode 값을 선택합니다.") + "\n\n")

	for i, m := range certModes {
		optFocused := s.focusIdx == i
		radio := styles.RadioOff
		labelStyle := styles.StyleMuted
		if i == s.selected {
			radio = styles.RadioOn
			labelStyle = styles.StylePrimary
		}
		line := radio + " " + labelStyle.Render(m.label)
		b.WriteString(RenderRow(line, optFocused, s.width) + "\n")
		b.WriteString(styles.StyleMuted.Render("  "+m.description) + "\n\n")
	}

	prevFocused := s.focusIdx == s6NavFocus && s.navIdx == 0
	nextFocused := s.focusIdx == s6NavFocus && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

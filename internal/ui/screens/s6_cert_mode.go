package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
)

var certModes = []struct {
	value       string
	label       string
	description string
}{
	{
		value:       "self_signed",
		label:       "self_signed",
		description: "CA를 자동 생성하고 TLS 인증서를 발급합니다.\n  추가 파일 제공 없이 설치가 가능합니다.",
	},
	{
		value:       "ca_provided",
		label:       "ca_provided",
		description: "고객이 ca.crt + ca.key를 제공합니다.\n  TLS 인증서 발급은 설치 도구가 수행합니다.",
	},
	{
		value:       "tls_provided",
		label:       "tls_provided",
		description: "고객이 tls.crt + tls.key를 모두 제공합니다.\n  인증서 발급 단계를 건너뜁니다.",
	},
}

type S6CertMode struct {
	selected int
	width    int
	height   int
}

func NewS6CertMode() *S6CertMode { return &S6CertMode{} }

func (s *S6CertMode) Title() string      { return "인증서 모드" }
func (s *S6CertMode) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S6CertMode) FooterHelp() string {
	return "↑/↓: 선택  ctrl+n: 다음 (미리보기)  ctrl+p: 이전"
}

func (s *S6CertMode) SyncFromState(st *state.AppState) {
	s.selected = 0
	for i, m := range certModes {
		if m.value == st.KiCertMode {
			s.selected = i
			break
		}
	}
}

func (s *S6CertMode) SyncToState(st *state.AppState) {
	st.KiCertMode = certModes[s.selected].value
}

func (s *S6CertMode) Validate() []string { return nil }

func (s *S6CertMode) Init() tea.Cmd { return nil }

func (s *S6CertMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.selected > 0 {
				s.selected--
			}
		case "down", "j":
			if s.selected < len(certModes)-1 {
				s.selected++
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		}
	}
	return s, nil
}

func (s *S6CertMode) View() string {
	var b strings.Builder
	b.WriteString(ui.StyleTitle.Render("인증서 모드 선택") + "\n")
	b.WriteString(ui.StyleMuted.Render("ki_cert_mode 값을 선택합니다.") + "\n\n")

	for i, m := range certModes {
		radio := ui.RadioOff
		labelStyle := ui.StyleMuted
		if i == s.selected {
			radio = ui.RadioOn
			labelStyle = ui.StylePrimary
		}
		b.WriteString(radio + " " + labelStyle.Render(m.label) + "\n")
		b.WriteString(ui.StyleMuted.Render("  "+m.description) + "\n\n")
	}

	return b.String()
}

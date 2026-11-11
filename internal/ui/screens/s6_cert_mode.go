package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// certModes are the modes offered; the descriptions are English here, said
// through i18n.T where they are drawn.
var certModes = []struct {
	value       string
	label       string
	description string
}{
	{"self_signed", "self_signed", "Creates a CA and issues the TLS certificates.\n  Installs with no extra files."},
	{"ca_provided", "ca_provided", "The customer provides ca.crt + ca.key.\n  The installer issues the TLS certificates."},
	{"tls_provided", "tls_provided", "The customer provides both tls.crt and tls.key.\n  The step that issues certificates is skipped."},
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

func (s *S6CertMode) Title() string { return i18n.T("Certificate settings") }
func (s *S6CertMode) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S6CertMode) SyncFromState(st *state.AppState) {
	s.selected = 0
	for i, m := range certModes {
		if m.value == st.AipubCertMode {
			s.selected = i
			break
		}
	}
	s.focusIdx = s.selected
	s.navIdx = 0
}

func (s *S6CertMode) SyncToState(st *state.AppState) {
	st.AipubCertMode = certModes[s.selected].value
}

func (s *S6CertMode) Validate() []string { return nil }

func (s *S6CertMode) Init() tea.Cmd { return nil }

func (s *S6CertMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	navFocus := len(certModes)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx > 0 {
				s.focusIdx--
			}
		case "down", "j":
			if s.focusIdx < navFocus {
				s.focusIdx++
				if s.focusIdx == navFocus {
					s.navIdx = 1 // 다음
				}
			}
		case "left", "h":
			if s.focusIdx == navFocus && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == navFocus && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			return s.activate()
		}
	}
	return s, nil
}

func (s *S6CertMode) activate() (tea.Model, tea.Cmd) {
	if s.focusIdx == len(certModes) {
		if s.navIdx == 0 {
			return s, Prev()
		}
		return s, Next()
	}
	s.selected = s.focusIdx
	return s, nil
}

func (s *S6CertMode) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(i18n.T("Choose the certificate mode")) + "\n")
	b.WriteString(styles.StyleMuted.Render("aipub_cert_mode: "+short("aipub_cert_mode")) + "\n\n")

	// the focused option stays in sight on a low screen
	fs := focusSpan{b: &b}
	for i, m := range certModes {
		optFocused := s.focusIdx == i
		fs.mark(optFocused)
		radio := styles.RadioOff()
		labelStyle := styles.StyleMuted
		if i == s.selected {
			radio = styles.RadioOn()
			labelStyle = styles.StylePrimary
		}
		line := radio + " " + labelStyle.Render(m.label)
		b.WriteString(RenderRow(line, optFocused, s.width) + "\n")
		b.WriteString(styles.StyleMuted.Render("  "+i18n.T(m.description)) + "\n\n")
	}

	prevFocused := s.focusIdx == len(certModes) && s.navIdx == 0
	nextFocused := s.focusIdx == len(certModes) && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(s.focusIdx == len(certModes))
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))

	return fs.fit(s.width, s.height)
}

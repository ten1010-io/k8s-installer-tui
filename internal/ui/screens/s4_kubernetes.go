package screens

import (
	"net"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The cluster as k8s-installer builds it (group_vars/all/vars.yml). The load
// balancers that were here belong to AIPub now (S6AipubLB).
//
// Focus layout:
//   0  : k8s minor version
//   1  : pod subnet
//   2  : service subnet
//   3  : certificate validity
//   4  : nav slot (←/→: back/next)

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

func (s *S4Kubernetes) Title() string { return i18n.T("Kubernetes settings") }
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
		return []string{i18n.T("Kubernetes minor version is not valid (e.g. 1.36)")}
	}
	for _, f := range []struct{ name, val string }{
		{"Pod subnet", s.podSubnet.Value()},
		{"Service subnet", s.serviceSubnet.Value()},
	} {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(f.val)); err != nil {
			return []string{i18n.T("%s is not in CIDR form (e.g. 10.244.0.0/16)", i18n.T(f.name))}
		}
	}
	if strings.TrimSpace(s.certValidity.Value()) == "" {
		return []string{i18n.T("Enter the certificate validity (e.g. 26280h)")}
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
				if s.focusIdx == s4FocusNav {
					s.navIdx = 1 // next
				}
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
	b.WriteString(styles.StyleTitle.Render(i18n.T("Kubernetes settings")) + "\n")
	b.WriteString(styles.StyleMuted.Render(i18n.T("Values k8s-installer uses to build the cluster (group_vars/all/vars.yml).")) + "\n\n")

	f := s.focusIdx
	// the focused part stays in sight on a low screen
	fs := focusSpan{b: &b}
	fs.mark(f == s4FocusMinor)
	b.WriteString(RenderSectionHeader(i18n.T("Minor version"), f == s4FocusMinor) + "  " + s.minorVersion.View() +
		styles.StyleMuted.Render("  "+short("k8s_minor_version")) + "\n\n")
	fs.mark(f == s4FocusPodSubnet)
	b.WriteString(RenderSectionHeader(i18n.T("Pod subnet"), f == s4FocusPodSubnet) + "  " + s.podSubnet.View() + "\n")
	fs.mark(f == s4FocusServiceSubnet)
	b.WriteString(RenderSectionHeader(i18n.T("Service subnet"), f == s4FocusServiceSubnet) + "  " + s.serviceSubnet.View() + "\n")
	b.WriteString(styles.StyleMuted.Render("    "+i18n.T("Must not overlap the networks used by nodes and services. Cannot be changed once the cluster is created")) + "\n\n")
	fs.mark(f == s4FocusCert)
	b.WriteString(RenderSectionHeader(i18n.T("Certificate validity"), f == s4FocusCert) + "  " + s.certValidity.View() +
		styles.StyleMuted.Render("  "+i18n.T("(e.g. 26280h = 3 years)")) + "\n\n")

	fs.mark(false)
	b.WriteString(styles.StyleMuted.Render(i18n.T("%s. Set in step 5", short("k8s_kube_proxy_enabled"))) + "\n")

	prevFocused := f == s4FocusNav && s.navIdx == 0
	nextFocused := f == s4FocusNav && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(f == s4FocusNav)
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))
	return fs.fit(s.width, s.height)
}

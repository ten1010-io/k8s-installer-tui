package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// The variables of the python services (setup-python.yml of aipub-installer:
// coaster, usage-manager, workload-controller-manager, the Gateways) a site
// decides before the install: the cluster name its coaster license keys were
// issued for, which nodes are GPU nodes, how the components are deployed, and
// whether aipub-network-operator goes in (InfiniBand sites). Only AIPub has
// them (skipped without it).
//
// What a person brings from elsewhere - the license keys themselves, the
// InfiniBand nodes and ports - is set in files/aipub/vars.yml by hand; the
// wizard keeps it as it is and the site-values tab of the preview lists it.
//
// Whether the python services are installed at all is not asked here: it is a
// choice of the run (the install screen, --no-python), as monitoring is.
//
// Focus layout:
//   0 : cluster name
//   1 : GPU nodes (checkboxes)
//   2 : deploy tool (choice)
//   3 : network operator (toggle)
//   4 : nav slot

const (
	s5cFocusClusterName = iota
	s5cFocusGpuNodes
	s5cFocusDeployTool
	s5cFocusNetworkOperator
	s5cFocusNav
	s5cFocusMax = s5cFocusNav
)

type S5cPython struct {
	clusterName     textinput.Model
	k8sNodes        []string // the nodes that may be picked
	nodeNames       []string // those, and names the state holds that are not
	nodeChecks      []bool
	nodeCursor      int
	deployTool      string
	networkOperator bool

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS5cPython() *S5cPython {
	t := textinput.New()
	t.Placeholder = "cluster1"
	t.CharLimit = 64
	return &S5cPython{clusterName: t}
}

func (s *S5cPython) Title() string { return i18n.T("Python services settings") }
func (s *S5cPython) SetSize(w, h int) {
	s.width = w
	s.height = h
}
func (s *S5cPython) Init() tea.Cmd { return nil }

func (s *S5cPython) SyncFromState(st *state.AppState) {
	s.clusterName.SetValue(st.PythonCoasterClusterName)
	s.deployTool = st.PythonDeployTool
	s.networkOperator = st.PythonNetworkOperatorEnabled

	// the k8s nodes, and a name the state holds that is not one, so that the
	// validation names it instead of the screen dropping it
	s.k8sNodes = append([]string{}, st.K8sNodes...)
	s.nodeNames = append([]string{}, st.K8sNodes...)
	for _, n := range st.PythonCoasterGpuNodes {
		if !st.IsK8sNode(n) {
			s.nodeNames = append(s.nodeNames, n)
		}
	}
	s.nodeChecks = make([]bool, len(s.nodeNames))
	for i, n := range s.nodeNames {
		for _, m := range st.PythonCoasterGpuNodes {
			if n == m {
				s.nodeChecks[i] = true
			}
		}
	}
	s.focusIdx, s.navIdx, s.nodeCursor = 0, 0, 0
	s.syncFocus()
}

func (s *S5cPython) SyncToState(st *state.AppState) {
	st.PythonCoasterClusterName = strings.TrimSpace(s.clusterName.Value())
	st.PythonDeployTool = s.deployTool
	st.PythonNetworkOperatorEnabled = s.networkOperator
	st.PythonCoasterGpuNodes = nil
	for i, n := range s.nodeNames {
		if s.nodeChecks[i] {
			st.PythonCoasterGpuNodes = append(st.PythonCoasterGpuNodes, n)
		}
	}
}

func (s *S5cPython) Validate() []string {
	// the checks of the schema, on what the screen holds
	st := state.AppState{K8sNodes: s.k8sNodes}
	s.SyncToState(&st)
	var msgs []string
	for _, e := range validator.Python(&st) {
		msgs = append(msgs, e.Error())
	}
	return msgs
}

func (s *S5cPython) syncFocus() {
	if s.focusIdx == s5cFocusClusterName {
		s.clusterName.Focus()
	} else {
		s.clusterName.Blur()
	}
}

func (s *S5cPython) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Letters typed into the text input are text: h/j/k/l move only elsewhere
		if msg.Type == tea.KeyRunes && s.focusIdx == s5cFocusClusterName {
			break
		}
		switch msg.String() {
		case "up", "k":
			if s.focusIdx == s5cFocusGpuNodes && s.nodeCursor > 0 {
				s.nodeCursor--
				return s, nil
			}
			if s.focusIdx > 0 {
				s.focusIdx--
				if s.focusIdx == s5cFocusGpuNodes {
					s.nodeCursor = max(0, len(s.nodeChecks)-1)
				}
				s.syncFocus()
			}
		case "down", "j":
			if s.focusIdx == s5cFocusGpuNodes && s.nodeCursor < len(s.nodeChecks)-1 {
				s.nodeCursor++
				return s, nil
			}
			if s.focusIdx < s5cFocusMax {
				s.focusIdx++
				if s.focusIdx == s5cFocusNav {
					s.navIdx = 1 // next
				}
				s.nodeCursor = 0
				s.syncFocus()
			}
		case "left", "h":
			if s.focusIdx == s5cFocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s5cFocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s5cFocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s5cFocusGpuNodes:
				if s.nodeCursor < len(s.nodeChecks) {
					s.nodeChecks[s.nodeCursor] = !s.nodeChecks[s.nodeCursor]
				}
			case s5cFocusDeployTool:
				s.deployTool = nextChoice("aipub_python_deploy_tool", s.deployTool)
			case s5cFocusNetworkOperator:
				s.networkOperator = !s.networkOperator
			}
		}
	}
	var cmd tea.Cmd
	if s.focusIdx == s5cFocusClusterName {
		s.clusterName, cmd = s.clusterName.Update(msg)
	}
	return s, cmd
}

func (s *S5cPython) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(i18n.T("Python services settings")) + "\n")
	b.WriteString(styles.StyleMuted.Render(i18n.T("coaster, usage-manager, workload-controller-manager, the Gateways (setup-python). "+
		"Whether they are installed is chosen on the install screen")) + "\n\n")

	f := s.focusIdx
	// the focused part stays in sight on a low screen
	fs := focusSpan{b: &b}

	fs.mark(f == s5cFocusClusterName)
	b.WriteString(RenderSectionHeader(i18n.T("Cluster name"), f == s5cFocusClusterName) + "  " + s.clusterName.View() +
		styles.StyleMuted.Render("  "+short("aipub_python_coaster_cluster_name")) + "\n")

	fs.mark(f == s5cFocusGpuNodes)
	b.WriteString(RenderSectionHeader(i18n.T("GPU nodes"), f == s5cFocusGpuNodes) +
		styles.StyleMuted.Render("  "+short("aipub_python_coaster_gpu_nodes")) + "\n")
	for i, n := range s.nodeNames {
		mark := styles.CheckOff()
		if s.nodeChecks[i] {
			mark = styles.CheckOn()
		}
		b.WriteString(RenderRow(fmt.Sprintf("    %s %s", mark, n), f == s5cFocusGpuNodes && s.nodeCursor == i, s.width) + "\n")
	}
	if len(s.nodeNames) == 0 {
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("(shown after the nodes are set in steps 1 and 2)")) + "\n")
	}

	fs.mark(f == s5cFocusDeployTool)
	b.WriteString(RenderSectionHeader(i18n.T("Python deploy tool"), f == s5cFocusDeployTool) + "  " +
		choiceView("aipub_python_deploy_tool", s.deployTool) + "\n")

	op := styles.RadioOff() + " " + i18n.T("Off")
	if s.networkOperator {
		op = styles.RadioOn() + " " + i18n.T("On")
	}
	fs.mark(f == s5cFocusNetworkOperator)
	b.WriteString(RenderSectionHeader(i18n.T("Network operator"), f == s5cFocusNetworkOperator) + "  " + op +
		styles.StyleMuted.Render("  "+i18n.T("InfiniBand sites only")) + "\n")
	fs.mark(false)
	if s.networkOperator {
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("Set the InfiniBand nodes and ports in files/aipub/vars.yml "+
			"(aipub_python_network_operator_ib_nodes, _ib_pfs): setup-python stops with the value to paste when they are empty")) + "\n")
	}
	b.WriteString(styles.StyleMuted.Render("    "+i18n.T("The coaster license keys (aipub_python_coaster_license_keys) are set in files/aipub/vars.yml, "+
		"or patched by hand after the install; the wizard keeps what is there")) + "\n")

	prevFocused := f == s5cFocusNav && s.navIdx == 0
	nextFocused := f == s5cFocusNav && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(f == s5cFocusNav)
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))
	return fs.fit(s.width, s.height)
}

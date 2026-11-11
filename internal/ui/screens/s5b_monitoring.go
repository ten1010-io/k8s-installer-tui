package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// The variables every site of the monitoring module sets: how much the volumes
// hold and for how long, the rightsizing scan, the replicas of its API and the
// nodes OpenSearch runs on. Only AIPub has them (skipped without it); the defaults
// are those of aipub-monitoring.yml of aipub-installer and are written only when
// changed.
//
// Focus layout:
//   0 : OpenSearch volume size
//   1 : log retention
//   2 : VictoriaMetrics volume size
//   3 : metrics retention
//   4 : rightsizing (toggle)
//   5 : monitoring-api replicas
//   6 : nodes (checkboxes; none checked: every k8s control plane node)
//   7 : nav slot

const (
	s5bFocusOpensearchSize = iota
	s5bFocusLogRetention
	s5bFocusMetricsSize
	s5bFocusMetricsRetention
	s5bFocusRightsizing
	s5bFocusApiReplicas
	s5bFocusNodes
	s5bFocusNav
	s5bFocusMax = s5bFocusNav
)

type S5bMonitoring struct {
	opensearchSize   textinput.Model
	logRetention     textinput.Model
	metricsSize      textinput.Model
	metricsRetention textinput.Model
	rightsizing      bool
	apiReplicas      textinput.Model
	k8sCpNodes       []string // the nodes that may be picked
	nodeNames        []string // those, and names the state holds that are not
	nodeChecks       []bool
	nodeCursor       int

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS5bMonitoring() *S5bMonitoring {
	newInput := func(placeholder string) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = 64
		return t
	}
	return &S5bMonitoring{
		opensearchSize:   newInput("3Ti"),
		logRetention:     newInput("180d"),
		metricsSize:      newInput("1Ti"),
		metricsRetention: newInput("180d"),
		apiReplicas:      newInput("2"),
	}
}

func (s *S5bMonitoring) Title() string { return i18n.T("Monitoring settings") }
func (s *S5bMonitoring) SetSize(w, h int) {
	s.width = w
	s.height = h
}
func (s *S5bMonitoring) Init() tea.Cmd { return nil }

func (s *S5bMonitoring) SyncFromState(st *state.AppState) {
	s.opensearchSize.SetValue(st.MonitoringOpensearchStorageSize)
	s.logRetention.SetValue(st.MonitoringOpensearchLogRetention)
	s.metricsSize.SetValue(st.MonitoringMetricsStorageSize)
	s.metricsRetention.SetValue(st.MonitoringMetricsRetentionPeriod)
	s.rightsizing = st.MonitoringRightsizingEnabled
	s.apiReplicas.SetValue(strconv.Itoa(st.MonitoringApiReplicas))

	// the control plane nodes, and a name the state holds that is not one, so
	// that the validation names it instead of the screen dropping it
	s.k8sCpNodes = append([]string{}, st.K8sCpNodes...)
	s.nodeNames = append([]string{}, st.K8sCpNodes...)
	for _, n := range st.MonitoringCpNodes {
		if !st.IsK8sCpNode(n) {
			s.nodeNames = append(s.nodeNames, n)
		}
	}
	s.nodeChecks = make([]bool, len(s.nodeNames))
	for i, n := range s.nodeNames {
		for _, m := range st.MonitoringCpNodes {
			if n == m {
				s.nodeChecks[i] = true
			}
		}
	}
	s.focusIdx, s.navIdx, s.nodeCursor = 0, 0, 0
	s.syncFocus()
}

func (s *S5bMonitoring) SyncToState(st *state.AppState) {
	st.MonitoringOpensearchStorageSize = strings.TrimSpace(s.opensearchSize.Value())
	st.MonitoringOpensearchLogRetention = strings.TrimSpace(s.logRetention.Value())
	st.MonitoringMetricsStorageSize = strings.TrimSpace(s.metricsSize.Value())
	st.MonitoringMetricsRetentionPeriod = strings.TrimSpace(s.metricsRetention.Value())
	st.MonitoringRightsizingEnabled = s.rightsizing
	// not a number: 0, which the validation refuses
	st.MonitoringApiReplicas, _ = strconv.Atoi(strings.TrimSpace(s.apiReplicas.Value()))
	st.MonitoringCpNodes = nil
	for i, n := range s.nodeNames {
		if s.nodeChecks[i] {
			st.MonitoringCpNodes = append(st.MonitoringCpNodes, n)
		}
	}
}

func (s *S5bMonitoring) Validate() []string {
	// the checks of the schema, on what the screen holds
	st := state.AppState{K8sCpNodes: s.k8sCpNodes}
	s.SyncToState(&st)
	var msgs []string
	for _, e := range validator.Monitoring(&st) {
		msgs = append(msgs, e.Error())
	}
	return msgs
}

func (s *S5bMonitoring) inputs() []*textinput.Model {
	return []*textinput.Model{
		s5bFocusOpensearchSize:   &s.opensearchSize,
		s5bFocusLogRetention:     &s.logRetention,
		s5bFocusMetricsSize:      &s.metricsSize,
		s5bFocusMetricsRetention: &s.metricsRetention,
		s5bFocusRightsizing:      nil,
		s5bFocusApiReplicas:      &s.apiReplicas,
		s5bFocusNodes:            nil,
	}
}

func (s *S5bMonitoring) syncFocus() {
	inps := s.inputs()
	for _, inp := range inps {
		if inp != nil {
			inp.Blur()
		}
	}
	if s.focusIdx < len(inps) && inps[s.focusIdx] != nil {
		inps[s.focusIdx].Focus()
	}
}

func (s *S5bMonitoring) inputFocused() bool {
	inps := s.inputs()
	return s.focusIdx < len(inps) && inps[s.focusIdx] != nil
}

func (s *S5bMonitoring) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Letters typed into a text input are text: h/j/k/l move only elsewhere
		if msg.Type == tea.KeyRunes && s.inputFocused() {
			break
		}
		switch msg.String() {
		case "up", "k":
			if s.focusIdx == s5bFocusNodes && s.nodeCursor > 0 {
				s.nodeCursor--
				return s, nil
			}
			if s.focusIdx > 0 {
				s.focusIdx--
				if s.focusIdx == s5bFocusNodes {
					s.nodeCursor = max(0, len(s.nodeChecks)-1)
				}
				s.syncFocus()
			}
		case "down", "j":
			if s.focusIdx == s5bFocusNodes && s.nodeCursor < len(s.nodeChecks)-1 {
				s.nodeCursor++
				return s, nil
			}
			if s.focusIdx < s5bFocusMax {
				s.focusIdx++
				if s.focusIdx == s5bFocusNav {
					s.navIdx = 1 // next
				}
				s.nodeCursor = 0
				s.syncFocus()
			}
		case "left", "h":
			if s.focusIdx == s5bFocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s5bFocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s5bFocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s5bFocusRightsizing:
				s.rightsizing = !s.rightsizing
			case s5bFocusNodes:
				if s.nodeCursor < len(s.nodeChecks) {
					s.nodeChecks[s.nodeCursor] = !s.nodeChecks[s.nodeCursor]
				}
			}
		}
	}
	var cmd tea.Cmd
	inps := s.inputs()
	if s.focusIdx < len(inps) && inps[s.focusIdx] != nil {
		*inps[s.focusIdx], cmd = inps[s.focusIdx].Update(msg)
	}
	return s, cmd
}

func (s *S5bMonitoring) View() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render(i18n.T("Monitoring settings")) + "\n")
	b.WriteString(styles.StyleMuted.Render(i18n.T("Defaults are those of aipub-installer; only changed values are written to files/aipub/vars.yml")) + "\n\n")

	f := s.focusIdx
	// the focused part stays in sight on a low screen
	fs := focusSpan{b: &b}
	row := func(label string, idx int, in textinput.Model, key string) {
		fs.mark(f == idx)
		b.WriteString(RenderSectionHeader(label, f == idx) + "  " + in.View() +
			styles.StyleMuted.Render("  "+short(key)) + "\n")
	}
	b.WriteString(styles.StylePrimary.Render(i18n.T("OpenSearch (logs)")) + "\n")
	row(i18n.T("Volume size"), s5bFocusOpensearchSize, s.opensearchSize, "aipub_monitoring_opensearch_storage_size")
	row(i18n.T("Log retention"), s5bFocusLogRetention, s.logRetention, "aipub_monitoring_opensearch_log_retention")
	fs.mark(false)
	b.WriteString("\n" + styles.StylePrimary.Render(i18n.T("VictoriaMetrics (metrics)")) + "\n")
	row(i18n.T("Volume size"), s5bFocusMetricsSize, s.metricsSize, "aipub_monitoring_metrics_storage_size")
	row(i18n.T("Metrics retention"), s5bFocusMetricsRetention, s.metricsRetention, "aipub_monitoring_metrics_retention_period")

	fs.mark(false)
	b.WriteString("\n" + styles.StylePrimary.Render(i18n.T("Other")) + "\n")
	rs := styles.RadioOff() + " " + i18n.T("Off")
	if s.rightsizing {
		rs = styles.RadioOn() + " " + i18n.T("On")
	}
	fs.mark(f == s5bFocusRightsizing)
	b.WriteString(RenderSectionHeader(i18n.T("Rightsizing"), f == s5bFocusRightsizing) + "  " + rs +
		styles.StyleMuted.Render("  "+short("aipub_monitoring_rightsizing_enabled")) + "\n")
	row(i18n.T("API replicas"), s5bFocusApiReplicas, s.apiReplicas, "aipub_monitoring_api_replicas")

	fs.mark(f == s5bFocusNodes)
	b.WriteString(RenderSectionHeader(i18n.T("OpenSearch nodes"), f == s5bFocusNodes) +
		styles.StyleMuted.Render("  "+i18n.T("none chosen means all k8s control plane nodes")) + "\n")
	for i, n := range s.nodeNames {
		mark := styles.CheckOff()
		if s.nodeChecks[i] {
			mark = styles.CheckOn()
		}
		b.WriteString(RenderRow(fmt.Sprintf("    %s %s", mark, n), f == s5bFocusNodes && s.nodeCursor == i, s.width) + "\n")
	}
	if len(s.nodeNames) == 0 {
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("(shown after the CP nodes are set in step 2)")) + "\n")
	}

	prevFocused := f == s5bFocusNav && s.navIdx == 0
	nextFocused := f == s5bFocusNav && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(f == s5bFocusNav)
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))
	return fs.fit(s.width, s.height)
}

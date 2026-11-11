package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : AIPub install (toggle)
//   --- only when enabled ---
//   1  : ingress zone
//   2  : HA mode toggle
//   3  : storage class (HA) / harbor data node (without HA)
//   4  : CP nodes section
//   5  : harbor subdomain
//   6  : harbor replicas
//   7  : harbor registry storage
//   8  : harbor postgresql storage
//   9  : harbor redis storage
//   10 : harbor trivy storage
//   11 : edition (enterprise | lite)
//   12 : AIPub deploy tool (installer | helmfile)
//   13 : helmfile path            (only when helmfile)
//   14 : monitoring deploy tool (installer | helmfile)
//   15 : monitoring helmfile path  (only when monitoring is helmfile)
//   16 : monitoring kustomize path (")
//   17 : monitoring helm path      (")
//   18 : helmfile dry-run          (")
//   19 : nav slot

const (
	s5FocusEnabled = iota
	s5FocusIngressZone
	s5FocusHaMode
	s5FocusStorageClass
	s5FocusCpNodes
	s5FocusHarborSubdomain
	s5FocusHarborReplicas
	s5FocusHarborRegistry
	s5FocusHarborPg
	s5FocusHarborRedis
	s5FocusHarborTrivy
	s5FocusEdition
	s5FocusAppTool
	s5FocusAppBin
	s5FocusMonTool
	s5FocusMonHelmfileBin
	s5FocusMonKustomizeBin
	s5FocusMonHelmBin
	s5FocusMonDryRun
	s5FocusNav
	s5FocusMax = s5FocusNav
)

type S5Aipub struct {
	enabled         bool
	ingressZone     textinput.Model
	haMode          bool
	storageClass    textinput.Model
	localPVNode     textinput.Model // aipub_harbor_local_pv_node, without HA
	k8sNodeNames    []string
	cpNodeChecks    []bool
	cpNodeCursor    int
	harborSubdomain textinput.Model
	harborReplicas  textinput.Model
	harborRegistry  textinput.Model
	harborPg        textinput.Model
	harborRedis     textinput.Model
	harborTrivy     textinput.Model

	edition         string
	appTool         string
	appBin          textinput.Model
	monTool         string
	monHelmfileBin  textinput.Model
	monKustomizeBin textinput.Model
	monHelmBin      textinput.Model
	monDryRun       bool

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS5Aipub() *S5Aipub {
	newInput := func(placeholder string) textinput.Model {
		t := textinput.New()
		t.Placeholder = placeholder
		t.CharLimit = 128
		return t
	}
	return &S5Aipub{
		enabled:         false,
		ingressZone:     newInput("example.com"),
		storageClass:    newInput("ontap-nas"),
		localPVNode:     newInput(""),
		harborSubdomain: newInput("aipub-harbor"),
		harborReplicas:  newInput("3"),
		harborRegistry:  newInput("512Gi"),
		harborPg:        newInput("128Gi"),
		harborRedis:     newInput("32Gi"),
		harborTrivy:     newInput("5Gi"),

		appBin:          newInput(""),
		monHelmfileBin:  newInput(""),
		monKustomizeBin: newInput(""),
		monHelmBin:      newInput("helm"),
	}
}

func (s *S5Aipub) Title() string { return i18n.T("AIPub") }
func (s *S5Aipub) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S5Aipub) SyncFromState(st *state.AppState) {
	s.enabled = st.AipubEnabled
	s.ingressZone.SetValue(st.AipubIngressZone)
	s.haMode = st.AipubHaMode
	s.storageClass.SetValue(st.AipubHaModeStorageClass)
	s.localPVNode.SetValue(st.AipubHarborLocalPVNode)
	s.k8sNodeNames = append([]string{}, st.K8sCpNodes...)
	s.cpNodeChecks = make([]bool, len(s.k8sNodeNames))
	for i, n := range s.k8sNodeNames {
		if len(st.AipubCpNodes) > 0 {
			s.cpNodeChecks[i] = st.IsAipubCpNode(n)
		} else {
			s.cpNodeChecks[i] = true
		}
	}
	s.harborSubdomain.SetValue(st.HarborIngressSubdomain)
	s.harborReplicas.SetValue(strconv.Itoa(st.HarborReplicaCount))
	s.harborRegistry.SetValue(st.HarborRegistryStorageSize)
	s.harborPg.SetValue(st.HarborPostgresqlStorageSize)
	s.harborRedis.SetValue(st.HarborRedisStorageSize)
	s.harborTrivy.SetValue(st.HarborTrivyStorageSize)
	s.edition = st.AipubAppEdition
	s.appTool = st.AipubAppDeployTool
	s.appBin.SetValue(st.AipubAppHelmfileBin)
	s.monTool = st.AipubMonitoringDeployTool
	s.monHelmfileBin.SetValue(st.AipubMonitoringHelmfileBin)
	s.monKustomizeBin.SetValue(st.AipubMonitoringKustomizeBin)
	s.monHelmBin.SetValue(st.AipubMonitoringHelmBin)
	s.monDryRun = st.AipubMonitoringHelmfileDryRun
	s.focusIdx = 0
	s.navIdx = 0
	s.cpNodeCursor = 0
	s.syncFocus()
}

func (s *S5Aipub) SyncToState(st *state.AppState) {
	st.AipubEnabled = s.enabled
	if !s.enabled {
		return
	}
	st.AipubIngressZone = s.ingressZone.Value()
	st.AipubHaMode = s.haMode
	st.AipubHaModeStorageClass = s.storageClass.Value()
	st.AipubHarborLocalPVNode = strings.TrimSpace(s.localPVNode.Value())
	st.AipubCpNodes = nil
	for i, n := range s.k8sNodeNames {
		if s.cpNodeChecks[i] {
			st.AipubCpNodes = append(st.AipubCpNodes, n)
		}
	}
	st.HarborIngressSubdomain = s.harborSubdomain.Value()
	rep, _ := strconv.Atoi(s.harborReplicas.Value())
	st.HarborReplicaCount = rep
	st.HarborRegistryStorageSize = s.harborRegistry.Value()
	st.HarborPostgresqlStorageSize = s.harborPg.Value()
	st.HarborRedisStorageSize = s.harborRedis.Value()
	st.HarborTrivyStorageSize = s.harborTrivy.Value()
	st.AipubAppEdition = s.edition
	st.AipubAppDeployTool = s.appTool
	st.AipubAppHelmfileBin = strings.TrimSpace(s.appBin.Value())
	st.AipubMonitoringDeployTool = s.monTool
	st.AipubMonitoringHelmfileBin = strings.TrimSpace(s.monHelmfileBin.Value())
	st.AipubMonitoringKustomizeBin = strings.TrimSpace(s.monKustomizeBin.Value())
	st.AipubMonitoringHelmBin = strings.TrimSpace(s.monHelmBin.Value())
	st.AipubMonitoringHelmfileDryRun = s.monDryRun
}

func (s *S5Aipub) Validate() []string {
	if !s.enabled {
		return nil
	}
	if s.ingressZone.Value() == "" {
		return []string{i18n.T("Enter the ingress domain")}
	}
	if s.haMode && s.storageClass.Value() == "" {
		return []string{i18n.T("Enter the StorageClass when AIPub HA mode is enabled")}
	}
	checked := map[string]bool{}
	for i, n := range s.k8sNodeNames {
		if s.cpNodeChecks[i] {
			checked[n] = true
		}
	}
	if len(checked) == 0 {
		return []string{i18n.T("Select at least one AIPub control plane node (Harbor and the AIPub services run on those nodes)")}
	}
	if !s.haMode {
		node := strings.TrimSpace(s.localPVNode.Value())
		if node == "" {
			return []string{i18n.T("Without HA, enter the node that holds the Harbor data")}
		}
		if !checked[node] {
			return []string{i18n.T("Harbor data node '%s' is not a selected AIPub control plane node", node)}
		}
	}
	if s.harborSubdomain.Value() == "" {
		return []string{i18n.T("Enter the Harbor subdomain")}
	}
	rep, err := strconv.Atoi(s.harborReplicas.Value())
	if err != nil || rep < 1 {
		return []string{i18n.T("The Harbor replica count must be an integer of 1 or more")}
	}
	return nil
}

func (s *S5Aipub) Init() tea.Cmd { return nil }

func (s *S5Aipub) inputs() []*textinput.Model {
	storage := &s.storageClass
	if !s.haMode {
		storage = &s.localPVNode
	}
	return []*textinput.Model{
		s5FocusEnabled:         nil,
		s5FocusIngressZone:     &s.ingressZone,
		s5FocusHaMode:          nil,
		s5FocusStorageClass:    storage,
		s5FocusCpNodes:         nil,
		s5FocusHarborSubdomain: &s.harborSubdomain,
		s5FocusHarborReplicas:  &s.harborReplicas,
		s5FocusHarborRegistry:  &s.harborRegistry,
		s5FocusHarborPg:        &s.harborPg,
		s5FocusHarborRedis:     &s.harborRedis,
		s5FocusHarborTrivy:     &s.harborTrivy,
		s5FocusEdition:         nil,
		s5FocusAppTool:         nil,
		s5FocusAppBin:          &s.appBin,
		s5FocusMonTool:         nil,
		s5FocusMonHelmfileBin:  &s.monHelmfileBin,
		s5FocusMonKustomizeBin: &s.monKustomizeBin,
		s5FocusMonHelmBin:      &s.monHelmBin,
		s5FocusMonDryRun:       nil,
	}
}

func (s *S5Aipub) syncFocus() {
	s.storageClass.Blur()
	s.localPVNode.Blur()
	for _, inp := range s.inputs() {
		if inp != nil {
			inp.Blur()
		}
	}
	inps := s.inputs()
	if s.focusIdx < len(inps) && inps[s.focusIdx] != nil {
		inps[s.focusIdx].Focus()
	}
}

func (s *S5Aipub) inputFocused() bool {
	inps := s.inputs()
	return s.focusIdx < len(inps) && inps[s.focusIdx] != nil
}

// shown reports whether a row is on the screen: without AIPub only the toggle,
// and the paths of a deploy tool's binaries only while that tool is helmfile.
func (s *S5Aipub) shown(idx int) bool {
	if !s.enabled {
		return idx == s5FocusEnabled || idx == s5FocusNav
	}
	switch idx {
	case s5FocusAppBin:
		return s.appTool == helmfileTool
	case s5FocusMonHelmfileBin, s5FocusMonKustomizeBin, s5FocusMonHelmBin, s5FocusMonDryRun:
		return s.monTool == helmfileTool
	}
	return true
}

func (s *S5Aipub) nextFocus() int {
	i := s.focusIdx + 1
	for i < s5FocusMax && !s.shown(i) {
		i++
	}
	return i
}

func (s *S5Aipub) prevFocus() int {
	i := s.focusIdx - 1
	for i > 0 && !s.shown(i) {
		i--
	}
	return i
}

func (s *S5Aipub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Letters typed into a text input are text: h/j/k/l move only elsewhere
		if msg.Type == tea.KeyRunes && s.inputFocused() {
			break
		}
		switch msg.String() {
		case "up", "k":
			if s.focusIdx == s5FocusCpNodes && s.cpNodeCursor > 0 {
				s.cpNodeCursor--
				return s, nil
			}
			if prev := s.prevFocus(); prev >= 0 {
				s.focusIdx = prev
				if s.focusIdx == s5FocusCpNodes {
					s.cpNodeCursor = len(s.cpNodeChecks) - 1
					if s.cpNodeCursor < 0 {
						s.cpNodeCursor = 0
					}
				}
				s.syncFocus()
			}
		case "down", "j":
			if s.focusIdx == s5FocusCpNodes && s.cpNodeCursor < len(s.cpNodeChecks)-1 {
				s.cpNodeCursor++
				return s, nil
			}
			if next := s.nextFocus(); next <= s5FocusMax {
				if next == s5FocusNav {
					s.navIdx = 1 // next
				}
				s.focusIdx = next
				s.cpNodeCursor = 0
				s.syncFocus()
			}
		case "left", "h":
			if s.focusIdx == s5FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right", "l":
			if s.focusIdx == s5FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s5FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s5FocusEnabled:
				s.enabled = !s.enabled
			case s5FocusHaMode:
				s.haMode = !s.haMode
				s.syncFocus()
			case s5FocusCpNodes:
				if s.cpNodeCursor < len(s.cpNodeChecks) {
					s.cpNodeChecks[s.cpNodeCursor] = !s.cpNodeChecks[s.cpNodeCursor]
				}
			case s5FocusEdition:
				s.edition = nextChoice("aipub_app_edition", s.edition)
			case s5FocusAppTool:
				s.appTool = nextChoice("aipub_app_deploy_tool", s.appTool)
			case s5FocusMonTool:
				s.monTool = nextChoice("aipub_monitoring_deploy_tool", s.monTool)
			case s5FocusMonDryRun:
				s.monDryRun = !s.monDryRun
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

func (s *S5Aipub) View() string {
	var b strings.Builder
	s.localPVNode.Placeholder = i18n.T("e.g. node1")
	s.appBin.Placeholder = i18n.T("default: the helmfile the package puts there")
	s.monHelmfileBin.Placeholder = i18n.T("default: same as the helmfile of AIPub")
	s.monKustomizeBin.Placeholder = i18n.T("default: the kustomize next to helmfile")
	b.WriteString(styles.StyleTitle.Render(i18n.T("AIPub")) + "\n\n")

	f := s.focusIdx
	// the focused part stays in sight on a low screen
	fs := focusSpan{b: &b}

	// install or not
	enabledStr := styles.RadioOff() + " " + i18n.T("Not installed")
	if s.enabled {
		enabledStr = styles.RadioOn() + " " + i18n.T("Install")
	}
	hint := ""
	if f == s5FocusEnabled {
		hint = styles.StyleMuted.Render("  Enter")
	}
	fs.mark(f == s5FocusEnabled)
	b.WriteString(RenderSectionHeader(i18n.T("AIPub install"), f == s5FocusEnabled) + hint + "\n")
	b.WriteString("    " + enabledStr + "\n\n")

	if !s.enabled {
		prevFocused := f == s5FocusNav && s.navIdx == 0
		nextFocused := f == s5FocusNav && s.navIdx == 1
		fs.mark(f == s5FocusNav)
		b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))
		return fs.fit(s.width, s.height)
	}

	fs.mark(f == s5FocusIngressZone)
	b.WriteString(RenderSectionHeader(i18n.T("Ingress domain"), f == s5FocusIngressZone) +
		"  " + s.ingressZone.View() + "\n")

	haStr := styles.RadioOff() + " " + i18n.T("Disabled")
	if s.haMode {
		haStr = styles.RadioOn() + " " + i18n.T("Enabled")
	}
	fs.mark(f == s5FocusHaMode)
	b.WriteString(RenderSectionHeader(i18n.T("AIPub HA mode"), f == s5FocusHaMode) +
		"  " + haStr + "\n")

	fs.mark(f == s5FocusStorageClass)
	if s.haMode {
		b.WriteString(RenderSectionHeader("StorageClass", f == s5FocusStorageClass) +
			"  " + s.storageClass.View() + styles.StyleMuted.Render("  "+short("aipub_ha_mode_storage_class")) + "\n\n")
	} else {
		b.WriteString(RenderSectionHeader(i18n.T("Harbor data node"), f == s5FocusStorageClass) +
			"  " + s.localPVNode.View() + styles.StyleMuted.Render("  "+i18n.T("one of the AIPub control plane nodes chosen below")) + "\n")
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("Without HA, the Harbor data is stored in a directory on this node. If the node is lost, so is the data")) + "\n\n")
	}

	fs.mark(f == s5FocusCpNodes)
	b.WriteString(RenderSectionHeader(i18n.T("AIPub control plane nodes"), f == s5FocusCpNodes) + "\n")
	for i, n := range s.k8sNodeNames {
		nodeFocused := f == s5FocusCpNodes && s.cpNodeCursor == i
		mark := styles.CheckOff()
		if s.cpNodeChecks[i] {
			mark = styles.CheckOn()
		}
		row := fmt.Sprintf("    %s %s", mark, n)
		b.WriteString(RenderRow(row, nodeFocused, s.width) + "\n")
	}
	if len(s.k8sNodeNames) == 0 {
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("(shown after the CP nodes are set in step 2)")) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(styles.StylePrimary.Render(i18n.T("Harbor settings")) + "\n")
	row := func(idx int, line string) {
		fs.mark(f == idx)
		b.WriteString(line + "\n")
	}
	row(s5FocusHarborSubdomain, RenderSectionHeader(i18n.T("Subdomain"), f == s5FocusHarborSubdomain)+"  "+s.harborSubdomain.View())
	row(s5FocusHarborReplicas, RenderSectionHeader(i18n.T("Replicas"), f == s5FocusHarborReplicas)+"  "+s.harborReplicas.View())
	row(s5FocusHarborRegistry, RenderSectionHeader(i18n.T("Registry storage"), f == s5FocusHarborRegistry)+"  "+s.harborRegistry.View())
	row(s5FocusHarborPg, RenderSectionHeader(i18n.T("PostgreSQL storage"), f == s5FocusHarborPg)+"  "+s.harborPg.View())
	row(s5FocusHarborRedis, RenderSectionHeader(i18n.T("Redis storage"), f == s5FocusHarborRedis)+"  "+s.harborRedis.View())
	row(s5FocusHarborTrivy, RenderSectionHeader(i18n.T("Trivy storage"), f == s5FocusHarborTrivy)+"  "+s.harborTrivy.View())

	fs.mark(false)
	b.WriteString("\n" + styles.StylePrimary.Render(i18n.T("Edition and deploy tools")) + "\n")
	row(s5FocusEdition, RenderSectionHeader(i18n.T("Edition"), f == s5FocusEdition)+"  "+
		choiceView("aipub_app_edition", s.edition)+styles.StyleMuted.Render("  "+i18n.T("lite leaves out aipub-nav")))
	row(s5FocusAppTool, RenderSectionHeader(i18n.T("AIPub deploy tool"), f == s5FocusAppTool)+"  "+
		choiceView("aipub_app_deploy_tool", s.appTool))
	if s.appTool == helmfileTool {
		row(s5FocusAppBin, RenderSectionHeader("  "+i18n.T("helmfile path"), f == s5FocusAppBin)+"  "+s.appBin.View())
	}
	row(s5FocusMonTool, RenderSectionHeader(i18n.T("Monitoring deploy tool"), f == s5FocusMonTool)+"  "+
		choiceView("aipub_monitoring_deploy_tool", s.monTool))
	if s.monTool == helmfileTool {
		row(s5FocusMonHelmfileBin, RenderSectionHeader("  "+i18n.T("helmfile path"), f == s5FocusMonHelmfileBin)+"  "+s.monHelmfileBin.View())
		row(s5FocusMonKustomizeBin, RenderSectionHeader("  "+i18n.T("kustomize path"), f == s5FocusMonKustomizeBin)+"  "+s.monKustomizeBin.View())
		row(s5FocusMonHelmBin, RenderSectionHeader("  "+i18n.T("helm path"), f == s5FocusMonHelmBin)+"  "+s.monHelmBin.View())
		dry := styles.RadioOff() + " " + i18n.T("Real deploy")
		if s.monDryRun {
			dry = styles.RadioOn() + " " + i18n.T("dry-run (writes nothing to the cluster)")
		}
		row(s5FocusMonDryRun, RenderSectionHeader("  helmfile dry-run", f == s5FocusMonDryRun)+"  "+dry)
	}
	if s.appTool == helmfileTool || s.monTool == helmfileTool {
		b.WriteString(styles.StyleMuted.Render("    "+i18n.T("helmfile is installed by the installer when the package carries aipub-bin. Leave it empty to use that path")) + "\n")
	}

	prevFocused := f == s5FocusNav && s.navIdx == 0
	nextFocused := f == s5FocusNav && s.navIdx == 1
	b.WriteString("\n")
	fs.mark(f == s5FocusNav)
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), prevFocused, nextFocused, s.width))

	return fs.fit(s.width, s.height)
}

// helmfileTool is the value of a deploy tool that asks for the helmfile binaries.
const helmfileTool = "helmfile"

// nextChoice is the value after cur among the values the schema allows for key,
// the first one for a value it does not know.
func nextChoice(key, cur string) string {
	v, ok := schema.Get(key)
	if !ok || len(v.Enum) == 0 {
		return cur
	}
	for i, e := range v.Enum {
		if e == cur {
			return v.Enum[(i+1)%len(v.Enum)]
		}
	}
	return v.Enum[0]
}

// choiceView shows the values the schema allows for key, the chosen one marked.
func choiceView(key, cur string) string {
	v, _ := schema.Get(key)
	parts := make([]string, len(v.Enum))
	for i, e := range v.Enum {
		if e == cur {
			parts[i] = styles.RadioOn() + " " + e
		} else {
			parts[i] = styles.RadioOff() + " " + e
		}
	}
	return strings.Join(parts, "   ")
}

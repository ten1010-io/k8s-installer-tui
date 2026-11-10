package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : AIPub 설치 여부 (toggle)
//   --- enabled일 때만 ---
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
//   11 : 에디션 (enterprise | lite)
//   12 : AIPub 배포 도구 (installer | helmfile)
//   13 : helmfile 경로            (helmfile 일 때만)
//   14 : 모니터링 배포 도구 (installer | helmfile)
//   15 : 모니터링 helmfile 경로    (모니터링이 helmfile 일 때만)
//   16 : 모니터링 kustomize 경로   (")
//   17 : 모니터링 helm 경로        (")
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
		localPVNode:     newInput("예: node1"),
		harborSubdomain: newInput("aipub-harbor"),
		harborReplicas:  newInput("3"),
		harborRegistry:  newInput("512Gi"),
		harborPg:        newInput("128Gi"),
		harborRedis:     newInput("32Gi"),
		harborTrivy:     newInput("5Gi"),

		appBin:          newInput("기본: 패키지가 두는 helmfile"),
		monHelmfileBin:  newInput("기본: AIPub 의 helmfile 과 같음"),
		monKustomizeBin: newInput("기본: helmfile 옆의 kustomize"),
		monHelmBin:      newInput("helm"),
	}
}

func (s *S5Aipub) Title() string { return "AIPub 설정" }
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
		return []string{"인그레스 도메인을 입력해주세요"}
	}
	if s.haMode && s.storageClass.Value() == "" {
		return []string{"AIPub HA 모드 활성화 시 StorageClass를 입력해주세요"}
	}
	checked := map[string]bool{}
	for i, n := range s.k8sNodeNames {
		if s.cpNodeChecks[i] {
			checked[n] = true
		}
	}
	if len(checked) == 0 {
		return []string{"AIPub CP 노드를 1개 이상 선택해주세요 (Harbor와 AIPub 서비스가 그 노드에서 돕니다)"}
	}
	if !s.haMode {
		node := strings.TrimSpace(s.localPVNode.Value())
		if node == "" {
			return []string{"HA가 아니면 Harbor 데이터를 둘 노드를 입력해주세요"}
		}
		if !checked[node] {
			return []string{"Harbor 데이터 노드 '" + node + "'는 선택한 AIPub CP 노드가 아닙니다"}
		}
	}
	if s.harborSubdomain.Value() == "" {
		return []string{"Harbor 서브도메인을 입력해주세요"}
	}
	rep, err := strconv.Atoi(s.harborReplicas.Value())
	if err != nil || rep < 1 {
		return []string{"Harbor 레플리카 수는 1 이상의 정수여야 합니다"}
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
					s.navIdx = 1 // 다음
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
	b.WriteString(styles.StyleTitle.Render("AIPub 설정") + "\n\n")

	f := s.focusIdx

	// AIPub 설치 여부
	enabledStr := styles.RadioOff() + " 미설치"
	if s.enabled {
		enabledStr = styles.RadioOn() + " 설치"
	}
	hint := ""
	if f == s5FocusEnabled {
		hint = styles.StyleMuted.Render("  Enter")
	}
	b.WriteString(RenderSectionHeader("AIPub 설치", f == s5FocusEnabled) + hint + "\n")
	b.WriteString("    " + enabledStr + "\n\n")

	if !s.enabled {
		prevFocused := f == s5FocusNav && s.navIdx == 0
		nextFocused := f == s5FocusNav && s.navIdx == 1
		b.WriteString(RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))
		return b.String()
	}

	b.WriteString(RenderSectionHeader("인그레스 도메인", f == s5FocusIngressZone) +
		"  " + s.ingressZone.View() + "\n")

	haStr := styles.RadioOff() + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn() + " 활성"
	}
	b.WriteString(RenderSectionHeader("AIPub HA 모드", f == s5FocusHaMode) +
		"  " + haStr + "\n")

	if s.haMode {
		b.WriteString(RenderSectionHeader("StorageClass", f == s5FocusStorageClass) +
			"  " + s.storageClass.View() + styles.StyleMuted.Render("  "+short("aipub_ha_mode_storage_class")) + "\n\n")
	} else {
		b.WriteString(RenderSectionHeader("Harbor 데이터 노드", f == s5FocusStorageClass) +
			"  " + s.localPVNode.View() + styles.StyleMuted.Render("  아래에서 선택한 CP 노드 중 하나") + "\n")
		b.WriteString(styles.StyleMuted.Render("    HA가 아니면 Harbor 데이터가 이 노드의 디렉터리에 저장됩니다. 노드를 잃으면 데이터도 잃습니다") + "\n\n")
	}

	b.WriteString(RenderSectionHeader("AIPub CP 노드", f == s5FocusCpNodes) + "\n")
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
		b.WriteString(styles.StyleMuted.Render("    (2단계에서 마스터 노드 지정 후 표시)") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(styles.StylePrimary.Render("Harbor 설정") + "\n")
	b.WriteString(RenderSectionHeader("서브도메인", f == s5FocusHarborSubdomain) + "  " + s.harborSubdomain.View() + "\n")
	b.WriteString(RenderSectionHeader("레플리카 수", f == s5FocusHarborReplicas) + "  " + s.harborReplicas.View() + "\n")
	b.WriteString(RenderSectionHeader("레지스트리 스토리지", f == s5FocusHarborRegistry) + "  " + s.harborRegistry.View() + "\n")
	b.WriteString(RenderSectionHeader("PostgreSQL 스토리지", f == s5FocusHarborPg) + "  " + s.harborPg.View() + "\n")
	b.WriteString(RenderSectionHeader("Redis 스토리지", f == s5FocusHarborRedis) + "  " + s.harborRedis.View() + "\n")
	b.WriteString(RenderSectionHeader("Trivy 스토리지", f == s5FocusHarborTrivy) + "  " + s.harborTrivy.View() + "\n")

	b.WriteString("\n" + styles.StylePrimary.Render("에디션과 배포 도구") + "\n")
	b.WriteString(RenderSectionHeader("에디션", f == s5FocusEdition) + "  " +
		choiceView("aipub_app_edition", s.edition) + styles.StyleMuted.Render("  lite 는 aipub-nav 를 뺍니다") + "\n")
	b.WriteString(RenderSectionHeader("AIPub 배포 도구", f == s5FocusAppTool) + "  " +
		choiceView("aipub_app_deploy_tool", s.appTool) + "\n")
	if s.appTool == helmfileTool {
		b.WriteString(RenderSectionHeader("  helmfile 경로", f == s5FocusAppBin) + "  " + s.appBin.View() + "\n")
	}
	b.WriteString(RenderSectionHeader("모니터링 배포 도구", f == s5FocusMonTool) + "  " +
		choiceView("aipub_monitoring_deploy_tool", s.monTool) + "\n")
	if s.monTool == helmfileTool {
		b.WriteString(RenderSectionHeader("  helmfile 경로", f == s5FocusMonHelmfileBin) + "  " + s.monHelmfileBin.View() + "\n")
		b.WriteString(RenderSectionHeader("  kustomize 경로", f == s5FocusMonKustomizeBin) + "  " + s.monKustomizeBin.View() + "\n")
		b.WriteString(RenderSectionHeader("  helm 경로", f == s5FocusMonHelmBin) + "  " + s.monHelmBin.View() + "\n")
		dry := styles.RadioOff() + " 실제 배포"
		if s.monDryRun {
			dry = styles.RadioOn() + " dry-run (클러스터에 쓰지 않음)"
		}
		b.WriteString(RenderSectionHeader("  helmfile dry-run", f == s5FocusMonDryRun) + "  " + dry + "\n")
	}
	if s.appTool == helmfileTool || s.monTool == helmfileTool {
		b.WriteString(styles.StyleMuted.Render("    helmfile 은 패키지가 aipub-bin 을 실었을 때 설치기가 깔아 줍니다. 비워 두면 그 경로를 씁니다") + "\n")
	}

	prevFocused := f == s5FocusNav && s.navIdx == 0
	nextFocused := f == s5FocusNav && s.navIdx == 1
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
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

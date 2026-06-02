package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// Focus layout:
//   0  : ingress zone
//   1  : HA mode toggle
//   2  : storage class
//   3  : CP nodes section header (→ inline list navigation)
//   4  : harbor subdomain
//   5  : harbor replicas
//   6  : harbor registry storage
//   7  : harbor postgresql storage
//   8  : harbor redis storage
//   9  : harbor trivy storage
//   10 : < 이전 >
//   11 : < 다음 >

const (
	s5FocusIngressZone = iota
	s5FocusHaMode
	s5FocusStorageClass
	s5FocusCpNodes
	s5FocusHarborSubdomain
	s5FocusHarborReplicas
	s5FocusHarborRegistry
	s5FocusHarborPg
	s5FocusHarborRedis
	s5FocusHarborTrivy
	s5FocusPrev
	s5FocusNext
	s5FocusMax = s5FocusNext
)

type S5Aipub struct {
	ingressZone     textinput.Model
	haMode          bool
	storageClass    textinput.Model
	k8sNodeNames    []string
	cpNodeChecks    []bool
	cpNodeCursor    int // cursor within CP nodes list (when s5FocusCpNodes is active)
	harborSubdomain textinput.Model
	harborReplicas  textinput.Model
	harborRegistry  textinput.Model
	harborPg        textinput.Model
	harborRedis     textinput.Model
	harborTrivy     textinput.Model

	focusIdx int
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
		ingressZone:     newInput("example.com"),
		storageClass:    newInput("ontap-nas"),
		harborSubdomain: newInput("aipub-harbor"),
		harborReplicas:  newInput("3"),
		harborRegistry:  newInput("512Gi"),
		harborPg:        newInput("128Gi"),
		harborRedis:     newInput("32Gi"),
		harborTrivy:     newInput("5Gi"),
	}
}

func (s *S5Aipub) Title() string { return "AIPub 설정" }
func (s *S5Aipub) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *S5Aipub) SyncFromState(st *state.AppState) {
	s.ingressZone.SetValue(st.AipubIngressZone)
	s.haMode = st.AipubHaMode
	s.storageClass.SetValue(st.AipubHaModeStorageClass)
	s.k8sNodeNames = append([]string{}, st.K8sNodes...)
	s.cpNodeChecks = make([]bool, len(s.k8sNodeNames))
	for i, n := range s.k8sNodeNames {
		s.cpNodeChecks[i] = st.IsAipubCpNode(n)
	}
	s.harborSubdomain.SetValue(st.HarborIngressSubdomain)
	s.harborReplicas.SetValue(strconv.Itoa(st.HarborReplicaCount))
	s.harborRegistry.SetValue(st.HarborRegistryStorageSize)
	s.harborPg.SetValue(st.HarborPostgresqlStorageSize)
	s.harborRedis.SetValue(st.HarborRedisStorageSize)
	s.harborTrivy.SetValue(st.HarborTrivyStorageSize)
	s.focusIdx = 0
	s.cpNodeCursor = 0
	s.syncFocus()
}

func (s *S5Aipub) SyncToState(st *state.AppState) {
	st.AipubIngressZone = s.ingressZone.Value()
	st.AipubHaMode = s.haMode
	st.AipubHaModeStorageClass = s.storageClass.Value()
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
}

func (s *S5Aipub) Validate() []string {
	if s.ingressZone.Value() == "" {
		return []string{"인그레스 도메인을 입력해주세요"}
	}
	if s.haMode && s.storageClass.Value() == "" {
		return []string{"AIPub HA 모드 활성화 시 StorageClass를 입력해주세요"}
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
	return []*textinput.Model{
		s5FocusIngressZone:     &s.ingressZone,
		s5FocusHaMode:          nil,
		s5FocusStorageClass:    &s.storageClass,
		s5FocusCpNodes:         nil,
		s5FocusHarborSubdomain: &s.harborSubdomain,
		s5FocusHarborReplicas:  &s.harborReplicas,
		s5FocusHarborRegistry:  &s.harborRegistry,
		s5FocusHarborPg:        &s.harborPg,
		s5FocusHarborRedis:     &s.harborRedis,
		s5FocusHarborTrivy:     &s.harborTrivy,
	}
}

func (s *S5Aipub) syncFocus() {
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

func (s *S5Aipub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.focusIdx == s5FocusCpNodes && s.cpNodeCursor > 0 {
				s.cpNodeCursor--
				return s, nil
			}
			if s.focusIdx > 0 {
				s.focusIdx--
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
			if s.focusIdx < s5FocusMax {
				s.focusIdx++
				s.cpNodeCursor = 0
				s.syncFocus()
			}
		case "enter", " ":
			switch s.focusIdx {
			case s5FocusPrev:
				return s, Prev()
			case s5FocusNext:
				return s, Next()
			case s5FocusHaMode:
				s.haMode = !s.haMode
			case s5FocusCpNodes:
				if s.cpNodeCursor < len(s.cpNodeChecks) {
					s.cpNodeChecks[s.cpNodeCursor] = !s.cpNodeChecks[s.cpNodeCursor]
				}
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
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

	b.WriteString(RenderSectionHeader("인그레스 도메인", f == s5FocusIngressZone) +
		"  " + s.ingressZone.View() + "\n")

	haStr := styles.RadioOff + " 비활성"
	if s.haMode {
		haStr = styles.RadioOn + " 활성"
	}
	b.WriteString(RenderSectionHeader("AIPub HA 모드", f == s5FocusHaMode) +
		"  " + haStr + "\n")

	scView := s.storageClass.View()
	if !s.haMode {
		scView = styles.StyleMuted.Render("(HA 비활성)")
	}
	b.WriteString(RenderSectionHeader("StorageClass", f == s5FocusStorageClass) +
		"  " + scView + "\n\n")

	b.WriteString(RenderSectionHeader("AIPub CP 노드", f == s5FocusCpNodes) + "\n")
	for i, n := range s.k8sNodeNames {
		nodeFocused := f == s5FocusCpNodes && s.cpNodeCursor == i
		mark := styles.CheckOff
		if s.cpNodeChecks[i] {
			mark = styles.CheckOn
		}
		row := fmt.Sprintf("    %s %s", mark, n)
		b.WriteString(RenderRow(row, nodeFocused, s.width) + "\n")
	}
	if len(s.k8sNodeNames) == 0 {
		b.WriteString(styles.StyleMuted.Render("    (2단계에서 k8s_node 지정 후 표시)") + "\n")
	}
	b.WriteString("\n")

	b.WriteString(styles.StylePrimary.Render("Harbor 설정") + "\n")
	b.WriteString(RenderSectionHeader("서브도메인", f == s5FocusHarborSubdomain) + "  " + s.harborSubdomain.View() + "\n")
	b.WriteString(RenderSectionHeader("레플리카 수", f == s5FocusHarborReplicas) + "  " + s.harborReplicas.View() + "\n")
	b.WriteString(RenderSectionHeader("레지스트리 스토리지", f == s5FocusHarborRegistry) + "  " + s.harborRegistry.View() + "\n")
	b.WriteString(RenderSectionHeader("PostgreSQL 스토리지", f == s5FocusHarborPg) + "  " + s.harborPg.View() + "\n")
	b.WriteString(RenderSectionHeader("Redis 스토리지", f == s5FocusHarborRedis) + "  " + s.harborRedis.View() + "\n")
	b.WriteString(RenderSectionHeader("Trivy 스토리지", f == s5FocusHarborTrivy) + "  " + s.harborTrivy.View() + "\n")

	prevFocused := f == s5FocusPrev
	nextFocused := f == s5FocusNext
	b.WriteString("\n" + RenderNavButtons("이전", "다음", prevFocused, nextFocused, s.width))

	return b.String()
}

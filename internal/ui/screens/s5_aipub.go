package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
)

const (
	s5IngressZone = iota
	s5HaMode
	s5StorageClass
	s5CpNodes
	s5HarborSubdomain
	s5HarborReplicas
	s5HarborRegistry
	s5HarborPg
	s5HarborRedis
	s5HarborTrivy
	s5FieldCount
)

type S5Aipub struct {
	ingressZone    textinput.Model
	haMode         bool
	storageClass   textinput.Model
	k8sNodeNames   []string
	cpNodeChecks   []bool
	harborSubdomain textinput.Model
	harborReplicas  textinput.Model
	harborRegistry  textinput.Model
	harborPg        textinput.Model
	harborRedis     textinput.Model
	harborTrivy     textinput.Model

	focus  int
	width  int
	height int
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

func (s *S5Aipub) Title() string      { return "AIPub 설정" }
func (s *S5Aipub) SetSize(w, h int)   { s.width = w; s.height = h }
func (s *S5Aipub) FooterHelp() string {
	return "tab: 필드 이동  space: HA 토글  숫자: CP 노드 토글  ctrl+n: 다음"
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

func (s *S5Aipub) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			s.focus = (s.focus + 1) % s5FieldCount
			s.syncFocus()
		case "shift+tab":
			s.focus = (s.focus - 1 + s5FieldCount) % s5FieldCount
			s.syncFocus()
		case " ":
			if s.focus == s5HaMode {
				s.haMode = !s.haMode
			}
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		default:
			// Numeric keys toggle cp nodes
			if s.focus == s5CpNodes && len(msg.String()) == 1 {
				idx := int(msg.String()[0] - '1')
				if idx >= 0 && idx < len(s.cpNodeChecks) {
					s.cpNodeChecks[idx] = !s.cpNodeChecks[idx]
				}
			}
		}
	}
	var cmd tea.Cmd
	switch s.focus {
	case s5IngressZone:
		s.ingressZone, cmd = s.ingressZone.Update(msg)
	case s5StorageClass:
		s.storageClass, cmd = s.storageClass.Update(msg)
	case s5HarborSubdomain:
		s.harborSubdomain, cmd = s.harborSubdomain.Update(msg)
	case s5HarborReplicas:
		s.harborReplicas, cmd = s.harborReplicas.Update(msg)
	case s5HarborRegistry:
		s.harborRegistry, cmd = s.harborRegistry.Update(msg)
	case s5HarborPg:
		s.harborPg, cmd = s.harborPg.Update(msg)
	case s5HarborRedis:
		s.harborRedis, cmd = s.harborRedis.Update(msg)
	case s5HarborTrivy:
		s.harborTrivy, cmd = s.harborTrivy.Update(msg)
	}
	return s, cmd
}

func (s *S5Aipub) syncFocus() {
	inputs := []*textinput.Model{
		s5IngressZone: &s.ingressZone,
		s5StorageClass: &s.storageClass,
		s5HarborSubdomain: &s.harborSubdomain,
		s5HarborReplicas: &s.harborReplicas,
		s5HarborRegistry: &s.harborRegistry,
		s5HarborPg: &s.harborPg,
		s5HarborRedis: &s.harborRedis,
		s5HarborTrivy: &s.harborTrivy,
	}
	for i, inp := range inputs {
		if inp == nil {
			continue
		}
		if i == s.focus {
			inp.Focus()
		} else {
			inp.Blur()
		}
	}
}

func (s *S5Aipub) View() string {
	var b strings.Builder
	b.WriteString(ui.StyleTitle.Render("AIPub 설정") + "\n\n")

	rf := func(label string, f int, view string) string {
		return renderField(label, view, s.focus == f)
	}

	b.WriteString(rf("인그레스 도메인", s5IngressZone, s.ingressZone.View()))

	// HA Mode
	haStr := ui.RadioOff + " 비활성"
	if s.haMode {
		haStr = ui.RadioOn + " 활성"
	}
	haLabel := ui.StyleLabel.Render("AIPub HA 모드:")
	if s.focus == s5HaMode {
		haLabel = ui.StyleLabelFocused.Render("AIPub HA 모드:")
	}
	b.WriteString(haLabel + "  " + haStr + "\n")

	scView := s.storageClass.View()
	if !s.haMode {
		scView = ui.StyleMuted.Render("(HA 비활성 — 불필요)")
	}
	b.WriteString(rf("StorageClass", s5StorageClass, scView))

	// CP Nodes checklist
	cpLabel := ui.StyleLabel.Render("AIPub CP 노드:")
	if s.focus == s5CpNodes {
		cpLabel = ui.StyleLabelFocused.Render("AIPub CP 노드:")
	}
	b.WriteString(cpLabel + "\n")
	for i, n := range s.k8sNodeNames {
		mark := ui.CheckOff
		if s.cpNodeChecks[i] {
			mark = ui.CheckOn
		}
		b.WriteString(fmt.Sprintf("  [%d] %s %s\n", i+1, mark, n))
	}
	b.WriteString(ui.StyleMuted.Render("  숫자 키로 토글") + "\n")

	b.WriteString("\n" + ui.StylePrimary.Render("Harbor 설정") + "\n")
	b.WriteString(rf("서브도메인", s5HarborSubdomain, s.harborSubdomain.View()))
	b.WriteString(rf("레플리카 수", s5HarborReplicas, s.harborReplicas.View()))
	b.WriteString(rf("레지스트리 스토리지", s5HarborRegistry, s.harborRegistry.View()))
	b.WriteString(rf("PostgreSQL 스토리지", s5HarborPg, s.harborPg.View()))
	b.WriteString(rf("Redis 스토리지", s5HarborRedis, s.harborRedis.View()))
	b.WriteString(rf("Trivy 스토리지", s5HarborTrivy, s.harborTrivy.View()))

	return b.String()
}

package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// What k8s-installer adds to the static pods of the control plane, through the
// ClusterConfiguration of kubeadm (group_vars/all/vars.yml → k8s_*_extra_args,
// k8s_*_extra_volumes, k8s_cp_extra_files). kubeadm keeps them and applies them
// again to a node that joins and on every upgrade, which a hand edit of
// /etc/kubernetes/manifests does not survive.
//
// Focus layout:
//   0  : AIPub 감사 로그 (toggle)
//   1  : runtime-config (toggle)
//   2  : AIPub 인증 웹훅 (toggle)
//   3  : 웹훅 URL
//   4  : 추가할 대상 (←/→)
//   5  : 추가할 항목 (Enter: 추가)
//   6… : 직접 넣은 항목 (d: 삭제)
//   last: nav slot (←/→: 이전/다음)

const (
	s4bFocusAudit = iota
	s4bFocusRuntime
	s4bFocusWebhook
	s4bFocusURL
	s4bFocusTarget
	s4bFocusEntry
	s4bFocusList // the first entry, then the nav slot after the last
)

// cpTarget is a list an entry typed in goes to.
type cpTarget struct {
	label     string
	kind      string // arg, volume, file
	component string
	hint      string
}

var cpTargets = []cpTarget{
	{"apiserver 인자", "arg", "apiserver", "이름=값 (대시 없이), 예: oidc-issuer-url=https://idp.example.com"},
	{"apiserver 볼륨", "volume", "apiserver", "이름 hostPath mountPath [ro] [pathType], 예: oidc /etc/oidc /etc/oidc ro Directory"},
	{"controller-manager 인자", "arg", "controller_manager", "이름=값, 예: node-cidr-mask-size=22"},
	{"controller-manager 볼륨", "volume", "controller_manager", "이름 hostPath mountPath [ro] [pathType]"},
	{"scheduler 인자", "arg", "scheduler", "이름=값"},
	{"scheduler 볼륨", "volume", "scheduler", "이름 hostPath mountPath [ro] [pathType]"},
	{"etcd 인자", "arg", "etcd", "이름=값, 예: quota-backend-bytes=8589934592 (etcd 는 볼륨 없음)"},
	{"컨트롤플레인 파일", "file", "", "src dest [mode], 예: oidc/ca.crt /etc/oidc/ca.crt 0644 (src 는 ansible/files 아래)"},
}

var volumePathTypes = []string{"DirectoryOrCreate", "Directory", "FileOrCreate", "File"}

// cpEntry is one entry typed in, as listed on the screen.
type cpEntry struct {
	target int
	index  int
	text   string
}

type S4bControlPlane struct {
	cp       state.ControlPlane
	url      textinput.Model
	entry    textinput.Model
	target   int
	entryErr string

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS4bControlPlane() *S4bControlPlane {
	url := textinput.New()
	url.Placeholder = "https://<AIPub 토큰 검증 서버>"
	url.CharLimit = 256
	entry := textinput.New()
	entry.CharLimit = 256
	return &S4bControlPlane{url: url, entry: entry}
}

func (s *S4bControlPlane) Title() string { return "컨트롤플레인" }
func (s *S4bControlPlane) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.url.Width = w - 34
	s.entry.Width = w - 34
}

// copyControlPlane copies the lists, so that the screen edits its own until it
// syncs.
func copyControlPlane(c state.ControlPlane) state.ControlPlane {
	out := c
	out.Args = map[string][]state.ExtraArg{}
	for k, v := range c.Args {
		out.Args[k] = append([]state.ExtraArg{}, v...)
	}
	out.Volumes = map[string][]state.ExtraVolume{}
	for k, v := range c.Volumes {
		out.Volumes[k] = append([]state.ExtraVolume{}, v...)
	}
	out.Files = append([]state.CpExtraFile(nil), c.Files...)
	return out
}

func (s *S4bControlPlane) SyncFromState(st *state.AppState) {
	s.cp = copyControlPlane(st.ControlPlane)
	s.url.SetValue(s.cp.WebhookURL)
	s.entry.SetValue("")
	s.entryErr = ""
	s.focusIdx = 0
	s.navIdx = 0
	s.syncFocus()
}

func (s *S4bControlPlane) SyncToState(st *state.AppState) {
	s.cp.WebhookURL = strings.TrimSpace(s.url.Value())
	st.ControlPlane = copyControlPlane(s.cp)
}

// Validate stops on what this screen alone can tell; the rules of the variables
// are the validator's, run on the preview.
func (s *S4bControlPlane) Validate() []string {
	if s.cp.Webhook && strings.TrimSpace(s.url.Value()) == "" {
		return []string{"AIPub 인증 웹훅을 켰으면 토큰 검증 서버 URL 을 입력해주세요"}
	}
	if strings.TrimSpace(s.entry.Value()) != "" {
		return []string{"추가할 항목에 입력한 것이 있습니다. Enter 로 추가하거나 지워주세요"}
	}
	return nil
}

func (s *S4bControlPlane) Init() tea.Cmd { return nil }

// entries lists the entries typed in, in the order of the targets.
func (s *S4bControlPlane) entries() []cpEntry {
	var out []cpEntry
	for ti, t := range cpTargets {
		switch t.kind {
		case "arg":
			for i, a := range s.cp.Args[t.component] {
				out = append(out, cpEntry{ti, i, a.Name + "=" + a.Value})
			}
		case "volume":
			for i, v := range s.cp.Volumes[t.component] {
				out = append(out, cpEntry{ti, i, volumeText(v)})
			}
		case "file":
			for i, f := range s.cp.Files {
				out = append(out, cpEntry{ti, i, f.Src + " → " + f.Dest + " (" + f.Mode + ")"})
			}
		}
	}
	return out
}

func volumeText(v state.ExtraVolume) string {
	text := v.Name + ": " + v.HostPath + " → " + v.MountPath
	if v.ReadOnly {
		text += " ro"
	}
	if v.PathType != "" {
		text += " " + v.PathType
	}
	return text
}

func (s *S4bControlPlane) navFocus() int { return s4bFocusList + len(s.entries()) }

func (s *S4bControlPlane) syncFocus() {
	if s.focusIdx == s4bFocusURL {
		s.url.Focus()
	} else {
		s.url.Blur()
	}
	if s.focusIdx == s4bFocusEntry {
		s.entry.Focus()
	} else {
		s.entry.Blur()
	}
}

// addEntry parses what was typed for the target and adds it.
func (s *S4bControlPlane) addEntry() {
	text := strings.TrimSpace(s.entry.Value())
	if text == "" {
		return
	}
	t := cpTargets[s.target]
	switch t.kind {
	case "arg":
		name, value, ok := strings.Cut(text, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			s.entryErr = "인자는 이름=값 으로 입력합니다"
			return
		}
		if s.cp.Args == nil {
			s.cp.Args = map[string][]state.ExtraArg{}
		}
		s.cp.Args[t.component] = append(s.cp.Args[t.component],
			state.ExtraArg{Name: name, Value: strings.TrimSpace(value)})
	case "volume":
		f := strings.Fields(text)
		if len(f) < 3 {
			s.entryErr = "볼륨은 이름 hostPath mountPath [ro] [pathType] 으로 입력합니다"
			return
		}
		v := state.ExtraVolume{Name: f[0], HostPath: f[1], MountPath: f[2]}
		for _, opt := range f[3:] {
			switch {
			case opt == "ro" || opt == "readOnly":
				v.ReadOnly = true
			case contains(volumePathTypes, opt):
				v.PathType = opt
			default:
				s.entryErr = fmt.Sprintf("'%s' 는 ro 또는 %s 가 아닙니다", opt, strings.Join(volumePathTypes, ", "))
				return
			}
		}
		if s.cp.Volumes == nil {
			s.cp.Volumes = map[string][]state.ExtraVolume{}
		}
		s.cp.Volumes[t.component] = append(s.cp.Volumes[t.component], v)
	case "file":
		f := strings.Fields(text)
		if len(f) < 2 || len(f) > 3 {
			s.entryErr = "파일은 src dest [mode] 로 입력합니다"
			return
		}
		file := state.CpExtraFile{Src: f[0], Dest: f[1], Mode: "0644"}
		if len(f) == 3 {
			file.Mode = f[2]
		}
		s.cp.Files = append(s.cp.Files, file)
	}
	s.entry.SetValue("")
	s.entryErr = ""
}

func (s *S4bControlPlane) removeEntry(e cpEntry) {
	t := cpTargets[e.target]
	switch t.kind {
	case "arg":
		l := s.cp.Args[t.component]
		s.cp.Args[t.component] = append(l[:e.index:e.index], l[e.index+1:]...)
	case "volume":
		l := s.cp.Volumes[t.component]
		s.cp.Volumes[t.component] = append(l[:e.index:e.index], l[e.index+1:]...)
	case "file":
		s.cp.Files = append(s.cp.Files[:e.index:e.index], s.cp.Files[e.index+1:]...)
	}
	if s.focusIdx > s.navFocus() {
		s.focusIdx = s.navFocus()
	}
}

func contains(l []string, x string) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

func (s *S4bControlPlane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		nav := s.navFocus()
		inInput := s.focusIdx == s4bFocusURL || s.focusIdx == s4bFocusEntry
		switch msg.String() {
		case "up":
			if s.focusIdx > 0 {
				s.focusIdx--
				s.syncFocus()
			}
			return s, nil
		case "down":
			if s.focusIdx < nav {
				s.focusIdx++
				s.syncFocus()
			}
			return s, nil
		case "ctrl+n":
			return s, Next()
		case "ctrl+p":
			return s, Prev()
		case "left", "right":
			switch s.focusIdx {
			case s4bFocusTarget:
				n := len(cpTargets)
				if msg.String() == "left" {
					s.target = (s.target + n - 1) % n
				} else {
					s.target = (s.target + 1) % n
				}
				s.entryErr = ""
				return s, nil
			case nav:
				if msg.String() == "left" {
					s.navIdx = 0
				} else {
					s.navIdx = 1
				}
				return s, nil
			}
		case "enter", " ":
			if msg.String() == " " && inInput {
				break
			}
			switch {
			case s.focusIdx == s4bFocusAudit:
				s.cp.Audit = !s.cp.Audit
			case s.focusIdx == s4bFocusRuntime:
				s.cp.RuntimeConfig = !s.cp.RuntimeConfig
			case s.focusIdx == s4bFocusWebhook:
				s.cp.Webhook = !s.cp.Webhook
			case s.focusIdx == s4bFocusEntry && msg.String() == "enter":
				s.addEntry()
			case s.focusIdx == nav && msg.String() == "enter":
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, Next()
			case s.focusIdx == s4bFocusURL || s.focusIdx == s4bFocusTarget:
				s.focusIdx++
				s.syncFocus()
			}
			return s, nil
		case "d", "delete":
			if s.focusIdx >= s4bFocusList && s.focusIdx < nav {
				s.removeEntry(s.entries()[s.focusIdx-s4bFocusList])
				return s, nil
			}
		}
	}
	var cmd tea.Cmd
	switch s.focusIdx {
	case s4bFocusURL:
		s.url, cmd = s.url.Update(msg)
	case s4bFocusEntry:
		s.entry, cmd = s.entry.Update(msg)
	}
	return s, cmd
}

func checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

func (s *S4bControlPlane) View() string {
	var b strings.Builder
	f := s.focusIdx
	w := s.width
	b.WriteString(styles.StyleTitle.Render("컨트롤플레인") + "\n")
	b.WriteString(styles.StyleMuted.Render("kube-apiserver 등에 더할 인자·볼륨·파일 (vars.yml). kubeadm 이 클러스터 설정으로 보관해 노드 추가·업그레이드에도") + "\n")
	b.WriteString(styles.StyleMuted.Render("다시 적용합니다. 바꾸면 update-cluster.yml 이 컨트롤 플레인 노드를 한 대씩 재시작합니다. 매니페스트를 손으로 고치지 마세요") + "\n\n")

	b.WriteString(styles.StylePrimary.Render("프리셋") + styles.StyleMuted.Render("  Space/Enter: 켜기/끄기") + "\n")
	b.WriteString(RenderRow(checkbox(s.cp.Audit)+" AIPub 감사 로그", f == s4bFocusAudit, w) + "\n")
	b.WriteString(styles.StyleMuted.Render("    audit-policy-file, audit-log-path=/var/log/aipub/usage/audit.log, 30일·30개·100MB. 첫 설치부터") + "\n")
	b.WriteString(styles.StyleMuted.Render("    정책 파일 ansible/files/aipub/audit-policy.yaml 은 사이트가 둡니다 (AIPub 모니터링/python 팀 제공)") + "\n")
	b.WriteString(RenderRow(checkbox(s.cp.RuntimeConfig)+" runtime-config (admissionregistration.k8s.io/v1beta1=true)", f == s4bFocusRuntime, w) + "\n")
	b.WriteString(styles.StyleMuted.Render("    첫 설치부터") + "\n")
	b.WriteString(RenderRow(checkbox(s.cp.Webhook)+" AIPub 인증 웹훅 (authentication-token-webhook, cache-ttl 2m)", f == s4bFocusWebhook, w) + "\n")
	b.WriteString(styles.StyleMuted.Render("    AIPub 설치 시에만. setup-cert 가 만드는 CA("+state.WebhookCAPath+")가 있어야 apiserver 가 뜨므로") + "\n")
	b.WriteString(styles.StyleMuted.Render("    클러스터 생성(K3)에서는 빼고, A2 setup-cert 다음 A2.5 update-cluster 로 적용합니다 (노드 한 대씩 재시작)") + "\n")
	b.WriteString(RenderSectionHeader("  웹훅 URL", f == s4bFocusURL) + "  " + s.url.View() + "\n")
	b.WriteString(styles.StyleMuted.Render("    ansible/files/"+state.WebhookConfigFile+" (kubeconfig) 로 만들어 둡니다. 평문 복사 — 비밀 금지") + "\n\n")

	b.WriteString(styles.StylePrimary.Render("직접 추가") + styles.StyleMuted.Render("  apiserver·controller-manager·scheduler·etcd 의 인자와 볼륨, 컨트롤플레인 파일") + "\n")
	t := cpTargets[s.target]
	b.WriteString(RenderSectionHeader("대상", f == s4bFocusTarget) + "  ◀ " + t.label + " ▶\n")
	b.WriteString(RenderSectionHeader("항목 (Enter: 추가)", f == s4bFocusEntry) + "  " + s.entry.View() + "\n")
	b.WriteString(styles.StyleMuted.Render("    "+t.hint) + "\n")
	if s.entryErr != "" {
		b.WriteString(styles.StyleError.Render("    ✗ "+s.entryErr) + "\n")
	}
	entries := s.entries()
	if len(entries) == 0 {
		b.WriteString(styles.StyleMuted.Render("    (직접 넣은 항목 없음)") + "\n")
	}
	for i, e := range entries {
		label := cpTargets[e.target].label
		pad := 24 - lipgloss.Width(label)
		if pad < 1 {
			pad = 1
		}
		row := "  " + label + strings.Repeat(" ", pad) + e.text
		if f == s4bFocusList+i {
			row += "   (d: 삭제)"
		}
		b.WriteString(RenderRow(row, f == s4bFocusList+i, w) + "\n")
	}
	b.WriteString(styles.StyleMuted.Render("    컨트롤플레인 파일은 모든 컨트롤 플레인 노드에 평문으로 복사됩니다. 비밀(키·토큰)은 넣지 마세요") + "\n")

	nav := s.navFocus()
	b.WriteString("\n" + RenderNavButtons("이전", "다음", f == nav && s.navIdx == 0, f == nav && s.navIdx == 1, w))
	return b.String()
}

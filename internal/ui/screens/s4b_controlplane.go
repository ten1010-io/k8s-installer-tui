package screens

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// What k8s-installer adds to the static pods of the control plane, through the
// ClusterConfiguration of kubeadm (group_vars/all/vars.yml → k8s_*_extra_args,
// k8s_*_extra_volumes, k8s_cp_extra_files). kubeadm keeps them and applies them
// again to a node that joins and on every upgrade, which a hand edit of
// /etc/kubernetes/manifests does not survive.
//
// The screen shows the presets, one row and one short line each; the details of
// the focused row are shown under it. What it shows depends on the state, and so
// do the stops of the focus (stops), in this order:
//
//	AIPub 감사 로그, runtime-config, AIPub 인증 (JWKS) (toggles)
//	  issuer URL, CA 파일                  (while JWKS is on)
//	AIPub 인증 웹훅, 레거시 (toggle)       (here on an existing site or when it was on)
//	  웹훅 URL                              (while the webhook is on)
//	▸ 고급: 직접 추가 (Enter/Space, →/←: 펼치기/접기)
//	  the webhook when not above, 추가할 대상 (←/→), 추가할 항목 (Enter: 추가),
//	  직접 넣은 항목 (d: 삭제)              (while expanded)
//	nav slot (←/→: 이전/다음)
//
// Off the text inputs, on an existing site: c reads what the apiserver of the
// first control plane node was given by hand (read only) and turns on the
// presets that stand for it; o on the audit log says the default audit policy
// may replace the nodes' (when theirs could not be read).

// The kinds of focus stop.
const (
	s4bAudit = iota
	s4bRuntime
	s4bJWKS
	s4bIssuer
	s4bCA
	s4bWebhook
	s4bURL
	s4bAdvanced
	s4bTarget
	s4bEntry
	s4bList // an entry typed in, by its place in entries()
	s4bNav
)

// s4bStop is a row the focus can be on.
type s4bStop struct {
	kind int
	i    int // the entry, for s4bList
}

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
	issuer   textinput.Model
	derived  string // the issuer that follows the zone, prefilled
	caFile   textinput.Model
	url      textinput.Model
	entry    textinput.Model
	target   int
	entryErr string

	// the check of an existing site (c)
	opts      plan.Options
	run       plan.RunFunc
	site      *state.AppState // the state the check reads the nodes of
	legacy    *plan.Legacy
	legacyErr string
	checking  bool

	advanced  bool // the section of the entries typed in is expanded
	webhookOn bool // the webhook was on when the screen was entered

	focus    s4bStop
	focusPos int // the place of the focus among the stops, where it lands when its row goes away
	navIdx   int
	width    int
	height   int
}

// legacyDoneMsg carries what the check of the apiserver of an existing site read.
type legacyDoneMsg struct {
	legacy *plan.Legacy
	err    error
}

func NewS4bControlPlane() *S4bControlPlane {
	url := textinput.New()
	url.Placeholder = "https://<AIPub 토큰 검증 서버>"
	url.CharLimit = 256
	issuer := textinput.New()
	issuer.Placeholder = "https://<aipub_app_host_prefix>.<aipub_ingress_zone>"
	issuer.CharLimit = 256
	caFile := textinput.New()
	caFile.Placeholder = state.JWKSCAPath
	caFile.CharLimit = 256
	entry := textinput.New()
	entry.CharLimit = 256
	return &S4bControlPlane{issuer: issuer, caFile: caFile, url: url, entry: entry, run: plan.ExecRun}
}

// SetSite tells the screen where k8s-installer is and whether the site existed
// before: only then is there an apiserver to read.
func (s *S4bControlPlane) SetSite(o plan.Options) { s.opts = o }

// SetRunner replaces how the check runs commands (tests).
func (s *S4bControlPlane) SetRunner(r plan.RunFunc) { s.run = r }

func (s *S4bControlPlane) Title() string { return "컨트롤플레인" }
func (s *S4bControlPlane) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.url.Width = w - 34
	s.issuer.Width = w - 34
	s.caFile.Width = w - 34
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
	s.derived = st.JWKSIssuerDerived()
	if s.cp.JWKSIssuerURL != "" {
		s.issuer.SetValue(s.cp.JWKSIssuerURL)
	} else {
		s.issuer.SetValue(s.derived)
	}
	if s.cp.JWKSCAFile != "" {
		s.caFile.SetValue(s.cp.JWKSCAFile)
	} else {
		s.caFile.SetValue(fileio.DefaultCAPath)
	}
	s.url.SetValue(s.cp.WebhookURL)
	s.entry.SetValue("")
	s.entryErr = ""
	s.site = st
	// nothing the site has is hidden: entries expand the section, a webhook on
	// is shown with the presets
	s.advanced = len(s.entries()) > 0
	s.webhookOn = s.cp.Webhook
	s.focus, s.focusPos = s4bStop{kind: s4bAudit}, 0
	s.navIdx = 0
	s.settle()
}

func (s *S4bControlPlane) SyncToState(st *state.AppState) {
	s.cp.WebhookURL = strings.TrimSpace(s.url.Value())
	s.cp.JWKSIssuerURL = s.issuerOverride()
	s.cp.JWKSCAFile = s.caOverride()
	st.ControlPlane = copyControlPlane(s.cp)
}

// caOverride is the CA file typed in, empty when it is the default one.
func (s *S4bControlPlane) caOverride() string {
	v := strings.TrimSpace(s.caFile.Value())
	if v == fileio.DefaultCAPath {
		return ""
	}
	return v
}

// ca is the CA the authentication config would be written with now.
func (s *S4bControlPlane) ca() fileio.CA {
	cp := s.cp
	cp.JWKSCAFile = s.caOverride()
	return fileio.LookupCA(&state.AppState{ControlPlane: cp})
}

// checkLegacy reads the apiserver of the first control plane node, read only.
func (s *S4bControlPlane) checkLegacy() tea.Cmd {
	if !s.opts.Existing || s.checking || s.site == nil {
		return nil
	}
	s.checking, s.legacy, s.legacyErr = true, nil, ""
	o, run, st := s.opts, s.run, *s.site
	return func() tea.Msg {
		l, err := plan.ReadLegacy(context.Background(), o, &st, run)
		return legacyDoneMsg{l, err}
	}
}

// applyLegacy turns on the presets the node runs with, and fills the webhook URL
// and the issuer from its files where they are not set: what update-cluster.yml
// then applies is what the node runs with now.
func (s *S4bControlPlane) applyLegacy(l *plan.Legacy) {
	for _, p := range state.Presets {
		if l.Presets[p.ID] {
			s.cp.Set(p.ID, true)
		}
	}
	if l.WebhookURL != "" && strings.TrimSpace(s.url.Value()) == "" {
		s.url.SetValue(l.WebhookURL)
	}
	// the audit policy: the node's taken into files/aipub, or none on the node
	// for the default to replace
	if !l.Presets[state.PresetAudit] {
		s.cp.AuditDefaultOK = true
	} else if l.Audit.Taken {
		s.cp.AuditFrom = l.Node
	}
	if v := strings.TrimSpace(s.issuer.Value()); l.Issuer != "" && (v == "" || v == s.derived) {
		s.issuer.SetValue(l.Issuer)
	}
}

// auditPlan is what saving would do with the audit policy, with the presets of
// the screen.
func (s *S4bControlPlane) auditPlan() fileio.AuditPolicyPlan {
	st := state.AppState{}
	if s.site != nil {
		st = *s.site
	}
	st.Existing, st.ControlPlane = s.opts.Existing, s.cp
	return fileio.PlanAuditPolicy(s.opts.AuditPolicy, &st)
}

// issuerOverride is the issuer typed in, empty when it is the derived one: that
// one follows a change of the zone made later.
func (s *S4bControlPlane) issuerOverride() string {
	v := strings.TrimSpace(s.issuer.Value())
	if v == s.derived {
		return ""
	}
	return v
}

// Validate stops on the rules of the variables of this screen, the webhook URL
// and an issuer typed in. That the webhook needs AIPub, and an issuer derived
// from a zone, are checked on the preview, once AIPub is chosen.
func (s *S4bControlPlane) Validate() []string {
	if strings.TrimSpace(s.entry.Value()) != "" {
		return []string{"추가할 항목에 입력한 것이 있습니다. Enter 로 추가하거나 지워주세요"}
	}
	cp := copyControlPlane(s.cp)
	cp.WebhookURL = strings.TrimSpace(s.url.Value())
	cp.JWKSIssuerURL = s.issuerOverride()
	var msgs []string
	for _, e := range validator.ControlPlane(cp) {
		msgs = append(msgs, e.Error())
	}
	// a CA file that is there but is not a CA: never copied, and said so. One
	// not there yet is waited for (setup-cert puts it there)
	if ca := s.ca(); cp.JWKS && ca.Err != nil && !errors.Is(ca.Err, fs.ErrPermission) {
		msgs = append(msgs, "JWKS CA 파일: "+ca.Err.Error())
	}
	return msgs
}

// RevealErrors expands 고급 when the refusal names text left in its input, so
// the input is in sight.
func (s *S4bControlPlane) RevealErrors() {
	if strings.TrimSpace(s.entry.Value()) != "" {
		s.advanced = true
	}
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

// webhookUp says whether the legacy webhook is listed with the presets: on an
// existing site, or when it was on or found on the node. A new site finds it in
// the advanced section.
func (s *S4bControlPlane) webhookUp() bool {
	return s.opts.Existing || s.webhookOn || (s.legacy != nil && s.legacy.Presets[state.PresetWebhook])
}

// stops are the rows the focus can be on now, in the order of the screen.
func (s *S4bControlPlane) stops() []s4bStop {
	st := []s4bStop{{kind: s4bAudit}, {kind: s4bRuntime}, {kind: s4bJWKS}}
	if s.cp.JWKS {
		st = append(st, s4bStop{kind: s4bIssuer}, s4bStop{kind: s4bCA})
	}
	webhook := func() {
		st = append(st, s4bStop{kind: s4bWebhook})
		if s.cp.Webhook {
			st = append(st, s4bStop{kind: s4bURL})
		}
	}
	if s.webhookUp() {
		webhook()
	}
	st = append(st, s4bStop{kind: s4bAdvanced})
	if s.advanced {
		if !s.webhookUp() {
			webhook()
		}
		st = append(st, s4bStop{kind: s4bTarget}, s4bStop{kind: s4bEntry})
		for i := range s.entries() {
			st = append(st, s4bStop{kind: s4bList, i: i})
		}
	}
	return append(st, s4bStop{kind: s4bNav})
}

func stopIndex(st []s4bStop, x s4bStop) int {
	for i, y := range st {
		if y == x {
			return i
		}
	}
	return -1
}

// settle keeps the focus on a row that is shown: one hidden under it hands the
// focus to the row now at its place. The text inputs follow the focus.
func (s *S4bControlPlane) settle() {
	st := s.stops()
	p := stopIndex(st, s.focus)
	if p < 0 {
		p = s.focusPos
		if p > len(st)-1 {
			p = len(st) - 1
		}
		s.focus = st[p]
	}
	s.focusPos = p
	s.syncFocus()
}

func (s *S4bControlPlane) syncFocus() {
	inputs := []struct {
		kind int
		in   *textinput.Model
	}{{s4bIssuer, &s.issuer}, {s4bCA, &s.caFile}, {s4bURL, &s.url}, {s4bEntry, &s.entry}}
	for _, x := range inputs {
		if s.focus.kind == x.kind {
			x.in.Focus()
		} else {
			x.in.Blur()
		}
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

// removeEntry takes an entry out; the focus stays in the list, on the entry
// after it or else the one before, and on the input when the list is empty.
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
	if n := len(s.entries()); s.focus.kind == s4bList && s.focus.i >= n {
		if n > 0 {
			s.focus.i = n - 1
		} else {
			s.focus = s4bStop{kind: s4bEntry}
		}
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
	if msg, ok := msg.(legacyDoneMsg); ok {
		s.checking = false
		if msg.err != nil {
			s.legacyErr = msg.err.Error()
			return s, nil
		}
		s.legacy = msg.legacy
		s.applyLegacy(msg.legacy)
		s.settle()
		return s, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		if cmd, done := s.key(msg); done {
			s.settle()
			return s, cmd
		}
	}
	var cmd tea.Cmd
	switch s.focus.kind {
	case s4bIssuer:
		s.issuer, cmd = s.issuer.Update(msg)
	case s4bCA:
		s.caFile, cmd = s.caFile.Update(msg)
	case s4bURL:
		s.url, cmd = s.url.Update(msg)
	case s4bEntry:
		s.entry, cmd = s.entry.Update(msg)
	}
	return s, cmd
}

// key handles the keys of the screen; done is false for a key that goes on to
// the focused text input.
func (s *S4bControlPlane) key(msg tea.KeyMsg) (cmd tea.Cmd, done bool) {
	s.settle()
	st := s.stops()
	p := stopIndex(st, s.focus)
	f := s.focus.kind
	inInput := f == s4bIssuer || f == s4bCA || f == s4bURL || f == s4bEntry
	switch msg.String() {
	case "c":
		if !inInput && s.opts.Existing {
			return s.checkLegacy(), true
		}
	case "o":
		if f == s4bAudit && s.opts.Existing {
			s.cp.AuditDefaultOK = !s.cp.AuditDefaultOK
			return nil, true
		}
	case "up":
		if p > 0 {
			s.focus = st[p-1]
		}
		return nil, true
	case "down":
		if p < len(st)-1 {
			s.focus = st[p+1]
			if s.focus.kind == s4bNav {
				s.navIdx = 1 // 다음
			}
		}
		return nil, true
	case "left", "right":
		switch f {
		case s4bTarget:
			n := len(cpTargets)
			if msg.String() == "left" {
				s.target = (s.target + n - 1) % n
			} else {
				s.target = (s.target + 1) % n
			}
			s.entryErr = ""
			return nil, true
		case s4bAdvanced:
			s.advanced = msg.String() == "right"
			return nil, true
		case s4bNav:
			if msg.String() == "left" {
				s.navIdx = 0
			} else {
				s.navIdx = 1
			}
			return nil, true
		}
	case "enter", " ":
		if msg.String() == " " && inInput {
			break
		}
		switch {
		case f == s4bAudit:
			s.cp.Audit = !s.cp.Audit
		case f == s4bRuntime:
			s.cp.RuntimeConfig = !s.cp.RuntimeConfig
		case f == s4bJWKS:
			s.cp.JWKS = !s.cp.JWKS
		case f == s4bWebhook:
			s.cp.Webhook = !s.cp.Webhook
		case f == s4bAdvanced:
			s.advanced = !s.advanced
		case f == s4bEntry && msg.String() == "enter":
			s.addEntry()
		case f == s4bNav && msg.String() == "enter":
			if s.navIdx == 0 {
				return Prev(), true
			}
			return Next(), true
		case inInput || f == s4bTarget:
			if p < len(st)-1 {
				s.focus = st[p+1]
			}
		}
		return nil, true
	case "d", "delete":
		if f == s4bList {
			s.removeEntry(s.entries()[s.focus.i])
			return nil, true
		}
	}
	return nil, false
}

// detected marks a preset the check found on the node, and warns when it is off.
func (s *S4bControlPlane) detected(id string) string {
	if s.legacy == nil || !s.legacy.Presets[id] {
		return ""
	}
	if !s.cp.On(id) {
		return "  ⚠ 노드에서 감지됨 — 끄면 update-cluster 뒤 이 기능이 사라짐"
	}
	return "  (노드에서 감지됨)"
}

// caStatus says in a line whether the authentication config of JWKS is written
// now; the help of the CA field says more.
func (s *S4bControlPlane) caStatus() string {
	ca := s.ca()
	var out []string
	switch {
	case ca.PEM != "" && !ca.FromSaved:
		out = append(out, styles.StyleSuccess.Render("    ✓ CA 있음: 저장하면 authentication-config.yaml 을 씁니다"))
	case ca.PEM != "":
		out = append(out, styles.StyleSuccess.Render("    ✓ CA: 전에 쓴 authentication-config.yaml 의 것을 그대로 둡니다"))
	default:
		out = append(out, styles.StyleWarning.Render("    ⚠ CA 대기: 아직 파일이 없음, setup-cert 뒤 A5.1 이 설정을 만듭니다"))
	}
	if ca.Err != nil {
		out = append(out, styles.StyleError.Render("    ✗ "+ca.Err.Error()))
	}
	return strings.Join(out, "\n") + "\n"
}

// help is what the screen says of the focused row, under it.
func (s *S4bControlPlane) help(kind int) []string {
	switch kind {
	case s4bAudit:
		out := []string{
			"audit-policy-file, audit-log-path=/var/log/aipub/usage/audit.log, 30일·30개·100MB. 첫 설치부터",
			"정책 ansible/files/" + state.AuditPolicyFile + ": 없을 때만 위저드가 기본 정책(AIP-2611)을 씀, 있는 파일은 덮어쓰지 않음",
		}
		if a := s.auditPlan(); a.Note != "" {
			out = append(out, "지금: "+a.Note)
		}
		if s.opts.Existing {
			ok := "꺼짐"
			if s.cp.AuditDefaultOK {
				ok = "켜짐"
			}
			out = append(out, "기존 사이트: c 로 노드의 정책을 가져옴. o: 노드의 정책 대신 기본 정책 쓰기 확인 (지금 "+ok+")")
		}
		return out
	case s4bRuntime:
		return []string{"runtime-config=admissionregistration.k8s.io/v1beta1=true. 첫 설치부터"}
	case s4bJWKS:
		return []string{
			"authentication-config. AIPub 설치 시에만 적용. aipub-installer k8s-jwks 와 같은 플래그·볼륨·파일을 kubeadm 이 보관하게 함",
			"새 사이트: issuer 는 AIPub 백엔드라 클러스터 생성(K3)에서는 빼고, A5 setup-aipub 뒤 A5.1 이 CA 를 넣어",
			"파일을 만들고 A5.2 update-cluster 로 적용합니다 (노드 한 대씩 재시작)",
		}
	case s4bIssuer:
		return []string{"기본은 https://<aipub_app_host_prefix>.<aipub_ingress_zone> (aipub-installer 와 같은 규칙, 존을 바꾸면 따라감)"}
	case s4bCA:
		ca := s.ca()
		out := []string{"setup-cert 가 두는 공개 CA. 있을 때만 ansible/files/" + state.JWKSConfigFile + " 를 만듭니다 (CA 인라인, 평문 복사)"}
		switch {
		case ca.PEM != "" && !ca.FromSaved:
			out = append(out, "CA 있음 ("+ca.Path+"): 저장하면 authentication-config.yaml 을 씁니다")
		case ca.PEM != "":
			out = append(out, "CA: 전에 쓴 authentication-config.yaml 의 것을 그대로 둡니다 ("+ca.Path+" 없음)")
		default:
			out = append(out, "CA 대기 ("+ca.Path+" 없음): authentication-config.yaml 을 쓰지 않고, setup-cert 뒤 A5.1 이 만듭니다")
		}
		return append(out, "CA 없이 update-cluster/노드 추가를 돌리면 설치기가 '소스 파일 없음'으로 거부합니다 (노드는 그대로)")
	case s4bWebhook:
		return []string{
			"authentication-token-webhook, cache-ttl 2m. 웹훅을 쓰던 사이트만. JWKS 와 함께 켤 수 있음",
			"(옮기는 동안 둘 다 돌리고, 확인 뒤 aipub-installer 의",
			"scripts/aipub/migration/jwks-auth/migrate-webhook-to-jwks.sh 로 웹훅을 뺀 다음 여기서도 끔)",
			"AIPub 설치 시에만. setup-cert 가 만드는 CA(" + state.WebhookCAPath + ")가 있어야 apiserver 가 뜨므로",
			"클러스터 생성(K3)에서는 빼고, A2 setup-cert 다음 A2.5 update-cluster 로 적용합니다 (노드 한 대씩 재시작)",
		}
	case s4bURL:
		return []string{"ansible/files/" + state.WebhookConfigFile + " (kubeconfig) 로 만들어 둡니다. 평문 복사 — 비밀 금지"}
	case s4bAdvanced:
		return []string{
			"apiserver·controller-manager·scheduler·etcd 의 인자와 볼륨, 컨트롤플레인 파일. 대부분의 사이트는 필요 없음",
			"vars.yml 에 들어가 kubeadm 이 클러스터 설정으로 보관하므로 노드 추가·업그레이드에도 다시 적용됩니다",
			"바꾸면 update-cluster.yml 이 컨트롤 플레인 노드를 한 대씩 재시작합니다. 매니페스트를 손으로 고치지 마세요",
		}
	case s4bTarget:
		return []string{"←/→ 로 항목을 넣을 곳을 고릅니다"}
	}
	return nil
}

// legacyView is what the check of an existing site found.
func (s *S4bControlPlane) legacyView() string {
	var b strings.Builder
	switch {
	case s.checking:
		b.WriteString(styles.StyleMuted.Render("확인 중... (ansible -m slurp "+plan.APIServerManifest+")") + "\n")
	case s.legacyErr != "":
		b.WriteString(styles.StyleError.Render("✗ apiserver 설정을 읽지 못했습니다: "+s.legacyErr) + "\n")
	case s.legacy != nil && !s.legacy.Any():
		b.WriteString(styles.StyleSuccess.Render("✓ "+s.legacy.Node+": 손으로 넣은 AIPub apiserver 설정 없음") + "\n")
	case s.legacy != nil:
		l := s.legacy
		b.WriteString(styles.StyleWarning.Render("⚠ "+l.Node+" 의 kube-apiserver.yaml 에 손으로 넣은 설정이 있습니다. 맞는 프리셋을 켰습니다:") + "\n")
		for _, f := range l.Found {
			b.WriteString(styles.StyleWarning.Render("   • "+f) + "\n")
		}
		for _, d := range l.Differs {
			b.WriteString(styles.StyleMuted.Render("   값이 다름 (update-cluster 뒤 프리셋 값): "+d) + "\n")
		}
		if l.WebhookURL != "" {
			b.WriteString(styles.StyleMuted.Render("   웹훅 URL 을 노드의 kubeconfig 에서 가져옴") + "\n")
		}
		if l.Issuer != "" {
			b.WriteString(styles.StyleMuted.Render("   issuer 를 노드의 authentication-config.yaml 에서 가져옴") + "\n")
		}
		for _, r := range l.Refused {
			b.WriteString(styles.StyleWarning.Render("   가져오지 않음: "+r) + "\n")
		}
		if l.Audit.Note != "" {
			b.WriteString(styles.StyleMuted.Render("   감사 정책: "+l.Audit.Note) + "\n")
		}
		if l.AuditUnread != "" && !l.Audit.Kept && !s.cp.AuditDefaultOK {
			b.WriteString(styles.StyleWarning.Render("   감사 정책을 읽지 못함 ("+l.AuditUnread+"): 기본 정책을 쓰지 않습니다. 파일을 직접 두거나 감사 로그 줄에서 o 로 확인") + "\n")
		}
		b.WriteString(styles.StyleWarning.Render("   update-cluster 가 손으로 고친 것을 설치기가 관리하는 같은 플래그로 바꿉니다 (이제 업그레이드·노드 변경에도 유지)") + "\n")
		b.WriteString(styles.StyleWarning.Render("   감지된 프리셋을 끄면 update-cluster 뒤 그 기능이 클러스터에서 사라집니다") + "\n")
	}
	return b.String()
}

func checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

func (s *S4bControlPlane) View() string {
	var b strings.Builder
	w := s.width
	// the lines of the focused row: where it starts, and where its status and
	// help end (the line after them)
	focusLine, focusEnd := 0, 0
	// at says whether a row is focused, and marks the line it starts on
	at := func(x s4bStop) bool {
		if s.focus != x {
			return false
		}
		focusLine = strings.Count(b.String(), "\n")
		focusEnd = focusLine + 1
		return true
	}
	// help is written under the focused row only
	help := func(indent string, kind int) {
		for _, l := range s.help(kind) {
			b.WriteString(styles.StyleMuted.Render(indent+l) + "\n")
		}
		focusEnd = strings.Count(b.String(), "\n")
	}
	preset := func(kind int, indent, id, label, short string) {
		focused := at(s4bStop{kind: kind})
		b.WriteString(RenderRow(indent+checkbox(s.cp.On(id))+" "+label+s.detected(id), focused, w) + "\n")
		b.WriteString(styles.StyleMuted.Render(indent+"    "+short) + "\n")
		if focused {
			help(indent+"      ", kind)
		}
	}
	field := func(kind int, indent, label string, in textinput.Model, status string) {
		focused := at(s4bStop{kind: kind})
		b.WriteString(RenderSectionHeader(indent+label, focused) + "  " + in.View() + "\n")
		b.WriteString(status)
		if focused {
			help(indent+"      ", kind)
		}
	}
	webhook := func(indent string) {
		preset(s4bWebhook, indent, state.PresetWebhook, "AIPub 인증 웹훅 (레거시)", "JWKS 이전의 토큰 웹훅 인증. 웹훅을 쓰던 사이트만 켭니다")
		if s.cp.Webhook {
			field(s4bURL, indent+"  ", "웹훅 URL", s.url, "")
		}
	}

	b.WriteString(styles.StyleTitle.Render("컨트롤플레인") + "\n")
	b.WriteString(styles.StyleMuted.Render("kube-apiserver 등 컨트롤 플레인에 켤 기능을 고릅니다 (고른 줄 아래에 설명)") + "\n")
	if s.opts.Existing {
		b.WriteString(styles.StyleMuted.Render("기존 사이트: c 로 첫 컨트롤 플레인 노드의 kube-apiserver.yaml 에 손으로 넣은 설정을 읽습니다 (읽기만 함, 입력 칸 밖에서)") + "\n")
	} else {
		b.WriteString(styles.StyleMuted.Render("새 사이트는 권장 프리셋이 이미 켜져 있어 그대로 다음(Ctrl+N)으로 가면 됩니다") + "\n")
	}
	b.WriteString(s.legacyView())
	b.WriteString("\n")

	b.WriteString(styles.StylePrimary.Render("프리셋") + styles.StyleMuted.Render("  Space/Enter: 켜기/끄기") + "\n")
	audit := "AIPub 사용 기록용 apiserver 감사 로그"
	if s.cp.Audit {
		audit += " (" + s.auditPlan().Short() + ")"
	}
	preset(s4bAudit, "", state.PresetAudit, "AIPub 감사 로그", audit)
	preset(s4bRuntime, "", state.PresetRuntimeConfig, "runtime-config", "admissionregistration v1beta1 API 를 켭니다")
	preset(s4bJWKS, "", state.PresetJWKS, "AIPub 인증 (JWKS)", "AIPub 이 발급한 토큰을 apiserver 가 믿게 합니다 (AIPub 설치 시만)")
	if s.cp.JWKS {
		field(s4bIssuer, "  ", "issuer URL", s.issuer, "")
		field(s4bCA, "  ", "CA 파일", s.caFile, s.caStatus())
	}
	if s.webhookUp() {
		webhook("")
	}
	b.WriteString("\n")

	entries := s.entries()
	arrow := "▸"
	if s.advanced {
		arrow = "▾"
	}
	advFocused := at(s4bStop{kind: s4bAdvanced})
	b.WriteString(RenderRow(fmt.Sprintf("%s 고급: 직접 추가 (항목 %d개)", arrow, len(entries)), advFocused, w) + "\n")
	if advFocused {
		help("    ", s4bAdvanced)
	}
	if s.advanced {
		if !s.webhookUp() {
			webhook("  ")
		}
		t := cpTargets[s.target]
		targetFocused := at(s4bStop{kind: s4bTarget})
		b.WriteString(RenderSectionHeader("  대상", targetFocused) + "  ◀ " + t.label + " ▶\n")
		if targetFocused {
			help("      ", s4bTarget)
		}
		at(s4bStop{kind: s4bEntry})
		b.WriteString(RenderSectionHeader("  항목 (Enter: 추가)", s.focus.kind == s4bEntry) + "  " + s.entry.View() + "\n")
		b.WriteString(styles.StyleMuted.Render("      "+t.hint) + "\n")
		if s.entryErr != "" {
			b.WriteString(styles.StyleError.Render("      ✗ "+s.entryErr) + "\n")
		}
		if len(entries) == 0 {
			b.WriteString(styles.StyleMuted.Render("      (직접 넣은 항목 없음)") + "\n")
		}
		for i, e := range entries {
			focused := at(s4bStop{kind: s4bList, i: i})
			label := cpTargets[e.target].label
			pad := 24 - lipgloss.Width(label)
			if pad < 1 {
				pad = 1
			}
			row := "    " + label + strings.Repeat(" ", pad) + e.text
			if focused {
				row += "   (d: 삭제)"
			}
			b.WriteString(RenderRow(row, focused, w) + "\n")
		}
		b.WriteString(styles.StyleWarning.Render("    ⚠ 컨트롤플레인 파일은 모든 컨트롤 플레인 노드에 평문으로 복사됩니다. 비밀(키·토큰)은 넣지 마세요") + "\n")
	}

	b.WriteString("\n")
	at(s4bStop{kind: s4bNav})
	b.WriteString(RenderNavButtons("이전", "다음", s.focus.kind == s4bNav && s.navIdx == 0, s.focus.kind == s4bNav && s.navIdx == 1, w))
	return FitView(b.String(), focusLine, focusEnd, w, s.height)
}

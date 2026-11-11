package screens

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
//	AIPub audit log, runtime-config, AIPub authentication (JWKS) (toggles)
//	  issuer URL, CA file                  (while JWKS is on)
//	AIPub auth webhook, legacy (toggle)    (here on an existing site or when it was on)
//	  webhook URL                           (while the webhook is on)
//	▸ Advanced: add by hand (Enter/Space, →/←: expand/collapse)
//	  the webhook when not above, the target (←/→), the entry (Enter: add),
//	  the entries added by hand (d: delete) (while expanded)
//	nav slot (←/→: back/next)
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
// label and hint are English, and go through i18n.T where they are drawn.
type cpTarget struct {
	label     string
	kind      string // arg, volume, file
	component string
	hint      string
}

var cpTargets = []cpTarget{
	{"apiserver args", "arg", "apiserver", "name=value (no dash), e.g. oidc-issuer-url=https://idp.example.com"},
	{"apiserver volumes", "volume", "apiserver", "name hostPath mountPath [ro] [pathType], e.g. oidc /etc/oidc /etc/oidc ro Directory"},
	{"controller-manager args", "arg", "controller_manager", "name=value, e.g. node-cidr-mask-size=22"},
	{"controller-manager volumes", "volume", "controller_manager", "name hostPath mountPath [ro] [pathType]"},
	{"scheduler args", "arg", "scheduler", "name=value"},
	{"scheduler volumes", "volume", "scheduler", "name hostPath mountPath [ro] [pathType]"},
	{"etcd args", "arg", "etcd", "name=value, e.g. quota-backend-bytes=8589934592 (etcd has no volumes)"},
	{"control plane files", "file", "", "src dest [mode], e.g. oidc/ca.crt /etc/oidc/ca.crt 0644 (src is under ansible/files)"},
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

func (s *S4bControlPlane) Title() string { return i18n.T("Control plane") }
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
		return []string{i18n.T("The entry field has text. Press Enter to add it, or clear it")}
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
		msgs = append(msgs, i18n.T("JWKS CA file: %s", ca.Err.Error()))
	}
	return msgs
}

// RevealErrors expands Advanced when the refusal names text left in its input, so
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
			s.entryErr = i18n.T("An argument is entered as name=value")
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
			s.entryErr = i18n.T("A volume is entered as name hostPath mountPath [ro] [pathType]")
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
				s.entryErr = i18n.T("'%s' is not ro or one of %s", opt, strings.Join(volumePathTypes, ", "))
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
			s.entryErr = i18n.T("A file is entered as src dest [mode]")
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

// Busy holds the wizard here while c reads the apiserver of the node: what it
// read comes to the screen shown when it ends, so this one only.
func (s *S4bControlPlane) Busy() bool { return s.checking }

// BusyReason is what the wizard says when it does not leave.
func (s *S4bControlPlane) BusyReason() string {
	return i18n.T("Reading the existing apiserver settings. Move on when it ends")
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
				s.navIdx = 1 // next
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
		return "  ⚠ " + i18n.T("detected on the node: turning it off removes this feature after update-cluster")
	}
	return "  " + i18n.T("(detected on the node)")
}

// caStatus says in a line whether the authentication config of JWKS is written
// now; the help of the CA field says more.
func (s *S4bControlPlane) caStatus() string {
	ca := s.ca()
	var out []string
	switch {
	case ca.PEM != "" && !ca.FromSaved:
		out = append(out, styles.StyleSuccess.Render("    ✓ "+i18n.T("CA found: saving writes authentication-config.yaml")))
	case ca.PEM != "":
		out = append(out, styles.StyleSuccess.Render("    ✓ "+i18n.T("CA: keeping the one in the authentication-config.yaml written earlier")))
	default:
		out = append(out, styles.StyleWarning.Render("    ⚠ "+i18n.T("CA pending: no file yet, A5.1 creates the config after setup-cert")))
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
			i18n.T("audit-policy-file, audit-log-path=/var/log/aipub/usage/audit.log, 30 days · 30 files · 100MB. From the first install"),
			i18n.T("policy ansible/files/%s: the wizard writes the default policy (AIP-2611) only when it is missing, an existing file is not overwritten", state.AuditPolicyFile),
		}
		if a := s.auditPlan(); a.Note != "" {
			out = append(out, i18n.T("now: %s", a.Note))
		}
		if s.opts.Existing {
			if s.cp.AuditDefaultOK {
				out = append(out, i18n.T("existing site: c takes the node's policy. o: confirm writing the default policy instead of the node's (now on)"))
			} else {
				out = append(out, i18n.T("existing site: c takes the node's policy. o: confirm writing the default policy instead of the node's (now off)"))
			}
		}
		return out
	case s4bRuntime:
		return []string{i18n.T("runtime-config=admissionregistration.k8s.io/v1beta1=true. From the first install")}
	case s4bJWKS:
		return []string{
			i18n.T("authentication-config. Applied only when AIPub is installed. Makes kubeadm keep the same flags, volumes and files as aipub-installer k8s-jwks"),
			i18n.T("New site: the issuer is the AIPub backend, so it is left out of cluster creation (K3) and, after A5 setup-aipub, A5.1 adds the CA"),
			i18n.T("and creates the file; A5.2 update-cluster applies it (one node restarted at a time)"),
		}
	case s4bIssuer:
		return []string{i18n.T("Default is https://<aipub_app_host_prefix>.<aipub_ingress_zone> (same rule as aipub-installer, follows a change of the zone)")}
	case s4bCA:
		ca := s.ca()
		out := []string{i18n.T("The public CA that setup-cert puts there. ansible/files/%s is made only when it exists (CA inline, copied as plain text)", state.JWKSConfigFile)}
		switch {
		case ca.PEM != "" && !ca.FromSaved:
			out = append(out, i18n.T("CA found (%s): saving writes authentication-config.yaml", ca.Path))
		case ca.PEM != "":
			out = append(out, i18n.T("CA: keeping the one in the authentication-config.yaml written earlier (%s is missing)", ca.Path))
		default:
			out = append(out, i18n.T("CA pending (%s is missing): authentication-config.yaml is not written, A5.1 creates it after setup-cert", ca.Path))
		}
		return append(out, i18n.T("Running update-cluster or adding a node without the CA makes the installer refuse with 'source file missing' (the nodes are left as they are)"))
	case s4bWebhook:
		return []string{
			i18n.T("authentication-token-webhook, cache-ttl 2m. Only for a site that used the webhook. Can be on together with JWKS"),
			i18n.T("(run both while moving over; after checking, take the webhook out with aipub-installer's"),
			i18n.T("scripts/aipub/migration/jwks-auth/migrate-webhook-to-jwks.sh, then turn it off here too)"),
			i18n.T("Only when AIPub is installed. The apiserver starts only with the CA (%s) that setup-cert makes, so", state.WebhookCAPath),
			i18n.T("it is left out of cluster creation (K3) and applied by A2.5 update-cluster after A2 setup-cert (one node restarted at a time)"),
		}
	case s4bURL:
		return []string{i18n.T("Made as ansible/files/%s (kubeconfig). Copied as plain text, no secrets", state.WebhookConfigFile)}
	case s4bAdvanced:
		return []string{
			i18n.T("Arguments and volumes of apiserver, controller-manager, scheduler and etcd, and control plane files. Most sites do not need them"),
			i18n.T("They go into vars.yml and kubeadm keeps them in the cluster configuration, so they are applied again when adding a node or upgrading"),
			i18n.T("A change makes update-cluster.yml restart the control plane nodes one at a time. Do not edit the manifests by hand"),
		}
	case s4bTarget:
		return []string{i18n.T("←/→ chooses where the entry goes")}
	}
	return nil
}

// legacyView is what the check of an existing site found.
func (s *S4bControlPlane) legacyView() string {
	var b strings.Builder
	switch {
	case s.checking:
		b.WriteString(styles.StyleMuted.Render(i18n.T("Checking... (ansible -m slurp %s)", plan.APIServerManifest)) + "\n")
	case s.legacyErr != "":
		b.WriteString(styles.StyleError.Render("✗ "+i18n.T("Could not read the apiserver settings: %s", s.legacyErr)) + "\n")
	case s.legacy != nil && !s.legacy.Any():
		b.WriteString(styles.StyleSuccess.Render("✓ "+i18n.T("%s: no AIPub apiserver settings added by hand", s.legacy.Node)) + "\n")
	case s.legacy != nil:
		l := s.legacy
		b.WriteString(styles.StyleWarning.Render("⚠ "+i18n.T("%s has settings added by hand in kube-apiserver.yaml. The matching presets were turned on:", l.Node)) + "\n")
		for _, f := range l.Found {
			b.WriteString(styles.StyleWarning.Render("   • "+f) + "\n")
		}
		for _, d := range l.Differs {
			b.WriteString(styles.StyleMuted.Render("   "+i18n.T("value differs (the preset's value after update-cluster): %s", d)) + "\n")
		}
		if l.WebhookURL != "" {
			b.WriteString(styles.StyleMuted.Render("   "+i18n.T("webhook URL taken from the node's kubeconfig")) + "\n")
		}
		if l.Issuer != "" {
			b.WriteString(styles.StyleMuted.Render("   "+i18n.T("issuer taken from the node's authentication-config.yaml")) + "\n")
		}
		for _, r := range l.Refused {
			b.WriteString(styles.StyleWarning.Render("   "+i18n.T("not taken: %s", r)) + "\n")
		}
		if l.Audit.Note != "" {
			b.WriteString(styles.StyleMuted.Render("   "+i18n.T("audit policy: %s", l.Audit.Note)) + "\n")
		}
		if l.AuditUnread != "" && !l.Audit.Kept && !s.cp.AuditDefaultOK {
			b.WriteString(styles.StyleWarning.Render("   "+i18n.T("audit policy could not be read (%s): the default policy is not written. Put the file there yourself, or confirm with o on the audit log row", l.AuditUnread)) + "\n")
		}
		b.WriteString(styles.StyleWarning.Render("   "+i18n.T("update-cluster replaces the hand edits with the same flags the installer manages (kept from now on through upgrades and node changes)")) + "\n")
		b.WriteString(styles.StyleWarning.Render("   "+i18n.T("turning off a detected preset removes that feature from the cluster after update-cluster")) + "\n")
	}
	return b.String()
}

// labelCol is the width of the column of the targets in the list of entries:
// the widest label, and a space. In Korean it is 24.
func labelCol() int {
	w := 0
	for _, t := range cpTargets {
		w = max(w, lipgloss.Width(i18n.T(t.label)))
	}
	return w + 1
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
		preset(s4bWebhook, indent, state.PresetWebhook, i18n.T("AIPub auth webhook (legacy)"), i18n.T("Token webhook authentication from before JWKS. Turn it on only for a site that used the webhook"))
		if s.cp.Webhook {
			field(s4bURL, indent+"  ", i18n.T("Webhook URL"), s.url, "")
		}
	}

	s.url.Placeholder = i18n.T("https://<AIPub token review server>")
	b.WriteString(styles.StyleTitle.Render(i18n.T("Control plane")) + "\n")
	b.WriteString(styles.StyleMuted.Render(i18n.T("Choose the features to turn on in the control plane, such as kube-apiserver (explained under the focused row)")) + "\n")
	if s.opts.Existing {
		b.WriteString(styles.StyleMuted.Render(i18n.T("Existing site: c reads the settings added by hand to kube-apiserver.yaml of the first control plane node (read only, outside the input fields)")) + "\n")
	} else {
		b.WriteString(styles.StyleMuted.Render(i18n.T("On a new site the recommended presets are already on, so just go on to the next step (Ctrl+N)")) + "\n")
	}
	b.WriteString(s.legacyView())
	b.WriteString("\n")

	b.WriteString(styles.StylePrimary.Render(i18n.T("Presets")) + styles.StyleMuted.Render("  "+i18n.T("Space/Enter: turn on/off")) + "\n")
	audit := i18n.T("apiserver audit log for AIPub usage records")
	if s.cp.Audit {
		audit += " (" + s.auditPlan().Short() + ")"
	}
	preset(s4bAudit, "", state.PresetAudit, i18n.T("AIPub audit log"), audit)
	preset(s4bRuntime, "", state.PresetRuntimeConfig, "runtime-config", i18n.T("Turns on the admissionregistration v1beta1 API"))
	preset(s4bJWKS, "", state.PresetJWKS, i18n.T("AIPub authentication (JWKS)"), i18n.T("Makes the apiserver trust tokens issued by AIPub (only when AIPub is installed)"))
	if s.cp.JWKS {
		field(s4bIssuer, "  ", "issuer URL", s.issuer, "")
		field(s4bCA, "  ", i18n.T("CA file"), s.caFile, s.caStatus())
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
	b.WriteString(RenderRow(i18n.T("%s Advanced: add by hand (%d entries)", arrow, len(entries)), advFocused, w) + "\n")
	if advFocused {
		help("    ", s4bAdvanced)
	}
	if s.advanced {
		if !s.webhookUp() {
			webhook("  ")
		}
		t := cpTargets[s.target]
		targetFocused := at(s4bStop{kind: s4bTarget})
		b.WriteString(RenderSectionHeader("  "+i18n.T("Target"), targetFocused) + "  ◀ " + i18n.T(t.label) + " ▶\n")
		if targetFocused {
			help("      ", s4bTarget)
		}
		at(s4bStop{kind: s4bEntry})
		b.WriteString(RenderSectionHeader("  "+i18n.T("Entry (Enter: add)"), s.focus.kind == s4bEntry) + "  " + s.entry.View() + "\n")
		b.WriteString(styles.StyleMuted.Render("      "+i18n.T(t.hint)) + "\n")
		if s.entryErr != "" {
			b.WriteString(styles.StyleError.Render("      ✗ "+s.entryErr) + "\n")
		}
		if len(entries) == 0 {
			b.WriteString(styles.StyleMuted.Render("      "+i18n.T("(no entries added by hand)")) + "\n")
		}
		for i, e := range entries {
			focused := at(s4bStop{kind: s4bList, i: i})
			label := i18n.T(cpTargets[e.target].label)
			pad := labelCol() - lipgloss.Width(label)
			if pad < 1 {
				pad = 1
			}
			row := "    " + label + strings.Repeat(" ", pad) + e.text
			if focused {
				row += "   " + i18n.T("(d: delete)")
			}
			b.WriteString(RenderRow(row, focused, w) + "\n")
		}
		b.WriteString(styles.StyleWarning.Render("    ⚠ "+i18n.T("Control plane files are copied as plain text to every control plane node. Do not put secrets (keys, tokens) in them")) + "\n")
	}

	b.WriteString("\n")
	at(s4bStop{kind: s4bNav})
	b.WriteString(RenderNavButtons(i18n.T("Back"), i18n.T("Next"), s.focus.kind == s4bNav && s.navIdx == 0, s.focus.kind == s4bNav && s.navIdx == 1, w))
	return FitView(b.String(), focusLine, focusEnd, w, s.height)
}

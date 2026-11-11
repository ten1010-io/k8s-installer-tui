package screens

import (
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// english runs a test in English, and puts Korean back, which the other tests
// of the package run in.
func english(t *testing.T) {
	t.Helper()
	i18n.Set(i18n.EN)
	t.Cleanup(func() { i18n.Set(i18n.KO) })
}

func hangulIn(s string) string {
	for _, l := range strings.Split(s, "\n") {
		for _, r := range l {
			if unicode.Is(unicode.Hangul, r) {
				return l
			}
		}
	}
	return ""
}

// noHangul fails for the first line of text that has Hangul in it. The
// descriptions of the schema are left out: their English is the schema's own.
func noHangul(t *testing.T, what, text string) {
	t.Helper()
	shown := text
	for _, v := range schema.All() {
		if v.Short != "" {
			shown = strings.ReplaceAll(shown, v.Short, "")
		}
	}
	if l := hangulIn(shown); l != "" {
		t.Errorf("%s: Korean in English: %q", what, l)
	}
	if strings.TrimSpace(text) == "" {
		t.Errorf("%s: nothing drawn", what)
	}
}

func keyMsg(k tea.KeyType) tea.Msg { return tea.KeyMsg{Type: k} }

func TestEnglishS1Nodes(t *testing.T) {
	english(t)
	s := NewS1Nodes()
	s.SetSize(100, 60)
	noHangul(t, "S1 title", s.Title())
	s.SyncFromState(state.DefaultState())
	noHangul(t, "S1 empty", s.View())
	for _, m := range s.Validate() {
		noHangul(t, "S1 validate empty", m)
	}

	st := state.DefaultState()
	st.Nodes = []state.NodeConfig{{Name: "node1", AnsibleHost: "10.0.0.1"}, {Name: "node2", AnsibleHost: "10.0.0.2", AnsiblePort: "2222", AnsibleUser: "ops"}}
	s.SyncFromState(st)
	noHangul(t, "S1 filled", s.View())
	if v := s.View(); !strings.Contains(v, "SSH user") || !strings.Contains(v, "< Add >") || !strings.Contains(v, "< Next >") {
		t.Errorf("S1 filled: header or buttons missing:\n%s", v)
	}

	// the form: new, with the error of an empty save, then an edit
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	noHangul(t, "S1 new form", s.View())
	if v := s.View(); !strings.Contains(v, "22 (default)") || !strings.Contains(v, "root (default)") {
		t.Errorf("S1 form: placeholders not English:\n%s", v)
	}
	if err := s.saveForm(); err == "" {
		t.Fatal("an empty form saved")
	} else {
		noHangul(t, "S1 save error", err)
		s.formErr = err
	}
	noHangul(t, "S1 form with error", s.View())
	s.fields[s1FieldName].SetValue("node1")
	s.fields[s1FieldHost].SetValue("x")
	s.editIdx = -1
	if err := s.saveForm(); err == "" {
		t.Fatal("a duplicate saved")
	} else {
		noHangul(t, "S1 duplicate", err)
	}
	s.editing = false
	s.openForm(1, s.nodes[1])
	noHangul(t, "S1 edit form", s.View())

	s.nodes = []state.NodeConfig{{Name: ""}}
	noHangul(t, "S1 validate no name", strings.Join(s.Validate(), "\n"))
	s.nodes = []state.NodeConfig{{Name: "a"}}
	noHangul(t, "S1 validate no host", strings.Join(s.Validate(), "\n"))
	s.nodes = []state.NodeConfig{{Name: "a", AnsibleHost: "h"}, {Name: "a", AnsibleHost: "h"}}
	noHangul(t, "S1 validate duplicate", strings.Join(s.Validate(), "\n"))
}

func TestEnglishS2Groups(t *testing.T) {
	english(t)
	s := NewS2Groups()
	s.SetSize(100, 60)
	noHangul(t, "S2 title", s.Title())
	s.SyncFromState(state.DefaultState())
	noHangul(t, "S2 empty", s.View())
	noHangul(t, "S2 validate", strings.Join(s.Validate(), "\n"))

	st := state.DefaultState()
	st.Nodes = []state.NodeConfig{{Name: "node1"}, {Name: "node2"}}
	st.KiCpNodes = []string{"node1"}
	s.SyncFromState(st)
	noHangul(t, "S2 filled", s.View())
	if v := s.View(); !strings.Contains(v, "CP node") {
		t.Errorf("S2: column label missing:\n%s", v)
	}
	noHangul(t, "S2 validate no cp", strings.Join(s.Validate(), "\n"))
	s.rows[0].checks[colKiCp] = false
	noHangul(t, "S2 validate no ki", strings.Join(s.Validate(), "\n"))
}

func TestEnglishS3Network(t *testing.T) {
	english(t)
	s := NewS3Network()
	s.SetSize(100, 80)
	noHangul(t, "S3 title", s.Title())
	s.SyncFromState(&state.AppState{})
	noHangul(t, "S3 empty", s.View())
	for _, m := range s.Validate() {
		noHangul(t, "S3 validate", m)
	}

	st := state.DefaultState()
	st.InternalNetworkSubnets = []string{"10.0.0.0/16"}
	st.KiCpDnsUpstreamServers = []string{"8.8.8.8"}
	st.KiCpNtpUpstreamServers = []string{"time1.google.com"}
	st.InternalDNSServerEnabled = true
	for _, ha := range []bool{false, true} {
		st.KiCpHaMode = ha
		s.SyncFromState(st)
		noHangul(t, "S3 filled", s.View())
		s.focusIdx = s3FocusSubnet
		noHangul(t, "S3 subnet focused", s.View())
		s.startEdit(editSubnet, "x.x.x.x/xx")
		noHangul(t, "S3 subnet edit", s.View())
	}
	s.SyncFromState(&state.AppState{KiCpHaMode: true})
	noHangul(t, "S3 validate no vip", strings.Join(s.Validate(), "\n"))
	s.subnets = []string{"10.0.0.0/16"}
	noHangul(t, "S3 validate no vip", strings.Join(s.Validate(), "\n"))
	s.vip.SetValue("1.1.1.1")
	noHangul(t, "S3 validate no dns", strings.Join(s.Validate(), "\n"))
	s.dnsServers = []string{"8.8.8.8"}
	noHangul(t, "S3 validate no ntp", strings.Join(s.Validate(), "\n"))
}

func TestEnglishS4Kubernetes(t *testing.T) {
	english(t)
	s := NewS4Kubernetes()
	s.SetSize(100, 60)
	noHangul(t, "S4 title", s.Title())
	s.SyncFromState(&state.AppState{})
	noHangul(t, "S4 empty", s.View())
	noHangul(t, "S4 validate minor", strings.Join(s.Validate(), "\n"))
	s.SyncFromState(state.DefaultState())
	noHangul(t, "S4 filled", s.View())
	s.podSubnet.SetValue("nope")
	noHangul(t, "S4 validate pod", strings.Join(s.Validate(), "\n"))
	s.podSubnet.SetValue("10.244.0.0/16")
	s.serviceSubnet.SetValue("nope")
	noHangul(t, "S4 validate service", strings.Join(s.Validate(), "\n"))
	s.serviceSubnet.SetValue("10.96.0.0/12")
	s.certValidity.SetValue("")
	noHangul(t, "S4 validate cert", strings.Join(s.Validate(), "\n"))
}

// withoutPolicyText leaves out what the audit policy plan says of itself: it
// comes from fileio, which has its own English.
func withoutPolicyText(s *S4bControlPlane, view string) string {
	a := s.auditPlan()
	for _, x := range []string{a.Short(), a.Note} {
		if x != "" {
			view = strings.ReplaceAll(view, x, "")
		}
	}
	return view
}

func TestEnglishS4bControlPlane(t *testing.T) {
	english(t)
	for _, existing := range []bool{false, true} {
		st := s4bState()
		s := newS4b(st, existing)
		noHangul(t, "S4b title", s.Title())
		noHangul(t, "S4b new", withoutPolicyText(s, s.plain()))
		s.cp.Audit = false
		noHangul(t, "S4b audit off", withoutPolicyText(s, s.plain()))
		s.cp.Audit = true

		// every focus stop shows its help under it
		for _, stop := range s.stops() {
			s.focus = stop
			noHangul(t, "S4b stop", withoutPolicyText(s, s.plain()))
		}
		s.cp.AuditDefaultOK = true
		s.focus = s4bStop{kind: s4bAudit}
		noHangul(t, "S4b audit confirmed", withoutPolicyText(s, s.plain()))
		s.cp.AuditDefaultOK = false

		// JWKS on: the issuer and the CA, with the status of the CA
		s.cp.Set(state.PresetJWKS, true)
		for _, stop := range s.stops() {
			s.focus = stop
			noHangul(t, "S4b JWKS stop", withoutPolicyText(s, s.plain()))
		}

		// the advanced section, expanded: every target, entries, errors
		s.cp.Set(state.PresetWebhook, true)
		s.advanced = true
		s.cp.Args = map[string][]state.ExtraArg{"apiserver": {{Name: "oidc-issuer-url", Value: "https://idp.example.com"}}}
		s.cp.Volumes = map[string][]state.ExtraVolume{"apiserver": {{Name: "oidc", HostPath: "/etc/oidc", MountPath: "/etc/oidc", ReadOnly: true, PathType: "Directory"}}}
		s.cp.Files = []state.CpExtraFile{{Src: "oidc/ca.crt", Dest: "/etc/oidc/ca.crt", Mode: "0644"}}
		for i := range cpTargets {
			s.target = i
			s.focus = s4bStop{kind: s4bTarget}
			noHangul(t, "S4b target", withoutPolicyText(s, s.plain()))
		}
		for i, in := range []struct {
			target int
			text   string
		}{
			{0, "noequals"}, {1, "a b"}, {1, "a /b /c bogus"}, {7, "only"},
		} {
			s.target = in.target
			s.entry.SetValue(in.text)
			s.entryErr = ""
			s.addEntry()
			if s.entryErr == "" {
				t.Fatalf("entry %d %q was accepted", i, in.text)
			}
			noHangul(t, "S4b entry error", s.entryErr)
			s.focus = s4bStop{kind: s4bEntry}
			noHangul(t, "S4b entry error view", withoutPolicyText(s, s.plain()))
		}
		s.focus = s4bStop{kind: s4bList, i: 0}
		noHangul(t, "S4b list focused", withoutPolicyText(s, s.plain()))
		s.entry.SetValue("typed")
		noHangul(t, "S4b validate typed", strings.Join(s.Validate(), "\n"))
		s.entry.SetValue("")
		s.cp.Args, s.cp.Volumes, s.cp.Files = nil, nil, nil
		noHangul(t, "S4b no entries", withoutPolicyText(s, s.plain()))
		noHangul(t, "S4b busy", s.BusyReason())

		// what the check of an existing site found
		s.opts.Existing = true
		s.checking = true
		noHangul(t, "S4b checking", withoutPolicyText(s, s.plain()))
		s.checking = false
		s.legacyErr = "connection refused"
		noHangul(t, "S4b legacy error", withoutPolicyText(s, s.plain()))
		s.legacyErr = ""
		s.legacy = &plan.Legacy{Node: "node1", Presets: map[string]bool{}}
		noHangul(t, "S4b nothing found", withoutPolicyText(s, s.plain()))
		s.legacy = &plan.Legacy{
			Node: "node1", Presets: map[string]bool{state.PresetAudit: true, state.PresetJWKS: true},
			Found: []string{"--audit-policy-file"}, Differs: []string{"--audit-log-maxage 7 -> 30"},
			WebhookURL: "https://x", Issuer: "https://y", Refused: []string{"--oidc-ca-file (a credential)"},
			AuditUnread: "permission denied",
		}
		noHangul(t, "S4b found", withoutPolicyText(s, s.plain()))
		s.cp.Audit = false
		noHangul(t, "S4b found, off", withoutPolicyText(s, s.plain()))
	}
}

func TestEnglishS5Aipub(t *testing.T) {
	english(t)
	s := NewS5Aipub()
	s.SetSize(100, 80)
	noHangul(t, "S5 title", s.Title())
	st := state.DefaultState()
	s.SyncFromState(st)
	noHangul(t, "S5 off", s.View())
	s.focusIdx = s5FocusNav
	noHangul(t, "S5 off, nav", s.View())

	st = s5State()
	for _, ha := range []bool{false, true} {
		st.AipubHaMode = ha
		s.SyncFromState(st)
		noHangul(t, "S5 on", s.View())
		for f := 0; f <= s5FocusMax; f++ {
			s.focusIdx = f
			noHangul(t, "S5 on, focus", s.View())
		}
		s.appTool, s.monTool = helmfileTool, helmfileTool
		s.monDryRun = true
		noHangul(t, "S5 helmfile", s.View())
		s.monDryRun = false
		noHangul(t, "S5 helmfile, real deploy", s.View())
	}
	st.K8sCpNodes = nil
	s.SyncFromState(st)
	noHangul(t, "S5 no nodes", s.View())

	for _, edit := range []func(){
		func() { s.ingressZone.SetValue("") },
		func() { s.ingressZone.SetValue("example.com"); s.haMode = true; s.storageClass.SetValue("") },
		func() { s.storageClass.SetValue("sc"); s.k8sNodeNames = []string{"n1"}; s.cpNodeChecks = []bool{false} },
		func() { s.cpNodeChecks = []bool{true}; s.haMode = false; s.localPVNode.SetValue("") },
		func() { s.localPVNode.SetValue("other") },
		func() { s.localPVNode.SetValue("n1"); s.harborSubdomain.SetValue("") },
		func() { s.harborSubdomain.SetValue("h"); s.harborReplicas.SetValue("0") },
	} {
		edit()
		msgs := s.Validate()
		if len(msgs) == 0 {
			t.Fatal("no message for an invalid screen")
		}
		noHangul(t, "S5 validate", strings.Join(msgs, "\n"))
	}
}

func TestEnglishS5bMonitoring(t *testing.T) {
	english(t)
	s := NewS5bMonitoring()
	s.SetSize(100, 80)
	noHangul(t, "S5b title", s.Title())
	s.SyncFromState(&state.AppState{})
	noHangul(t, "S5b empty", s.View())

	st := state.DefaultState()
	st.AipubEnabled = true
	st.K8sCpNodes = []string{"node1", "node2"}
	st.MonitoringCpNodes = []string{"node1"}
	s.SyncFromState(st)
	for f := 0; f <= s5bFocusMax; f++ {
		s.focusIdx = f
		noHangul(t, "S5b filled", s.View())
	}
	s.rightsizing = true
	noHangul(t, "S5b rightsizing on", s.View())
}

// The navigation keys do the same in English: nothing depends on the text.
func TestEnglishScreensKeepTheirKeys(t *testing.T) {
	english(t)
	s := NewS1Nodes()
	s.SetSize(100, 60)
	st := state.DefaultState()
	st.Nodes = []state.NodeConfig{{Name: "node1", AnsibleHost: "h"}}
	s.SyncFromState(st)
	s.Update(keyMsg(tea.KeyDown))
	s.Update(keyMsg(tea.KeyDown))
	_, cmd := s.Update(keyMsg(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("Enter on Next did nothing")
	}
	if _, ok := cmd().(NavigateNextMsg); !ok {
		t.Errorf("Enter on Next gave %T", cmd())
	}
}

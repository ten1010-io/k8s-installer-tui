package fileio

import (
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// What is copied to the control plane nodes is English whatever the language:
// the audit policy, the authentication config and the webhook kubeconfig have
// no Korean when written in Korean.
func TestNodeFilesAreEnglishInKorean(t *testing.T) {
	if i18n.Current() != i18n.KO {
		t.Fatal("the tests run in Korean")
	}
	withCA(t, newTestCA(t, "aipub-ca"))
	s := controlPlaneState()
	s.AipubEnabled = true
	s.AipubIngressZone = "example.com"
	r, err := Render(DefaultPaths(t.TempDir()), s)
	must(t, err)
	for name, data := range map[string][]byte{
		"audit policy":       r.AuditPolicy,
		"audit template":     auditPolicyTemplate,
		"authentication":     r.AuthConfig,
		"webhook kubeconfig": r.WebhookConfig,
	} {
		if len(data) == 0 {
			t.Errorf("%s not rendered", name)
		}
		if hasHangul(string(data)) {
			t.Errorf("%s has Korean:\n%s", name, data)
		}
	}
}

// In English the messages of this package have no Korean.
func TestMessagesAreEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)
	var said []string
	say := func(s string) { said = append(said, s) }

	for _, src := range []AuditSource{AuditDefault, AuditSite, AuditWizard, AuditNode, AuditHeld, AuditBroken} {
		short := AuditPolicyPlan{Source: src}.Short()
		if short == "" {
			t.Errorf("no short text for source %d", src)
		}
		say(short)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "audit-policy.yaml")
	s := auditState()
	s.Existing = true
	say(PlanAuditPolicy(path, s).Note) // held: the policy of the nodes is not read
	s.Existing = false
	say(PlanAuditPolicy(path, s).Note) // the default
	must(t, os.WriteFile(path, []byte("rules: []\n"), 0644))
	say(PlanAuditPolicy(path, s).Note) // the site's
	s.ControlPlane.AuditFrom = "node1"
	say(PlanAuditPolicy(path, s).Note) // from a node
	s.ControlPlane.AuditFrom = ""
	must(t, os.WriteFile(path, nil, 0644))
	broken := PlanAuditPolicy(path, s)
	if broken.Source != AuditBroken {
		t.Errorf("empty file: %+v", broken)
	}
	say(broken.Note)
	must(t, os.Remove(path))
	must(t, os.Mkdir(path, 0755))
	say(PlanAuditPolicy(path, s).Note)
	a, _ := AdoptAuditPolicy(path, []byte("rules: []\n"))
	say(a.Note)
	must(t, os.Remove(path))

	// nothing there: the policy of a node is taken
	for _, node := range [][]byte{auditPolicyTemplate, []byte("rules: []\n")} {
		a, err := AdoptAuditPolicy(path, node)
		must(t, err)
		say(a.Note)
		must(t, os.RemoveAll(path))
	}
	// a file there: the site's, then the wizard's own
	for _, content := range []string{"rules: []\n", AuditPolicyMarker + "\nrules: []\n"} {
		must(t, os.WriteFile(path, []byte(content), 0644))
		for _, node := range [][]byte{nil, []byte("rules: []\n"), []byte("rules:\n- level: None\n")} {
			a, err := AdoptAuditPolicy(path, node)
			must(t, err)
			say(a.Note)
		}
	}

	for _, in := range []string{
		"-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIBkg\n-----END EC PRIVATE KEY-----",
		"-----BEGIN CERTIFICATE-----\nMHcC\n-----END CERTIFICATE-----",
		"-----BEGIN CERTIFICATE-----\nAAAA",
		"nothing here",
		newTestCA(t, "a") + "\n-----BEGIN CERTIFICATE-----\nbroken",
	} {
		if _, err := ParseCA([]byte(in)); err != nil {
			say(err.Error())
		} else {
			t.Errorf("accepted: %q", in)
		}
	}

	site := DefaultPaths(t.TempDir())
	installer := filepath.Join(t.TempDir(), "files")
	cp := controlPlaneState()
	for _, c := range []struct {
		name string
		fn   func()
	}{
		{"no AIPub", func() { cp.AipubEnabled = false }},
		{"JWKS off", func() { cp.AipubEnabled, cp.ControlPlane.JWKS = true, false }},
		{"no issuer", func() { cp.ControlPlane.JWKS, cp.AipubIngressZone, cp.ControlPlane.JWKSIssuerURL = true, "", "" }},
		{"no CA", func() { cp.AipubIngressZone = "example.com"; withCA(t, "") }},
		{"bad CA", func() { withCA(t, "not a certificate") }},
	} {
		c.fn()
		_, err := FillAuthConfig(site, cp, installer)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		say(err.Error())
	}

	for _, m := range said {
		if m == "" || hasHangul(m) {
			t.Errorf("not English: %q", m)
		}
	}
}

package fileio

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// newTestCA is a self-signed CA certificate in PEM, as setup-cert makes one. The
// key is thrown away.
func newTestCA(t *testing.T, cn string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(t, err)
	return strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
}

// withCA points the default CA path at a file holding data (nothing there when
// data is empty) for the test.
func withCA(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	if data != "" {
		must(t, os.WriteFile(path, []byte(data), 0644))
	}
	old := DefaultCAPath
	DefaultCAPath = path
	t.Cleanup(func() { DefaultCAPath = old })
	return path
}

func TestParseCA(t *testing.T) {
	a, b := newTestCA(t, "a"), newTestCA(t, "b")
	keyPEM := "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIBkg\n-----END EC PRIVATE KEY-----"
	for _, c := range []struct {
		name, in, want string
		bad            bool
	}{
		{name: "one", in: a + "\n", want: a},
		{name: "crlf", in: strings.ReplaceAll(a, "\n", "\r\n") + "\r\n", want: a},
		{name: "bundle", in: a + "\n" + b + "\n", want: a + "\n" + b},
		{name: "text around it, written again", in: "subject=CN = a\n" + a + "\n", want: a},
		{name: "a key", in: keyPEM, bad: true},
		{name: "a certificate and its key", in: a + "\n" + keyPEM, bad: true},
		// pem.Decode passes over the key, whose END line names another type
		{name: "a broken key before a certificate", in: strings.Replace(keyPEM, "END EC PRIVATE", "END RSA PRIVATE", 1) + "\n" + a, bad: true},
		{name: "a broken block before a certificate", in: "-----BEGIN CERTIFICATE-----\n!!!!\n-----END CERTIFICATE-----\n" + a + "\n", bad: true},
		{name: "not a certificate", in: "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n", bad: true},
		{name: "the placeholder", in: "REPLACE_ME_CA", bad: true},
		{name: "empty", in: "", bad: true},
	} {
		got, err := ParseCA([]byte(c.in))
		if c.bad {
			if err == nil {
				t.Errorf("%s: taken: %q", c.name, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v", c.name, got, err)
		}
	}
}

// The file named on the screen, else the default; else the CA a config written
// before holds.
func TestLookupCA(t *testing.T) {
	def, own := newTestCA(t, "default"), newTestCA(t, "own")
	defPath := withCA(t, def)
	s := state.DefaultState()
	if ca := LookupCA(s); ca.PEM != def || ca.Path != defPath || ca.FromSaved || ca.Err != nil {
		t.Errorf("default: %+v", ca)
	}
	ownPath := filepath.Join(t.TempDir(), "own.crt")
	must(t, os.WriteFile(ownPath, []byte(own), 0644))
	s.ControlPlane.JWKSCAFile = ownPath
	if ca := LookupCA(s); ca.PEM != own || ca.Path != ownPath {
		t.Errorf("the file named: %+v", ca)
	}

	// not there: the saved one, or nothing
	s.ControlPlane.JWKSCAFile = filepath.Join(t.TempDir(), "gone.crt")
	if ca := LookupCA(s); ca.PEM != "" || ca.Err != nil {
		t.Errorf("nothing anywhere: %+v", ca)
	}
	s.ControlPlane.JWKSCA = own
	if ca := LookupCA(s); ca.PEM != own || !ca.FromSaved {
		t.Errorf("saved: %+v", ca)
	}

	// there but not a CA: said, and never copied
	must(t, os.WriteFile(ownPath, []byte("-----BEGIN RSA PRIVATE KEY-----\nAA\n-----END RSA PRIVATE KEY-----\n"), 0644))
	s.ControlPlane.JWKSCAFile, s.ControlPlane.JWKSCA = ownPath, ""
	if ca := LookupCA(s); ca.PEM != "" || ca.Err == nil {
		t.Errorf("a key: %+v", ca)
	}
}

// The CA is inlined as the template has it, every line under
// certificateAuthority: | indented by eight, and read back as it was.
func TestAuthConfigInlinesCA(t *testing.T) {
	ca := newTestCA(t, "aipub-ca")
	withCA(t, ca)
	p := DefaultPaths(t.TempDir())
	s := controlPlaneState()
	must(t, WriteAll(p, s))
	out, err := os.ReadFile(p.AuthConfig())
	must(t, err)
	var block strings.Builder
	block.WriteString("      certificateAuthority: |\n")
	for _, l := range strings.Split(ca, "\n") {
		block.WriteString("        " + l + "\n")
	}
	block.WriteString("    # Access tokens only.")
	if !strings.Contains(string(out), block.String()) {
		t.Errorf("CA not inlined under certificateAuthority:\n%s", out)
	}
	var doc struct {
		JWT []struct {
			Issuer struct {
				CertificateAuthority string `yaml:"certificateAuthority"`
			} `yaml:"issuer"`
		} `yaml:"jwt"`
	}
	must(t, yaml.Unmarshal(out, &doc))
	if got := doc.JWT[0].Issuer.CertificateAuthority; got != ca+"\n" {
		t.Errorf("CA as YAML reads it: %q", got)
	}

	// read back, and saved again with the CA file gone: kept
	back := state.DefaultState()
	must(t, LoadAll(p, back))
	if back.ControlPlane.JWKSCA != ca {
		t.Errorf("CA read back: %q", back.ControlPlane.JWKSCA)
	}
	withCA(t, "")
	must(t, WriteAll(p, back))
	again, err := os.ReadFile(p.AuthConfig())
	must(t, err)
	if string(again) != string(out) {
		t.Errorf("saved again without the CA file:\n%s\nwant\n%s", again, out)
	}
}

// --fill-auth-config writes the config whole to the saved site and to the
// installer, the same bytes, and running it again changes nothing.
func TestFillAuthConfig(t *testing.T) {
	site := DefaultPaths(t.TempDir())
	installer := filepath.Join(t.TempDir(), "ansible", "files")
	s := controlPlaneState()

	// before setup-cert: no CA, an error, nothing written
	withCA(t, "")
	must(t, WriteAll(site, s))
	if _, err := os.Stat(site.AuthConfig()); err == nil {
		t.Fatal("authentication config written without a CA")
	}
	back := state.DefaultState()
	must(t, LoadAll(site, back))
	if _, err := FillAuthConfig(site, back, installer); err == nil || !strings.Contains(err.Error(), "CA 가 없습니다") {
		t.Errorf("no CA: %v", err)
	}
	for _, f := range []string{site.AuthConfig(), filepath.Join(installer, "aipub", "authentication-config.yaml")} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s written without a CA", f)
		}
	}

	// after setup-cert
	ca := newTestCA(t, "aipub-ca")
	withCA(t, ca)
	lines, err := FillAuthConfig(site, back, installer)
	must(t, err)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "written: ") || !strings.HasPrefix(lines[1], "written: ") {
		t.Errorf("first run: %v", lines)
	}
	saved, err := os.ReadFile(site.AuthConfig())
	must(t, err)
	installed, err := os.ReadFile(filepath.Join(installer, "aipub", "authentication-config.yaml"))
	must(t, err)
	if string(saved) != string(installed) || !strings.Contains(string(saved), "        "+strings.Split(ca, "\n")[1]+"\n") ||
		!strings.Contains(string(saved), "url: https://aipub.aipub.example.com\n") {
		t.Errorf("the copies differ or lack the CA:\n%s\n---\n%s", saved, installed)
	}
	// both in the history of the site: the save, and the installer's as a copy
	siteDir := filepath.Dir(site.Inventory)
	if got := kinds(t, siteDir, "files/aipub/authentication-config.yaml"); got != "save copy" {
		t.Errorf("history of the auth config: %q", got)
	}

	// again, as the plan may be run again: nothing changes, no backup made
	back = state.DefaultState()
	must(t, LoadAll(site, back))
	lines, err = FillAuthConfig(site, back, installer)
	must(t, err)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "unchanged: ") || !strings.HasPrefix(lines[1], "unchanged: ") {
		t.Errorf("second run: %v", lines)
	}
	if _, err := os.Stat(site.AuthConfig() + ".bak"); err == nil {
		t.Error("a backup made by a run that changed nothing")
	}

	// the CA file gone or broken (a control node rebuilt, one that is no k8s
	// node): the CA the config holds is not taken for it, as it may be another
	// cluster's, and nothing is written
	before, err := os.ReadFile(site.AuthConfig())
	must(t, err)
	for _, c := range []struct{ file, says string }{{"", "CA 가 없습니다"}, {"not a certificate", "CA 를 읽지 못했습니다"}} {
		withCA(t, c.file)
		if got := LookupCA(back); !got.FromSaved {
			t.Fatalf("CA file %q: the saved CA is not offered, so this tests nothing: %+v", c.file, got)
		}
		if _, err := FillAuthConfig(site, back, installer); err == nil || !strings.Contains(err.Error(), c.says) ||
			!strings.Contains(err.Error(), "저장된 인증 설정의 CA") {
			t.Errorf("CA file %q: %v", c.file, err)
		}
		if now, _ := os.ReadFile(site.AuthConfig()); string(now) != string(before) {
			t.Errorf("CA file %q: the config was written", c.file)
		}
	}
	withCA(t, ca)

	// saved where the installer reads it: one copy
	inPlace := DefaultPaths(filepath.Dir(installer))
	must(t, WriteAll(inPlace, s))
	lines, err = FillAuthConfig(inPlace, s, installer)
	must(t, err)
	if len(lines) != 1 {
		t.Errorf("saved in the installer: %v", lines)
	}

	// the preset off: refused
	s.ControlPlane.JWKS = false
	if _, err := FillAuthConfig(site, s, installer); err == nil {
		t.Error("filled with the preset off")
	}
}

// Nothing the wizard writes holds the placeholder of the template: every fixture,
// rendered with no CA and with one.
func TestNoPlaceholderInFixtures(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "test", "simulate", "fixtures", "*"))
	must(t, err)
	if len(dirs) == 0 {
		t.Fatal("no fixtures")
	}
	ca := newTestCA(t, "fixture-ca")
	for _, dir := range dirs {
		for _, withAuthCA := range []bool{false, true} {
			if withAuthCA {
				// the fixture's own fake CA, as simulate.sh passes it, else one made here
				own, err := os.ReadFile(filepath.Join(dir, "jwks-ca.crt"))
				if err != nil {
					own = []byte(ca)
				} else if _, err := ParseCA(own); err != nil {
					t.Errorf("%s: jwks-ca.crt is not a CA: %v", filepath.Base(dir), err)
				}
				withCA(t, string(own))
			} else {
				withCA(t, "")
			}
			s := state.DefaultState()
			p := DefaultPaths(dir)
			must(t, LoadAll(p, s))
			r, err := Render(p, s)
			must(t, err)
			for _, data := range [][]byte{r.Inventory, r.Vars, r.AipubVars, r.WebhookConfig, r.AuthConfig} {
				if strings.Contains(string(data), "REPLACE_ME") {
					t.Errorf("%s (CA %v): a placeholder written:\n%s", filepath.Base(dir), withAuthCA, data)
				}
			}
			jwks := s.AppliedControlPlane().JWKS
			if got := r.AuthConfig != nil; got != (jwks && withAuthCA) {
				t.Errorf("%s (CA %v): authentication config written %v with JWKS %v", filepath.Base(dir), withAuthCA, got, jwks)
			}
			if r.JWKSPending(s) != (jwks && !withAuthCA) {
				t.Errorf("%s (CA %v): pending %v", filepath.Base(dir), withAuthCA, r.JWKSPending(s))
			}
		}
	}
}

// The authentication config is copied with the mode k8s-jwks gives it.
func TestJWKSFileMode(t *testing.T) {
	var jwks state.Preset
	for _, p := range state.Presets {
		if p.ID == state.PresetJWKS {
			jwks = p
		}
	}
	want := []state.CpExtraFile{{Src: state.JWKSConfigFile, Dest: state.JWKSConfigPath, Mode: "0600"}}
	if !reflect.DeepEqual(jwks.Files, want) {
		t.Errorf("JWKS files %v", jwks.Files)
	}
}

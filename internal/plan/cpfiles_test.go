package plan

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// testCA is a self-signed CA in PEM, the shape setup-cert leaves at ca.crt.
func testCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// caAt points the CA of the JWKS preset at a file of the test, there or not yet.
func caAt(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	old := fileio.DefaultCAPath
	fileio.DefaultCAPath = path
	t.Cleanup(func() { fileio.DefaultCAPath = old })
	return path
}

// presentFiles are the files k8s-installer would find under ansible/files: the
// regular ones that are not empty, by their path relative to it.
func presentFiles(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			rel, _ := filepath.Rel(dir, path)
			out[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// effectiveCpFiles is k8s_cp_extra_files as a run of the step sees it: the last
// -e that sets it (extra vars win over vars.yml for the whole run), else what
// vars.yml holds.
func effectiveCpFiles(t *testing.T, st Step, varsPath string) []state.CpExtraFile {
	t.Helper()
	var files []state.CpExtraFile
	set := false
	for i := 0; i+1 < len(st.Args); i++ {
		if st.Args[i] != "-e" || !strings.HasPrefix(st.Args[i+1], "{") {
			continue
		}
		var extra map[string]json.RawMessage
		if err := json.Unmarshal([]byte(st.Args[i+1]), &extra); err != nil {
			t.Fatalf("%s: -e %s: %v", st.ID, st.Args[i+1], err)
		}
		if raw, ok := extra[state.CpExtraFilesVar]; ok {
			files, set = nil, true
			if err := json.Unmarshal(raw, &files); err != nil {
				t.Fatalf("%s: %v", st.ID, err)
			}
		}
	}
	if set {
		return files
	}
	data, err := os.ReadFile(varsPath)
	if err != nil {
		t.Fatal(err)
	}
	var vars struct {
		Files []state.CpExtraFile `yaml:"k8s_cp_extra_files"`
	}
	if err := yaml.Unmarshal(data, &vars); err != nil {
		t.Fatal(err)
	}
	return vars.Files
}

// Every playbook that imports tasks/prepare-vars.yml has k8s-installer check
// that each source of k8s_cp_extra_files is under <root>/ansible/files
// (scripts/preflight/validate-hostvars.py, validate_k8s_cp_extra_files), before
// it touches a node: setup-control-node, push-user-registry-artifacts, the
// update-cluster runs and every aipub playbook. The plan is walked in order with
// the files there at each step - what saving wrote, the authentication config
// of JWKS only once A5.1 has filled it in with the CA - and no step may name one
// that is not there.
func TestEveryPlaybookFindsItsCpFiles(t *testing.T) {
	withJWKSIssuer := func(s *state.AppState) *state.AppState {
		s.AipubIngressZone = "lkjh.local"
		s.ControlPlane.Audit = true
		return s
	}
	for _, c := range []struct {
		name string
		// state and options of the site; caAtSave: the CA is there when it is saved
		site     func() (*state.AppState, Options)
		caAtSave bool
		want     string // the steps, to know the case is the shape it means
	}{
		{name: "new site, JWKS without a CA", site: func() (*state.AppState, Options) {
			return withJWKSIssuer(aipubState()), testOptions()
		}, want: "K1 K2 K3 (K4) A1 A2 M1 A3 A4 A5 A5.1 A5.2"},
		{name: "new site, webhook and JWKS", site: func() (*state.AppState, Options) {
			s := withJWKSIssuer(aipubState())
			s.ControlPlane.Webhook, s.ControlPlane.WebhookURL = true, "https://aipub.lkjh.local/api/v1/token-review"
			return s, testOptions()
		}, want: "K1 K2 K3 (K4) A1 A2 M1 A2.5 A3 A4 A5 A5.1 A5.2"},
		{name: "existing cluster, AIPub installed now (staged)", site: func() (*state.AppState, Options) {
			o := testOptions()
			o.Existing = true
			return withJWKSIssuer(aipubState()), o
		}, want: "(K1) (K2) K3 (K4) A1 A2 M1 A3 A4 A5 A5.1 A5.2"},
		{name: "existing site with AIPub and the CA (direct)", site: func() (*state.AppState, Options) {
			o := testOptions()
			o.Existing, o.AipubInstalled = true, true
			s := withJWKSIssuer(aipubState())
			s.ControlPlane.Webhook, s.ControlPlane.WebhookURL = true, "https://aipub.lkjh.local/api/v1/token-review"
			return s, o
		}, caAtSave: true, want: "(K1) (K2) (K3) (K4) A1 A2 M1 A2.5 A3 A4 A5"},
		{name: "without AIPub", site: func() (*state.AppState, Options) {
			s := state.DefaultState()
			s.ControlPlane.Audit = true
			return s, testOptions()
		}, want: "K1 K2 K3 (K4)"},
		{name: "existing cluster without AIPub", site: func() (*state.AppState, Options) {
			s := state.DefaultState()
			s.ControlPlane.Audit = true
			o := testOptions()
			o.Existing = true
			return s, o
		}, want: "(K1) (K2) (K3) (K4) K5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ca := caAt(t)
			if c.caAtSave {
				if err := os.WriteFile(ca, []byte(testCA(t)), 0644); err != nil {
					t.Fatal(err)
				}
			}
			s, o := c.site()
			dir := t.TempDir()
			p := fileio.DefaultPaths(dir)
			if err := fileio.WriteAll(p, s); err != nil {
				t.Fatal(err)
			}
			o.SiteDir = dir
			o.JWKSCAReady = fileio.LookupCA(s).PEM != ""
			steps := Build(s, o)
			var run []string
			for _, st := range steps {
				if st.Kind != Check {
					run = append(run, ids([]Step{st}))
				}
			}
			if got := strings.Join(run, " "); got != c.want {
				t.Fatalf("plan %s, want %s", got, c.want)
			}

			filesDir := filepath.Join(dir, "files")
			for _, st := range steps {
				if st.Skip {
					continue
				}
				switch {
				case st.ID == "A5.1":
					// setup-cert has put the CA by now; --fill-auth-config writes the
					// config to the saved site
					if err := os.WriteFile(ca, []byte(testCA(t)), 0644); err != nil {
						t.Fatal(err)
					}
					if _, err := fileio.FillAuthConfig(p, s, t.TempDir()); err != nil {
						t.Fatalf("A5.1: %v", err)
					}
				case st.Kind == Playbook:
					there := presentFiles(t, filesDir)
					for _, f := range effectiveCpFiles(t, st, p.Vars) {
						if !there[f.Src] {
							t.Errorf("%s %s names files/%s, which is not there yet (k8s-installer: File[%s] of the control node is not there)",
								st.ID, st.Playbook, f.Src, f.Src)
						}
					}
				}
			}
		})
	}
}

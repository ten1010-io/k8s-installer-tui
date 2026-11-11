package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func helmfileState() *state.AppState {
	s := aipubState()
	s.AipubAppDeployTool = "helmfile"
	return s
}

func rootWithPackage(t *testing.T, record string) Options {
	t.Helper()
	o := testOptions()
	o.Root = t.TempDir()
	if record != "" {
		if err := os.WriteFile(filepath.Join(o.Root, PackageFile), []byte(record), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

func TestCheckAipubBin(t *testing.T) {
	// installer as the tool: nothing to check
	if r := CheckAipubBin(rootWithPackage(t, ""), aipubState()); !r.Skipped || !r.OK {
		t.Errorf("installer: %+v", r)
	}

	// helmfile, and the package carried the binaries
	r := CheckAipubBin(rootWithPackage(t, `{"aipub_bin": ["helmfile", "kustomize", "k9s"]}`), helmfileState())
	if !r.OK || r.Warn || !strings.Contains(r.Lines[0], "helmfile, kustomize, k9s") {
		t.Errorf("with aipub-bin: %+v", r)
	}
	// entries that are mappings
	r = CheckAipubBin(rootWithPackage(t, `{"aipub_bin": [{"name": "helmfile", "version": "1.1"}]}`), helmfileState())
	if r.Warn || !strings.Contains(r.Lines[0], "helmfile") {
		t.Errorf("mapping entries: %+v", r)
	}

	// built with --no-aipub-bin, or no record at all: a warning that names the variables
	for name, rec := range map[string]string{"empty list": `{"aipub_bin": []}`, "no key": `{}`, "no file": "", "not json": "{"} {
		r := CheckAipubBin(rootWithPackage(t, rec), helmfileState())
		all := strings.Join(r.Lines, "\n")
		if !r.OK || !r.Warn || !strings.Contains(all, "aipub_app_helmfile_bin") {
			t.Errorf("%s: %+v", name, r)
		}
	}

	// both tools: monitoring's own two as well
	s := helmfileState()
	s.AipubMonitoringDeployTool = "helmfile"
	r = CheckAipubBin(rootWithPackage(t, `{"aipub_bin": []}`), s)
	all := strings.Join(r.Lines, "\n")
	for _, v := range []string{"aipub_app_helmfile_bin", "aipub_monitoring_kustomize_bin"} {
		if !strings.Contains(all, v) {
			t.Errorf("both tools: %s not named: %s", v, all)
		}
	}

	// the site laid them and said where: no warning
	s.AipubAppHelmfileBin = "/opt/bin/helmfile"
	r = CheckAipubBin(rootWithPackage(t, `{"aipub_bin": []}`), s)
	if r.Warn || !r.OK {
		t.Errorf("paths set by the site: %+v", r)
	}
}

func TestBuildP4OnlyWithHelmfile(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/a.tgz"
	if got := ids(Build(aipubState(), o)); strings.Contains(got, "P4") {
		t.Errorf("P4 without helmfile: %s", got)
	}
	if got, want := ids(Build(helmfileState(), o)), "P1 P2 P4 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("with helmfile: %s, want %s", got, want)
	}
	s := state.DefaultState() // no AIPub: the tool is not used
	s.AipubAppDeployTool = "helmfile"
	if got := ids(Build(s, o)); strings.Contains(got, "P4") {
		t.Errorf("P4 without AIPub: %s", got)
	}
}

package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// optionalKeys are the variables of aipub written only when changed.
func optionalKeys() []string {
	var out []string
	for _, v := range schema.ByFile(schema.FileAipub) {
		if v.Optional {
			out = append(out, v.Key)
		}
	}
	return out
}

// A site left at the defaults gets none of the optional variables, so that its
// file follows the defaults of the installer.
func TestOptionalNotWrittenAtDefault(t *testing.T) {
	r, err := Render(Paths{}, sampleState())
	must(t, err)
	a := decode(t, r.AipubVars)
	for _, k := range optionalKeys() {
		if _, ok := a[k]; ok {
			t.Errorf("%s written at its default", k)
		}
	}
}

// Changed values are written, and read back into the state by the loader.
func TestOptionalWrittenWhenChanged(t *testing.T) {
	s := sampleState()
	s.AipubAppEdition = "lite"
	s.AipubAppDeployTool = "helmfile"
	s.AipubMonitoringDeployTool = "helmfile"
	s.AipubAppHelmfileBin = "/usr/local/bin/helmfile"
	s.AipubMonitoringKustomizeBin = "/usr/local/bin/kustomize"
	s.AipubMonitoringHelmBin = "/usr/local/bin/helm"
	s.AipubMonitoringHelmfileDryRun = true
	r, err := Render(Paths{}, s)
	must(t, err)
	a := decode(t, r.AipubVars)
	want := map[string]any{
		"aipub_app_edition":                 "lite",
		"aipub_app_deploy_tool":             "helmfile",
		"aipub_monitoring_deploy_tool":      "helmfile",
		"aipub_app_helmfile_bin":            "/usr/local/bin/helmfile",
		"aipub_monitoring_kustomize_bin":    "/usr/local/bin/kustomize",
		"aipub_monitoring_helm_bin":         "/usr/local/bin/helm",
		"aipub_monitoring_helmfile_dry_run": true,
	}
	for k, w := range want {
		if a[k] != w {
			t.Errorf("%s = %#v, want %#v", k, a[k], w)
		}
	}
	// not changed: the monitoring helmfile follows aipub's
	if _, ok := a["aipub_monitoring_helmfile_bin"]; ok {
		t.Error("aipub_monitoring_helmfile_bin written empty")
	}

	dir := t.TempDir()
	p := DefaultPaths(dir)
	must(t, os.MkdirAll(filepath.Dir(p.AipubVars), 0755))
	must(t, os.WriteFile(p.AipubVars, r.AipubVars, 0644))
	back := state.DefaultState()
	must(t, LoadAipubVars(p.AipubVars, back))
	if back.AipubAppEdition != "lite" || back.AipubAppDeployTool != "helmfile" || back.AipubMonitoringDeployTool != "helmfile" ||
		back.AipubAppHelmfileBin != s.AipubAppHelmfileBin || back.AipubMonitoringKustomizeBin != s.AipubMonitoringKustomizeBin ||
		back.AipubMonitoringHelmBin != s.AipubMonitoringHelmBin || !back.AipubMonitoringHelmfileDryRun {
		t.Errorf("not read back: %+v", back)
	}
}

// Back at the default, a key the file holds with another value goes; one that
// holds the default already stays, with its comment; an empty text counts as
// the default for a path.
func TestOptionalBackToDefault(t *testing.T) {
	site := "aipub_ingress_zone: \"a.example.com\"\n" +
		"aipub_app_edition: \"lite\"\n" +
		"# the helm of this site\naipub_monitoring_helm_bin: \"helm\"\n" +
		"aipub_app_helmfile_bin: \"/old/helmfile\"\n" +
		"aipub_monitoring_helmfile_dry_run: true\n"
	s := sampleState()
	r, err := renderAipubVars([]byte(site), s) // the state is at every default
	must(t, err)
	a := decode(t, r)
	for _, k := range []string{"aipub_app_edition", "aipub_app_helmfile_bin", "aipub_monitoring_helmfile_dry_run"} {
		if _, ok := a[k]; ok {
			t.Errorf("%s kept although the state is at the default", k)
		}
	}
	if a["aipub_monitoring_helm_bin"] != "helm" || !strings.Contains(string(r), "# the helm of this site") {
		t.Errorf("a key at its default was touched:\n%s", r)
	}
}

// The site that has them is shown them: loaded, rendered, loaded again is stable.
func TestOptionalIdempotent(t *testing.T) {
	s := sampleState()
	s.AipubAppEdition = "lite"
	s.AipubMonitoringDeployTool = "helmfile"
	first, err := renderAipubVars(nil, s)
	must(t, err)
	second, err := renderAipubVars(first, s)
	must(t, err)
	if string(first) != string(second) {
		t.Errorf("changed when rendered again:\n%s\n---\n%s", first, second)
	}
}

package fileio

import (
	"os"
	"path/filepath"
	"reflect"
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

// The monitoring variables: written only when changed, the list of nodes only
// when some are picked, and all of them read back.
func TestMonitoringOptional(t *testing.T) {
	r, err := Render(Paths{}, sampleState())
	must(t, err)
	a := decode(t, r.AipubVars)
	for k := range a {
		if strings.HasPrefix(k, "aipub_monitoring_") {
			t.Errorf("%s written at its default", k)
		}
	}

	s := sampleState()
	s.MonitoringOpensearchStorageSize = "500Gi"
	s.MonitoringOpensearchLogRetention = "30d"
	s.MonitoringMetricsStorageSize = "200Gi"
	s.MonitoringMetricsRetentionPeriod = "90d"
	s.MonitoringRightsizingEnabled = false
	s.MonitoringApiReplicas = 1
	s.MonitoringCpNodes = []string{"node1", "node2"}
	r, err = Render(Paths{}, s)
	must(t, err)
	a = decode(t, r.AipubVars)
	want := map[string]any{
		"aipub_monitoring_opensearch_storage_size":  "500Gi",
		"aipub_monitoring_opensearch_log_retention": "30d",
		"aipub_monitoring_metrics_storage_size":     "200Gi",
		"aipub_monitoring_metrics_retention_period": "90d",
		"aipub_monitoring_rightsizing_enabled":      false,
		"aipub_monitoring_api_replicas":             1,
	}
	for k, w := range want {
		if a[k] != w {
			t.Errorf("%s = %#v, want %#v", k, a[k], w)
		}
	}
	if got := a["aipub_monitoring_cp_nodes"]; !reflect.DeepEqual(got, []any{"node1", "node2"}) {
		t.Errorf("cp nodes = %#v", got)
	}

	p := DefaultPaths(t.TempDir())
	must(t, os.MkdirAll(filepath.Dir(p.AipubVars), 0755))
	must(t, os.WriteFile(p.AipubVars, r.AipubVars, 0644))
	back := state.DefaultState()
	must(t, LoadAipubVars(p.AipubVars, back))
	if back.MonitoringOpensearchStorageSize != "500Gi" || back.MonitoringOpensearchLogRetention != "30d" ||
		back.MonitoringMetricsStorageSize != "200Gi" || back.MonitoringMetricsRetentionPeriod != "90d" ||
		back.MonitoringRightsizingEnabled || back.MonitoringApiReplicas != 1 ||
		!reflect.DeepEqual(back.MonitoringCpNodes, []string{"node1", "node2"}) {
		t.Errorf("not read back: %+v", back)
	}

	// the nodes cleared: the key goes, the others stay
	back.MonitoringCpNodes = nil
	back.AipubIngressZone = "aipub.example.com"
	again, err := renderAipubVars(r.AipubVars, back)
	must(t, err)
	a = decode(t, again)
	if _, ok := a["aipub_monitoring_cp_nodes"]; ok {
		t.Error("cp nodes kept after they were cleared")
	}
	if a["aipub_monitoring_api_replicas"] != 1 {
		t.Errorf("a value that did not change was lost: %v", a)
	}
}

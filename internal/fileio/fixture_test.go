package fileio

import (
	"reflect"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
)

// fixtureDir is a site of test/simulate/fixtures, laid out as the ansible directory.
const fixtureDir = "../../test/simulate/fixtures/"

// A site that sets the lite edition, helmfile and monitoring values: it loads,
// validates, and renders the same values back, with the keys it did not set left out.
func TestFixtureHelmfile(t *testing.T) {
	p := DefaultPaths(fixtureDir + "new-site-helmfile")
	s := state.DefaultState()
	must(t, LoadAll(p, s))
	if s.AipubAppEdition != "lite" || s.AipubAppDeployTool != "helmfile" || s.AipubMonitoringDeployTool != "helmfile" ||
		!s.AipubMonitoringHelmfileDryRun || s.MonitoringOpensearchStorageSize != "500Gi" || s.MonitoringApiReplicas != 1 ||
		s.MonitoringRightsizingEnabled || !reflect.DeepEqual(s.MonitoringCpNodes, []string{"master1"}) {
		t.Fatalf("not loaded: %+v", s)
	}
	for _, e := range validator.Validate(s) {
		t.Errorf("fixture refused: %v", e)
	}

	r, err := Render(p, s)
	must(t, err)
	a := decode(t, r.AipubVars)
	for k, want := range map[string]any{
		"aipub_app_edition": "lite", "aipub_app_deploy_tool": "helmfile", "aipub_monitoring_deploy_tool": "helmfile",
		"aipub_monitoring_helmfile_dry_run": true, "aipub_monitoring_opensearch_storage_size": "500Gi",
		"aipub_monitoring_opensearch_log_retention": "30d", "aipub_monitoring_metrics_storage_size": "200Gi",
		"aipub_monitoring_metrics_retention_period": "90d", "aipub_monitoring_rightsizing_enabled": false,
		"aipub_monitoring_api_replicas": 1, "aipub_monitoring_cp_nodes": []any{"master1"},
	} {
		if !reflect.DeepEqual(a[k], want) {
			t.Errorf("%s = %#v, want %#v", k, a[k], want)
		}
	}
	for _, k := range []string{"aipub_app_helmfile_bin", "aipub_monitoring_helmfile_bin", "aipub_monitoring_kustomize_bin", "aipub_monitoring_helm_bin"} {
		if _, ok := a[k]; ok {
			t.Errorf("%s written without being set", k)
		}
	}
}

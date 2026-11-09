package schema

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "rewrite testdata/site-values.golden.yml")

// A small site: two nodes, AIPub without HA, one value moved off its default.
func goldenState() *state.AppState {
	s := state.DefaultState()
	s.Nodes = []state.NodeConfig{
		{Name: "node1", AnsibleHost: "192.168.0.11"},
		{Name: "node2", AnsibleHost: "192.168.0.12", AnsiblePort: "2222"},
	}
	s.KiCpNodes = []string{"node1"}
	s.K8sNodes = []string{"node1", "node2"}
	s.K8sCpNodes = []string{"node1"}
	s.KiCpHaModeVIP = "192.168.0.10"
	s.AipubEnabled = true
	s.AipubIngressZone = "aipub.example.com"
	s.AipubHaMode = false
	s.AipubHarborLocalPVNode = "node1"
	s.AipubCpNodes = []string{"node1"}
	s.K8sIngressVIP = "192.168.0.100"
	s.HarborReplicaCount = 1
	return s
}

func TestSiteValuesGolden(t *testing.T) {
	got := SiteValues(goldenState())
	const path = "testdata/site-values.golden.yml"
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/schema -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("site values differ from %s (run with -update to accept):\n%s", path, got)
	}
}

// What the view says is yaml, and its live lines are the values of the state.
func TestSiteValuesIsYAML(t *testing.T) {
	s := goldenState()
	var doc map[string]any
	if err := yaml.Unmarshal(SiteValues(s), &doc); err != nil {
		t.Fatalf("not yaml: %v", err)
	}
	for k, want := range map[string]any{
		"aipub_ingress_zone":         "aipub.example.com",
		"aipub_harbor_local_pv_node": "node1",
		"aipub_harbor_replica_count": 1,
		"k8s_kube_proxy_enabled":     false,
		"ki_cp_ha_mode_vip":          "192.168.0.10",
		"ansible_port":               map[string]any{"node2": 2222},
	} {
		if got := Normalize(Var{}, doc[k]); !equal(got, Normalize(Var{}, want)) {
			t.Errorf("%s: %#v, want %#v", k, got, want)
		}
	}
	// at their default, commented out
	for _, k := range []string{"aipub_ha_mode_storage_class", "aipub_harbor_ingress_subdomain", "ki_cp_service_timezone"} {
		if _, ok := doc[k]; ok {
			t.Errorf("%s is live", k)
		}
	}
	text := string(SiteValues(s))
	for _, want := range []string{
		"# aipub_cert_mode: \"self_signed\"",
		"aipub_harbor_local_pv_node: \"node1\"",
		"# preflight: `*.<이 도메인>`",
		"#  [AIPUB]  →  files/aipub/vars.yml",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q", want)
		}
	}

	s.AipubEnabled = false
	if text := string(SiteValues(s)); strings.Contains(text, "aipub_ingress_zone") {
		t.Error("aipub variables shown without AIPub")
	}
}

func equal(a, b any) bool { return yamlText(a) == yamlText(b) }

func yamlText(v any) string {
	out, _ := yaml.Marshal(v)
	return string(out)
}

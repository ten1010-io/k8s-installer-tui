package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// A site file with the python section of aipub-installer (setup-python.yml): the
// four values the wizard edits are read and written back, and the ones it does
// not edit (license keys, the InfiniBand nodes and ports) stay as they were, with
// their comments.
func TestPythonSection(t *testing.T) {
	dir := t.TempDir()
	p := DefaultPaths(dir)
	must(t, os.MkdirAll(filepath.Dir(p.AipubVars), 0755))
	site := `aipub_ingress_zone: "py.example.com"
aipub_python_coaster_cluster_name: "seoul-a"
# the two GPU nodes of this site
aipub_python_coaster_gpu_nodes: ["node3", "node4"]
# issued 2026-11
aipub_python_coaster_license_keys:
  node3: "v2.key-of-node3"
aipub_python_deploy_tool: "dag"
aipub_python_network_operator_enabled: true
aipub_python_network_operator_ib_nodes: {"node3": "ib-1"}
aipub_python_network_operator_ib_pfs: {"node3": "0000:3b:00.0"}
`
	must(t, os.WriteFile(p.AipubVars, []byte(site), 0644))

	s := sampleState()
	must(t, LoadAll(p, s))
	if s.PythonCoasterClusterName != "seoul-a" || strings.Join(s.PythonCoasterGpuNodes, ",") != "node3,node4" ||
		s.PythonDeployTool != "dag" || !s.PythonNetworkOperatorEnabled {
		t.Fatalf("python values not loaded: %q %v %q %v", s.PythonCoasterClusterName, s.PythonCoasterGpuNodes,
			s.PythonDeployTool, s.PythonNetworkOperatorEnabled)
	}

	r, err := Render(p, s)
	must(t, err)
	a := decode(t, r.AipubVars)
	if a["aipub_python_coaster_cluster_name"] != "seoul-a" || a["aipub_python_deploy_tool"] != "dag" ||
		a["aipub_python_network_operator_enabled"] != true {
		t.Errorf("python values changed by a save with nothing changed: %v", a)
	}
	keys, _ := a["aipub_python_coaster_license_keys"].(map[string]any)
	if keys["node3"] != "v2.key-of-node3" {
		t.Errorf("license keys lost: %v", a["aipub_python_coaster_license_keys"])
	}
	for _, want := range []string{"# the two GPU nodes of this site", "# issued 2026-11",
		`aipub_python_network_operator_ib_nodes: {"node3": "ib-1"}`,
		`aipub_python_network_operator_ib_pfs: {"node3": "0000:3b:00.0"}`} {
		if !strings.Contains(string(r.AipubVars), want) {
			t.Errorf("lost %q:\n%s", want, r.AipubVars)
		}
	}

	// Changed on the python screen: written. Back at the default of the installer:
	// taken out again, so the file keeps what the installer defaults to.
	s.PythonCoasterClusterName = "seoul-b"
	s.PythonCoasterGpuNodes = []string{"node4"}
	s.PythonDeployTool = "installer"
	s.PythonNetworkOperatorEnabled = false
	r, err = Render(p, s)
	must(t, err)
	a = decode(t, r.AipubVars)
	if a["aipub_python_coaster_cluster_name"] != "seoul-b" {
		t.Errorf("cluster name %v", a["aipub_python_coaster_cluster_name"])
	}
	if nodes, _ := a["aipub_python_coaster_gpu_nodes"].([]any); len(nodes) != 1 || nodes[0] != "node4" {
		t.Errorf("gpu nodes %v", a["aipub_python_coaster_gpu_nodes"])
	}
	for _, k := range []string{"aipub_python_deploy_tool", "aipub_python_network_operator_enabled"} {
		if _, ok := a[k]; ok {
			t.Errorf("%s is at the default of the installer and still in the file", k)
		}
	}
}

// A new site file starts from the example of aipub-installer, which names the
// cluster for coaster; the rest of the python section stays commented out.
func TestPythonSectionOfANewSite(t *testing.T) {
	s := sampleState()
	s.AipubEnabled = true
	r, err := Render(DefaultPaths(t.TempDir()), s)
	must(t, err)
	a := decode(t, r.AipubVars)
	if a["aipub_python_coaster_cluster_name"] != state.DefaultState().PythonCoasterClusterName {
		t.Errorf("cluster name of a new site: %v", a["aipub_python_coaster_cluster_name"])
	}
	for k := range a {
		if strings.HasPrefix(k, "aipub_python_") && k != "aipub_python_coaster_cluster_name" {
			t.Errorf("%s set in a new site file", k)
		}
	}
	if !strings.Contains(string(r.AipubVars), "#aipub_python_coaster_license_keys:") {
		t.Errorf("the commented python keys of the example are not in the new file: %s", tailOf(r.AipubVars, 600))
	}
}

func tailOf(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}

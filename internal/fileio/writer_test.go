package fileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// sampleState is a site of four nodes: two ki cp nodes, three k8s control plane
// nodes that also run base and the services of aipub, and one worker.
func sampleState() *state.AppState {
	s := state.DefaultState()
	for i, ip := range []string{"192.168.0.11", "192.168.0.12", "192.168.0.13", "192.168.0.21"} {
		s.Nodes = append(s.Nodes, state.NodeConfig{Name: []string{"node1", "node2", "node3", "node4"}[i], AnsibleHost: ip})
	}
	s.KiCpNodes = []string{"node1", "node2"}
	s.K8sNodes = []string{"node1", "node2", "node3", "node4"}
	s.K8sCpNodes = []string{"node1", "node2", "node3"}
	s.KiCpHaModeVIP = "192.168.0.10"
	s.K8sIngressVIP = "192.168.0.100"
	s.AipubEnabled = true
	s.AipubIngressZone = "aipub.example.com"
	s.AipubHaModeStorageClass = "nfs-client"
	s.AipubCpNodes = []string{"node1", "node2", "node3"}
	return s
}

func decode(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("not yaml: %v\n%s", err, data)
	}
	return m
}

func dig(m map[string]interface{}, path ...string) interface{} {
	var cur interface{} = m
	for _, p := range path {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func TestNewSite(t *testing.T) {
	r, err := Render(Paths{}, sampleState())
	if err != nil {
		t.Fatal(err)
	}

	vars := decode(t, r.Vars)
	for k := range vars {
		if strings.HasPrefix(k, "aipub_") {
			t.Errorf("vars.yml has %s; k8s-installer refuses aipub_ variables", k)
		}
	}
	for _, k := range legacyK8sVars {
		if _, ok := vars[k]; ok {
			t.Errorf("vars.yml has %s, which k8s-installer refuses", k)
		}
	}
	if vars["k8s_kube_proxy_enabled"] != false {
		t.Errorf("k8s_kube_proxy_enabled = %v, want false with AIPub (cilium replaces kube-proxy)", vars["k8s_kube_proxy_enabled"])
	}
	// Variables of the template the tool does not manage are kept
	for _, k := range []string{"ansible_user", "kubelet_auto_memory_available_min", "k8s_apiserver_extra_args"} {
		if _, ok := vars[k]; !ok {
			t.Errorf("vars.yml lost %s of the template", k)
		}
	}
	if !strings.Contains(string(r.Vars), "# Whether kubeadm installs kube-proxy") {
		t.Error("vars.yml lost the comments of the template")
	}

	a := decode(t, r.AipubVars)
	for k := range a {
		if !strings.HasPrefix(k, "aipub_") {
			t.Errorf("files/aipub/vars.yml has %s; load-site-vars.yml refuses anything but aipub_*", k)
		}
	}
	lbs, _ := a["aipub_load_balancers"].([]interface{})
	if len(lbs) != 1 || dig(lbs[0].(map[string]interface{}), "vip") != "192.168.0.100" {
		t.Errorf("aipub_load_balancers = %v", a["aipub_load_balancers"])
	}
	if dig(a, "aipub_default_ingress_class", "load_balancer") != "lb1" {
		t.Errorf("aipub_default_ingress_class = %v", a["aipub_default_ingress_class"])
	}
	if a["aipub_cert_mode"] != "self_signed" {
		t.Errorf("aipub_cert_mode = %v", a["aipub_cert_mode"])
	}

	inv := decode(t, r.Inventory)
	if dig(inv, "control_node", "hosts", "localhost", "control_node_ih") != "node1" {
		t.Errorf("control_node_ih = %v, want the first ki cp node", dig(inv, "control_node", "hosts", "localhost", "control_node_ih"))
	}
	for _, n := range []string{"node1", "node2", "node3"} {
		if _, ok := dig(inv, "k8s_node", "hosts", n, "k8s_node_labels").(map[string]interface{})[AipubCpNodeLabel]; !ok {
			t.Errorf("%s has no %s label", n, AipubCpNodeLabel)
		}
	}
	if dig(inv, "k8s_node", "hosts", "node4", "k8s_node_labels") != nil {
		t.Error("node4 is not an aipub control plane node but has labels")
	}
}

func TestWithoutAipub(t *testing.T) {
	s := sampleState()
	s.AipubEnabled = false
	r, err := Render(Paths{}, s)
	if err != nil {
		t.Fatal(err)
	}
	if r.AipubVars != nil {
		t.Error("files/aipub/vars.yml rendered without AIPub")
	}
	if decode(t, r.Vars)["k8s_kube_proxy_enabled"] != true {
		t.Error("kube-proxy turned off without AIPub; the site's cni may need it")
	}
	if strings.Contains(string(r.Inventory), AipubCpNodeLabel) {
		t.Error("aipub label written without AIPub")
	}
}

// Files written by version 0.1.x of this tool
const legacyInventory = `control_node:
  hosts:
    localhost:
      ansible_connection: local
all:
  hosts:
    node1:
      ansible_host: 192.168.0.11
      ansible_ssh_user: ubuntu
    node2:
      ansible_host: 192.168.0.12
ki_cp_node:
  hosts:
    node1: {}
k8s_node:
  hosts:
    node1:
      k8s_cp: true
      nvidia_gpu: true
      kernel_cmdline_extra_args:
        - "hugepages=64"
    node2:
      nvidia_gpu: true
`

const legacyVars = `ki_var_root_path: "/var/lib/k8s-installer"
containerd_root_path: "/var/lib/containerd"
docker_root_path: "/var/lib/docker"

internal_network_subnets:
  - "192.168.0.0/24"

ki_cp_ha_mode: false
ki_cp_ha_mode_vip: null
ki_cp_dns_dnssec_validation: false
ki_cp_dns_server_upstream_servers:
  - "8.8.8.8"
ki_cp_ntp_server_upstream_servers:
  - "time1.google.com"

k8s_certificate_validity_period: "26280h" # 26280h = 3 year
k8s_load_balancers:
  - name: "lb1"
    vip: "192.168.0.100"
    nodes:
      - "node1"
k8s_gateways:
  - name: "lb1"
    vip: "192.168.0.100"
    nodes:
      - "node1"
k8s_default_ingress_class:
  load_balancer: "lb1"
  host_network_port: 443

aipub_ingress_zone: "legacy.example.com"
aipub_ha_mode: false
aipub_ha_mode_storage_class: null
aipub_cp_nodes:
  - "node1"
aipub_harbor_replica_count: 1

ki_cert_mode: "ca_provided"
`

func writeLegacy(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	p := DefaultPaths(dir)
	must(t, os.MkdirAll(filepath.Dir(p.Vars), 0755))
	must(t, os.WriteFile(p.Inventory, []byte(legacyInventory), 0644))
	must(t, os.WriteFile(p.Vars, []byte(legacyVars), 0644))
	return p
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacy(t *testing.T) {
	p := writeLegacy(t)
	s := state.DefaultState()
	must(t, LoadAll(p, s))
	s.AipubHarborLocalPVNode = "node1" // asked on the AIPub screen, which 0.1.x did not have
	r, err := Render(p, s)
	must(t, err)

	vars := decode(t, r.Vars)
	for _, k := range []string{"containerd_root_path", "k8s_load_balancers", "k8s_gateways", "k8s_default_ingress_class", "ki_cert_mode", "aipub_ingress_zone"} {
		if _, ok := vars[k]; ok {
			t.Errorf("vars.yml still has %s", k)
		}
	}
	if _, ok := vars["k8s_minor_version"]; !ok {
		t.Error("vars.yml of 0.1.x not brought up to the template: no k8s_minor_version")
	}

	a := decode(t, r.AipubVars)
	if a["aipub_ingress_zone"] != "legacy.example.com" || a["aipub_cert_mode"] != "ca_provided" {
		t.Errorf("aipub values not carried over: zone=%v cert=%v", a["aipub_ingress_zone"], a["aipub_cert_mode"])
	}
	if lbs, _ := a["aipub_load_balancers"].([]interface{}); len(lbs) != 1 {
		t.Errorf("aipub_load_balancers = %v, want the one k8s_load_balancers had", a["aipub_load_balancers"])
	}
	if a["aipub_harbor_replica_count"] != 1 || a["aipub_ha_mode"] != false {
		t.Errorf("harbor values not carried over: %v %v", a["aipub_harbor_replica_count"], a["aipub_ha_mode"])
	}

	inv := decode(t, r.Inventory)
	if dig(inv, "all", "hosts", "node1", "ansible_user") != "ubuntu" || dig(inv, "all", "hosts", "node1", "ansible_ssh_user") != nil {
		t.Error("ansible_ssh_user not moved to ansible_user")
	}
	if dig(inv, "k8s_node", "hosts", "node1", "nvidia_gpu") != nil || dig(inv, "k8s_node", "hosts", "node2", "nvidia_gpu") != nil {
		t.Error("nvidia_gpu kept; k8s-installer refuses it")
	}
	if args, _ := dig(inv, "k8s_node", "hosts", "node1", "kernel_cmdline_extra_args").([]interface{}); len(args) != 1 {
		t.Error("kernel_cmdline_extra_args set by hand was lost")
	}
	if _, ok := dig(inv, "k8s_node", "hosts", "node1", "k8s_node_labels").(map[string]interface{})[AipubCpNodeLabel]; !ok {
		t.Error("aipub_cp_nodes of 0.1.x not turned into the node label")
	}
	if dig(inv, "broken_node") == nil {
		t.Error("no broken_node group")
	}
}

// Saving what was loaded changes nothing the second time.
func TestIdempotent(t *testing.T) {
	p := writeLegacy(t)
	s := state.DefaultState()
	must(t, LoadAll(p, s))
	s.AipubHarborLocalPVNode = "node1"
	must(t, WriteAll(p, s))

	first := [3][]byte{readOrNil(p.Inventory), readOrNil(p.Vars), readOrNil(p.AipubVars)}
	s2 := state.DefaultState()
	must(t, LoadAll(p, s2))
	must(t, WriteAll(p, s2))
	second := [3][]byte{readOrNil(p.Inventory), readOrNil(p.Vars), readOrNil(p.AipubVars)}
	for i, name := range []string{"inventory.yml", "vars.yml", "files/aipub/vars.yml"} {
		if string(first[i]) != string(second[i]) {
			t.Errorf("%s changed when saved again:\n--- first\n%s\n--- second\n%s", name, first[i], second[i])
		}
	}
}

// A site file written with the old aipub_java_* names (aipub-installer still
// accepts them as aliases of aipub_app_*) loads, and saving keeps those keys and
// their comments as they were.
func TestAipubJavaNamesKept(t *testing.T) {
	dir := t.TempDir()
	p := DefaultPaths(dir)
	must(t, os.MkdirAll(filepath.Dir(p.AipubVars), 0755))
	site := "aipub_ingress_zone: \"java.example.com\"\n# old name of aipub_app_edition\naipub_java_edition: \"enterprise\"\naipub_java_file_storage_size: \"20Gi\"\n"
	must(t, os.WriteFile(p.AipubVars, []byte(site), 0644))

	s := state.DefaultState()
	must(t, LoadAll(p, s))
	if !s.AipubEnabled || s.AipubIngressZone != "java.example.com" {
		t.Fatalf("site file with aipub_java_* not loaded: enabled=%v zone=%q", s.AipubEnabled, s.AipubIngressZone)
	}
	r, err := Render(p, s)
	must(t, err)
	a := decode(t, r.AipubVars)
	if a["aipub_java_edition"] != "enterprise" || a["aipub_java_file_storage_size"] != "20Gi" {
		t.Errorf("aipub_java_* values lost: %v", a)
	}
	if _, ok := a["aipub_app_edition"]; ok {
		t.Error("aipub_app_edition added next to aipub_java_edition")
	}
	if !strings.Contains(string(r.AipubVars), "# old name of aipub_app_edition") {
		t.Error("comment on aipub_java_edition lost")
	}
}

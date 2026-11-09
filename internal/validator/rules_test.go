package validator

import (
	"sort"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// valid is a site that passes every rule: three nodes, AIPub with HA.
func valid() *state.AppState {
	s := state.DefaultState()
	s.Nodes = []state.NodeConfig{
		{Name: "node1", AnsibleHost: "192.168.0.11"},
		{Name: "node2", AnsibleHost: "192.168.0.12", AnsiblePort: "2222"},
		{Name: "node3", AnsibleHost: "192.168.0.21"},
	}
	s.KiCpNodes = []string{"node1"}
	s.K8sNodes = []string{"node1", "node2", "node3"}
	s.K8sCpNodes = []string{"node1", "node2"}
	s.KiCpHaModeVIP = "192.168.0.100"
	s.AipubEnabled = true
	s.AipubCpNodes = []string{"node1"}
	s.AipubHaModeStorageClass = "nfs-client"
	s.K8sIngressVIP = "192.168.0.200"
	return s
}

func fields(errs []Error) string {
	f := make([]string, len(errs))
	for i, e := range errs {
		f[i] = e.Field
	}
	sort.Strings(f)
	return strings.Join(f, " ")
}

// The fields each broken site is rejected on. What the user sees rejected is
// what these pin down; the wording of the messages is free to change.
func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(s *state.AppState)
		fields []string
	}{
		{"valid", func(s *state.AppState) {}, nil},
		{"empty site", func(s *state.AppState) { *s = *state.DefaultState() },
			[]string{"k8s_node", "ki_cp_ha_mode_vip", "nodes"}},
		{"nodes", func(s *state.AppState) {
			s.Nodes[0].Name = ""
			s.Nodes[1].AnsibleHost = ""
			s.Nodes[1].AnsiblePort = "70000"
			s.Nodes[2].Name = "node2"
			s.Nodes[2].AnsiblePort = "x"
		}, []string{"ki_cp_node", "k8s_node", "k8s_node", "nodes[0].name", "nodes[1].ansible_host",
			"nodes[1].ansible_port", "nodes[2].ansible_port", "nodes[2].name"}},
		{"no k8s nodes", func(s *state.AppState) { s.K8sNodes = nil },
			[]string{"k8s_node"}},
		{"network formats", func(s *state.AppState) {
			s.InternalNetworkSubnets = []string{"192.168.0.0/24", "x"}
			s.KiCpHaModeVIP = "abc"
			s.KiCpDnsUpstreamServers = []string{"8.8.8.8", "1.2.3"}
		}, []string{"internal_network_subnets[1]", "ki_cp_dns_server_upstream_servers[1]",
			"ki_cp_ha_mode_vip", "ki_cp_ha_mode_vip"}},
		{"vip outside the subnet", func(s *state.AppState) { s.KiCpHaModeVIP = "10.0.0.1" },
			[]string{"ki_cp_ha_mode_vip"}},
		{"no vip without ha", func(s *state.AppState) { s.KiCpHaMode = false; s.KiCpHaModeVIP = "" }, nil},
		{"empty lists", func(s *state.AppState) {
			s.InternalNetworkSubnets = nil
			s.KiCpDnsUpstreamServers = nil
			s.KiCpNtpUpstreamServers = nil
		}, []string{"internal_network_subnets", "ki_cp_dns_server_upstream_servers", "ki_cp_ntp_server_upstream_servers"}},
		{"kubernetes formats", func(s *state.AppState) {
			s.K8sCertificateValidityPeriod = "3y"
			s.K8sMinorVersion = "2"
			s.K8sPodSubnet = "bad"
		}, []string{"k8s_certificate_validity_period", "k8s_minor_version", "k8s_pod_subnet"}},
		{"empty kubernetes values", func(s *state.AppState) {
			s.K8sCertificateValidityPeriod = ""
			s.K8sMinorVersion = ""
			s.K8sServiceSubnet = ""
		}, []string{"k8s_certificate_validity_period", "k8s_minor_version", "k8s_service_subnet"}},
		{"overlapping subnets", func(s *state.AppState) {
			s.K8sServiceSubnet = "10.244.128.0/17"
			s.InternalNetworkSubnets = []string{"192.168.0.0/24", "10.244.0.0/24"}
		}, []string{"k8s_pod_subnet", "k8s_service_subnet"}},
		{"aipub formats", func(s *state.AppState) {
			s.AipubIngressZone = "bad"
			s.AipubHaModeStorageClass = ""
			s.HarborIngressSubdomain = "Bad_"
			s.HarborReplicaCount = 0
			s.HarborRegistryStorageSize = "1G"
			s.HarborTrivyStorageSize = ""
			s.AipubCertMode = "x"
		}, []string{"aipub_cert_mode", "aipub_ha_mode_storage_class", "aipub_harbor_ingress_subdomain",
			"aipub_harbor_registry_storage_size", "aipub_harbor_replica_count",
			"aipub_harbor_trivy_storage_size", "aipub_ingress_zone"}},
		{"aipub without ha", func(s *state.AppState) {
			s.AipubHaMode = false
			s.AipubHaModeStorageClass = ""
		}, []string{"aipub_harbor_local_pv_node"}},
		{"local pv node not an aipub node", func(s *state.AppState) {
			s.AipubHaMode = false
			s.AipubHarborLocalPVNode = "node2"
		}, []string{"aipub_harbor_local_pv_node"}},
		{"aipub nodes", func(s *state.AppState) { s.AipubCpNodes = []string{"node3"} },
			[]string{"aipub_cp_nodes"}},
		{"load balancers", func(s *state.AppState) {
			s.K8sIngressMode = 1
			s.K8sGatewayLBName = s.K8sIngressLBName
			s.K8sGatewayVIP = s.K8sIngressVIP
			s.K8sIngressHostNetworkPort = 0
		}, []string{"k8s_gateway_lb_name", "k8s_gateway_vip", "k8s_ingress_host_network_port"}},
		{"load balancer values", func(s *state.AppState) {
			s.K8sIngressLBName = ""
			s.K8sIngressVIP = "x"
			s.K8sCpNodes = nil
			s.AipubCpNodes = nil
		}, []string{"aipub_cp_nodes", "aipub_load_balancers", "k8s_ingress_lb_name", "k8s_ingress_vip"}},
		{"aipub values ignored without aipub", func(s *state.AppState) {
			s.AipubEnabled = false
			s.AipubIngressZone = "bad"
			s.HarborReplicaCount = 0
			s.AipubCertMode = "x"
			s.K8sIngressVIP = ""
			s.AipubCpNodes = nil
		}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := valid()
			c.edit(s)
			want := append([]string{}, c.fields...)
			sort.Strings(want)
			if got := fields(Validate(s)); got != strings.Join(want, " ") {
				t.Errorf("rejected on\n  %s\nwant\n  %s\nerrors: %v", got, strings.Join(want, " "), Validate(s))
			}
		})
	}
}

// The per-variable checks follow the schema: required always or under a
// condition, and the format of the type, for items of a list too.
func TestCheckVar(t *testing.T) {
	cond := schema.Var{Key: "x", Type: schema.TypeIP, Required: "ki_cp_ha_mode=true"}
	list := schema.Var{Key: "l", Type: schema.TypeList, Elem: schema.TypeCIDR, Required: schema.Always}
	enum := schema.Var{Key: "e", Type: schema.TypeEnum, Enum: []string{"a", "b"}}
	size := schema.Var{Key: "z", Type: schema.TypeSize}
	cases := []struct {
		v        schema.Var
		val      any
		required bool
		want     string
	}{
		{cond, nil, true, "x"},
		{cond, nil, false, ""},
		{cond, "1.2.3", false, "x"},
		{cond, "1.2.3.4", true, ""},
		{list, []string{}, true, "l"},
		{list, []string{"10.0.0.0/8", "x"}, true, "l[1]"},
		{enum, "c", false, "e"},
		{enum, "b", false, ""},
		{size, "", false, "z"}, // empty text is a value of the wrong format
		{size, nil, false, ""}, // unset is fine for an optional one
		{size, "1Gi", false, ""},
	}
	for _, c := range cases {
		if got := fields(checkVar(c.v, c.v.Key, c.val, c.required)); got != c.want {
			t.Errorf("%s=%#v required=%v: rejected on %q, want %q", c.v.Key, c.val, c.required, got, c.want)
		}
	}
	if !strings.Contains(requiredMsg(cond, nil), "ki_cp_ha_mode=true") {
		t.Error("the condition is not named")
	}
}

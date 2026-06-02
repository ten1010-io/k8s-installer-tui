package fileio

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// inventoryYAML mirrors the structure of inventory.yml for parsing.
type inventoryYAML struct {
	All struct {
		Hosts map[string]allHostEntry `yaml:"hosts"`
	} `yaml:"all"`
	KiCpNode struct {
		Hosts map[string]interface{} `yaml:"hosts"`
	} `yaml:"ki_cp_node"`
	K8sNode struct {
		Hosts map[string]k8sNodeEntry `yaml:"hosts"`
	} `yaml:"k8s_node"`
}

type allHostEntry struct {
	AnsibleHost    string `yaml:"ansible_host"`
	AnsiblePort    int    `yaml:"ansible_port"`
	AnsibleSSHUser string `yaml:"ansible_ssh_user"`
}

type k8sNodeEntry struct {
	K8sCp     bool `yaml:"k8s_cp"`
	NvidiaGPU bool `yaml:"nvidia_gpu"`
}

// varsYAML mirrors the structure of group_vars/all/vars.yml for parsing.
type varsYAML struct {
	KiVarRootPath      string   `yaml:"ki_var_root_path"`
	ContainerdRootPath string   `yaml:"containerd_root_path"`
	DockerRootPath     string   `yaml:"docker_root_path"`
	InternalNetworkSubnets []string `yaml:"internal_network_subnets"`
	InternalNetworkExtraZone string `yaml:"internal_network_extra_zone"`
	InternalNetworkExtraZoneARecords []struct {
		Name string `yaml:"name"`
		IP   string `yaml:"ip"`
	} `yaml:"internal_network_extra_zone_a_records"`

	KiCpHaMode              bool     `yaml:"ki_cp_ha_mode"`
	KiCpHaModeVIP           string   `yaml:"ki_cp_ha_mode_vip"`
	KiCpDnsDnssecValidation bool     `yaml:"ki_cp_dns_dnssec_validation"`
	KiCpDnsServerUpstreamServers []string `yaml:"ki_cp_dns_server_upstream_servers"`
	KiCpNtpServerUpstreamServers []string `yaml:"ki_cp_ntp_server_upstream_servers"`

	K8sCertificateValidityPeriod string `yaml:"k8s_certificate_validity_period"`
	K8sLoadBalancers []struct {
		Name  string   `yaml:"name"`
		VIP   string   `yaml:"vip"`
		Nodes []string `yaml:"nodes"`
	} `yaml:"k8s_load_balancers"`
	K8sDefaultIngressClass struct {
		LoadBalancer     string `yaml:"load_balancer"`
		HostNetworkPort  int    `yaml:"host_network_port"`
	} `yaml:"k8s_default_ingress_class"`

	AipubIngressZone        string   `yaml:"aipub_ingress_zone"`
	AipubHaMode             bool     `yaml:"aipub_ha_mode"`
	AipubHaModeStorageClass string   `yaml:"aipub_ha_mode_storage_class"`
	AipubCpNodes            []string `yaml:"aipub_cp_nodes"`

	AipubHarborIngressSubdomain      string `yaml:"aipub_harbor_ingress_subdomain"`
	AipubHarborReplicaCount          int    `yaml:"aipub_harbor_replica_count"`
	AipubHarborRegistryStorageSize   string `yaml:"aipub_harbor_registry_storage_size"`
	AipubHarborPostgresqlStorageSize string `yaml:"aipub_harbor_postgresql_storage_size"`
	AipubHarborRedisStorageSize      string `yaml:"aipub_harbor_redis_storage_size"`
	AipubHarborTrivyStorageSize      string `yaml:"aipub_harbor_trivy_storage_size"`

	KiCertMode string `yaml:"ki_cert_mode"`
}

// LoadInventory reads inventory.yml and populates the node/group fields of AppState.
// Returns nil error and leaves state unchanged if the file does not exist.
func LoadInventory(path string, s *state.AppState) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var inv inventoryYAML
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Nodes — preserve order by iterating map (order may vary, acceptable)
	s.Nodes = nil
	for name, h := range inv.All.Hosts {
		nc := state.NodeConfig{
			Name:        name,
			AnsibleHost: h.AnsibleHost,
		}
		if h.AnsiblePort != 0 {
			nc.AnsiblePort = strconv.Itoa(h.AnsiblePort)
		}
		if h.AnsibleSSHUser != "" {
			nc.AnsibleUser = h.AnsibleSSHUser
		}
		s.Nodes = append(s.Nodes, nc)
	}

	// Groups
	s.KiCpNodes = nil
	for name := range inv.KiCpNode.Hosts {
		s.KiCpNodes = append(s.KiCpNodes, name)
	}

	s.K8sNodes = nil
	s.K8sCpNodes = nil
	s.NvidiaGPUNodes = nil
	for name, entry := range inv.K8sNode.Hosts {
		s.K8sNodes = append(s.K8sNodes, name)
		if entry.K8sCp {
			s.K8sCpNodes = append(s.K8sCpNodes, name)
		}
		if entry.NvidiaGPU {
			s.NvidiaGPUNodes = append(s.NvidiaGPUNodes, name)
		}
	}

	return nil
}

// LoadVars reads group_vars/all/vars.yml and populates the config fields of AppState.
// Returns nil error and leaves state unchanged if the file does not exist.
func LoadVars(path string, s *state.AppState) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var v varsYAML
	if err := yaml.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if len(v.InternalNetworkSubnets) > 0 {
		s.InternalNetworkSubnets = v.InternalNetworkSubnets
	}
	s.InternalNetworkExtraZone = v.InternalNetworkExtraZone
	for _, r := range v.InternalNetworkExtraZoneARecords {
		s.InternalNetworkExtraZoneARecords = append(s.InternalNetworkExtraZoneARecords, state.ARecord{Name: r.Name, IP: r.IP})
	}

	s.KiCpHaMode = v.KiCpHaMode
	if v.KiCpHaModeVIP != "" {
		s.KiCpHaModeVIP = v.KiCpHaModeVIP
	}
	s.KiCpDnsDnssecValidation = v.KiCpDnsDnssecValidation
	if len(v.KiCpDnsServerUpstreamServers) > 0 {
		s.KiCpDnsUpstreamServers = v.KiCpDnsServerUpstreamServers
	}
	if len(v.KiCpNtpServerUpstreamServers) > 0 {
		s.KiCpNtpUpstreamServers = v.KiCpNtpServerUpstreamServers
	}

	if v.K8sCertificateValidityPeriod != "" {
		s.K8sCertificateValidityPeriod = v.K8sCertificateValidityPeriod
	}
	for _, lb := range v.K8sLoadBalancers {
		s.K8sLoadBalancers = append(s.K8sLoadBalancers, state.LBConfig{
			Name:  lb.Name,
			VIP:   lb.VIP,
			Nodes: lb.Nodes,
		})
	}
	s.K8sDefaultIngressClass = state.IngressConfig{
		LoadBalancer: v.K8sDefaultIngressClass.LoadBalancer,
		Port:         v.K8sDefaultIngressClass.HostNetworkPort,
	}

	if v.AipubIngressZone != "" {
		s.AipubIngressZone = v.AipubIngressZone
	}
	s.AipubHaMode = v.AipubHaMode
	s.AipubHaModeStorageClass = v.AipubHaModeStorageClass
	if len(v.AipubCpNodes) > 0 {
		s.AipubCpNodes = v.AipubCpNodes
	}

	if v.AipubHarborIngressSubdomain != "" {
		s.HarborIngressSubdomain = v.AipubHarborIngressSubdomain
	}
	if v.AipubHarborReplicaCount > 0 {
		s.HarborReplicaCount = v.AipubHarborReplicaCount
	}
	if v.AipubHarborRegistryStorageSize != "" {
		s.HarborRegistryStorageSize = v.AipubHarborRegistryStorageSize
	}
	if v.AipubHarborPostgresqlStorageSize != "" {
		s.HarborPostgresqlStorageSize = v.AipubHarborPostgresqlStorageSize
	}
	if v.AipubHarborRedisStorageSize != "" {
		s.HarborRedisStorageSize = v.AipubHarborRedisStorageSize
	}
	if v.AipubHarborTrivyStorageSize != "" {
		s.HarborTrivyStorageSize = v.AipubHarborTrivyStorageSize
	}

	if v.KiCertMode != "" {
		s.KiCertMode = v.KiCertMode
	}

	return nil
}

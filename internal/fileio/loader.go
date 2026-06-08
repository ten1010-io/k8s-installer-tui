package fileio

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// allHostEntry and k8sNodeEntry are used for decoding individual host entries.
type allHostEntry struct {
	AnsibleHost    string `yaml:"ansible_host"`
	AnsiblePort    int    `yaml:"ansible_port"`
	AnsibleSSHUser string `yaml:"ansible_ssh_user"`
}

type k8sNodeEntry struct {
	K8sCp     bool `yaml:"k8s_cp"`
	NvidiaGPU bool `yaml:"nvidia_gpu"`
}

// mappingGet returns the value node for the given key in a YAML mapping node,
// or nil if not found. Mapping node Content is [key0, val0, key1, val1, ...].
func mappingGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
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
	K8sIngressClasses []struct {
		Name          string `yaml:"name"`
		HaMode        bool   `yaml:"ha_mode"`
		HaModeVIP     string `yaml:"ha_mode_vip"`
		HttpHostport  int    `yaml:"http_hostport"`
		HttpsHostport int    `yaml:"https_hostport"`
	} `yaml:"k8s_ingress_classes"`

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
// Nodes are added in the exact order they appear in the YAML file.
// Returns nil error and leaves state unchanged if the file does not exist.
func LoadInventory(path string, s *state.AppState) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// Decode into yaml.Node to preserve mapping key order.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0] // DocumentNode → MappingNode

	// all.hosts — iterate Content in YAML file order to preserve node sequence.
	if hostsNode := mappingGet(mappingGet(root, "all"), "hosts"); hostsNode != nil {
		s.Nodes = nil
		for i := 0; i+1 < len(hostsNode.Content); i += 2 {
			name := hostsNode.Content[i].Value
			var h allHostEntry
			_ = hostsNode.Content[i+1].Decode(&h)
			nc := state.NodeConfig{Name: name, AnsibleHost: h.AnsibleHost}
			if h.AnsiblePort != 0 {
				nc.AnsiblePort = strconv.Itoa(h.AnsiblePort)
			}
			if h.AnsibleSSHUser != "" {
				nc.AnsibleUser = h.AnsibleSSHUser
			}
			s.Nodes = append(s.Nodes, nc)
		}
	}

	// ki_cp_node.hosts — names only, order preserved.
	if hostsNode := mappingGet(mappingGet(root, "ki_cp_node"), "hosts"); hostsNode != nil {
		s.KiCpNodes = nil
		for i := 0; i+1 < len(hostsNode.Content); i += 2 {
			s.KiCpNodes = append(s.KiCpNodes, hostsNode.Content[i].Value)
		}
	}

	// k8s_node.hosts — names and role flags, order preserved.
	if hostsNode := mappingGet(mappingGet(root, "k8s_node"), "hosts"); hostsNode != nil {
		s.K8sNodes = nil
		s.K8sCpNodes = nil
		s.NvidiaGPUNodes = nil
		for i := 0; i+1 < len(hostsNode.Content); i += 2 {
			name := hostsNode.Content[i].Value
			s.K8sNodes = append(s.K8sNodes, name)
			var entry k8sNodeEntry
			_ = hostsNode.Content[i+1].Decode(&entry)
			if entry.K8sCp {
				s.K8sCpNodes = append(s.K8sCpNodes, name)
			}
			if entry.NvidiaGPU {
				s.NvidiaGPUNodes = append(s.NvidiaGPUNodes, name)
			}
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
	if len(v.K8sIngressClasses) > 0 {
		ic := v.K8sIngressClasses[0]
		if ic.Name != "" {
			s.K8sIngressClassName = ic.Name
		}
		s.K8sIngressHaMode = ic.HaMode
		s.K8sIngressHaModeVIP = ic.HaModeVIP
		if ic.HttpHostport > 0 {
			s.K8sIngressHttpPort = ic.HttpHostport
		}
		if ic.HttpsHostport > 0 {
			s.K8sIngressHttpsPort = ic.HttpsHostport
		}
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

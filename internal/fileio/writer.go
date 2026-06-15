package fileio

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// WriteInventory writes inventory.yml, backing up any existing file first.
func WriteInventory(path string, s *state.AppState) error {
	if err := backup(path); err != nil {
		return err
	}

	content, err := renderInventory(s)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0644)
}

// WriteVars writes group_vars/all/vars.yml, backing up any existing file first.
func WriteVars(path string, s *state.AppState) error {
	if err := backup(path); err != nil {
		return err
	}

	content, err := renderVars(s)
	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0644)
}

// RenderInventoryString returns the inventory.yml content as a string (for preview).
func RenderInventoryString(s *state.AppState) (string, error) {
	b, err := renderInventory(s)
	return string(b), err
}

// RenderVarsString returns the vars.yml content as a string (for preview).
func RenderVarsString(s *state.AppState) (string, error) {
	b, err := renderVars(s)
	return string(b), err
}

func backup(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Rename(path, path+".bak")
}

func renderInventory(s *state.AppState) ([]byte, error) {
	// Build using ordered yaml.Node to guarantee key order in output.
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	// control_node section
	root.Content = append(root.Content,
		strNode("control_node"),
		mappingNode(
			"hosts", mappingNode(
				"localhost", mappingNode(
					"ansible_connection", strNode("local"),
				),
			),
		),
	)

	// all.hosts
	allHosts := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, n := range s.Nodes {
		hostEntry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		addStr(hostEntry, "ansible_host", n.AnsibleHost)
		if n.AnsiblePort != "" {
			port, _ := strconv.Atoi(n.AnsiblePort)
			addInt(hostEntry, "ansible_port", port)
		}
		if n.AnsibleUser != "" {
			addStr(hostEntry, "ansible_ssh_user", n.AnsibleUser)
		}
		allHosts.Content = append(allHosts.Content, strNode(n.Name), hostEntry)
	}
	root.Content = append(root.Content,
		strNode("all"),
		mappingNode("hosts", allHosts),
	)

	// ki_cp_node
	kiCpHosts := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range s.KiCpNodes {
		kiCpHosts.Content = append(kiCpHosts.Content,
			strNode(name),
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
	}
	root.Content = append(root.Content,
		strNode("ki_cp_node"),
		mappingNode("hosts", kiCpHosts),
	)

	// k8s_node
	k8sHosts := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range s.K8sNodes {
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if s.IsK8sCpNode(name) {
			addBool(entry, "k8s_cp", true)
		}
		if s.IsNvidiaGPUNode(name) {
			addBool(entry, "nvidia_gpu", true)
		}
		k8sHosts.Content = append(k8sHosts.Content, strNode(name), entry)
	}
	root.Content = append(root.Content,
		strNode("k8s_node"),
		mappingNode("hosts", k8sHosts),
	)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}); err != nil {
		return nil, fmt.Errorf("encode inventory: %w", err)
	}
	return buf.Bytes(), nil
}

func renderVars(s *state.AppState) ([]byte, error) {
	out := map[string]interface{}{
		"ki_var_root_path":      "/var/lib/k8s-installer",
		"containerd_root_path":  "/var/lib/containerd",
		"docker_root_path":      "/var/lib/docker",

		"internal_network_subnets": s.InternalNetworkSubnets,

		"ki_cp_ha_mode":              s.KiCpHaMode,
		"ki_cp_ha_mode_vip":          nullIfEmpty(s.KiCpHaModeVIP),
		"ki_cp_dns_dnssec_validation": s.KiCpDnsDnssecValidation,
		"ki_cp_dns_server_upstream_servers": s.KiCpDnsUpstreamServers,
		"ki_cp_ntp_server_upstream_servers": s.KiCpNtpUpstreamServers,

		"k8s_certificate_validity_period": s.K8sCertificateValidityPeriod,
		"k8s_load_balancers": func() []map[string]interface{} {
			lbs := []map[string]interface{}{{
				"name":  s.K8sIngressLBName,
				"vip":   s.K8sIngressVIP,
				"nodes": s.KiCpNodes,
			}}
			if s.K8sIngressMode == 1 {
				lbs = append(lbs, map[string]interface{}{
					"name":  s.K8sGatewayLBName,
					"vip":   s.K8sGatewayVIP,
					"nodes": s.KiCpNodes,
				})
			}
			return lbs
		}(),
		"k8s_default_ingress_class": map[string]interface{}{
			"load_balancer":     s.K8sIngressLBName,
			"host_network_port": s.K8sIngressHostNetworkPort,
		},
		"k8s_gateways": func() []map[string]interface{} {
			if s.K8sIngressMode == 1 {
				return []map[string]interface{}{{
					"name":  s.K8sGatewayLBName,
					"vip":   s.K8sGatewayVIP,
					"nodes": s.KiCpNodes,
				}}
			}
			return []map[string]interface{}{{
				"name":  s.K8sIngressLBName,
				"vip":   s.K8sIngressVIP,
				"nodes": s.KiCpNodes,
			}}
		}(),

		"ki_cert_mode": s.KiCertMode,
	}

	if s.AipubEnabled {
		out["aipub_ingress_zone"] = s.AipubIngressZone
		out["aipub_ha_mode"] = s.AipubHaMode
		out["aipub_ha_mode_storage_class"] = nullIfEmpty(s.AipubHaModeStorageClass)
		out["aipub_cp_nodes"] = s.AipubCpNodes
		out["aipub_harbor_ingress_subdomain"] = s.HarborIngressSubdomain
		out["aipub_harbor_replica_count"] = s.HarborReplicaCount
		out["aipub_harbor_registry_storage_size"] = s.HarborRegistryStorageSize
		out["aipub_harbor_postgresql_storage_size"] = s.HarborPostgresqlStorageSize
		out["aipub_harbor_redis_storage_size"] = s.HarborRedisStorageSize
		out["aipub_harbor_trivy_storage_size"] = s.HarborTrivyStorageSize
	}

	if s.InternalNetworkExtraZone != "" {
		out["internal_network_extra_zone"] = s.InternalNetworkExtraZone
		if len(s.InternalNetworkExtraZoneARecords) > 0 {
			type aRec struct {
				Name string `yaml:"name"`
				IP   string `yaml:"ip"`
			}
			recs := make([]aRec, len(s.InternalNetworkExtraZoneARecords))
			for i, r := range s.InternalNetworkExtraZoneARecords {
				recs[i] = aRec{Name: r.Name, IP: r.IP}
			}
			out["internal_network_extra_zone_a_records"] = recs
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("encode vars: %w", err)
	}
	return buf.Bytes(), nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// --- yaml.Node helpers ---

func strNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func mappingNode(key string, val *yaml.Node) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, strNode(key), val)
	return m
}

func addStr(m *yaml.Node, key, val string) {
	m.Content = append(m.Content, strNode(key), strNode(val))
}

func addInt(m *yaml.Node, key string, val int) {
	m.Content = append(m.Content, strNode(key), &yaml.Node{
		Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(val),
	})
}

func addBool(m *yaml.Node, key string, val bool) {
	v := "false"
	if val {
		v = "true"
	}
	m.Content = append(m.Content, strNode(key), &yaml.Node{
		Kind: yaml.ScalarNode, Tag: "!!bool", Value: v,
	})
}

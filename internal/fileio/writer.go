package fileio

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

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
	var b strings.Builder

	// Static header
	b.WriteString("ki_var_root_path: \"/var/lib/k8s-installer\"\n")
	b.WriteString("containerd_root_path: \"/var/lib/containerd\"\n")
	b.WriteString("docker_root_path: \"/var/lib/docker\"\n")
	b.WriteString("\n")

	// internal_network_subnets
	b.WriteString("internal_network_subnets:\n")
	for _, subnet := range s.InternalNetworkSubnets {
		fmt.Fprintf(&b, "  - %q\n", subnet)
	}
	b.WriteString("\n")

	// internal_network_extra_zone: commented out when internal DNS server is disabled;
	// when enabled, points the ingress domain's apex/wildcard records at the ingress VIP.
	if !s.InternalDNSServerEnabled {
		b.WriteString("#internal_network_extra_zone: \"example.com\"\n")
		b.WriteString("#internal_network_extra_zone_a_records:\n")
		b.WriteString("#  - name: \"@\"\n")
		b.WriteString("#    ip: \"172.16.0.254\"\n")
		b.WriteString("#  - name: \"*\"\n")
		b.WriteString("#    ip: \"172.16.0.254\"\n")
	} else {
		fmt.Fprintf(&b, "internal_network_extra_zone: %q\n", s.AipubIngressZone)
		b.WriteString("internal_network_extra_zone_a_records:\n")
		fmt.Fprintf(&b, "  - name: \"@\"\n    ip: %q\n", s.K8sIngressVIP)
		fmt.Fprintf(&b, "  - name: \"*\"\n    ip: %q\n", s.K8sIngressVIP)
	}
	b.WriteString("\n")

	// ki_cp section
	fmt.Fprintf(&b, "ki_cp_ha_mode: %v\n", s.KiCpHaMode)
	if s.KiCpHaModeVIP == "" {
		b.WriteString("ki_cp_ha_mode_vip: null\n")
	} else {
		fmt.Fprintf(&b, "ki_cp_ha_mode_vip: %q\n", s.KiCpHaModeVIP)
	}
	fmt.Fprintf(&b, "ki_cp_dns_dnssec_validation: %v\n", s.KiCpDnsDnssecValidation)
	b.WriteString("ki_cp_dns_server_upstream_servers:\n")
	for _, srv := range s.KiCpDnsUpstreamServers {
		fmt.Fprintf(&b, "  - %q\n", srv)
	}
	b.WriteString("ki_cp_ntp_server_upstream_servers:\n")
	for _, srv := range s.KiCpNtpUpstreamServers {
		fmt.Fprintf(&b, "  - %q\n", srv)
	}
	b.WriteString("\n")

	// k8s section
	fmt.Fprintf(&b, "k8s_certificate_validity_period: %q # 26280h = 3 year\n", s.K8sCertificateValidityPeriod)

	// k8s_load_balancers — ingress LB only (gateway LB is defined in k8s_gateways for mode 1)
	b.WriteString("k8s_load_balancers:\n")
	fmt.Fprintf(&b, "  - name: %q\n", s.K8sIngressLBName)
	if s.K8sIngressVIP == "" {
		b.WriteString("    vip: null\n")
	} else {
		fmt.Fprintf(&b, "    vip: %q\n", s.K8sIngressVIP)
	}
	b.WriteString("    nodes:\n")
	for _, n := range s.KiCpNodes {
		fmt.Fprintf(&b, "      - %q\n", n)
	}

	// k8s_gateways
	// Mode 1: separate gateway LB (lb2 with its own VIP)
	// Mode 2: shared IP — gateway references the same LB as ingress
	b.WriteString("k8s_gateways:\n")
	if s.K8sIngressMode == 1 {
		// 별도 IP 모드: gateway 자체 LB명과 VIP
		fmt.Fprintf(&b, "  - name: %q\n", s.K8sGatewayLBName)
		if s.K8sGatewayVIP == "" {
			b.WriteString("    vip: \"\"\n")
		} else {
			fmt.Fprintf(&b, "    vip: %q\n", s.K8sGatewayVIP)
		}
	} else {
		// 공유 IP 모드: ingress LB명과 VIP 복사
		fmt.Fprintf(&b, "  - name: %q\n", s.K8sIngressLBName)
		if s.K8sIngressVIP == "" {
			b.WriteString("    vip: \"\"\n")
		} else {
			fmt.Fprintf(&b, "    vip: %q\n", s.K8sIngressVIP)
		}
	}
	b.WriteString("    nodes:\n")
	for _, n := range s.KiCpNodes {
		fmt.Fprintf(&b, "      - %q\n", n)
	}

	// k8s_default_ingress_class
	b.WriteString("k8s_default_ingress_class:\n")
	fmt.Fprintf(&b, "  load_balancer: %q\n", s.K8sIngressLBName)
	fmt.Fprintf(&b, "  host_network_port: %d\n", s.K8sIngressHostNetworkPort)
	b.WriteString("\n")
	b.WriteString("\n")

	// aipub section (omitted entirely when not installing AIPub)
	if s.AipubEnabled {
		fmt.Fprintf(&b, "aipub_ingress_zone: %q\n", s.AipubIngressZone)
		fmt.Fprintf(&b, "aipub_ha_mode: %v\n", s.AipubHaMode)
		if s.AipubHaModeStorageClass == "" {
			b.WriteString("aipub_ha_mode_storage_class: null\n")
		} else {
			fmt.Fprintf(&b, "aipub_ha_mode_storage_class: %q\n", s.AipubHaModeStorageClass)
		}
		b.WriteString("aipub_cp_nodes:\n")
		for _, n := range s.AipubCpNodes {
			fmt.Fprintf(&b, "  - %q\n", n)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "aipub_harbor_ingress_subdomain: %q\n", s.HarborIngressSubdomain)
		fmt.Fprintf(&b, "aipub_harbor_replica_count: %d\n", s.HarborReplicaCount)
		fmt.Fprintf(&b, "aipub_harbor_registry_storage_size: %s\n", s.HarborRegistryStorageSize)
		fmt.Fprintf(&b, "aipub_harbor_postgresql_storage_size: %s\n", s.HarborPostgresqlStorageSize)
		fmt.Fprintf(&b, "aipub_harbor_redis_storage_size: %s\n", s.HarborRedisStorageSize)
		fmt.Fprintf(&b, "aipub_harbor_trivy_storage_size: %s\n", s.HarborTrivyStorageSize)
		b.WriteString("\n")
	}

	// ki_cert_mode
	b.WriteString("# ki_cert_mode options:\n")
	b.WriteString("# - \"self_signed\": CA를 자동 생성하고 TLS 인증서를 발급 (기본값)\n")
	b.WriteString("# - \"ca_provided\":  고객이 ca.crt + ca.key를 제공, TLS 인증서만 발급\n")
	b.WriteString("# - \"tls_provided\": 고객이 tls.crt + tls.key를 모두 제공, 인증서 발급 생략\n")
	fmt.Fprintf(&b, "ki_cert_mode: %q\n", s.KiCertMode)

	return []byte(b.String()), nil
}

// --- yaml.Node helpers (used by renderInventory) ---

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

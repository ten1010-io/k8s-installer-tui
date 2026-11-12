package fileio

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// LoadAll reads the three files into the state. A file that does not exist leaves
// the defaults. Files written by older versions of this tool (aipub_* and the
// load balancers in group_vars/all/vars.yml) are read too, so that saving moves
// those values to files/aipub/vars.yml.
func LoadAll(p Paths, s *state.AppState) error {
	if err := LoadInventory(p.Inventory, s); err != nil {
		return err
	}
	if err := LoadVars(p.Vars, s); err != nil {
		return err
	}
	loadWebhookURL(p.WebhookConfig(), s)
	if err := LoadAipubVars(p.AipubVars, s); err != nil {
		return err
	}
	loadAuthConfig(p.AuthConfig(), s)
	return nil
}

// loadAuthConfig reads the issuer and the CA back from the authentication config
// the JWKS preset wrote. An issuer that is the derived one is kept as following
// it (empty), so that changing the zone moves it. Read after files/aipub/vars.yml,
// which the derived issuer comes from. Nothing when there is none or it does not
// parse.
func loadAuthConfig(path string, s *state.AppState) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var ac struct {
		JWT []struct {
			Issuer struct {
				URL                  string `yaml:"url"`
				CertificateAuthority string `yaml:"certificateAuthority"`
			} `yaml:"issuer"`
		} `yaml:"jwt"`
	}
	if yaml.Unmarshal(data, &ac) != nil || len(ac.JWT) == 0 {
		return
	}
	is := ac.JWT[0].Issuer
	s.ControlPlane.JWKSIssuerURL = is.URL
	if is.URL == s.JWKSIssuerDerived() {
		s.ControlPlane.JWKSIssuerURL = ""
	}
	// only a CA: not a placeholder, nor anything else that is not a certificate
	if ca, err := ParseCA([]byte(is.CertificateAuthority)); err == nil {
		s.ControlPlane.JWKSCA = ca
	}
}

// loadWebhookURL reads the token review server back from the kubeconfig the
// webhook preset wrote. Nothing when there is none or it does not parse.
func loadWebhookURL(path string, s *state.AppState) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var kc struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if yaml.Unmarshal(data, &kc) == nil && len(kc.Clusters) > 0 {
		s.ControlPlane.WebhookURL = kc.Clusters[0].Cluster.Server
	}
}

type allHostEntry struct {
	AnsibleHost    string `yaml:"ansible_host"`
	AnsiblePort    int    `yaml:"ansible_port"`
	AnsibleUser    string `yaml:"ansible_user"`
	AnsibleSSHUser string `yaml:"ansible_ssh_user"` // older versions of this tool
}

type k8sNodeEntry struct {
	K8sCp         bool              `yaml:"k8s_cp"`
	NvidiaGPU     bool              `yaml:"nvidia_gpu"`
	K8sNodeLabels map[string]string `yaml:"k8s_node_labels"`
}

// mappingGet returns the value node of key in a mapping node, or nil.
func mappingGet(m *yaml.Node, key string) *yaml.Node { return getKey(m, key) }

func readDoc(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	top, err := parseDoc(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return top, nil
}

// LoadInventory reads nodes and groups in the order the file lists them.
func LoadInventory(path string, s *state.AppState) error {
	root, err := readDoc(path)
	if err != nil || root == nil {
		return err
	}

	if hostsNode := mappingGet(mappingGet(root, "all"), "hosts"); hostsNode != nil {
		s.Nodes = nil
		for i := 0; i+1 < len(hostsNode.Content); i += 2 {
			var h allHostEntry
			_ = hostsNode.Content[i+1].Decode(&h)
			nc := state.NodeConfig{Name: hostsNode.Content[i].Value, AnsibleHost: h.AnsibleHost}
			if h.AnsiblePort != 0 {
				nc.AnsiblePort = strconv.Itoa(h.AnsiblePort)
			}
			nc.AnsibleUser = h.AnsibleUser
			if nc.AnsibleUser == "" {
				nc.AnsibleUser = h.AnsibleSSHUser
			}
			s.Nodes = append(s.Nodes, nc)
		}
	}

	if hostsNode := mappingGet(mappingGet(root, "ki_cp_node"), "hosts"); hostsNode != nil {
		s.KiCpNodes = keys(hostsNode)
	}

	if hostsNode := mappingGet(mappingGet(root, "k8s_node"), "hosts"); hostsNode != nil {
		s.K8sNodes, s.K8sCpNodes, s.NvidiaGPUNodes = nil, nil, nil
		var labelled []string
		for i := 0; i+1 < len(hostsNode.Content); i += 2 {
			name := hostsNode.Content[i].Value
			s.K8sNodes = append(s.K8sNodes, name)
			var e k8sNodeEntry
			_ = hostsNode.Content[i+1].Decode(&e)
			if e.K8sCp {
				s.K8sCpNodes = append(s.K8sCpNodes, name)
			}
			if e.NvidiaGPU {
				s.NvidiaGPUNodes = append(s.NvidiaGPUNodes, name)
			}
			if _, ok := e.K8sNodeLabels[AipubCpNodeLabel]; ok {
				labelled = append(labelled, name)
			}
		}
		if len(labelled) > 0 {
			s.AipubCpNodes = labelled
		}
	}
	return nil
}

type lbYAML struct {
	Name  string   `yaml:"name"`
	VIP   string   `yaml:"vip"`
	Nodes []string `yaml:"nodes"`
}

type ingressClassYAML struct {
	LoadBalancer    string `yaml:"load_balancer"`
	HostNetworkPort int    `yaml:"host_network_port"`
}

// aipubYAML is the aipub part, read from files/aipub/vars.yml or, written by older
// versions of this tool, from group_vars/all/vars.yml.
type aipubYAML struct {
	LoadBalancers       []lbYAML         `yaml:"aipub_load_balancers"`
	DefaultIngressClass ingressClassYAML `yaml:"aipub_default_ingress_class"`
	CertMode            string           `yaml:"aipub_cert_mode"`

	IngressZone          string   `yaml:"aipub_ingress_zone"`
	AppHostPrefix        string   `yaml:"aipub_app_host_prefix"`
	HaMode               *bool    `yaml:"aipub_ha_mode"`
	HaModeStorageClass   string   `yaml:"aipub_ha_mode_storage_class"`
	HarborLocalPVNode    string   `yaml:"aipub_harbor_local_pv_node"`
	CpNodes              []string `yaml:"aipub_cp_nodes"` // older versions of this tool
	HarborIngressSub     string   `yaml:"aipub_harbor_ingress_subdomain"`
	HarborReplicaCount   int      `yaml:"aipub_harbor_replica_count"`
	HarborRegistrySize   string   `yaml:"aipub_harbor_registry_storage_size"`
	HarborPostgresqlSize string   `yaml:"aipub_harbor_postgresql_storage_size"`
	HarborRedisSize      string   `yaml:"aipub_harbor_redis_storage_size"`
	HarborTrivySize      string   `yaml:"aipub_harbor_trivy_storage_size"`

	AppEdition        string `yaml:"aipub_app_edition"`
	AppDeployTool     string `yaml:"aipub_app_deploy_tool"`
	AppHelmfileBin    string `yaml:"aipub_app_helmfile_bin"`
	MonDeployTool     string `yaml:"aipub_monitoring_deploy_tool"`
	MonHelmfileBin    string `yaml:"aipub_monitoring_helmfile_bin"`
	MonKustomizeBin   string `yaml:"aipub_monitoring_kustomize_bin"`
	MonHelmBin        string `yaml:"aipub_monitoring_helm_bin"`
	MonHelmfileDryRun *bool  `yaml:"aipub_monitoring_helmfile_dry_run"`

	MonOpensearchSize      string   `yaml:"aipub_monitoring_opensearch_storage_size"`
	MonOpensearchRetention string   `yaml:"aipub_monitoring_opensearch_log_retention"`
	MonMetricsSize         string   `yaml:"aipub_monitoring_metrics_storage_size"`
	MonMetricsRetention    string   `yaml:"aipub_monitoring_metrics_retention_period"`
	MonRightsizing         *bool    `yaml:"aipub_monitoring_rightsizing_enabled"`
	MonApiReplicas         int      `yaml:"aipub_monitoring_api_replicas"`
	MonCpNodes             []string `yaml:"aipub_monitoring_cp_nodes"`

	PyClusterName     string   `yaml:"aipub_python_coaster_cluster_name"`
	PyGpuNodes        []string `yaml:"aipub_python_coaster_gpu_nodes"`
	PyDeployTool      string   `yaml:"aipub_python_deploy_tool"`
	PyNetworkOperator *bool    `yaml:"aipub_python_network_operator_enabled"`
}

type varsYAML struct {
	aipubYAML `yaml:",inline"`

	InternalNetworkSubnets   []string `yaml:"internal_network_subnets"`
	InternalNetworkExtraZone string   `yaml:"internal_network_extra_zone"`

	KiCpHaMode                   bool     `yaml:"ki_cp_ha_mode"`
	KiCpHaModeVIP                string   `yaml:"ki_cp_ha_mode_vip"`
	KiCpDnsDnssecValidation      bool     `yaml:"ki_cp_dns_dnssec_validation"`
	KiCpDnsServerUpstreamServers []string `yaml:"ki_cp_dns_server_upstream_servers"`
	KiCpNtpServerUpstreamServers []string `yaml:"ki_cp_ntp_server_upstream_servers"`

	K8sMinorVersion              string `yaml:"k8s_minor_version"`
	K8sPodSubnet                 string `yaml:"k8s_pod_subnet"`
	K8sServiceSubnet             string `yaml:"k8s_service_subnet"`
	K8sCertificateValidityPeriod string `yaml:"k8s_certificate_validity_period"`

	K8sApiserverExtraArgs            []state.ExtraArg    `yaml:"k8s_apiserver_extra_args"`
	K8sControllerManagerExtraArgs    []state.ExtraArg    `yaml:"k8s_controller_manager_extra_args"`
	K8sSchedulerExtraArgs            []state.ExtraArg    `yaml:"k8s_scheduler_extra_args"`
	K8sEtcdExtraArgs                 []state.ExtraArg    `yaml:"k8s_etcd_extra_args"`
	K8sApiserverExtraVolumes         []state.ExtraVolume `yaml:"k8s_apiserver_extra_volumes"`
	K8sControllerManagerExtraVolumes []state.ExtraVolume `yaml:"k8s_controller_manager_extra_volumes"`
	K8sSchedulerExtraVolumes         []state.ExtraVolume `yaml:"k8s_scheduler_extra_volumes"`
	K8sCpExtraFiles                  []state.CpExtraFile `yaml:"k8s_cp_extra_files"`

	// older versions of this tool
	K8sLoadBalancers       []lbYAML         `yaml:"k8s_load_balancers"`
	K8sGateways            []lbYAML         `yaml:"k8s_gateways"`
	K8sDefaultIngressClass ingressClassYAML `yaml:"k8s_default_ingress_class"`
	KiCertMode             string           `yaml:"ki_cert_mode"`
}

// LoadVars reads group_vars/all/vars.yml.
func LoadVars(path string, s *state.AppState) error {
	root, err := readDoc(path)
	if err != nil || root == nil {
		return err
	}
	var v varsYAML
	if err := root.Decode(&v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if len(v.InternalNetworkSubnets) > 0 {
		s.InternalNetworkSubnets = v.InternalNetworkSubnets
	}
	s.InternalDNSServerEnabled = v.InternalNetworkExtraZone != ""
	s.KiCpHaMode = v.KiCpHaMode
	s.KiCpHaModeVIP = v.KiCpHaModeVIP
	s.KiCpDnsDnssecValidation = v.KiCpDnsDnssecValidation
	if len(v.KiCpDnsServerUpstreamServers) > 0 {
		s.KiCpDnsUpstreamServers = v.KiCpDnsServerUpstreamServers
	}
	if len(v.KiCpNtpServerUpstreamServers) > 0 {
		s.KiCpNtpUpstreamServers = v.KiCpNtpServerUpstreamServers
	}
	setIf(&s.K8sMinorVersion, v.K8sMinorVersion)
	setIf(&s.K8sPodSubnet, v.K8sPodSubnet)
	setIf(&s.K8sServiceSubnet, v.K8sServiceSubnet)
	setIf(&s.K8sCertificateValidityPeriod, v.K8sCertificateValidityPeriod)

	// The control plane: the presets it holds whole, and the rest as entries
	s.ControlPlane.Absorb(map[string][]state.ExtraArg{
		"apiserver":          v.K8sApiserverExtraArgs,
		"controller_manager": v.K8sControllerManagerExtraArgs,
		"scheduler":          v.K8sSchedulerExtraArgs,
		"etcd":               v.K8sEtcdExtraArgs,
	}, map[string][]state.ExtraVolume{
		"apiserver":          v.K8sApiserverExtraVolumes,
		"controller_manager": v.K8sControllerManagerExtraVolumes,
		"scheduler":          v.K8sSchedulerExtraVolumes,
	}, v.K8sCpExtraFiles)

	// Written by older versions of this tool: the same values under the k8s_ names
	legacy := v.aipubYAML
	if len(legacy.LoadBalancers) == 0 && len(v.K8sLoadBalancers) > 0 {
		legacy.LoadBalancers = v.K8sLoadBalancers
		if len(v.K8sGateways) > 0 && v.K8sGateways[0].Name != v.K8sLoadBalancers[0].Name {
			legacy.LoadBalancers = append(legacy.LoadBalancers, v.K8sGateways[0])
		}
		legacy.DefaultIngressClass = v.K8sDefaultIngressClass
	}
	if legacy.CertMode == "" {
		legacy.CertMode = v.KiCertMode
	}
	applyAipub(legacy, s)
	return nil
}

// LoadAipubVars reads files/aipub/vars.yml. Its values win over what an older
// vars.yml carried.
func LoadAipubVars(path string, s *state.AppState) error {
	root, err := readDoc(path)
	if err != nil || root == nil {
		return err
	}
	var a aipubYAML
	if err := root.Decode(&a); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	s.AipubEnabled = true
	applyAipub(a, s)
	return nil
}

func applyAipub(a aipubYAML, s *state.AppState) {
	if a.IngressZone != "" {
		s.AipubEnabled = true
		s.AipubIngressZone = a.IngressZone
	}
	if n := len(a.LoadBalancers); n > 0 {
		s.K8sIngressLBName, s.K8sIngressVIP = a.LoadBalancers[0].Name, a.LoadBalancers[0].VIP
		s.K8sIngressMode = 2
		if n > 1 {
			s.K8sIngressMode = 1
			s.K8sGatewayLBName, s.K8sGatewayVIP = a.LoadBalancers[1].Name, a.LoadBalancers[1].VIP
		}
	}
	if a.DefaultIngressClass.HostNetworkPort > 0 {
		s.K8sIngressHostNetworkPort = a.DefaultIngressClass.HostNetworkPort
	}
	setIf(&s.AipubCertMode, a.CertMode)
	setIf(&s.AipubAppHostPrefix, a.AppHostPrefix)
	if a.HaMode != nil {
		s.AipubHaMode = *a.HaMode
	}
	setIf(&s.AipubHaModeStorageClass, a.HaModeStorageClass)
	setIf(&s.AipubHarborLocalPVNode, a.HarborLocalPVNode)
	if len(a.CpNodes) > 0 && len(s.AipubCpNodes) == 0 {
		s.AipubCpNodes = a.CpNodes
	}
	setIf(&s.HarborIngressSubdomain, a.HarborIngressSub)
	if a.HarborReplicaCount > 0 {
		s.HarborReplicaCount = a.HarborReplicaCount
	}
	setIf(&s.HarborRegistryStorageSize, a.HarborRegistrySize)
	setIf(&s.HarborPostgresqlStorageSize, a.HarborPostgresqlSize)
	setIf(&s.HarborRedisStorageSize, a.HarborRedisSize)
	setIf(&s.HarborTrivyStorageSize, a.HarborTrivySize)

	setIf(&s.AipubAppEdition, a.AppEdition)
	setIf(&s.AipubAppDeployTool, a.AppDeployTool)
	setIf(&s.AipubAppHelmfileBin, a.AppHelmfileBin)
	setIf(&s.AipubMonitoringDeployTool, a.MonDeployTool)
	setIf(&s.AipubMonitoringHelmfileBin, a.MonHelmfileBin)
	setIf(&s.AipubMonitoringKustomizeBin, a.MonKustomizeBin)
	setIf(&s.AipubMonitoringHelmBin, a.MonHelmBin)
	if a.MonHelmfileDryRun != nil {
		s.AipubMonitoringHelmfileDryRun = *a.MonHelmfileDryRun
	}

	setIf(&s.MonitoringOpensearchStorageSize, a.MonOpensearchSize)
	setIf(&s.MonitoringOpensearchLogRetention, a.MonOpensearchRetention)
	setIf(&s.MonitoringMetricsStorageSize, a.MonMetricsSize)
	setIf(&s.MonitoringMetricsRetentionPeriod, a.MonMetricsRetention)
	if a.MonRightsizing != nil {
		s.MonitoringRightsizingEnabled = *a.MonRightsizing
	}
	if a.MonApiReplicas > 0 {
		s.MonitoringApiReplicas = a.MonApiReplicas
	}
	if len(a.MonCpNodes) > 0 {
		s.MonitoringCpNodes = a.MonCpNodes
	}

	setIf(&s.PythonCoasterClusterName, a.PyClusterName)
	if len(a.PyGpuNodes) > 0 {
		s.PythonCoasterGpuNodes = a.PyGpuNodes
	}
	setIf(&s.PythonDeployTool, a.PyDeployTool)
	if a.PyNetworkOperator != nil {
		s.PythonNetworkOperatorEnabled = *a.PyNetworkOperator
	}
}

func setIf(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// LoadSite reads what the files of the site hold, for the wizard to tell later
// which values it changed (schema.FixedChanges). nil for a new site: one without
// group_vars/all/vars.yml. A file that does not exist is left out.
func LoadSite(p Paths) (*schema.Loaded, error) {
	vars, err := readDoc(p.Vars)
	if err != nil || vars == nil {
		return nil, err
	}
	l := &schema.Loaded{Values: map[schema.File]map[string]any{}}
	var k8s map[string]any
	if err := vars.Decode(&k8s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p.Vars, err)
	}
	l.Values[schema.FileK8s] = k8s
	if aipub, err := readDoc(p.AipubVars); err != nil {
		return nil, err
	} else if aipub != nil {
		var m map[string]any
		if err := aipub.Decode(&m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p.AipubVars, err)
		}
		l.Values[schema.FileAipub] = m
	}

	inv, err := readDoc(p.Inventory)
	if err != nil || inv == nil {
		return l, err
	}
	// the variables of the hosts, from every group they are set in
	hostVars := map[string]any{}
	for _, group := range keys(inv) {
		hosts := mappingGet(mappingGet(inv, group), "hosts")
		if hosts == nil || hosts.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(hosts.Content); i += 2 {
			host := hosts.Content[i].Value
			var hv map[string]any
			_ = hosts.Content[i+1].Decode(&hv)
			for k, v := range hv {
				m, _ := hostVars[k].(map[string]any)
				if m == nil {
					m = map[string]any{}
					hostVars[k] = m
				}
				m[host] = v
			}
		}
	}
	l.Values[schema.FileInventory] = hostVars
	l.Hosts = keys(mappingGet(mappingGet(inv, "all"), "hosts"))
	return l, nil
}

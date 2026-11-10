package state

import "strings"

// AppState holds everything the screens edit. It is written to three files:
//
//	inventory.yml               nodes, groups, the aipub control plane label   (k8s-installer)
//	group_vars/all/vars.yml     the cluster                                    (k8s-installer)
//	files/aipub/vars.yml        aipub_* only, when AIPub is installed          (aipub-installer)
//
// k8s-installer refuses a variable of vars.yml it does not classify, and
// aipub-installer refuses anything but aipub_* in its file, so no variable is
// written to the other file.
type AppState struct {
	// Existing: the site had its files when the wizard started (fileio.LoadSite),
	// so its cluster is there. Not saved
	Existing bool

	// S1: Nodes (inventory.yml → all.hosts)
	Nodes []NodeConfig

	// S2: Groups (inventory.yml → ki_cp_node, k8s_node)
	KiCpNodes  []string
	K8sNodes   []string
	K8sCpNodes []string
	// Read from older inventories only. k8s-installer removed nvidia_gpu (the
	// node runtime finds the gpus itself) and refuses an inventory that sets it
	NvidiaGPUNodes []string

	// S3: Network (vars.yml)
	InternalNetworkSubnets   []string
	KiCpHaMode               bool
	KiCpHaModeVIP            string
	KiCpDnsDnssecValidation  bool
	KiCpDnsUpstreamServers   []string
	KiCpNtpUpstreamServers   []string
	InternalDNSServerEnabled bool

	// S4: Kubernetes (vars.yml)
	K8sMinorVersion              string
	K8sPodSubnet                 string
	K8sServiceSubnet             string
	K8sCertificateValidityPeriod string

	// S4b: Control plane (vars.yml → k8s_*_extra_args, k8s_*_extra_volumes,
	// k8s_cp_extra_files; the kubeconfig of the webhook preset under files/)
	ControlPlane ControlPlane

	// S4: Load balancers of AIPub (files/aipub/vars.yml → aipub_load_balancers,
	// aipub_default_ingress_class). Named K8s* from when k8s-installer carried them
	K8sIngressMode            int // 1=별도IP (ingress lb + gateway lb), 2=공유IP
	K8sIngressLBName          string
	K8sIngressVIP             string
	K8sGatewayLBName          string // mode 1 only
	K8sGatewayVIP             string // mode 1 only
	K8sIngressHostNetworkPort int

	// S5: AIPub (files/aipub/vars.yml)
	AipubEnabled     bool
	AipubIngressZone string
	// aipub_app_host_prefix, read from the site file and never written: the
	// issuer of the JWKS preset is derived from it
	AipubAppHostPrefix          string
	AipubHaMode                 bool
	AipubHaModeStorageClass     string
	AipubHarborLocalPVNode      string // without HA: the node harbor keeps its data on
	AipubCpNodes                []string
	HarborIngressSubdomain      string
	HarborReplicaCount          int
	HarborRegistryStorageSize   string
	HarborPostgresqlStorageSize string
	HarborRedisStorageSize      string
	HarborTrivyStorageSize      string

	// S5: edition and deploy tools (files/aipub/vars.yml), written only when they
	// differ from the default of aipub-installer (schema.Var.Optional)
	AipubAppEdition               string // aipub_app_edition: enterprise | lite
	AipubAppDeployTool            string // aipub_app_deploy_tool: installer | helmfile
	AipubMonitoringDeployTool     string // aipub_monitoring_deploy_tool: installer | helmfile
	AipubAppHelmfileBin           string // empty: the helmfile of the package
	AipubMonitoringHelmfileBin    string // empty: the same as aipub_app_helmfile_bin
	AipubMonitoringKustomizeBin   string // empty: kustomize beside the helmfile
	AipubMonitoringHelmBin        string // empty or helm: the helm of k8s-installer
	AipubMonitoringHelmfileDryRun bool

	// S5b: the monitoring stack (files/aipub/vars.yml), also written only when
	// changed
	MonitoringOpensearchStorageSize  string
	MonitoringOpensearchLogRetention string
	MonitoringMetricsStorageSize     string
	MonitoringMetricsRetentionPeriod string
	MonitoringRightsizingEnabled     bool
	MonitoringApiReplicas            int
	// MonitoringCpNodes are the nodes OpenSearch and Alertmanager run a replica
	// on, by node name; empty: every k8s control plane node
	MonitoringCpNodes []string

	// S6: Certificate mode (files/aipub/vars.yml → aipub_cert_mode)
	AipubCertMode string
}

func DefaultState() *AppState {
	return &AppState{
		KiCpHaMode:                   true,
		KiCpDnsDnssecValidation:      false,
		KiCpDnsUpstreamServers:       []string{"8.8.8.8", "8.8.4.4"},
		KiCpNtpUpstreamServers:       []string{"time1.google.com", "time2.google.com"},
		InternalNetworkSubnets:       []string{"192.168.0.0/24"},
		K8sMinorVersion:              "1.36",
		K8sPodSubnet:                 "10.244.0.0/16",
		K8sServiceSubnet:             "10.96.0.0/12",
		K8sCertificateValidityPeriod: "26280h",
		K8sIngressMode:               2,
		K8sIngressLBName:             "lb1",
		K8sGatewayLBName:             "lb2",
		K8sIngressHostNetworkPort:    443,
		AipubEnabled:                 false,
		AipubIngressZone:             "example.com",
		AipubAppHostPrefix:           "aipub",
		AipubHaMode:                  true,
		HarborIngressSubdomain:       "aipub-harbor",
		HarborReplicaCount:           3,
		HarborRegistryStorageSize:    "512Gi",
		HarborPostgresqlStorageSize:  "128Gi",
		HarborRedisStorageSize:       "32Gi",
		HarborTrivyStorageSize:       "5Gi",
		AipubCertMode:                "self_signed",

		AipubAppEdition:           "enterprise",
		AipubAppDeployTool:        "installer",
		AipubMonitoringDeployTool: "installer",
		AipubMonitoringHelmBin:    "helm",

		MonitoringOpensearchStorageSize:  "3Ti",
		MonitoringOpensearchLogRetention: "180d",
		MonitoringMetricsStorageSize:     "1Ti",
		MonitoringMetricsRetentionPeriod: "180d",
		MonitoringRightsizingEnabled:     true,
		MonitoringApiReplicas:            2,

		// The authentication of AIPub on a new site (aipub-installer k8s-jwks:
		// "A new install has no webhook")
		ControlPlane: ControlPlane{JWKS: true},
	}
}

// AppliedControlPlane is the control plane as it is written: the JWKS preset
// only when AIPub is installed, as its issuer is AIPub. The copy shares the lists.
func (s *AppState) AppliedControlPlane() *ControlPlane {
	cp := s.ControlPlane
	if !s.AipubEnabled {
		cp.JWKS = false
	}
	return &cp
}

// JWKSIssuerDerived is the issuer aipub-installer gives k8s-jwks: AIPUB_URL,
// https://<aipubHostPrefix>.<aipubDomain> (scripts/aipub/installer/lib/config.sh),
// which site.yaml.j2 fills from aipub_app_host_prefix and aipub_ingress_zone.
// Empty without a zone.
func (s *AppState) JWKSIssuerDerived() string {
	if s.AipubIngressZone == "" {
		return ""
	}
	prefix := s.AipubAppHostPrefix
	if prefix == "" {
		prefix = "aipub"
	}
	return "https://" + prefix + "." + s.AipubIngressZone
}

// JWKSIssuer is the issuer the JWKS preset writes: the one the site set, else
// the derived one.
func (s *AppState) JWKSIssuer() string {
	if v := strings.TrimSpace(s.ControlPlane.JWKSIssuerURL); v != "" {
		return v
	}
	return s.JWKSIssuerDerived()
}

// KubeProxyEnabled is what k8s_kube_proxy_enabled is written as. AIPub brings
// cilium up with kubeProxyReplacement, and setup-network.yml stops on a cluster
// that has kube-proxy; without AIPub the cluster keeps kube-proxy for whatever
// cni the site installs.
func (s *AppState) KubeProxyEnabled() bool { return !s.AipubEnabled }

func (s *AppState) NodeNames() []string {
	names := make([]string, len(s.Nodes))
	for i, n := range s.Nodes {
		names[i] = n.Name
	}
	return names
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func (s *AppState) IsKiCpNode(name string) bool      { return contains(s.KiCpNodes, name) }
func (s *AppState) IsK8sNode(name string) bool       { return contains(s.K8sNodes, name) }
func (s *AppState) IsK8sCpNode(name string) bool     { return contains(s.K8sCpNodes, name) }
func (s *AppState) IsNvidiaGPUNode(name string) bool { return contains(s.NvidiaGPUNodes, name) }
func (s *AppState) IsAipubCpNode(name string) bool   { return contains(s.AipubCpNodes, name) }

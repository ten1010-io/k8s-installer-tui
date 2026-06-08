package state

type AppState struct {
	// S1: Nodes (inventory.yml → all.hosts)
	Nodes []NodeConfig

	// S2: Groups (inventory.yml → ki_cp_node, k8s_node)
	KiCpNodes      []string
	K8sNodes       []string
	K8sCpNodes     []string
	NvidiaGPUNodes []string

	// S3: Network (vars.yml)
	InternalNetworkSubnets           []string
	KiCpHaMode                       bool
	KiCpHaModeVIP                    string
	KiCpDnsDnssecValidation          bool
	KiCpDnsUpstreamServers           []string
	KiCpNtpUpstreamServers           []string
	InternalNetworkExtraZone         string
	InternalNetworkExtraZoneARecords []ARecord

	// S4: Kubernetes (vars.yml)
	K8sCertificateValidityPeriod string
	K8sIngressClassName          string
	K8sIngressHaMode             bool
	K8sIngressHaModeVIP          string
	K8sIngressHttpPort           int
	K8sIngressHttpsPort          int

	// S5: AIPub (vars.yml)
	AipubIngressZone            string
	AipubHaMode                 bool
	AipubHaModeStorageClass     string
	AipubCpNodes                []string
	HarborIngressSubdomain      string
	HarborReplicaCount          int
	HarborRegistryStorageSize   string
	HarborPostgresqlStorageSize string
	HarborRedisStorageSize      string
	HarborTrivyStorageSize      string

	// S6: Certificate mode (vars.yml)
	KiCertMode string
}

func DefaultState() *AppState {
	return &AppState{
		KiCpHaMode:                   true,
		KiCpDnsDnssecValidation:      false,
		KiCpDnsUpstreamServers:       []string{"8.8.8.8", "8.8.4.4"},
		KiCpNtpUpstreamServers:       []string{"time1.google.com", "time2.google.com"},
		InternalNetworkSubnets:       []string{"192.168.0.0/24"},
		K8sCertificateValidityPeriod: "26280h",
		K8sIngressClassName:          "lb1",
		K8sIngressHttpPort:           80,
		K8sIngressHttpsPort:          443,
		AipubIngressZone:             "example.com",
		AipubHaMode:                  true,
		HarborIngressSubdomain:       "aipub-harbor",
		HarborReplicaCount:           3,
		HarborRegistryStorageSize:    "512Gi",
		HarborPostgresqlStorageSize:  "128Gi",
		HarborRedisStorageSize:       "32Gi",
		HarborTrivyStorageSize:       "5Gi",
		KiCertMode:                   "self_signed",
	}
}

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

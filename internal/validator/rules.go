package validator

import (
	"fmt"
	"net"
	"regexp"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

var (
	reStorage   = regexp.MustCompile(`^[0-9]+[EPTGMK]i$`)
	reValidity  = regexp.MustCompile(`^[0-9]+h$`)
	reFQDN      = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)
	reSubdomain = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-.]*[a-z0-9])?$`)
	reK8sName   = reSubdomain
)

type Error struct {
	Field string
	Msg   string
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

// Validate runs all cross-field and per-field validation rules.
// Returns a slice of errors; empty slice means valid.
func Validate(s *state.AppState) []Error {
	var errs []Error
	errs = append(errs, validateNodes(s)...)
	errs = append(errs, validateGroups(s)...)
	errs = append(errs, validateNetwork(s)...)
	errs = append(errs, validateKubernetes(s)...)
	errs = append(errs, validateAipub(s)...)
	return errs
}

func validateNodes(s *state.AppState) []Error {
	var errs []Error
	seen := map[string]bool{}
	for i, n := range s.Nodes {
		prefix := fmt.Sprintf("nodes[%d]", i)
		if n.Name == "" {
			errs = append(errs, Error{prefix + ".name", "이름을 입력해주세요"})
		} else if seen[n.Name] {
			errs = append(errs, Error{prefix + ".name", fmt.Sprintf("노드 이름 '%s'가 중복됩니다", n.Name)})
		} else {
			seen[n.Name] = true
		}
		if n.AnsibleHost == "" {
			errs = append(errs, Error{prefix + ".ansible_host", "ansible_host를 입력해주세요"})
		}
		if n.AnsiblePort != "" {
			p, err := strconv.Atoi(n.AnsiblePort)
			if err != nil || p < 1 || p > 65535 {
				errs = append(errs, Error{prefix + ".ansible_port", "포트는 1-65535 범위여야 합니다"})
			}
		}
	}
	if len(s.Nodes) == 0 {
		errs = append(errs, Error{"nodes", "노드를 최소 1개 이상 추가해주세요"})
	}
	return errs
}

func validateGroups(s *state.AppState) []Error {
	var errs []Error
	nodeNames := map[string]bool{}
	for _, n := range s.Nodes {
		nodeNames[n.Name] = true
	}

	for _, name := range s.KiCpNodes {
		if !nodeNames[name] {
			errs = append(errs, Error{"ki_cp_node", fmt.Sprintf("'%s'는 노드 목록에 없습니다", name)})
		}
	}
	for _, name := range s.K8sNodes {
		if !nodeNames[name] {
			errs = append(errs, Error{"k8s_node", fmt.Sprintf("'%s'는 노드 목록에 없습니다", name)})
		}
	}
	if len(s.K8sNodes) == 0 {
		errs = append(errs, Error{"k8s_node", "k8s_node 그룹에 최소 1개 이상의 노드가 필요합니다"})
	}
	return errs
}

func validateNetwork(s *state.AppState) []Error {
	var errs []Error

	// R2: ki_cp_ha_mode=true → vip 필수
	if s.KiCpHaMode && s.KiCpHaModeVIP == "" {
		errs = append(errs, Error{"ki_cp_ha_mode_vip", "HA 모드가 활성화된 경우 VIP를 설정해야 합니다"})
	}
	if s.KiCpHaModeVIP != "" && net.ParseIP(s.KiCpHaModeVIP) == nil {
		errs = append(errs, Error{"ki_cp_ha_mode_vip", "유효한 IPv4 주소를 입력해주세요"})
	}

	// R1: VIP는 ki_cp_node 서브넷 내에 있어야 함 (subnet 정보가 있을 때만)
	if s.KiCpHaMode && s.KiCpHaModeVIP != "" && len(s.InternalNetworkSubnets) > 0 {
		vip := net.ParseIP(s.KiCpHaModeVIP)
		inSubnet := false
		for _, cidr := range s.InternalNetworkSubnets {
			_, network, err := net.ParseCIDR(cidr)
			if err == nil && vip != nil && network.Contains(vip) {
				inSubnet = true
				break
			}
		}
		if !inSubnet {
			errs = append(errs, Error{"ki_cp_ha_mode_vip",
				fmt.Sprintf("VIP %s가 내부 서브넷 범위 밖입니다", s.KiCpHaModeVIP)})
		}
	}

	if len(s.InternalNetworkSubnets) == 0 {
		errs = append(errs, Error{"internal_network_subnets", "서브넷을 최소 1개 이상 입력해주세요"})
	}
	for i, cidr := range s.InternalNetworkSubnets {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			errs = append(errs, Error{
				fmt.Sprintf("internal_network_subnets[%d]", i),
				fmt.Sprintf("'%s'는 유효한 CIDR 형식이 아닙니다 (예: 192.168.0.0/24)", cidr),
			})
		}
	}

	if len(s.KiCpDnsUpstreamServers) == 0 {
		errs = append(errs, Error{"ki_cp_dns_server_upstream_servers", "DNS 서버를 최소 1개 이상 입력해주세요"})
	}
	for i, srv := range s.KiCpDnsUpstreamServers {
		if net.ParseIP(srv) == nil {
			errs = append(errs, Error{
				fmt.Sprintf("ki_cp_dns_server_upstream_servers[%d]", i),
				fmt.Sprintf("'%s'는 유효한 IPv4 주소가 아닙니다", srv),
			})
		}
	}

	if len(s.KiCpNtpUpstreamServers) == 0 {
		errs = append(errs, Error{"ki_cp_ntp_server_upstream_servers", "NTP 서버를 최소 1개 이상 입력해주세요"})
	}

	if s.InternalNetworkExtraZone != "" && !reFQDN.MatchString(s.InternalNetworkExtraZone) {
		errs = append(errs, Error{"internal_network_extra_zone", "유효한 도메인 형식이 아닙니다 (예: example.com)"})
	}

	return errs
}

func validateKubernetes(s *state.AppState) []Error {
	var errs []Error

	if !reValidity.MatchString(s.K8sCertificateValidityPeriod) {
		errs = append(errs, Error{"k8s_certificate_validity_period", "형식이 올바르지 않습니다 (예: 26280h)"})
	}

	lbNames := map[string]bool{}
	for i, lb := range s.K8sLoadBalancers {
		prefix := fmt.Sprintf("k8s_load_balancers[%d]", i)
		if lb.Name == "" {
			errs = append(errs, Error{prefix + ".name", "로드밸런서 이름을 입력해주세요"})
		} else if !reK8sName.MatchString(lb.Name) {
			errs = append(errs, Error{prefix + ".name", "K8s 오브젝트 네이밍 규칙을 따라야 합니다 (소문자, 숫자, 하이픈)"})
		} else if lbNames[lb.Name] {
			errs = append(errs, Error{prefix + ".name", fmt.Sprintf("로드밸런서 이름 '%s'가 중복됩니다", lb.Name)})
		} else {
			lbNames[lb.Name] = true
		}
		if net.ParseIP(lb.VIP) == nil {
			errs = append(errs, Error{prefix + ".vip", "유효한 IPv4 주소를 입력해주세요"})
		}
		// R3: LB nodes ⊆ k8s_node group
		k8sNodeSet := map[string]bool{}
		for _, n := range s.K8sNodes {
			k8sNodeSet[n] = true
		}
		for _, n := range lb.Nodes {
			if !k8sNodeSet[n] {
				errs = append(errs, Error{prefix + ".nodes",
					fmt.Sprintf("'%s'는 k8s_node 그룹에 속하지 않습니다", n)})
			}
		}
	}

	// R4: default ingress LB must reference an existing LB
	if s.K8sDefaultIngressClass.LoadBalancer != "" && !lbNames[s.K8sDefaultIngressClass.LoadBalancer] {
		errs = append(errs, Error{"k8s_default_ingress_class.load_balancer",
			fmt.Sprintf("'%s'는 정의된 로드밸런서가 아닙니다", s.K8sDefaultIngressClass.LoadBalancer)})
	}

	return errs
}

func validateAipub(s *state.AppState) []Error {
	var errs []Error

	if !reFQDN.MatchString(s.AipubIngressZone) {
		errs = append(errs, Error{"aipub_ingress_zone", "유효한 도메인 형식이 아닙니다 (예: example.com)"})
	}

	// R5: aipub_ha_mode=true → storage_class 필수
	if s.AipubHaMode && s.AipubHaModeStorageClass == "" {
		errs = append(errs, Error{"aipub_ha_mode_storage_class", "AIPub HA 모드에서는 StorageClass를 지정해야 합니다"})
	}

	// R6: aipub_cp_nodes ⊆ k8s_node
	k8sNodeSet := map[string]bool{}
	for _, n := range s.K8sNodes {
		k8sNodeSet[n] = true
	}
	for _, n := range s.AipubCpNodes {
		if !k8sNodeSet[n] {
			errs = append(errs, Error{"aipub_cp_nodes",
				fmt.Sprintf("'%s'는 k8s_node 그룹에 속하지 않습니다", n)})
		}
	}

	if !reSubdomain.MatchString(s.HarborIngressSubdomain) {
		errs = append(errs, Error{"aipub_harbor_ingress_subdomain", "유효한 서브도메인 형식이 아닙니다"})
	}
	if s.HarborReplicaCount < 1 {
		errs = append(errs, Error{"aipub_harbor_replica_count", "레플리카 수는 최소 1 이상이어야 합니다"})
	}
	for _, field := range []struct{ name, val string }{
		{"aipub_harbor_registry_storage_size", s.HarborRegistryStorageSize},
		{"aipub_harbor_postgresql_storage_size", s.HarborPostgresqlStorageSize},
		{"aipub_harbor_redis_storage_size", s.HarborRedisStorageSize},
		{"aipub_harbor_trivy_storage_size", s.HarborTrivyStorageSize},
	} {
		if !reStorage.MatchString(field.val) {
			errs = append(errs, Error{field.name, fmt.Sprintf("'%s'는 올바른 스토리지 크기 형식이 아닙니다 (예: 512Gi)", field.val)})
		}
	}

	certModes := map[string]bool{"self_signed": true, "ca_provided": true, "tls_provided": true}
	if !certModes[s.KiCertMode] {
		errs = append(errs, Error{"ki_cert_mode", "self_signed, ca_provided, tls_provided 중 하나를 선택해주세요"})
	}

	return errs
}

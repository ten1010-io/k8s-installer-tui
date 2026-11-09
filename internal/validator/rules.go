package validator

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

var (
	reSize      = regexp.MustCompile(`^[0-9]+[EPTGMK]i$`)
	reDuration  = regexp.MustCompile(`^[0-9]+h$`)
	reFQDN      = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)
	reSubdomain = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-.]*[a-z0-9])?$`)
	reMinor     = regexp.MustCompile(`^1\.[0-9]+$`)
)

type Error struct {
	Field string
	Msg   string
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

// Validate runs every rule. Whether a variable is required, and the format of
// its value, come from the schema; the rules written out below are the ones
// that relate several values (a VIP inside a subnet, subnets that must not
// overlap, names that must be nodes) or that are not a variable of the files
// (the list of nodes, the load balancer screen).
// Returns a slice of errors; empty slice means valid.
func Validate(s *state.AppState) []Error {
	var errs []Error
	errs = append(errs, validateSchema(s)...)
	errs = append(errs, validateNodes(s)...)
	errs = append(errs, validateGroups(s)...)
	errs = append(errs, validateNetwork(s)...)
	errs = append(errs, validateKubernetes(s)...)
	errs = append(errs, validateAipub(s)...)
	return errs
}

// --- per variable, from the schema ---

func validateSchema(s *state.AppState) []Error {
	var errs []Error
	for _, v := range schema.All() {
		if !schema.Written(s, v.File) {
			continue
		}
		val, managed := schema.Value(s, v)
		if !managed {
			continue
		}
		required := v.RequiredIn(s)
		if hosts, ok := val.([]schema.HostValue); ok {
			for i, h := range hosts {
				errs = append(errs, checkVar(v, hostField(s, i, h.Host, v.Key), h.Value, required)...)
			}
			continue
		}
		errs = append(errs, checkVar(v, v.Key, val, required)...)
	}
	return errs
}

// hostField names a variable of a node the way the node screen does: by the
// position of the node, the i-th value when the values follow the list of nodes
// (two nodes may carry one name until that is fixed).
func hostField(s *state.AppState, i int, host, key string) string {
	if i < len(s.Nodes) && s.Nodes[i].Name == host {
		return fmt.Sprintf("nodes[%d].%s", i, key)
	}
	for i, n := range s.Nodes {
		if n.Name == host {
			return fmt.Sprintf("nodes[%d].%s", i, key)
		}
	}
	return fmt.Sprintf("%s(%s)", key, host)
}

// checkVar checks one value: given when required, and of the format of its type.
// An unset (null) optional value is fine; an empty text is checked against the
// format like any other.
func checkVar(v schema.Var, field string, val any, required bool) []Error {
	if schema.Missing(val) && required {
		return []Error{{field, requiredMsg(v, val)}}
	}
	if val == nil {
		return nil
	}
	if items, ok := val.([]string); ok {
		var errs []Error
		for i, item := range items {
			if msg := formatMsg(v.Elem, v.Enum, item); msg != "" {
				errs = append(errs, Error{fmt.Sprintf("%s[%d]", field, i), msg})
			}
		}
		return errs
	}
	if msg := formatMsg(v.Type, v.Enum, val); msg != "" {
		return []Error{{field, msg}}
	}
	return nil
}

func requiredMsg(v schema.Var, val any) string {
	msg := "값을 입력해주세요"
	if _, isList := val.([]string); isList || v.Type == schema.TypeList {
		msg = "최소 1개 이상 입력해주세요"
	}
	if key, want, ok := v.Condition(); ok {
		msg = fmt.Sprintf("%s=%s 이면 %s", key, want, msg)
	}
	if v.Short != "" {
		msg += " (" + v.Short + ")"
	}
	return msg
}

// formatMsg says what is wrong with a value of type t, or "" when it is right.
func formatMsg(t schema.Type, enum []string, val any) string {
	text := fmt.Sprint(val)
	switch t {
	case schema.TypeInt:
		if n, ok := toInt(val); !ok || n < 1 {
			return "1 이상의 정수여야 합니다"
		}
	case schema.TypePort:
		if n, ok := toInt(val); !ok || n < 1 || n > 65535 {
			return "포트는 1-65535 범위여야 합니다"
		}
	case schema.TypeCIDR:
		if _, _, err := net.ParseCIDR(text); err != nil {
			return fmt.Sprintf("'%s'는 유효한 CIDR 형식이 아닙니다 (예: x.x.x.x/xx)", text)
		}
	case schema.TypeIP:
		if net.ParseIP(text) == nil {
			return fmt.Sprintf("'%s'는 유효한 IPv4 주소가 아닙니다", text)
		}
	case schema.TypeFQDN:
		if !reFQDN.MatchString(text) {
			return fmt.Sprintf("'%s'는 유효한 도메인 형식이 아닙니다 (예: example.com)", text)
		}
	case schema.TypeSubdomain:
		if !reSubdomain.MatchString(text) {
			return fmt.Sprintf("'%s'는 유효한 서브도메인 형식이 아닙니다", text)
		}
	case schema.TypeMinor:
		if !reMinor.MatchString(text) {
			return fmt.Sprintf("'%s'는 형식이 올바르지 않습니다 (예: 1.36)", text)
		}
	case schema.TypeDuration:
		if !reDuration.MatchString(text) {
			return fmt.Sprintf("'%s'는 형식이 올바르지 않습니다 (예: 26280h)", text)
		}
	case schema.TypeSize:
		if !reSize.MatchString(text) {
			return fmt.Sprintf("'%s'는 올바른 스토리지 크기 형식이 아닙니다 (예: 512Gi)", text)
		}
	case schema.TypeEnum:
		for _, e := range enum {
			if text == e {
				return ""
			}
		}
		return strings.Join(enum, ", ") + " 중 하나를 선택해주세요"
	}
	return ""
}

func toInt(val any) (int, bool) {
	switch n := val.(type) {
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}

// --- across variables, and what is not a variable ---

// validateNodes checks the list of nodes: there is one, and every one has a name
// of its own. Their addresses and ports are variables (validateSchema).
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
	}
	if len(s.Nodes) == 0 {
		errs = append(errs, Error{"nodes", "노드를 최소 1개 이상 추가해주세요"})
	}
	return errs
}

// validateGroups checks that the members of the groups are nodes.
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
	return errs
}

func validateNetwork(s *state.AppState) []Error {
	var errs []Error

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
	return errs
}

func validateKubernetes(s *state.AppState) []Error {
	var errs []Error

	// k8s-installer: the pod and service subnets must not overlap each other or the
	// network of the nodes (validate_k8s_subnets). A subnet that does not parse is
	// reported by validateSchema
	parsed := map[string]*net.IPNet{}
	for name, cidr := range map[string]string{"k8s_pod_subnet": s.K8sPodSubnet, "k8s_service_subnet": s.K8sServiceSubnet} {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			parsed[name] = n
		}
	}
	if p, sv := parsed["k8s_pod_subnet"], parsed["k8s_service_subnet"]; p != nil && sv != nil && overlaps(p, sv) {
		errs = append(errs, Error{"k8s_service_subnet", "파드 서브넷과 서비스 서브넷이 겹칩니다"})
	}
	for _, cidr := range s.InternalNetworkSubnets {
		_, in, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		for name, n := range parsed {
			if overlaps(in, n) {
				errs = append(errs, Error{name, fmt.Sprintf("내부 서브넷 %s와 겹칩니다", cidr)})
			}
		}
	}

	// The load balancers below belong to AIPub (aipub_load_balancers); without it
	// nothing reads them. They are the fields of the load balancer screen, which
	// make up two entries of one variable, so they are checked here
	if !s.AipubEnabled {
		return errs
	}

	if s.K8sIngressMode != 1 && s.K8sIngressMode != 2 {
		errs = append(errs, Error{"k8s_ingress_mode", "인그레스 모드는 1 또는 2여야 합니다"})
	}

	if s.K8sIngressLBName == "" {
		errs = append(errs, Error{"k8s_ingress_lb_name", "Ingress LB명을 입력해주세요"})
	}

	if s.K8sIngressVIP == "" {
		errs = append(errs, Error{"k8s_ingress_vip", "Ingress VIP를 입력해주세요"})
	} else if net.ParseIP(s.K8sIngressVIP) == nil {
		errs = append(errs, Error{"k8s_ingress_vip", "유효한 IPv4 주소를 입력해주세요"})
	}

	if s.K8sIngressMode == 1 {
		if s.K8sGatewayLBName == "" {
			errs = append(errs, Error{"k8s_gateway_lb_name", "별도 IP 모드에서는 Gateway LB명을 입력해주세요"})
		} else if s.K8sGatewayLBName == s.K8sIngressLBName {
			errs = append(errs, Error{"k8s_gateway_lb_name", "별도 IP 모드에서는 Ingress LB명과 Gateway LB명이 달라야 합니다"})
		}
		if s.K8sGatewayVIP == "" {
			errs = append(errs, Error{"k8s_gateway_vip", "별도 IP 모드에서는 Gateway VIP를 입력해주세요"})
		} else if net.ParseIP(s.K8sGatewayVIP) == nil {
			errs = append(errs, Error{"k8s_gateway_vip", "유효한 IPv4 주소를 입력해주세요"})
		} else if s.K8sGatewayVIP == s.K8sIngressVIP {
			errs = append(errs, Error{"k8s_gateway_vip", "별도 IP 모드에서는 Ingress VIP와 Gateway VIP가 달라야 합니다"})
		}
	}

	if s.K8sIngressHostNetworkPort <= 0 || s.K8sIngressHostNetworkPort > 65535 {
		errs = append(errs, Error{"k8s_ingress_host_network_port", "포트는 1-65535 범위여야 합니다"})
	}

	// aipub_load_balancers: the k8s control plane nodes answer ARP for the VIPs
	if len(s.K8sCpNodes) == 0 {
		errs = append(errs, Error{"aipub_load_balancers", "VIP를 받을 마스터(k8s_cp) 노드가 없습니다"})
	}

	return errs
}

func overlaps(a, b *net.IPNet) bool { return a.Contains(b.IP) || b.Contains(a.IP) }

func validateAipub(s *state.AppState) []Error {
	if !s.AipubEnabled {
		return nil
	}
	var errs []Error

	// R5: without HA the local PV node (required, validateSchema) has to carry the
	// aipub label (check-base.yml)
	if !s.AipubHaMode && s.AipubHarborLocalPVNode != "" && !s.IsAipubCpNode(s.AipubHarborLocalPVNode) {
		errs = append(errs, Error{"aipub_harbor_local_pv_node",
			fmt.Sprintf("'%s'는 AIPub 컨트롤 플레인 노드가 아닙니다", s.AipubHarborLocalPVNode)})
	}

	// The aipub label is a variable of the nodes (k8s_node_labels), chosen on the
	// AIPub screen as a list of nodes
	if len(s.AipubCpNodes) == 0 {
		errs = append(errs, Error{"aipub_cp_nodes", "AIPub 컨트롤 플레인 노드를 1개 이상 선택해야 합니다 (Harbor가 그 노드에서 돕니다)"})
	}

	// R6: aipub_cp_nodes ⊆ k8s_cp_node
	k8sCpNodeSet := map[string]bool{}
	for _, n := range s.K8sCpNodes {
		k8sCpNodeSet[n] = true
	}
	for _, n := range s.AipubCpNodes {
		if !k8sCpNodeSet[n] {
			errs = append(errs, Error{"aipub_cp_nodes",
				fmt.Sprintf("'%s'는 마스터 노드 그룹에 속하지 않습니다", n)})
		}
	}

	return errs
}

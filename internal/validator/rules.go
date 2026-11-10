package validator

import (
	"fmt"
	"net"
	"net/url"
	"path"
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
	// the monitoring installer reads a retention as days: values.yaml.j2 takes the
	// digits of "180d" for the longest window of rightsizing, so a week ("2w") or
	// hours it would read as zero
	reDays = regexp.MustCompile(`^[1-9][0-9]*d$`)
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
	errs = append(errs, validateControlPlane(s)...)
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
	switch items := val.(type) {
	case []state.ExtraArg:
		return checkExtraArgs(v, field, items)
	case []state.ExtraVolume:
		return checkExtraVolumes(field, items)
	case []state.CpExtraFile:
		return checkCpExtraFiles(field, items)
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
	case schema.TypeDays:
		if !reDays.MatchString(text) {
			return fmt.Sprintf("'%s'는 형식이 올바르지 않습니다 (일 단위, 예: 180d)", text)
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

	// the VIPs that passed the checks above, against the rest of the network
	if ip := net.ParseIP(s.K8sIngressVIP); ip != nil {
		errs = append(errs, checkLBVIP(s, "k8s_ingress_vip", ip)...)
	}
	if s.K8sIngressMode == 1 && s.K8sGatewayVIP != s.K8sIngressVIP {
		if ip := net.ParseIP(s.K8sGatewayVIP); ip != nil {
			errs = append(errs, checkLBVIP(s, "k8s_gateway_vip", ip)...)
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

// Warnings are what may be meant and is worth a look, which saving does not
// wait on: a load balancer VIP outside every internal subnet works on a second
// (external) network of the nodes, nothing of the installers limits it to the
// internal one.
func Warnings(s *state.AppState) []string {
	if !s.AipubEnabled {
		return nil
	}
	var subnets []*net.IPNet
	for _, cidr := range s.InternalNetworkSubnets {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			subnets = append(subnets, n)
		}
	}
	if len(subnets) == 0 {
		return nil
	}
	vips := []struct{ field, vip string }{{"k8s_ingress_vip", s.K8sIngressVIP}}
	if s.K8sIngressMode == 1 && s.K8sGatewayVIP != s.K8sIngressVIP {
		vips = append(vips, struct{ field, vip string }{"k8s_gateway_vip", s.K8sGatewayVIP})
	}
	var out []string
	for _, v := range vips {
		ip := net.ParseIP(v.vip)
		if ip == nil {
			continue
		}
		inside := false
		for _, n := range subnets {
			inside = inside || n.Contains(ip)
		}
		if !inside {
			out = append(out, fmt.Sprintf("%s: VIP %s가 내부망 대역 (%s) 밖 — 의도한 구성인지 확인하세요",
				v.field, v.vip, strings.Join(s.InternalNetworkSubnets, ", ")))
		}
	}
	return out
}

// checkLBVIP checks a VIP of the AIPub load balancers against the rest of the
// network: it must not be taken by the ki cp VIP or a node, nor fall inside the
// subnets of the cluster. A sub-check whose other side is empty or does not
// parse is left to the rule of that value. Outside the internal subnets is only
// a warning (Warnings).
func checkLBVIP(s *state.AppState, field string, vip net.IP) []Error {
	var errs []Error
	if s.KiCpHaMode {
		if cp := net.ParseIP(s.KiCpHaModeVIP); cp != nil && cp.Equal(vip) {
			errs = append(errs, Error{field, fmt.Sprintf("VIP %s가 ki cp HA VIP (ki_cp_ha_mode_vip) 와 같습니다. 둘 중 하나를 다른 IP로 바꿔주세요", vip)})
		}
	}
	for _, n := range s.Nodes {
		if ip := net.ParseIP(n.AnsibleHost); ip != nil && ip.Equal(vip) {
			errs = append(errs, Error{field, fmt.Sprintf("VIP %s가 노드 %s의 ansible_host 와 같습니다. 어느 노드도 쓰지 않는 IP로 바꿔주세요", vip, n.Name)})
		}
	}
	for _, sub := range []struct{ name, cidr string }{{"k8s_pod_subnet", s.K8sPodSubnet}, {"k8s_service_subnet", s.K8sServiceSubnet}} {
		if _, n, err := net.ParseCIDR(sub.cidr); err == nil && n.Contains(vip) {
			errs = append(errs, Error{field, fmt.Sprintf("VIP %s가 %s (%s) 안입니다. 내부 서브넷의 IP로 바꾸거나 %s 를 바꿔주세요",
				vip, sub.name, sub.cidr, sub.name)})
		}
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

	errs = append(errs, monitoringCpNodes(s)...)
	return errs
}

// monitoringCpNodes: aipub_monitoring_cp_nodes ⊆ k8s_cp_node. OpenSearch and
// Alertmanager run a replica on each, with volumes that are directories of those
// nodes without HA, so they have to be nodes of the cluster that run workloads
// of the control plane (set-facts.yml of the monitoring module defaults to every
// k8s control plane node).
func monitoringCpNodes(s *state.AppState) []Error {
	var errs []Error
	for _, n := range s.MonitoringCpNodes {
		if !s.IsK8sCpNode(n) {
			errs = append(errs, Error{"aipub_monitoring_cp_nodes",
				fmt.Sprintf("'%s'는 마스터 노드 그룹에 속하지 않습니다", n)})
		}
	}
	return errs
}

// Monitoring checks what the monitoring screen edits on its own: its variables
// and the nodes it picks, for the screen to stop on before the preview.
func Monitoring(s *state.AppState) []Error {
	var errs []Error
	for _, v := range schema.ByFile(schema.FileAipub) {
		if v.Screen != "5b.모니터링" {
			continue
		}
		if val, ok := schema.Value(s, v); ok {
			errs = append(errs, checkVar(v, v.Key, val, false)...)
		}
	}
	return append(errs, monitoringCpNodes(s)...)
}

// --- the control plane (k8s_*_extra_args, k8s_*_extra_volumes, k8s_cp_extra_files) ---
//
// The rules of validate-hostvars.py of k8s-installer (ExtraArgModel,
// ExtraVolumeModel, CpExtraFileModel, validate_control_plane_extra_args,
// validate_control_plane_extra_volumes, validate_k8s_cp_extra_files), so that the
// wizard does not write a value the installer refuses before touching a node.

var (
	reArgName  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	reDNSLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	reFileMode = regexp.MustCompile(`^0[0-7]{3}$`)
)

// kubeadmVolumeNames are the volumes kubeadm gives the control plane pods
// itself; one declared under the same name replaces it (KUBEADM_CONTROL_PLANE_VOLUME_NAMES).
var kubeadmVolumeNames = []string{
	"ca-certs", "etc-ca-certificates", "etc-pki", "flexvolume-dir", "k8s-certs",
	"kubeconfig", "usr-local-share-ca-certificates", "usr-share-ca-certificates",
}

var volumePathTypes = []string{"DirectoryOrCreate", "Directory", "FileOrCreate", "File"}

// cpOwnedPaths are the trees kubeadm or k8s-installer write and remove on their
// own, where a k8s_cp_extra_files dest is refused: k8s_pki_path,
// k8s_manifests_path, k8s_etcd_data_path, ki_etc_root_path, ki_opt_root_path
// (constant-vars.yml) and ki_var_root_path at the default of vars.yml.
var cpOwnedPaths = []string{
	"/etc/kubernetes/pki", "/etc/kubernetes/manifests", "/var/lib/etcd",
	"/etc/k8s-installer", "/opt/k8s-installer", "/var/lib/k8s-installer",
}

func checkExtraArgs(v schema.Var, field string, args []state.ExtraArg) []Error {
	var errs []Error
	for i, a := range args {
		f := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case strings.HasPrefix(a.Name, "-"):
			errs = append(errs, Error{f, fmt.Sprintf("'%s': 인자 이름은 앞의 대시(--) 없이 씁니다", a.Name)})
		case !reArgName.MatchString(a.Name):
			errs = append(errs, Error{f, fmt.Sprintf("'%s': 인자 이름은 소문자·숫자·대시만 씁니다", a.Name)})
		case contains(v.Decided, a.Name):
			errs = append(errs, Error{f, fmt.Sprintf("'%s' 는 설치기가 직접 정하는 인자라 넣을 수 없습니다", a.Name)})
		}
	}
	return errs
}

func checkExtraVolumes(field string, vols []state.ExtraVolume) []Error {
	var errs []Error
	seen := map[string]bool{}
	for i, vol := range vols {
		f := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case !reDNSLabel.MatchString(vol.Name):
			errs = append(errs, Error{f, fmt.Sprintf("'%s': 볼륨 이름은 DNS-1123 레이블입니다 (소문자·숫자·대시, 63자 이하)", vol.Name)})
		case contains(kubeadmVolumeNames, vol.Name):
			errs = append(errs, Error{f, fmt.Sprintf("'%s' 는 kubeadm 이 쓰는 볼륨 이름이라 쓸 수 없습니다", vol.Name)})
		case seen[vol.Name]:
			errs = append(errs, Error{f, fmt.Sprintf("볼륨 이름 '%s' 가 중복됩니다", vol.Name)})
		}
		seen[vol.Name] = true
		for _, p := range []struct{ name, path string }{{"hostPath", vol.HostPath}, {"mountPath", vol.MountPath}} {
			if !path.IsAbs(p.path) {
				errs = append(errs, Error{f + "." + p.name, fmt.Sprintf("'%s': 절대 경로여야 합니다", p.path)})
			}
		}
		if vol.PathType != "" && !contains(volumePathTypes, vol.PathType) {
			errs = append(errs, Error{f + ".pathType", strings.Join(volumePathTypes, ", ") + " 중 하나여야 합니다"})
		}
	}
	return errs
}

func checkCpExtraFiles(field string, files []state.CpExtraFile) []Error {
	var errs []Error
	seen := map[string]bool{}
	for i, file := range files {
		f := fmt.Sprintf("%s[%d]", field, i)
		switch {
		case file.Src == "" || path.IsAbs(file.Src):
			errs = append(errs, Error{f + ".src", "ansible/files 아래의 상대 경로여야 합니다"})
		case contains(strings.Split(file.Src, "/"), ".."):
			errs = append(errs, Error{f + ".src", "ansible/files 밖을 가리킬 수 없습니다 (..)"})
		}
		switch {
		case !path.IsAbs(file.Dest):
			errs = append(errs, Error{f + ".dest", fmt.Sprintf("'%s': 절대 경로여야 합니다", file.Dest)})
		case seen[file.Dest]:
			errs = append(errs, Error{f + ".dest", fmt.Sprintf("'%s' 에 두 파일을 둘 수 없습니다", file.Dest)})
		default:
			for _, owned := range cpOwnedPaths {
				if within(file.Dest, owned) {
					errs = append(errs, Error{f + ".dest", fmt.Sprintf("'%s' 는 kubeadm 이나 설치기가 관리하는 %s 안입니다", file.Dest, owned)})
				}
			}
		}
		seen[file.Dest] = true
		if !reFileMode.MatchString(file.Mode) {
			errs = append(errs, Error{f + ".mode", fmt.Sprintf("'%s': 0644 같은 네 자리 8진수여야 합니다", file.Mode)})
		}
	}
	return errs
}

func within(p, dir string) bool {
	p = path.Clean(p)
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func contains(l []string, x string) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

// validateControlPlane checks what is not a variable of vars.yml: the URL of the
// webhook preset, which goes into the kubeconfig it names, and that the preset
// has the CA of setup-cert of aipub-installer to start with; the issuer of the
// JWKS preset, which goes into the authentication config, when AIPub is
// installed (without it the preset is not applied).
func validateControlPlane(s *state.AppState) []Error {
	var errs []Error
	if s.ControlPlane.Webhook && !s.AipubEnabled {
		errs = append(errs, Error{"control_plane.webhook",
			"AIPub 인증 웹훅은 AIPub 을 설치할 때만 켤 수 있습니다 (CA 를 setup-cert 가 만듭니다)"})
	}
	errs = append(errs, checkWebhookURL(&s.ControlPlane)...)
	if s.AppliedControlPlane().JWKS {
		errs = append(errs, checkIssuerURL(s.JWKSIssuer(), true)...)
	}
	return errs
}

// ControlPlane checks what the control plane screen edits on its own: the eight
// variables and the webhook URL, for the screen to stop on before the preview.
func ControlPlane(cp state.ControlPlane) []Error {
	// as written with AIPub, which the screen does not know yet: the JWKS preset
	// counts, so that the indexes are those the preview gives
	s := &state.AppState{ControlPlane: cp, AipubEnabled: true}
	var errs []Error
	for _, v := range schema.ByFile(schema.FileK8s) {
		if v.Group != "control_plane" {
			continue
		}
		if val, ok := schema.Value(s, v); ok {
			errs = append(errs, checkVar(v, v.Key, val, false)...)
		}
	}
	errs = append(errs, checkWebhookURL(&cp)...)
	if cp.JWKS {
		// empty follows the zone, which is checked once AIPub is chosen
		errs = append(errs, checkIssuerURL(cp.JWKSIssuerURL, false)...)
	}
	return errs
}

// checkIssuerURL checks the issuer of the JWKS preset: the iss of the tokens of
// AIPub and where the apiserver finds its discovery document, so an https URL
// with no credential, query or fragment. required: empty is refused.
func checkIssuerURL(raw string, required bool) []Error {
	const field = "control_plane.jwks_issuer_url"
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if !required {
			return nil
		}
		return []Error{{field, "AIPub 인증 (JWKS) 을 켰으면 issuer URL 을 입력하거나 aipub_ingress_zone 을 정해주세요"}}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return []Error{{field, fmt.Sprintf("'%s': JWKS issuer 는 https://호스트[:포트][/경로] 형식이어야 합니다 (자격 증명·쿼리 없이)", raw)}}
	}
	return nil
}

func checkWebhookURL(cp *state.ControlPlane) []Error {
	if !cp.Webhook {
		return nil
	}
	var errs []Error
	const field = "control_plane.webhook_url"
	raw := strings.TrimSpace(cp.WebhookURL)
	if raw == "" {
		return append(errs, Error{field, "AIPub 인증 웹훅을 켰으면 토큰 검증 서버 URL 을 입력해주세요"})
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		errs = append(errs, Error{field, fmt.Sprintf("'%s': https://호스트[:포트]/경로 형식이어야 합니다 (자격 증명 없이)", raw)})
	}
	return errs
}

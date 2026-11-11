package validator

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
	msg := i18n.T("Enter a value")
	if _, isList := val.([]string); isList || v.Type == schema.TypeList {
		msg = i18n.T("Enter at least one")
	}
	if key, want, ok := v.Condition(); ok {
		msg = i18n.T("if %s=%s: %s", key, want, msg)
	}
	if v.Short != "" {
		msg += " (" + i18n.T(v.Short) + ")"
	}
	return msg
}

// formatMsg says what is wrong with a value of type t, or "" when it is right.
func formatMsg(t schema.Type, enum []string, val any) string {
	text := fmt.Sprint(val)
	switch t {
	case schema.TypeInt:
		if n, ok := toInt(val); !ok || n < 1 {
			return i18n.T("Must be an integer of 1 or more")
		}
	case schema.TypePort:
		if n, ok := toInt(val); !ok || n < 1 || n > 65535 {
			return i18n.T("The port must be in the range 1-65535")
		}
	case schema.TypeCIDR:
		if _, _, err := net.ParseCIDR(text); err != nil {
			return i18n.T("'%s' is not a valid CIDR (example: x.x.x.x/xx)", text)
		}
	case schema.TypeIP:
		if net.ParseIP(text) == nil {
			return i18n.T("'%s' is not a valid IPv4 address", text)
		}
	case schema.TypeFQDN:
		if !reFQDN.MatchString(text) {
			return i18n.T("'%s' is not a valid domain (example: example.com)", text)
		}
	case schema.TypeSubdomain:
		if !reSubdomain.MatchString(text) {
			return i18n.T("'%s' is not a valid subdomain", text)
		}
	case schema.TypeMinor:
		if !reMinor.MatchString(text) {
			return i18n.T("'%s' is not in the right format (example: 1.36)", text)
		}
	case schema.TypeDuration:
		if !reDuration.MatchString(text) {
			return i18n.T("'%s' is not in the right format (example: 26280h)", text)
		}
	case schema.TypeSize:
		if !reSize.MatchString(text) {
			return i18n.T("'%s' is not a valid storage size (example: 512Gi)", text)
		}
	case schema.TypeDays:
		if !reDays.MatchString(text) {
			return i18n.T("'%s' is not in the right format (in days, example: 180d)", text)
		}
	case schema.TypeEnum:
		for _, e := range enum {
			if text == e {
				return ""
			}
		}
		return i18n.T("Choose one of %s", strings.Join(enum, ", "))
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
			errs = append(errs, Error{prefix + ".name", i18n.T("Enter a name")})
		} else if seen[n.Name] {
			errs = append(errs, Error{prefix + ".name", i18n.T("Node name '%s' is duplicated", n.Name)})
		} else {
			seen[n.Name] = true
		}
	}
	if len(s.Nodes) == 0 {
		errs = append(errs, Error{"nodes", i18n.T("Add at least one node")})
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
			errs = append(errs, Error{"ki_cp_node", i18n.T("'%s' is not in the node list", name)})
		}
	}
	for _, name := range s.K8sNodes {
		if !nodeNames[name] {
			errs = append(errs, Error{"k8s_node", i18n.T("'%s' is not in the node list", name)})
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
				i18n.T("VIP %s is outside the internal subnets", s.KiCpHaModeVIP)})
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
		errs = append(errs, Error{"k8s_service_subnet", i18n.T("The pod subnet and the service subnet overlap")})
	}
	for _, cidr := range s.InternalNetworkSubnets {
		_, in, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		for name, n := range parsed {
			if overlaps(in, n) {
				errs = append(errs, Error{name, i18n.T("Overlaps the internal subnet %s", cidr)})
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
		errs = append(errs, Error{"k8s_ingress_mode", i18n.T("The ingress mode must be 1 or 2")})
	}

	if s.K8sIngressLBName == "" {
		errs = append(errs, Error{"k8s_ingress_lb_name", i18n.T("Enter the ingress LB name")})
	}

	if s.K8sIngressVIP == "" {
		errs = append(errs, Error{"k8s_ingress_vip", i18n.T("Enter the ingress VIP")})
	} else if net.ParseIP(s.K8sIngressVIP) == nil {
		errs = append(errs, Error{"k8s_ingress_vip", i18n.T("Enter a valid IPv4 address")})
	}

	if s.K8sIngressMode == 1 {
		if s.K8sGatewayLBName == "" {
			errs = append(errs, Error{"k8s_gateway_lb_name", i18n.T("Enter the gateway LB name in separate IP mode")})
		} else if s.K8sGatewayLBName == s.K8sIngressLBName {
			errs = append(errs, Error{"k8s_gateway_lb_name", i18n.T("In separate IP mode the ingress LB name and the gateway LB name must differ")})
		}
		if s.K8sGatewayVIP == "" {
			errs = append(errs, Error{"k8s_gateway_vip", i18n.T("Enter the gateway VIP in separate IP mode")})
		} else if net.ParseIP(s.K8sGatewayVIP) == nil {
			errs = append(errs, Error{"k8s_gateway_vip", i18n.T("Enter a valid IPv4 address")})
		} else if s.K8sGatewayVIP == s.K8sIngressVIP {
			errs = append(errs, Error{"k8s_gateway_vip", i18n.T("In separate IP mode the ingress VIP and the gateway VIP must differ")})
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
		errs = append(errs, Error{"k8s_ingress_host_network_port", i18n.T("The port must be in the range 1-65535")})
	}

	// aipub_load_balancers: the k8s control plane nodes answer ARP for the VIPs
	if len(s.K8sCpNodes) == 0 {
		errs = append(errs, Error{"aipub_load_balancers", i18n.T("No control plane node (k8s_cp) to take the VIP")})
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
			out = append(out, i18n.T("%s: VIP %s is outside the internal network range (%s) - check that this is intended",
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
			errs = append(errs, Error{field, i18n.T("VIP %s is the same as the ki cp HA VIP (ki_cp_ha_mode_vip). Change one of them to another IP", vip)})
		}
	}
	for _, n := range s.Nodes {
		if ip := net.ParseIP(n.AnsibleHost); ip != nil && ip.Equal(vip) {
			errs = append(errs, Error{field, i18n.T("VIP %s is the same as ansible_host of node %s. Change it to an IP no node uses", vip, n.Name)})
		}
	}
	for _, sub := range []struct{ name, cidr string }{{"k8s_pod_subnet", s.K8sPodSubnet}, {"k8s_service_subnet", s.K8sServiceSubnet}} {
		if _, n, err := net.ParseCIDR(sub.cidr); err == nil && n.Contains(vip) {
			errs = append(errs, Error{field, i18n.T("VIP %s is inside %s (%s). Change it to an IP of the internal subnet or change %s",
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
			i18n.T("'%s' is not an AIPub control plane node", s.AipubHarborLocalPVNode)})
	}

	// The aipub label is a variable of the nodes (k8s_node_labels), chosen on the
	// AIPub screen as a list of nodes
	if len(s.AipubCpNodes) == 0 {
		errs = append(errs, Error{"aipub_cp_nodes", i18n.T("Select at least one AIPub control plane node (Harbor runs on it)")})
	}

	// R6: aipub_cp_nodes ⊆ k8s_cp_node
	k8sCpNodeSet := map[string]bool{}
	for _, n := range s.K8sCpNodes {
		k8sCpNodeSet[n] = true
	}
	for _, n := range s.AipubCpNodes {
		if !k8sCpNodeSet[n] {
			errs = append(errs, Error{"aipub_cp_nodes",
				i18n.T("'%s' is not in the k8s control plane node group", n)})
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
				i18n.T("'%s' is not in the k8s control plane node group", n)})
		}
	}
	return errs
}

// Monitoring checks what the monitoring screen edits on its own: its variables
// and the nodes it picks, for the screen to stop on before the preview.
func Monitoring(s *state.AppState) []Error {
	var errs []Error
	for _, v := range schema.ByFile(schema.FileAipub) {
		if v.Screen != "5b.monitoring" {
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
			errs = append(errs, Error{f, i18n.T("'%s': write the argument name without the leading dashes (--)", a.Name)})
		case !reArgName.MatchString(a.Name):
			errs = append(errs, Error{f, i18n.T("'%s': the argument name has lowercase letters, digits and dashes only", a.Name)})
		case contains(v.Decided, a.Name):
			errs = append(errs, Error{f, i18n.T("'%s' is an argument the installer sets itself, so it cannot be added", a.Name)})
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
			errs = append(errs, Error{f, i18n.T("'%s': a volume name is a DNS-1123 label (lowercase letters, digits, dashes, at most 63 characters)", vol.Name)})
		case contains(kubeadmVolumeNames, vol.Name):
			errs = append(errs, Error{f, i18n.T("'%s' is a volume name kubeadm uses and cannot be used", vol.Name)})
		case seen[vol.Name]:
			errs = append(errs, Error{f, i18n.T("Volume name '%s' is duplicated", vol.Name)})
		}
		seen[vol.Name] = true
		for _, p := range []struct{ name, path string }{{"hostPath", vol.HostPath}, {"mountPath", vol.MountPath}} {
			if !path.IsAbs(p.path) {
				errs = append(errs, Error{f + "." + p.name, i18n.T("'%s': must be an absolute path", p.path)})
			}
		}
		if vol.PathType != "" && !contains(volumePathTypes, vol.PathType) {
			errs = append(errs, Error{f + ".pathType", i18n.T("Must be one of %s", strings.Join(volumePathTypes, ", "))})
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
			errs = append(errs, Error{f + ".src", i18n.T("Must be a relative path under ansible/files")})
		case contains(strings.Split(file.Src, "/"), ".."):
			errs = append(errs, Error{f + ".src", i18n.T("Cannot point outside ansible/files (..)")})
		}
		switch {
		case !path.IsAbs(file.Dest):
			errs = append(errs, Error{f + ".dest", i18n.T("'%s': must be an absolute path", file.Dest)})
		case seen[file.Dest]:
			errs = append(errs, Error{f + ".dest", i18n.T("Two files cannot go to '%s'", file.Dest)})
		default:
			for _, owned := range cpOwnedPaths {
				if within(file.Dest, owned) {
					errs = append(errs, Error{f + ".dest", i18n.T("'%s' is inside %s, which kubeadm or the installer manages", file.Dest, owned)})
				}
			}
		}
		seen[file.Dest] = true
		if !reFileMode.MatchString(file.Mode) {
			errs = append(errs, Error{f + ".mode", i18n.T("'%s': must be a four-digit octal number like 0644", file.Mode)})
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
			i18n.T("The AIPub authentication webhook can be turned on only when AIPub is installed (setup-cert makes the CA)")})
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
		return []Error{{field, i18n.T("With AIPub authentication (JWKS) on, enter the issuer URL or set aipub_ingress_zone")}}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return []Error{{field, i18n.T("'%s': the JWKS issuer must be https://host[:port][/path] (no credentials or query)", raw)}}
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
		return append(errs, Error{field, i18n.T("With the AIPub authentication webhook on, enter the token review server URL")})
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		errs = append(errs, Error{field, i18n.T("'%s': must be https://host[:port]/path (no credentials)", raw)})
	}
	return errs
}

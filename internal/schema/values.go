package schema

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// AipubCpNodeLabel is the node label base and the services of aipub run on
// (aipub_cp_node_label_key), set in inventory.yml.
const AipubCpNodeLabel = "node-role.aipub.ten1010.io/control-plane"

// HostValue is the value of a Host variable on one node. Value is nil when the
// node does not set it.
type HostValue struct {
	Host  string
	Value any
}

// LoadBalancer is an entry of aipub_load_balancers. VIP is nil while unset.
type LoadBalancer struct {
	Name  string   `yaml:"name"`
	VIP   any      `yaml:"vip"`
	Nodes []string `yaml:"nodes,flow"`
}

// IngressClass is aipub_default_ingress_class.
type IngressClass struct {
	LoadBalancer    string `yaml:"load_balancer"`
	HostNetworkPort int    `yaml:"host_network_port"`
}

// ARecord is an entry of internal_network_extra_zone_a_records.
type ARecord struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
}

// Written reports whether file f is written for the state: files/aipub/vars.yml
// only when AIPub is installed.
func Written(s *state.AppState, f File) bool {
	return f != FileAipub || s.AipubEnabled
}

// Value returns the value the state gives v, in the form the writer writes it,
// and false when the state does not manage v (the site file keeps what it has,
// or the writer leaves the key out). The value of a Host variable is a
// []HostValue in the order of the nodes.
func Value(s *state.AppState, v Var) (any, bool) {
	switch v.File {
	case FileInventory:
		return inventoryValue(s, v.Key)
	case FileK8s:
		return k8sValue(s, v.Key)
	case FileAipub:
		val, ok := aipubValue(s, v.Key)
		if ok && v.Optional && atDefault(v, val) {
			return nil, false
		}
		return val, ok
	}
	return nil, false
}

// atDefault reports whether the value of an Optional variable is its default,
// which the writer leaves out of the file.
func atDefault(v Var, val any) bool {
	if str, ok := val.(string); ok && v.Type == TypeString && str == "" {
		return true
	}
	if v.Default == "" && Missing(val) {
		return true
	}
	return reflect.DeepEqual(Normalize(v, val), Normalize(v, v.DefaultValue()))
}

func inventoryValue(s *state.AppState, key string) (any, bool) {
	perNode := func(f func(n state.NodeConfig) any) []HostValue {
		out := make([]HostValue, len(s.Nodes))
		for i, n := range s.Nodes {
			out[i] = HostValue{n.Name, f(n)}
		}
		return out
	}
	perK8sNode := func(f func(name string) any) []HostValue {
		out := make([]HostValue, len(s.K8sNodes))
		for i, n := range s.K8sNodes {
			out[i] = HostValue{n, f(n)}
		}
		return out
	}
	switch key {
	case "ansible_host":
		return perNode(func(n state.NodeConfig) any { return n.AnsibleHost }), true
	case "ansible_port":
		return perNode(func(n state.NodeConfig) any { return nilIfEmpty(n.AnsiblePort) }), true
	case "ansible_user":
		return perNode(func(n state.NodeConfig) any { return nilIfEmpty(n.AnsibleUser) }), true
	case "k8s_cp":
		return perK8sNode(func(n string) any { return s.IsK8sCpNode(n) }), true
	case "k8s_node_labels":
		return perK8sNode(func(n string) any {
			if s.AipubEnabled && s.IsAipubCpNode(n) {
				return map[string]string{AipubCpNodeLabel: ""}
			}
			return nil
		}), true
	case "ki_cp_node":
		return append([]string{}, s.KiCpNodes...), true
	case "k8s_node":
		return append([]string{}, s.K8sNodes...), true
	// the localhost entry of control_node
	case "control_node_ih":
		if len(s.KiCpNodes) > 0 {
			return []HostValue{{"localhost", s.KiCpNodes[0]}}, true
		}
	case "ansible_connection":
		return []HostValue{{"localhost", "local"}}, true
	}
	return nil, false
}

func k8sValue(s *state.AppState, key string) (any, bool) {
	switch key {
	case "internal_network_subnets":
		return append([]string{}, s.InternalNetworkSubnets...), true
	case "internal_network_extra_zone":
		if s.InternalDNSServerEnabled {
			return s.AipubIngressZone, true
		}
	case "internal_network_extra_zone_a_records":
		if s.InternalDNSServerEnabled {
			return []ARecord{{"@", s.K8sIngressVIP}, {"*", s.K8sIngressVIP}}, true
		}
	case "ki_cp_ha_mode":
		return s.KiCpHaMode, true
	case "ki_cp_ha_mode_vip":
		return nilIfEmpty(s.KiCpHaModeVIP), true
	case "ki_cp_dns_dnssec_validation":
		return s.KiCpDnsDnssecValidation, true
	case "ki_cp_dns_server_upstream_servers":
		return append([]string{}, s.KiCpDnsUpstreamServers...), true
	case "ki_cp_ntp_server_upstream_servers":
		return append([]string{}, s.KiCpNtpUpstreamServers...), true
	case "k8s_pod_subnet":
		return s.K8sPodSubnet, true
	case "k8s_service_subnet":
		return s.K8sServiceSubnet, true
	case "k8s_kube_proxy_enabled":
		return s.KubeProxyEnabled(), true
	case "k8s_minor_version":
		return s.K8sMinorVersion, true
	case "k8s_certificate_validity_period":
		return s.K8sCertificateValidityPeriod, true
	case state.CpExtraFilesVar:
		return s.AppliedControlPlane().ExtraFiles(state.StageAll), true
	}
	for _, comp := range state.CpComponents {
		if key == state.ArgsVar(comp) {
			return s.AppliedControlPlane().ExtraArgs(comp, state.StageAll), true
		}
	}
	for _, comp := range state.CpVolumeComponents {
		if key == state.VolumesVar(comp) {
			return s.AppliedControlPlane().ExtraVolumes(comp, state.StageAll), true
		}
	}
	return nil, false
}

func aipubValue(s *state.AppState, key string) (any, bool) {
	switch key {
	case "aipub_load_balancers":
		nodes := append([]string{}, s.K8sCpNodes...)
		lbs := []LoadBalancer{{s.K8sIngressLBName, nilIfEmpty(s.K8sIngressVIP), nodes}}
		if s.K8sIngressMode == 1 {
			lbs = append(lbs, LoadBalancer{s.K8sGatewayLBName, nilIfEmpty(s.K8sGatewayVIP), nodes})
		}
		return lbs, true
	case "aipub_default_ingress_class":
		return IngressClass{s.K8sIngressLBName, s.K8sIngressHostNetworkPort}, true
	case "aipub_cert_mode":
		return s.AipubCertMode, true
	case "aipub_ingress_zone":
		return s.AipubIngressZone, true
	case "aipub_harbor_ingress_subdomain":
		return s.HarborIngressSubdomain, true
	case "aipub_ha_mode":
		return s.AipubHaMode, true
	case "aipub_ha_mode_storage_class":
		if s.AipubHaMode {
			return s.AipubHaModeStorageClass, true
		}
	case "aipub_harbor_local_pv_node":
		if !s.AipubHaMode {
			return s.AipubHarborLocalPVNode, true
		}
	case "aipub_harbor_replica_count":
		return s.HarborReplicaCount, true
	case "aipub_harbor_registry_storage_size":
		return s.HarborRegistryStorageSize, true
	case "aipub_harbor_postgresql_storage_size":
		return s.HarborPostgresqlStorageSize, true
	case "aipub_harbor_redis_storage_size":
		return s.HarborRedisStorageSize, true
	case "aipub_harbor_trivy_storage_size":
		return s.HarborTrivyStorageSize, true

	// Optional: Value leaves them out at their default
	case "aipub_app_edition":
		return s.AipubAppEdition, true
	case "aipub_app_deploy_tool":
		return s.AipubAppDeployTool, true
	case "aipub_app_helmfile_bin":
		return s.AipubAppHelmfileBin, true
	case "aipub_monitoring_deploy_tool":
		return s.AipubMonitoringDeployTool, true
	case "aipub_monitoring_helmfile_bin":
		return s.AipubMonitoringHelmfileBin, true
	case "aipub_monitoring_kustomize_bin":
		return s.AipubMonitoringKustomizeBin, true
	case "aipub_monitoring_helm_bin":
		return s.AipubMonitoringHelmBin, true
	case "aipub_monitoring_helmfile_dry_run":
		return s.AipubMonitoringHelmfileDryRun, true
	}
	return nil, false
}

func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// RequiredIn reports whether v is required for the state: Always, or its
// condition holding.
func (v Var) RequiredIn(s *state.AppState) bool {
	if v.Required == Always {
		return true
	}
	key, want, ok := v.Condition()
	if !ok {
		return false
	}
	cv, found := GetIn(v.File, key)
	if !found {
		if cv, found = Get(key); !found {
			return false
		}
	}
	val, managed := Value(s, cv)
	return managed && fmt.Sprint(Normalize(cv, val)) == want
}

// Missing reports whether a value counts as not given: null, an empty string, an
// empty list or mapping.
func Missing(val any) bool {
	if val == nil {
		return true
	}
	if str, ok := val.(string); ok {
		return str == ""
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map:
		return rv.Len() == 0
	}
	return false
}

// Normalize turns a value into what yaml decodes it as (string, int, bool, nil,
// []any, map[string]any), so that a value of the state and a value read from a
// file compare equal when they are written the same. A port or a count typed
// as text becomes a number.
func Normalize(v Var, val any) any {
	if str, ok := val.(string); ok && (v.Type == TypePort || v.Type == TypeInt) {
		if n, err := strconv.Atoi(str); err == nil {
			val = n
		}
	}
	if hv, ok := val.([]HostValue); ok {
		m := map[string]any{}
		for _, h := range hv {
			m[h.Host] = Normalize(v, h.Value)
		}
		return m
	}
	data, err := yaml.Marshal(val)
	if err != nil {
		return val
	}
	var out any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return val
	}
	return out
}

// DefaultValue is Default decoded: text for the types that hold text, yaml for
// the others, nil when Default is empty.
func (v Var) DefaultValue() any {
	if v.Default == "" {
		return nil
	}
	switch v.Type {
	case TypeInt, TypePort, TypeBool, TypeList, TypeMap:
		var out any
		if err := yaml.Unmarshal([]byte(v.Default), &out); err == nil {
			return out
		}
	}
	return v.Default
}

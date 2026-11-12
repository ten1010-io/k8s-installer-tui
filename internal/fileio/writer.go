package fileio

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// The files of the installers this tool writes for, taken from them (see
// templates/README.md). A file the site does not have yet starts from these.
var (
	//go:embed templates/k8s-vars.yml
	k8sVarsTemplate []byte
	//go:embed templates/aipub-vars.yml
	aipubVarsTemplate []byte
	//go:embed templates/authentication-config.yaml
	authConfigTemplate []byte
	//go:embed templates/audit-policy.yaml
	auditPolicyTemplate []byte
)

// The label base and the services of aipub run on (aipub_cp_node_label_key).
// It is a node label of k8s-installer, set in inventory.yml
const AipubCpNodeLabel = schema.AipubCpNodeLabel

// Variables older versions of this tool wrote into group_vars/all/vars.yml.
// k8s-installer refuses them: they moved to aipub-installer (files/aipub/vars.yml)
// or were removed
var legacyK8sVars = []string{
	"containerd_root_path",
	"k8s_load_balancers",
	"k8s_gateways",
	"k8s_default_ingress_class",
	"ki_cert_mode",
}

// Paths are the three files the tool reads and writes.
type Paths struct {
	Inventory string // inventory.yml
	Vars      string // group_vars/all/vars.yml
	AipubVars string // files/aipub/vars.yml
}

// FilesDir is the files directory of the installer next to the saved files
// (ansible/files), where k8s_cp_extra_files reads src from.
func (p Paths) FilesDir() string { return filepath.Dir(filepath.Dir(p.AipubVars)) }

// WebhookConfig is where the kubeconfig of the webhook preset is written:
// files/aipub/webhook-token-auth.yaml.
func (p Paths) WebhookConfig() string {
	return filepath.Join(p.FilesDir(), filepath.FromSlash(state.WebhookConfigFile))
}

// AuthConfig is where the Structured Authentication Config of the JWKS preset is
// written: files/aipub/authentication-config.yaml.
func (p Paths) AuthConfig() string {
	return filepath.Join(p.FilesDir(), filepath.FromSlash(state.JWKSConfigFile))
}

// AuditPolicy is where the audit policy of the audit preset is written:
// files/aipub/audit-policy.yaml.
func (p Paths) AuditPolicy() string {
	return filepath.Join(p.FilesDir(), filepath.FromSlash(state.AuditPolicyFile))
}

// DefaultPaths are the places k8s-installer reads the files from, relative to
// its ansible directory (/opt/k8s-installer/ansible).
func DefaultPaths(ansibleDir string) Paths {
	return Paths{
		Inventory: filepath.Join(ansibleDir, "inventory.yml"),
		Vars:      filepath.Join(ansibleDir, "group_vars", "all", "vars.yml"),
		AipubVars: filepath.Join(ansibleDir, "files", "aipub", "vars.yml"),
	}
}

// Rendered is the content of the three files. AipubVars is nil when AIPub is not
// installed, and the file is then left alone. WebhookConfig is the kubeconfig of
// the webhook preset of the control plane and AuthConfig the authentication
// config of the JWKS preset, each nil while it is off (a file written before is
// left alone); AuthConfig is nil as well while there is no CA (CA says why).
// AuditPolicy is the default audit policy when it is written (Audit.Source is
// AuditDefault), nil otherwise; Audit says which policy is used.
type Rendered struct {
	Inventory     []byte
	Vars          []byte
	AipubVars     []byte
	WebhookConfig []byte
	AuthConfig    []byte
	AuditPolicy   []byte
	Audit         AuditPolicyPlan
	CA            CA
}

// JWKSPending reports whether the JWKS preset is applied and its config waits
// for the CA: nothing is written for it until the CA is there.
func (r *Rendered) JWKSPending(s *state.AppState) bool {
	return s.AppliedControlPlane().JWKS && s.JWKSIssuer() != "" && r.AuthConfig == nil
}

// Render renders the three files over what is at p, or over the templates.
func Render(p Paths, s *state.AppState) (*Rendered, error) {
	var r Rendered
	var err error
	if r.Inventory, err = renderInventory(readOrNil(p.Inventory), s); err != nil {
		return nil, fmt.Errorf("inventory.yml: %w", err)
	}
	if r.Vars, err = renderVars(readOrNil(p.Vars), s); err != nil {
		return nil, fmt.Errorf("vars.yml: %w", err)
	}
	if s.AipubEnabled {
		if r.AipubVars, err = renderAipubVars(readOrNil(p.AipubVars), s); err != nil {
			return nil, fmt.Errorf("files/aipub/vars.yml: %w", err)
		}
	}
	r.WebhookConfig = WebhookConfig(s)
	r.CA = LookupCA(s)
	r.AuthConfig = AuthConfig(s, r.CA.PEM)
	r.Audit = PlanAuditPolicy(p.AuditPolicy(), s)
	if r.Audit.Source == AuditDefault {
		r.AuditPolicy = r.Audit.Data
	}
	return &r, nil
}

// WriteAll renders and writes the files, each whole by rename, and appends them
// to history.jsonl beside inventory.yml, after what was there when the history
// does not have it (history.go). A file marked keepIfExists is written only
// where nothing is (not even a dangling symlink): the audit policy of a site is
// never written over.
func WriteAll(p Paths, s *state.AppState) error {
	r, err := Render(p, s)
	if err != nil {
		return err
	}
	dir := filepath.Dir(p.Inventory)
	h, err := ReadHistory(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", HistoryFile, err)
	}
	files := []struct {
		path         string
		data         []byte
		keepIfExists bool
	}{
		{p.Inventory, r.Inventory, false},
		{p.Vars, r.Vars, false},
		{p.AipubVars, r.AipubVars, false},
		{p.WebhookConfig(), r.WebhookConfig, false},
		{p.AuthConfig(), r.AuthConfig, false},
		{p.AuditPolicy(), r.AuditPolicy, true},
	}
	for _, f := range files {
		if f.data == nil {
			continue
		}
		if _, err := os.Lstat(f.path); f.keepIfExists && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if h, err = writeWithHistory(dir, f.path, f.data, h); err != nil {
			return err
		}
	}
	return nil
}

func readOrNil(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// --- group_vars/all/vars.yml ---

// renderVars edits the vars.yml of the site, or the template of k8s-installer.
// A file written by an older version of this tool (no k8s_minor_version) lacks
// most of what k8s-installer requires now, so it is replaced by the template and
// only the values this tool manages are carried over through the state.
func renderVars(existing []byte, s *state.AppState) ([]byte, error) {
	tpl, err := parseDoc(k8sVarsTemplate)
	if err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}
	top := tpl
	if existing != nil {
		cur, err := parseDoc(existing)
		if err != nil {
			return nil, err
		}
		if getKey(cur, "k8s_minor_version") != nil {
			top = cur
			addMissingKeys(top, tpl)
		}
	}

	for _, k := range legacyK8sVars {
		deleteKey(top, k)
	}
	for _, k := range keys(top) {
		if strings.HasPrefix(k, "aipub_") {
			deleteKey(top, k)
		}
	}

	setKey(top, "internal_network_subnets", quotedSeq(s.InternalNetworkSubnets))
	if s.InternalDNSServerEnabled {
		// The domain of AIPub resolved by the dns server of the ki cp nodes: the
		// apex and every name under it at the ingress VIP
		setKey(top, "internal_network_extra_zone", quoted(s.AipubIngressZone))
		setKey(top, "internal_network_extra_zone_a_records", seqNode(
			mapNode("name", quoted("@"), "ip", quoted(s.K8sIngressVIP)),
			mapNode("name", quoted("*"), "ip", quoted(s.K8sIngressVIP)),
		))
	} else {
		deleteKey(top, "internal_network_extra_zone")
		deleteKey(top, "internal_network_extra_zone_a_records")
	}

	setKey(top, "ki_cp_ha_mode", boolNode(s.KiCpHaMode))
	setKey(top, "ki_cp_ha_mode_vip", quotedOrNull(s.KiCpHaModeVIP))
	setKey(top, "ki_cp_dns_dnssec_validation", boolNode(s.KiCpDnsDnssecValidation))
	setKey(top, "ki_cp_dns_server_upstream_servers", quotedSeq(s.KiCpDnsUpstreamServers))
	setKey(top, "ki_cp_ntp_server_upstream_servers", quotedSeq(s.KiCpNtpUpstreamServers))

	setKey(top, "k8s_pod_subnet", quoted(s.K8sPodSubnet))
	setKey(top, "k8s_service_subnet", quoted(s.K8sServiceSubnet))
	setKey(top, "k8s_kube_proxy_enabled", boolNode(s.KubeProxyEnabled()))
	setKey(top, "k8s_minor_version", quoted(s.K8sMinorVersion))
	setKey(top, "k8s_certificate_validity_period", quoted(s.K8sCertificateValidityPeriod))

	// The control plane: presets and entries typed in, merged. An empty list is
	// written as the template writes it, [], rather than the key taken out: the
	// key carries the comment of the template above it, and addMissingKeys would
	// put it back on the next save anyway
	cp := s.AppliedControlPlane()
	for _, comp := range state.CpComponents {
		setKey(top, state.ArgsVar(comp), extraArgsNode(cp.ExtraArgs(comp, state.StageAll)))
	}
	for _, comp := range state.CpVolumeComponents {
		setKey(top, state.VolumesVar(comp), extraVolumesNode(cp.ExtraVolumes(comp, state.StageAll)))
	}
	setKey(top, state.CpExtraFilesVar, cpExtraFilesNode(cp.ExtraFiles(state.StageAll)))

	return encodeDoc(top)
}

func extraArgsNode(args []state.ExtraArg) *yaml.Node {
	items := make([]*yaml.Node, len(args))
	for i, a := range args {
		items[i] = mapNode("name", quoted(a.Name), "value", quoted(a.Value))
	}
	return listOrEmpty(items)
}

func extraVolumesNode(vols []state.ExtraVolume) *yaml.Node {
	items := make([]*yaml.Node, len(vols))
	for i, v := range vols {
		m := mapNode("name", quoted(v.Name), "hostPath", quoted(v.HostPath), "mountPath", quoted(v.MountPath))
		if v.ReadOnly {
			m.Content = append(m.Content, strKey("readOnly"), boolNode(true))
		}
		if v.PathType != "" {
			m.Content = append(m.Content, strKey("pathType"), quoted(v.PathType))
		}
		items[i] = m
	}
	return listOrEmpty(items)
}

func cpExtraFilesNode(files []state.CpExtraFile) *yaml.Node {
	items := make([]*yaml.Node, len(files))
	for i, f := range files {
		items[i] = mapNode("src", quoted(f.Src), "dest", quoted(f.Dest), "mode", quoted(f.Mode))
	}
	return listOrEmpty(items)
}

// listOrEmpty is a block sequence, or [] when there is nothing in it.
func listOrEmpty(items []*yaml.Node) *yaml.Node {
	n := seqNode(items...)
	if len(items) == 0 {
		n.Style = yaml.FlowStyle
	}
	return n
}

// --- the authentication config of the JWKS preset (files/aipub/authentication-config.yaml) ---

// The two lines of the template of aipub-installer k8s-jwks fills in. The marker
// of the CA is only ever replaced: what the wizard writes never holds it.
const (
	authConfigIssuerLine = "      url: https://REPLACE_ME\n"
	authConfigCALine     = "        REPLACE_ME_CA\n"
)

// AuthConfig is the Structured Authentication Config the apiserver checks the
// tokens of AIPub with (--authentication-config), nil when the JWKS preset is not
// applied, has no issuer, or there is no CA yet. It is the template k8s-jwks of
// aipub-installer fills (templates/authentication-config.yaml), filled in the
// same two places: the issuer URL, and the CA setup-cert puts on every node
// (LookupCA), inline because the schema takes no path.
//
// Without the CA nothing is written, never a file with a placeholder in it:
// k8s-installer copies k8s_cp_extra_files to every control plane node on any
// update-cluster.yml and on add-node.yml (tasks/apply-k8s-cp-extra-files.yml), and
// an apiserver given a config it can not read does not start. With the file not
// there, validate-hostvars.py refuses the run before it touches a node ("File ...
// of the control node is not there"); the plan fills the file (--fill-auth-config)
// after setup-cert, before it applies the preset. Nothing in it is secret: it is
// copied to the control plane nodes in the clear.
func AuthConfig(s *state.AppState, ca string) []byte {
	cp := s.AppliedControlPlane()
	issuer := s.JWKSIssuer()
	ca = strings.TrimSpace(ca)
	if !cp.JWKS || issuer == "" || ca == "" {
		return nil
	}
	t := string(authConfigTemplate)
	if !strings.Contains(t, authConfigIssuerLine) || !strings.Contains(t, authConfigCALine) {
		return nil // the template moved on; TestAuthConfig says so
	}
	issuerNode, err := yaml.Marshal(issuer)
	if err != nil {
		return nil
	}
	t = strings.Replace(t, authConfigIssuerLine, "      url: "+string(issuerNode), 1)
	var caLines strings.Builder
	for _, l := range strings.Split(ca, "\n") {
		caLines.WriteString("        " + strings.TrimRight(l, " \r") + "\n")
	}
	t = strings.Replace(t, authConfigCALine, caLines.String(), 1)
	header := "# Written by k8s-installer-tui (AIPub authentication (JWKS)) from the template of aipub-installer, filled in\n" +
		"# as k8s-jwks fills it. Copied to every control plane node in the clear by k8s_cp_extra_files:\n" +
		"# the issuer URL and the public CA only, never a credential. The CA is the one setup-cert put at\n" +
		"# " + state.JWKSCAPath + "; after it is replaced, run k8s-installer-tui --fill-auth-config again.\n"
	return []byte(header + t)
}

// --- the kubeconfig of the webhook preset (files/aipub/webhook-token-auth.yaml) ---

// WebhookConfig is the kubeconfig the apiserver reads the token review server of
// AIPub from (--authentication-token-webhook-config-file), nil when the webhook
// preset is off or has no URL. It names the server and the CA setup-cert of
// aipub-installer puts on every node, and holds nothing secret: it is copied to
// the control plane nodes in the clear (k8s_cp_extra_files).
func WebhookConfig(s *state.AppState) []byte {
	cp := &s.ControlPlane
	if !cp.Webhook || strings.TrimSpace(cp.WebhookURL) == "" {
		return nil
	}
	const name = "aipub-token-review"
	doc := mapNode(
		"apiVersion", strKey("v1"),
		"kind", strKey("Config"),
		"clusters", seqNode(mapNode(
			"name", strKey(name),
			"cluster", mapNode(
				"server", quoted(strings.TrimSpace(cp.WebhookURL)),
				"certificate-authority", strKey(state.WebhookCAPath),
			),
		)),
		"users", seqNode(mapNode("name", strKey("kube-apiserver"), "user", emptyMap())),
		"contexts", seqNode(mapNode(
			"name", strKey(name),
			"context", mapNode("cluster", strKey(name), "user", strKey("kube-apiserver")),
		)),
		"current-context", strKey(name),
	)
	doc.HeadComment = "Written by k8s-installer-tui (AIPub authentication webhook). Copied to every control plane node\n" +
		"in the clear by k8s_cp_extra_files: never put a credential here"
	out, err := encodeDoc(doc)
	if err != nil {
		return nil
	}
	return out
}

// addMissingKeys appends to cur, with their comments, the keys of the template it
// does not have: variables a newer installer added since the file was written.
func addMissingKeys(cur, tpl *yaml.Node) {
	for i := 0; i+1 < len(tpl.Content); i += 2 {
		if keyIndex(cur, tpl.Content[i].Value) < 0 {
			cur.Content = append(cur.Content, tpl.Content[i], tpl.Content[i+1])
		}
	}
}

// --- files/aipub/vars.yml ---

// renderAipubVars edits the site values of aipub-installer, or its example file.
// Only aipub_* variables are written; load-site-vars.yml refuses anything else.
func renderAipubVars(existing []byte, s *state.AppState) ([]byte, error) {
	src := aipubVarsTemplate
	if existing != nil {
		src = existing
	}
	top, err := parseDoc(src)
	if err != nil {
		return nil, err
	}

	// Network: the ingress VIP, and in mode 1 a second VIP for the gateway of the
	// workloads. The nodes answering ARP for them are the k8s control plane nodes
	lbNodes := s.K8sCpNodes
	lbs := []*yaml.Node{lbEntry(s.K8sIngressLBName, s.K8sIngressVIP, lbNodes)}
	if s.K8sIngressMode == 1 {
		lbs = append(lbs, lbEntry(s.K8sGatewayLBName, s.K8sGatewayVIP, lbNodes))
	}
	setKey(top, "aipub_load_balancers", seqNode(lbs...))
	setKey(top, "aipub_default_ingress_class", mapNode(
		"load_balancer", quoted(s.K8sIngressLBName),
		"host_network_port", intNode(s.K8sIngressHostNetworkPort),
	))

	setKey(top, "aipub_cert_mode", quoted(s.AipubCertMode))

	setKey(top, "aipub_ingress_zone", quoted(s.AipubIngressZone))
	setKey(top, "aipub_harbor_ingress_subdomain", quoted(s.HarborIngressSubdomain))
	setKey(top, "aipub_ha_mode", boolNode(s.AipubHaMode))
	if s.AipubHaMode {
		setKey(top, "aipub_ha_mode_storage_class", quoted(s.AipubHaModeStorageClass))
		deleteKey(top, "aipub_harbor_local_pv_node")
	} else {
		deleteKey(top, "aipub_ha_mode_storage_class")
		setKey(top, "aipub_harbor_local_pv_node", quoted(s.AipubHarborLocalPVNode))
	}
	setKey(top, "aipub_harbor_replica_count", intNode(s.HarborReplicaCount))
	setKey(top, "aipub_harbor_registry_storage_size", quoted(s.HarborRegistryStorageSize))
	setKey(top, "aipub_harbor_postgresql_storage_size", quoted(s.HarborPostgresqlStorageSize))
	setKey(top, "aipub_harbor_redis_storage_size", quoted(s.HarborRedisStorageSize))
	setKey(top, "aipub_harbor_trivy_storage_size", quoted(s.HarborTrivyStorageSize))

	// python: the example of aipub-installer sets the cluster name, so it is a key
	// of every site file; the rest of the python section is optional
	setKey(top, "aipub_python_coaster_cluster_name", quoted(s.PythonCoasterClusterName))

	setOptionalAipub(top, s)

	return encodeDoc(top)
}

// setOptionalAipub writes the variables the schema marks Optional only while the
// state differs from their default: the installer defaults to the rest, and a
// file that does not repeat it follows a new default of the installer. A key
// that holds the default already is left as the site wrote it; one that holds
// something else is taken out when the state is back at the default.
func setOptionalAipub(top *yaml.Node, s *state.AppState) {
	for _, v := range schema.ByFile(schema.FileAipub) {
		if !v.Optional {
			continue
		}
		if val, managed := schema.Value(s, v); managed {
			setKey(top, v.Key, optionalNode(val))
			continue
		}
		if cur := getKey(top, v.Key); cur != nil && (cur.Kind != yaml.ScalarNode || cur.Value != v.Default) {
			deleteKey(top, v.Key)
		}
	}
}

// optionalNode is the node of a value of an Optional variable.
func optionalNode(val any) *yaml.Node {
	switch x := val.(type) {
	case bool:
		return boolNode(x)
	case int:
		return intNode(x)
	case []string:
		return quotedSeq(x)
	}
	return quoted(fmt.Sprint(val))
}

func lbEntry(name, vip string, nodes []string) *yaml.Node {
	return mapNode("name", quoted(name), "vip", quotedOrNull(vip), "nodes", flowSeq(nodes))
}

// --- inventory.yml ---

// renderInventory edits the inventory of the site, or a new one. Hosts and group
// members are added, updated and removed to match the state; any other variable
// of a host (kernel arguments, labels, taints set by hand) is left as it was.
func renderInventory(existing []byte, s *state.AppState) ([]byte, error) {
	top, err := parseDoc(existing)
	if err != nil {
		return nil, err
	}

	// control_node: the control node is the first ki cp node
	localhost := subMap(subMap(subMap(top, "control_node"), "hosts"), "localhost")
	setKey(localhost, "ansible_connection", strKey("local"))
	if len(s.KiCpNodes) > 0 {
		setKey(localhost, "control_node_ih", strKey(s.KiCpNodes[0]))
	}

	// all.hosts: address and login of every node
	allHosts := subMap(subMap(top, "all"), "hosts")
	syncHosts(allHosts, s.NodeNames())
	for _, n := range s.Nodes {
		h := hostMap(allHosts, n.Name)
		setKey(h, "ansible_host", strKey(n.AnsibleHost))
		if n.AnsiblePort != "" {
			p, _ := strconv.Atoi(n.AnsiblePort)
			setKey(h, "ansible_port", intNode(p))
		} else {
			deleteKey(h, "ansible_port")
		}
		deleteKey(h, "ansible_ssh_user") // older versions of this tool
		if n.AnsibleUser != "" {
			setKey(h, "ansible_user", strKey(n.AnsibleUser))
		} else {
			deleteKey(h, "ansible_user")
		}
	}

	kiCpHosts := subMap(subMap(top, "ki_cp_node"), "hosts")
	syncHosts(kiCpHosts, s.KiCpNodes)

	k8sHosts := subMap(subMap(top, "k8s_node"), "hosts")
	syncHosts(k8sHosts, s.K8sNodes)
	for _, name := range s.K8sNodes {
		h := hostMap(k8sHosts, name)
		if s.IsK8sCpNode(name) {
			setKey(h, "k8s_cp", boolNode(true))
		} else {
			deleteKey(h, "k8s_cp")
		}
		deleteKey(h, "nvidia_gpu") // removed from k8s-installer, which refuses it now

		// The nodes base and the services of aipub run on carry this label
		labels := getKey(h, "k8s_node_labels")
		wantLabel := s.AipubEnabled && s.IsAipubCpNode(name)
		if wantLabel {
			if labels == nil || labels.Kind != yaml.MappingNode {
				labels = mapNode()
				setKey(h, "k8s_node_labels", labels)
			}
			setKey(labels, AipubCpNodeLabel, quoted(""))
		} else if labels != nil {
			deleteKey(labels, AipubCpNodeLabel)
			if len(labels.Content) == 0 {
				deleteKey(h, "k8s_node_labels")
			}
		}
		if len(h.Content) == 0 {
			h.Style = yaml.FlowStyle
		}
	}

	// broken_node: kept as the site has it; created empty in a new inventory
	if getKey(top, "broken_node") == nil {
		setKey(top, "broken_node", mapNode("hosts", emptyMap()))
	}

	return encodeDoc(top)
}

// syncHosts makes the members of a hosts mapping the names given, in that order:
// a member that stays keeps its variables, a new one starts as {}.
func syncHosts(hosts *yaml.Node, names []string) {
	old := map[string]*yaml.Node{}
	for i := 0; i+1 < len(hosts.Content); i += 2 {
		old[hosts.Content[i].Value] = hosts.Content[i+1]
	}
	var content []*yaml.Node
	for _, n := range names {
		v, ok := old[n]
		if !ok || v.Kind != yaml.MappingNode {
			v = emptyMap()
		}
		content = append(content, strKey(n), v)
	}
	hosts.Content = content
}

// hostMap is the variables mapping of a member, written in block style once it
// has any.
func hostMap(hosts *yaml.Node, name string) *yaml.Node {
	h := getKey(hosts, name)
	h.Style = 0
	return h
}

// MissingCpFiles lists the files of k8s_cp_extra_files that are in none of the
// files directories given (ansible/files next to the saved settings, first, then
// of the installer) as a regular file that is not empty. The kubeconfig of the
// webhook preset is left out while the wizard writes it, the authentication
// config of the JWKS preset while it has an issuer: written now, or waiting for
// the CA, which the plan fills in (--fill-auth-config) before it applies the
// preset; the preview says which. The audit policy is left out when saving
// writes the default to the first directory (PlanAuditPolicy). k8s-installer
// stops on such a file before it touches a node; the preview warns sooner.
func MissingCpFiles(s *state.AppState, filesDirs ...string) []state.CpExtraFile {
	generated := map[string]bool{
		state.WebhookConfigFile: WebhookConfig(s) != nil,
		state.JWKSConfigFile:    s.AppliedControlPlane().JWKS && s.JWKSIssuer() != "",
	}
	if len(filesDirs) > 0 {
		audit := PlanAuditPolicy(filepath.Join(filesDirs[0], filepath.FromSlash(state.AuditPolicyFile)), s)
		generated[state.AuditPolicyFile] = audit.Source == AuditDefault
	}
	var out []state.CpExtraFile
	for _, f := range s.AppliedControlPlane().ExtraFiles(state.StageAll) {
		if generated[f.Src] {
			continue
		}
		found := false
		for _, dir := range filesDirs {
			if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.Src))); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
				found = true
				break
			}
		}
		if !found {
			out = append(out, f)
		}
	}
	return out
}

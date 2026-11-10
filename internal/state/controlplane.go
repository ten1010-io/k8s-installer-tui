package state

// The control plane screen (S4b): what k8s-installer adds to the static pods of
// the control plane through the ClusterConfiguration of kubeadm, written to
// group_vars/all/vars.yml as k8s_<component>_extra_args, k8s_<component>_extra_volumes
// and k8s_cp_extra_files. kubeadm keeps them and applies them again on join and
// upgrade, which a hand edit of /etc/kubernetes/manifests does not survive.
//
// The state holds the presets as switches and the entries typed in by hand as
// lists; the variables are the two merged (presets first). A preset is a fixed
// set of entries, so turning it off takes out exactly those, and a file loaded
// back is split into the presets it holds whole and the rest.

// ExtraArg is an entry of k8s_<component>_extra_args: a flag without its dashes.
type ExtraArg struct {
	Name  string `yaml:"name" json:"name"`
	Value string `yaml:"value" json:"value"`
}

// ExtraVolume is an entry of k8s_<component>_extra_volumes: a host path mounted
// into the static pod.
type ExtraVolume struct {
	Name      string `yaml:"name" json:"name"`
	HostPath  string `yaml:"hostPath" json:"hostPath"`
	MountPath string `yaml:"mountPath" json:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly,omitempty" json:"readOnly,omitempty"`
	PathType  string `yaml:"pathType,omitempty" json:"pathType,omitempty"`
}

// CpExtraFile is an entry of k8s_cp_extra_files: a file of ansible/files of the
// installer put on every control plane node. In the clear: never a secret.
type CpExtraFile struct {
	Src  string `yaml:"src" json:"src"`
	Dest string `yaml:"dest" json:"dest"`
	Mode string `yaml:"mode" json:"mode"`
}

// The components of the control plane, as their variables name them. etcd takes
// arguments and no volumes.
var CpComponents = []string{"apiserver", "controller_manager", "scheduler", "etcd"}

// CpVolumeComponents are the components that take volumes.
var CpVolumeComponents = []string{"apiserver", "controller_manager", "scheduler"}

// ArgsVar and VolumesVar are the variables of vars.yml of a component.
func ArgsVar(component string) string    { return "k8s_" + component + "_extra_args" }
func VolumesVar(component string) string { return "k8s_" + component + "_extra_volumes" }

// CpExtraFilesVar is the variable of the files placed on the control plane nodes.
const CpExtraFilesVar = "k8s_cp_extra_files"

// The presets. Every one of them is of the apiserver.
const (
	PresetAudit         = "audit"
	PresetRuntimeConfig = "runtime_config"
	PresetWebhook       = "webhook"
	PresetJWKS          = "jwks"
)

// The steps of an installation a preset is applied by. An entry that reads what
// a later step makes is left out of the cluster as it is first built and applied
// by an update-cluster.yml run after that step.
const (
	// StageCluster is setup-cluster (K3): the cluster as kubeadm first builds it
	StageCluster = iota
	// StageAfterCert is after setup-cert of aipub-installer (A2), which puts the
	// CA on every node
	StageAfterCert
	// StageAfterAipub is after setup-aipub (A5): the issuer of AIPub answers
	StageAfterAipub
	// StageAll is every entry, as vars.yml holds them
	StageAll = StageAfterAipub
)

// AuditPolicyFile is the audit policy the audit preset points the apiserver at.
const AuditPolicyFile = "aipub/audit-policy.yaml"

// JWKSConfigFile is the Structured Authentication Config the JWKS preset points
// the apiserver at. The wizard writes it under ansible/files from the issuer URL,
// after the template of aipub-installer
// (scripts/aipub/installer/manifests/authentication-config.yaml).
const JWKSConfigFile = "aipub/authentication-config.yaml"

// JWKSConfigPath is where the file is put on the control plane nodes, the path
// k8s-jwks of aipub-installer uses.
const JWKSConfigPath = "/etc/kubernetes/authentication-config.yaml"

// JWKSCAPath is the CA the issuer is verified with: setup-cert of aipub-installer
// puts it at {{ aipub_cert_path }}/ca.crt on the nodes, the control node among
// them, as a file only (not in the trust store of the node). The config takes
// the CA inline (its schema has no path option), so the wizard reads it on the
// control node it runs on and copies it into the file rather than mounting it.
const JWKSCAPath = "/etc/k8s-installer/certs/ca.crt"

// JWKSConfigMode is the mode of the authentication config on the nodes: the one
// k8s-jwks of aipub-installer gives it (chmod 600), so that taking the file over
// from a site set up by k8s-jwks changes nothing on the node. It holds no secret.
const JWKSConfigMode = "0600"

// WebhookConfigFile is the kubeconfig the webhook preset points the apiserver at.
// The wizard writes it under ansible/files from the URL given on the screen.
const WebhookConfigFile = "aipub/webhook-token-auth.yaml"

// WebhookCAPath is the CA the kubeconfig verifies the token review server with,
// put on the nodes by setup-cert of aipub-installer: after the cluster exists.
const WebhookCAPath = "/etc/k8s-installer/certs/ca.crt"

// Preset is a fixed set of entries of the apiserver and the files they read.
type Preset struct {
	ID       string
	Title    string
	Args     []ExtraArg    // of k8s_apiserver_extra_args
	Volumes  []ExtraVolume // of k8s_apiserver_extra_volumes
	Files    []CpExtraFile // of k8s_cp_extra_files
	Provider string        // who provides the files under ansible/files, when the site does
	// Stage is the step it is applied by (StageCluster: built with the cluster).
	// The files a later one reads are made by a step after setup-cluster
	Stage int
}

var Presets = []Preset{
	{
		ID:    PresetAudit,
		Title: "AIPub 감사 로그",
		Args: []ExtraArg{
			{"audit-policy-file", "/etc/kubernetes/audit-policy.yaml"},
			{"audit-log-path", "/var/log/aipub/usage/audit.log"},
			{"audit-log-maxage", "30"},
			{"audit-log-maxbackup", "30"},
			{"audit-log-maxsize", "100"},
		},
		Volumes: []ExtraVolume{
			{Name: "audit-policy", HostPath: "/etc/kubernetes/audit-policy.yaml", MountPath: "/etc/kubernetes/audit-policy.yaml", ReadOnly: true, PathType: "File"},
			{Name: "audit-log", HostPath: "/var/log/aipub/usage", MountPath: "/var/log/aipub/usage", PathType: "DirectoryOrCreate"},
		},
		Files: []CpExtraFile{
			{Src: AuditPolicyFile, Dest: "/etc/kubernetes/audit-policy.yaml", Mode: "0644"},
		},
		Provider: "AIPub 모니터링/python 팀",
	},
	{
		ID:    PresetRuntimeConfig,
		Title: "runtime-config (admissionregistration v1beta1)",
		Args:  []ExtraArg{{"runtime-config", "admissionregistration.k8s.io/v1beta1=true"}},
	},
	{
		// k8s-jwks of aipub-installer as entries kubeadm keeps, where k8s-jwks
		// edits the static pod manifest of each node by hand
		ID:    PresetJWKS,
		Title: "AIPub 인증 (JWKS)",
		Args:  []ExtraArg{{"authentication-config", JWKSConfigPath}},
		Volumes: []ExtraVolume{
			{Name: "authentication-config", HostPath: JWKSConfigPath, MountPath: JWKSConfigPath, ReadOnly: true, PathType: "File"},
		},
		Files: []CpExtraFile{
			{Src: JWKSConfigFile, Dest: JWKSConfigPath, Mode: JWKSConfigMode},
		},
		Stage: StageAfterAipub,
	},
	{
		ID:    PresetWebhook,
		Title: "AIPub 인증 웹훅 (레거시 — JWKS 로 옮기기 전 사이트)",
		Args: []ExtraArg{
			{"authentication-token-webhook-config-file", "/etc/kubernetes/webhook-token-auth.yaml"},
			{"authentication-token-webhook-cache-ttl", "2m"},
		},
		Volumes: []ExtraVolume{
			{Name: "webhook-token-auth", HostPath: "/etc/kubernetes/webhook-token-auth.yaml", MountPath: "/etc/kubernetes/webhook-token-auth.yaml", ReadOnly: true, PathType: "File"},
			{Name: "k8s-installer-certs", HostPath: "/etc/k8s-installer/certs", MountPath: "/etc/k8s-installer/certs", ReadOnly: true},
		},
		Files: []CpExtraFile{
			{Src: WebhookConfigFile, Dest: "/etc/kubernetes/webhook-token-auth.yaml", Mode: "0644"},
		},
		Stage: StageAfterCert,
	},
}

// ControlPlane is what the control plane screen edits.
type ControlPlane struct {
	Audit         bool
	RuntimeConfig bool
	Webhook       bool
	// WebhookURL is the token review server of AIPub, written into the kubeconfig
	// of the webhook preset. Required while it is on; nothing guesses it
	WebhookURL string
	// JWKS is the JWKS preset: on for a new site, applied only when AIPub is
	// installed (AppState.AppliedControlPlane)
	JWKS bool
	// JWKSIssuerURL is the issuer when the site sets one; empty follows the rule
	// of aipub-installer, https://<aipub_app_host_prefix>.<aipub_ingress_zone>
	// (AppState.JWKSIssuer)
	JWKSIssuerURL string
	// JWKSCA is the PEM of the CA as the config file holds it, read back so that
	// saving again keeps it when the CA file is not there any more. Empty until a
	// config was written with a CA
	JWKSCA string
	// JWKSCAFile is where the CA is read from on the machine the wizard runs on
	// (the control node), when the site keeps it somewhere other than JWKSCAPath.
	// Empty: JWKSCAPath. Not written to any file; the CA itself is, inline
	JWKSCAFile string

	// Entries typed in by hand, by component (CpComponents)
	Args    map[string][]ExtraArg
	Volumes map[string][]ExtraVolume
	Files   []CpExtraFile
}

// On reports whether a preset is on.
func (c *ControlPlane) On(id string) bool {
	switch id {
	case PresetAudit:
		return c.Audit
	case PresetRuntimeConfig:
		return c.RuntimeConfig
	case PresetWebhook:
		return c.Webhook
	case PresetJWKS:
		return c.JWKS
	}
	return false
}

// Set turns a preset on or off.
func (c *ControlPlane) Set(id string, on bool) {
	switch id {
	case PresetAudit:
		c.Audit = on
	case PresetRuntimeConfig:
		c.RuntimeConfig = on
	case PresetWebhook:
		c.Webhook = on
	case PresetJWKS:
		c.JWKS = on
	}
}

// presets are the presets that are on and applied by step stage at the latest.
func (c *ControlPlane) presets(stage int) []Preset {
	var out []Preset
	for _, p := range Presets {
		if c.On(p.ID) && p.Stage <= stage {
			out = append(out, p)
		}
	}
	return out
}

// ExtraArgs is k8s_<component>_extra_args: the presets that are on, then the
// entries typed in, an entry the same as one before it left out. stage leaves out
// the presets applied after that step (StageAll: nothing left out).
func (c *ControlPlane) ExtraArgs(component string, stage int) []ExtraArg {
	var all []ExtraArg
	if component == "apiserver" {
		for _, p := range c.presets(stage) {
			all = append(all, p.Args...)
		}
	}
	all = append(all, c.Args[component]...)
	return dedupe(all)
}

// ExtraVolumes is k8s_<component>_extra_volumes, merged like ExtraArgs.
func (c *ControlPlane) ExtraVolumes(component string, stage int) []ExtraVolume {
	var all []ExtraVolume
	if component == "apiserver" {
		for _, p := range c.presets(stage) {
			all = append(all, p.Volumes...)
		}
	}
	all = append(all, c.Volumes[component]...)
	return dedupe(all)
}

// ExtraFiles is k8s_cp_extra_files, merged like ExtraArgs.
func (c *ControlPlane) ExtraFiles(stage int) []CpExtraFile {
	var all []CpExtraFile
	for _, p := range c.presets(stage) {
		all = append(all, p.Files...)
	}
	all = append(all, c.Files...)
	return dedupe(all)
}

// HasLater reports whether something is on that the first setup-cluster has to
// leave out.
func (c *ControlPlane) HasLater() bool {
	return len(c.presets(StageAll)) != len(c.presets(StageCluster))
}

// HasStage reports whether a preset that is on is applied by step stage.
func (c *ControlPlane) HasStage(stage int) bool {
	for _, p := range c.presets(stage) {
		if p.Stage == stage {
			return true
		}
	}
	return false
}

// Empty reports whether nothing is added to the control plane.
func (c *ControlPlane) Empty() bool {
	if len(c.presets(StageAll)) > 0 || len(c.Files) > 0 {
		return false
	}
	for _, comp := range CpComponents {
		if len(c.Args[comp]) > 0 || len(c.Volumes[comp]) > 0 {
			return false
		}
	}
	return true
}

func dedupe[T comparable](in []T) []T {
	out := make([]T, 0, len(in))
	seen := map[T]bool{}
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// Absorb sets the control plane from the lists a vars.yml holds: a preset whose
// entries are all there is turned on and its entries taken out, and what is left
// is kept as entries typed in by hand. The webhook URL and the JWKS issuer are
// read separately (they are in the files the presets name, not in vars.yml).
func (c *ControlPlane) Absorb(args map[string][]ExtraArg, volumes map[string][]ExtraVolume, files []CpExtraFile) {
	c.Args, c.Volumes, c.Files = map[string][]ExtraArg{}, map[string][]ExtraVolume{}, nil
	apiArgs := append([]ExtraArg{}, args["apiserver"]...)
	apiVols := append([]ExtraVolume{}, volumes["apiserver"]...)
	rest := append([]CpExtraFile{}, files...)
	for _, p := range Presets {
		if !(containsAll(apiArgs, p.Args) && containsAll(apiVols, p.Volumes) && containsAll(rest, p.Files)) {
			c.Set(p.ID, false)
			continue
		}
		c.Set(p.ID, true)
		apiArgs, apiVols, rest = removeAll(apiArgs, p.Args), removeAll(apiVols, p.Volumes), removeAll(rest, p.Files)
	}
	for _, comp := range CpComponents {
		a := args[comp]
		if comp == "apiserver" {
			a = apiArgs
		}
		if len(a) > 0 {
			c.Args[comp] = a
		}
	}
	for _, comp := range CpVolumeComponents {
		v := volumes[comp]
		if comp == "apiserver" {
			v = apiVols
		}
		if len(v) > 0 {
			c.Volumes[comp] = v
		}
	}
	if len(rest) > 0 {
		c.Files = rest
	}
}

func containsAll[T comparable](have, want []T) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// removeAll takes one of each of drop out of in.
func removeAll[T comparable](in, drop []T) []T {
	out := append([]T{}, in...)
	for _, d := range drop {
		for i, x := range out {
			if x == d {
				out = append(out[:i], out[i+1:]...)
				break
			}
		}
	}
	return out
}

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
)

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
	// Later is a preset that can not be in the cluster as it is first built: the
	// files it reads are made by a step after setup-cluster
	Later bool
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
			{Src: "aipub/audit-policy.yaml", Dest: "/etc/kubernetes/audit-policy.yaml", Mode: "0644"},
		},
		Provider: "AIPub 모니터링/python 팀",
	},
	{
		ID:    PresetRuntimeConfig,
		Title: "runtime-config (admissionregistration v1beta1)",
		Args:  []ExtraArg{{"runtime-config", "admissionregistration.k8s.io/v1beta1=true"}},
	},
	{
		ID:    PresetWebhook,
		Title: "AIPub 인증 웹훅",
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
		Later: true,
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
	}
}

// presets are the presets that are on; initial leaves out the ones applied
// after the cluster is built.
func (c *ControlPlane) presets(initial bool) []Preset {
	var out []Preset
	for _, p := range Presets {
		if c.On(p.ID) && !(initial && p.Later) {
			out = append(out, p)
		}
	}
	return out
}

// ExtraArgs is k8s_<component>_extra_args: the presets that are on, then the
// entries typed in, an entry the same as one before it left out. initial leaves
// out what is applied after the cluster is built (the webhook).
func (c *ControlPlane) ExtraArgs(component string, initial bool) []ExtraArg {
	var all []ExtraArg
	if component == "apiserver" {
		for _, p := range c.presets(initial) {
			all = append(all, p.Args...)
		}
	}
	all = append(all, c.Args[component]...)
	return dedupe(all)
}

// ExtraVolumes is k8s_<component>_extra_volumes, merged like ExtraArgs.
func (c *ControlPlane) ExtraVolumes(component string, initial bool) []ExtraVolume {
	var all []ExtraVolume
	if component == "apiserver" {
		for _, p := range c.presets(initial) {
			all = append(all, p.Volumes...)
		}
	}
	all = append(all, c.Volumes[component]...)
	return dedupe(all)
}

// ExtraFiles is k8s_cp_extra_files, merged like ExtraArgs.
func (c *ControlPlane) ExtraFiles(initial bool) []CpExtraFile {
	var all []CpExtraFile
	for _, p := range c.presets(initial) {
		all = append(all, p.Files...)
	}
	all = append(all, c.Files...)
	return dedupe(all)
}

// HasLater reports whether something is on that the first setup-cluster has to
// leave out.
func (c *ControlPlane) HasLater() bool { return len(c.presets(false)) != len(c.presets(true)) }

// Empty reports whether nothing is added to the control plane.
func (c *ControlPlane) Empty() bool {
	if len(c.presets(false)) > 0 || len(c.Files) > 0 {
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
// is kept as entries typed in by hand. The webhook URL is read separately (it is
// in the kubeconfig the preset names, not in vars.yml).
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

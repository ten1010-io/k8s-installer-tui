package plan

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// M1: the StorageClass driver is the site's own (NFS, Ceph, a vendor CSI), which
// neither installer brings, so the run stops for a person to install it. Its pods
// need the nodes Ready (setup-network, A1); the first step that claims a volume
// of a class is base with HA (harbor), then monitoring (aipub_monitoring_storage_type
// dynamic) and aipub (aipub_app_file_storage_type dynamic, the shared data volume
// of the backend) - aipub-installer README "Before setup-base.yml / setup-monitoring.yml
// / setup-aipub.yml" and ansible/aipub_vars/defaults. The stop runs beside
// setup-cert (A2), which touches no storage, so the person works while it runs; the
// next stage starts only when every class is there.

// StorageKeys are the keys of files/aipub/vars.yml that choose the classes, which
// the wizard does not manage: the site file keeps them as written.
var StorageKeys = []string{
	"aipub_app_storage_class_name",
	"aipub_app_file_storage_type",
	"aipub_app_file_storage_storage_class_name",
	"aipub_monitoring_storage_type",
	"aipub_monitoring_storage_class_name",
}

// ReadStorageVars reads StorageKeys from the AIPub site file; nothing when it is
// not there. A value that is a template ({{ ... }}) is the default, as unset.
func ReadStorageVars(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var all map[string]any
	if yaml.Unmarshal(data, &all) != nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range StorageKeys {
		v, ok := all[k].(string)
		if ok && !strings.Contains(v, "{{") {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// DefaultClass stands among the classes of StorageNeeds for the default class
// of the cluster, which a claim with no class name gets: what is checked is
// that some class is marked default. English whatever the language, as the
// classes are compared with it; shown through i18n.T (classNames).
const DefaultClass = "(cluster default StorageClass)"

// className is a class as it is shown: DefaultClass in the language spoken.
func className(c string) string {
	if c == DefaultClass {
		return i18n.T(DefaultClass)
	}
	return c
}

// classNames are classes as they are shown (className).
func classNames(classes []string) []string {
	out := make([]string, len(classes))
	for i, c := range classes {
		out[i] = className(c)
	}
	return out
}

// isDefaultClass is the annotation that marks the default StorageClass.
const isDefaultClass = "storageclass.kubernetes.io/is-default-class"

// StorageNeeds are the StorageClasses the site claims volumes on, with the steps
// that claim them, by class name (DefaultClass for the default one).
func StorageNeeds(s *state.AppState, o Options) map[string][]string {
	needs := map[string][]string{}
	if !s.AipubEnabled {
		return needs
	}
	v := func(k, def string) string {
		if x, ok := o.StorageVars[k]; ok {
			return x
		}
		return def
	}
	ha := ""
	if s.AipubHaMode {
		ha = strings.TrimSpace(s.AipubHaModeStorageClass)
		if ha != "" {
			needs[ha] = append(needs[ha], "A3 setup-base")
		}
	}
	if o.Monitoring {
		def := "static"
		if s.AipubHaMode {
			def = "dynamic"
		}
		if c := v("aipub_monitoring_storage_class_name", ha); v("aipub_monitoring_storage_type", def) == "dynamic" {
			if c == "" {
				// an empty name takes the default class of the cluster
				c = DefaultClass
			}
			needs[c] = append(needs[c], "A4 setup-monitoring")
		}
	}
	if v("aipub_app_file_storage_type", "dynamic") == "dynamic" {
		// empty: check-aipub.yml stops and says so
		if c := v("aipub_app_file_storage_storage_class_name", v("aipub_app_storage_class_name", ha)); c != "" {
			needs[c] = append(needs[c], "A5 setup-aipub")
		}
	}
	return needs
}

func storageStep(s *state.AppState, o Options) Step {
	st := Step{ID: "M1", Title: i18n.T("Install the StorageClass driver"), Kind: Manual, WithPrev: true}
	needs := StorageNeeds(s, o)
	if len(needs) == 0 {
		st.Skip, st.SkipWhy = true, i18n.T("No step uses a StorageClass (not HA, monitoring and the shared volume static)")
		return st
	}
	var uses []string
	for _, c := range sortedKeys(needs) {
		st.Classes = append(st.Classes, c)
		uses = append(uses, className(c)+" ← "+strings.Join(needs[c], ", "))
	}
	st.Note = i18n.T("Waits alongside setup-cert. Install the storage driver (CSI) of the site to make the classes: %s. "+
		"The next stage starts once kubectl get storageclass on the first k8s control plane node shows them all", strings.Join(uses, "; "))
	return st
}

// ManualPrompt is what the operator is asked to do at a Manual step.
func ManualPrompt(st Step) string {
	return i18n.T("Install the StorageClass driver — the classes the wizard uses: %s", strings.Join(classNames(st.Classes), ", "))
}

// CheckClasses asks the first k8s control plane node which of classes exist
// (kubectl as root with the admin kubeconfig, read only, as P8), and returns the
// missing ones with a line for each class.
func CheckClasses(ctx context.Context, o Options, s *state.AppState, run RunFunc, classes []string) (missing, lines []string) {
	if len(s.K8sCpNodes) == 0 {
		return classes, []string{i18n.T("No k8s control plane node to check with")}
	}
	node := s.K8sCpNodes[0]
	for _, c := range classes {
		if c == DefaultClass {
			line, ok := checkDefaultClass(ctx, o, run, node)
			lines = append(lines, line)
			if !ok {
				missing = append(missing, c)
			}
			continue
		}
		if !storageClassName.MatchString(c) {
			missing = append(missing, c)
			lines = append(lines, i18n.T("StorageClass name %s is not valid", c))
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		res, out, err := adhoc(cctx, o, run, node, true, "command",
			"kubectl --kubeconfig "+adminKubeconfig+" get storageclass "+c+" -o name")
		cancel()
		got, found := res[node]
		switch {
		case found && got.ok():
			lines = append(lines, i18n.T("%s: StorageClass %s is there", node, c))
			continue
		case found && strings.Contains(got.Raw, "NotFound"):
			lines = append(lines, i18n.T("%s: StorageClass %s missing", node, c))
		case found:
			lines = append(lines, i18n.T("%s: could not check StorageClass %s (%s)", node, c, firstLine(strings.TrimSpace(got.Raw))))
		default:
			lines = append(lines, i18n.T("Could not check StorageClass %s: %s", c, couldNotRun(out, err)))
		}
		missing = append(missing, c)
	}
	sort.Strings(missing)
	return missing, lines
}

// checkDefaultClass asks node for the classes marked default (is-default-class),
// read only like the rest of CheckClasses; ok when there is one.
func checkDefaultClass(ctx context.Context, o Options, run RunFunc, node string) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// the jsonpath in single quotes for the split of the command module
	res, out, err := adhoc(cctx, o, run, node, true, "command",
		"kubectl --kubeconfig "+adminKubeconfig+" get storageclass -o "+
			`'jsonpath={.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}'`)
	got, found := res[node]
	switch {
	case found && got.ok() && strings.TrimSpace(got.Stdout) != "":
		return i18n.T("%s: default StorageClass %s is there", node, strings.TrimSpace(got.Stdout)), true
	case found && got.ok():
		return i18n.T("%s: no default StorageClass (no class with %s: \"true\")", node, isDefaultClass), false
	case found:
		return i18n.T("%s: could not check the default StorageClass (%s)", node, firstLine(strings.TrimSpace(got.Raw))), false
	}
	return i18n.T("Could not check the default StorageClass: %s", couldNotRun(out, err)), false
}

// ReadReloaderOff reports whether the AIPub site file turns reloader off
// (aipub_reloader_enabled: false), a variable the wizard does not manage:
// setup-base.yml asks the catalog for the reloader image unless it is.
func ReadReloaderOff(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var all map[string]any
	if yaml.Unmarshal(data, &all) != nil {
		return false
	}
	on, set := all["aipub_reloader_enabled"].(bool)
	return set && !on
}

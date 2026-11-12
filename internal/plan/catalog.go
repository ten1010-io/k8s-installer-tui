package plan

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"gopkg.in/yaml.v3"
)

// P5: the catalog of the package against what the aipub playbooks need. Each
// setup playbook of aipub-installer starts with tasks/common/check-catalog.yml,
// which refuses the run when the catalog (ansible/aipub_vars/catalog.yml) lacks
// an image its module needs, by NAME (the last part of the path, not the tag).
// The same rules are checked here, so that it fails before any playbook runs:
//
//   - every entry of a module is <allowed prefix>/<name>:<tag>
//   - no name appears twice in a module
//   - the module holds every name it needs (the tables below)
//
// The prefixes of a module are the paths catalog_modules of aipub-installer.yml
// maps to it; monitoring also pulls busybox from aipub/.

// The names each module needs, from the playbooks and defaults of aipub-installer
// (ansible/aipub-playbooks/*, ansible/aipub_vars/defaults/*, charts/aipub/services.yaml).
// A test (TestCatalogNamesMatchInstaller) derives them again from those files
// when a checkout of aipub-installer is beside this one, and fails on drift.
var (
	// catalog_names of setup-network.yml: cilium, its operator and envoy. The
	// hubble images are needed only with aipub_cilium_hubble_relay_enabled and
	// aipub_cilium_hubble_ui_enabled (aipub-network.yml, both false), which the
	// wizard does not set
	catalogNetworkNames = []string{"cilium", "operator-generic", "cilium-envoy"}

	// catalog_names of setup-base.yml: the harbor images (goharbor/)
	catalogBaseNames = []string{
		"harbor-portal", "harbor-core", "harbor-jobservice", "registry-photon",
		"harbor-registryctl", "trivy-adapter-photon", "harbor-db", "redis-photon",
	}

	// aipub_app_catalog_names (aipub-app.yml): the unique images of every release
	// of charts/aipub/services.yaml, aipub-nav too whatever the edition is (lite
	// only skips deploying it). busybox is not here: aipub does not need it, the
	// monitoring module does (catalogMonitoringNames) and takes it from here
	catalogAipubNames = []string{
		"aipub-backend-api", "aipub-backend-gateway", "aipub-backend-web", "aipub-web", "aipub-docs",
		"aipub-backend-estimate", "aipub-image-build-controller", "aipub-image-build-executor",
		"project-controller", "aipub-nav", "aipub-backend-informer",
	}

	// catalog_names of setup-monitoring.yml, checked against
	// aipub_monitoring_catalog_images (aipub-monitoring.yml): the monitoring images
	// of the catalog, and busybox from the aipub ones when the monitoring ones
	// have none. opensearch-postinstall-job is the one of the opensearch hooks
	catalogMonitoringNames = []string{
		"opensearch", "opensearch-dashboards", "opensearch-postinstall-job", "fluent-bit", "alertmanager",
		"victoriametrics-operator", "vmo-config-reloader", "victoria-metrics", "vmagent", "vmalert", "vmauth",
		"vm-replica-sync", "kube-state-metrics", "node-exporter", "prometheus-adapter", "metrics-server",
		"k8s-ephemeral-storage-metrics", "dcgm-exporter", "ipmi-exporter", "node-info-exporter",
		"node-problem-detector", "gpu-pod-exporter", "aipub-crd-exporter", "aipub-disk-exporter",
		"postgres-exporter", "aipub-monitoring-alerter", "aipub-monitoring-api", "report-dispatcher", "busybox",
	}
)

// catalogModules are the keys of aipub_catalog.images, in the order the
// playbooks run, with the path they pull from when catalog_modules says nothing.
var catalogModules = []struct {
	Name   string
	Prefix string
	Names  []string
}{
	{"network", "cilium", catalogNetworkNames},
	{"base", "goharbor", catalogBaseNames},
	{"aipub", "aipub", catalogAipubNames},
	{"monitoring", "aipub/monitoring", catalogMonitoringNames},
}

// CatalogPath is the catalog of the package under the installer root.
func (o Options) CatalogPath() string { return o.Root + "/ansible/aipub_vars/catalog.yml" }

// InstallerConfigPath is aipub-installer.yml, which catalog_modules is read from.
func (o Options) InstallerConfigPath() string { return o.Root + "/aipub-installer.yml" }

// RequiredCatalogPath is the list aipub-installer keeps of the images each module
// needs (catalog_required_file of aipub-installer.yml), which its playbooks and
// aipub-packager both read. An installer from before the list has no such file.
func (o Options) RequiredCatalogPath() string {
	return o.Root + "/ansible/aipub_vars/defaults/aipub-catalog.yml"
}

// requiredCatalog is aipub-catalog.yml: per module, "<path>/<name>" of every
// image it needs, and of the ones it needs only with a feature turned on.
type requiredCatalog struct {
	Required map[string][]string `yaml:"aipub_catalog_required"`
	Optional map[string][]string `yaml:"aipub_catalog_optional"`
}

// readRequiredCatalog reads the list of the installer, nil when the installer
// has none (or one that does not parse): the tables above are used then.
func readRequiredCatalog(path string) *requiredCatalog {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f requiredCatalog
	if yaml.Unmarshal(data, &f) != nil || len(f.Required) == 0 {
		return nil
	}
	return &f
}

// baseNames are the names of a list of "<path>/<name>": the last part of each.
func baseNames(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, imageName(p))
	}
	return out
}

// CatalogReport is what was found in a catalog: Found is false when there is no
// file to read; Problems are one line each, as `module[aipub]: aipub-web missing`.
type CatalogReport struct {
	Found    bool
	Problems []string
	// Counts is the number of images per module checked, "network 3 · base 8"
	Counts string
}

// CheckCatalogFile reads the catalog and checks it. Nothing is changed: the
// catalog and aipub-installer.yml are only read.
func CheckCatalogFile(o Options) CatalogReport {
	data, err := os.ReadFile(o.CatalogPath())
	if err != nil {
		return CatalogReport{}
	}
	rep := CatalogReport{Found: true}
	images, err := parseCatalog(data)
	if err != nil {
		rep.Problems = []string{"catalog.yml: " + err.Error()}
		return rep
	}
	prefixes := defaultCatalogPrefixes()
	if cfg, err := os.ReadFile(o.InstallerConfigPath()); err == nil {
		for module, paths := range parseCatalogModules(cfg) {
			prefixes[module] = paths
		}
	}
	rep.Problems, rep.Counts = checkCatalogWith(images, prefixes, o, readRequiredCatalog(o.RequiredCatalogPath()))
	return rep
}

// CheckCatalog is P5: skipped when AIPub brings no catalog to read, else
// OK when every module holds what it needs.
func CheckCatalog(o Options) CheckResult {
	rep := CheckCatalogFile(o)
	if !rep.Found {
		return CheckResult{OK: true, Skipped: true, Lines: []string{i18n.T("no catalog file (%s)", o.CatalogPath())}}
	}
	if len(rep.Problems) > 0 {
		return CheckResult{Lines: rep.Problems}
	}
	return CheckResult{OK: true, Lines: []string{i18n.T("Catalog OK: %s", rep.Counts)}}
}

// CatalogSummary is the one line for the preview, and whether the catalog was
// found. A catalog with problems says how many and the first.
func CatalogSummary(o Options) (line string, ok, found bool) {
	rep := CheckCatalogFile(o)
	switch {
	case !rep.Found:
		return "", false, false
	case len(rep.Problems) == 0:
		return i18n.T("Catalog OK: %s", rep.Counts), true, true
	}
	if len(rep.Problems) > 1 {
		return i18n.T("Catalog problems: %d, which the playbooks refuse (P5 of the install plan): %s and more", len(rep.Problems), rep.Problems[0]), false, true
	}
	return i18n.T("Catalog problems: %d, which the playbooks refuse (P5 of the install plan): %s", len(rep.Problems), rep.Problems[0]), false, true
}

type catalogImages map[string][]string

func parseCatalog(data []byte) (catalogImages, error) {
	var f struct {
		Catalog *struct {
			Images map[string][]string `yaml:"images"`
		} `yaml:"aipub_catalog"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	// the playbooks refuse the same: aipub-packager fills it when it makes a package
	if f.Catalog == nil || f.Catalog.Images == nil {
		return nil, errors.New(i18n.T("the catalog is empty (aipub-packager fills it when it makes a package)"))
	}
	return f.Catalog.Images, nil
}

func defaultCatalogPrefixes() map[string][]string {
	out := map[string][]string{}
	for _, m := range catalogModules {
		out[m.Name] = []string{m.Prefix}
	}
	return out
}

// parseCatalogModules reads catalog_modules of aipub-installer.yml, lines like
// "aipub/monitoring -> monitoring", into module -> paths. Nothing for a file
// that does not parse.
func parseCatalogModules(data []byte) map[string][]string {
	var f struct {
		Modules []string `yaml:"catalog_modules"`
	}
	if yaml.Unmarshal(data, &f) != nil {
		return nil
	}
	out := map[string][]string{}
	for _, line := range f.Modules {
		path, module, ok := strings.Cut(line, "->")
		if !ok {
			continue
		}
		path, module = strings.TrimSpace(path), strings.TrimSpace(module)
		if path != "" && module != "" {
			out[module] = append(out[module], path)
		}
	}
	return out
}

// checkCatalog applies the rules of check-catalog.yml to every module and
// returns the problems and the number of images per module. monitoring false
// leaves that module out (setup-monitoring.yml is not run).
func checkCatalog(images catalogImages, prefixes map[string][]string, monitoring bool) (problems []string, counts string) {
	return checkCatalogWith(images, prefixes, Options{Monitoring: monitoring}, nil)
}

// checkCatalogWith is checkCatalog with the list of the installer (req, nil when
// it has none). With it the names a module needs are the ones the installer
// lists rather than the tables here, so an image the installer starts to need
// is asked for without a change to this program, and two more modules are
// checked that only the list knows: reloader (setup-base.yml, unless the site
// turned it off) and python (setup-python.yml, when the plan installs it).
func checkCatalogWith(images catalogImages, prefixes map[string][]string, o Options, req *requiredCatalog) (problems []string, counts string) {
	monitoring := o.Monitoring
	// aipub: images.aipub, or images.java (the old key) when it is empty; both
	// filled and different is refused by renamed-vars.yml
	aipub := images["aipub"]
	if len(aipub) == 0 {
		aipub = images["java"]
	} else if java := images["java"]; len(java) > 0 && !sameList(java, aipub) {
		problems = append(problems, i18n.T("module[aipub]: images.aipub and images.java differ (keep one only)"))
	}

	var summary []string
	for _, m := range catalogModules {
		refs := images[m.Name]
		paths := prefixes[m.Name]
		switch m.Name {
		case "aipub":
			refs = aipub
		case "monitoring":
			if !monitoring {
				continue
			}
			// busybox is pulled from aipub/; the packager sorts it into the aipub
			// module, so it is read from there when monitoring has none
			if !hasName(refs, "busybox") {
				var busybox []string
				for _, r := range aipub {
					if strings.HasPrefix(r, "aipub/busybox:") {
						busybox = append(busybox, r)
					}
				}
				refs = append(append([]string{}, refs...), busybox...)
			}
			paths = appendOnce(paths, "aipub")
		}
		needed := m.Names
		if req != nil && len(req.Required[m.Name]) > 0 {
			needed = baseNames(req.Required[m.Name])
		}
		problems = append(problems, checkModule(m.Name, refs, paths, needed, false)...)
		summary = append(summary, fmt.Sprintf("%s %d", m.Name, len(refs)))
	}
	if req == nil {
		return problems, strings.Join(summary, " · ")
	}

	// reloader: setup-base.yml checks it while aipub_reloader_enabled (the default)
	if needed := baseNames(req.Required["reloader"]); len(needed) > 0 && !o.ReloaderOff {
		paths := prefixes["reloader"]
		if len(paths) == 0 {
			paths = []string{"stakater"}
		}
		refs := images["reloader"]
		problems = append(problems, checkModule("reloader", refs, paths, needed, false)...)
		summary = append(summary, fmt.Sprintf("reloader %d", len(refs)))
	}

	// python: tasks/python/check-catalog.yml. Its images are images.python and the
	// aipub ones python does not list itself (aipub_python_catalog_images), under
	// aipub/ or agentgateway/ with a path of their own allowed, and busybox with
	// them; the network operator and its multus only where the operator is on
	if needed := baseNames(req.Required["python"]); len(needed) > 0 && o.Python {
		own := map[string]bool{}
		refs := append([]string{}, images["python"]...)
		for _, r := range refs {
			own[imageName(r)] = true
		}
		for _, r := range aipub {
			if !own[imageName(r)] {
				refs = append(refs, r)
			}
		}
		needed = append(needed, "busybox")
		if o.NetworkOperator {
			needed = append(needed, baseNames(req.Optional["python"])...)
		}
		problems = append(problems, checkModule("python", refs, []string{"aipub", "agentgateway"}, needed, true)...)
		summary = append(summary, fmt.Sprintf("python %d", len(images["python"])))
	}
	return problems, strings.Join(summary, " · ")
}

// checkModule is check-catalog.yml for one module: the format of every entry,
// the names the module needs, and the names that appear more than once.
//
// subpaths lets an entry sit under a path of its own below the prefix
// (catalog_subpaths: aipub/linuxserver/openssh-server), as the python module does.
func checkModule(module string, refs, prefixes, needed []string, subpaths bool) []string {
	var out []string
	quoted := make([]string, len(prefixes))
	for i, p := range prefixes {
		quoted[i] = regexp.QuoteMeta(p)
	}
	sub := ""
	if subpaths {
		sub = `([A-Za-z0-9._-]+/)*`
	}
	re := regexp.MustCompile(`^(` + strings.Join(quoted, "|") + `)/` + sub + `[A-Za-z0-9._-]+:[A-Za-z0-9._-]+$`)
	want := i18n.T("%s/<name>:<tag>", strings.Join(prefixes, i18n.T("/ or ")))
	for _, r := range refs {
		if !re.MatchString(r) {
			out = append(out, i18n.T("module[%s]: %s is not of the form %s", module, r, want))
		}
	}

	count := map[string]int{}
	for _, r := range refs {
		count[imageName(r)]++
	}
	for _, n := range needed {
		if count[n] == 0 {
			line := i18n.T("module[%s]: %s missing", module, n)
			if module == "monitoring" && n == "busybox" {
				line += " " + i18n.T("(busybox of monitoring, or aipub/busybox of the aipub module)")
			}
			out = append(out, line)
		}
	}
	var dup []string
	for n, c := range count {
		if c > 1 {
			dup = append(dup, n)
		}
	}
	sort.Strings(dup)
	for _, n := range dup {
		out = append(out, i18n.T("module[%s]: the name %s appears more than once", module, n))
	}
	return out
}

// imageName is the name of an entry: the last part of the path, without the tag.
func imageName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	name, _, _ := strings.Cut(ref, ":")
	return name
}

func hasName(refs []string, name string) bool {
	for _, r := range refs {
		if imageName(r) == name {
			return true
		}
	}
	return false
}

func appendOnce(l []string, v string) []string {
	for _, x := range l {
		if x == v {
			return l
		}
	}
	return append(append([]string{}, l...), v)
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

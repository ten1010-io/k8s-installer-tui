package plan

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

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

// CatalogReport is what was found in a catalog: Found is false when there is no
// file to read; Problems are one line each, as `module[aipub]: aipub-web 없음`.
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
	rep.Problems, rep.Counts = checkCatalog(images, prefixes, o.Monitoring)
	return rep
}

// CheckCatalog is P5: skipped when AIPub brings no catalog to read, else
// OK when every module holds what it needs.
func CheckCatalog(o Options) CheckResult {
	rep := CheckCatalogFile(o)
	if !rep.Found {
		return CheckResult{OK: true, Skipped: true, Lines: []string{"건너뜀: 카탈로그 파일이 없음 (" + o.CatalogPath() + ")"}}
	}
	if len(rep.Problems) > 0 {
		return CheckResult{Lines: rep.Problems}
	}
	return CheckResult{OK: true, Lines: []string{"카탈로그 이상 없음: " + rep.Counts}}
}

// CatalogSummary is the one line for the preview, and whether the catalog was
// found. A catalog with problems says how many and the first.
func CatalogSummary(o Options) (line string, ok, found bool) {
	rep := CheckCatalogFile(o)
	switch {
	case !rep.Found:
		return "", false, false
	case len(rep.Problems) == 0:
		return "카탈로그 이상 없음: " + rep.Counts, true, true
	}
	line = fmt.Sprintf("카탈로그 문제 %d건, 설치 계획의 P5 에서 플레이북이 거부합니다: %s", len(rep.Problems), rep.Problems[0])
	if len(rep.Problems) > 1 {
		line += " 외"
	}
	return line, false, true
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
		return nil, fmt.Errorf("카탈로그가 비어 있습니다 (aipub-packager 가 패키지를 만들 때 채웁니다)")
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
	// aipub: images.aipub, or images.java (the old key) when it is empty; both
	// filled and different is refused by renamed-vars.yml
	aipub := images["aipub"]
	if len(aipub) == 0 {
		aipub = images["java"]
	} else if java := images["java"]; len(java) > 0 && !sameList(java, aipub) {
		problems = append(problems, "module[aipub]: images.aipub 와 images.java 가 다름 (하나만 두세요)")
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
		problems = append(problems, checkModule(m.Name, refs, paths, m.Names)...)
		summary = append(summary, fmt.Sprintf("%s %d", m.Name, len(refs)))
	}
	return problems, strings.Join(summary, " · ")
}

// checkModule is check-catalog.yml for one module: the format of every entry,
// the names the module needs, and the names that appear more than once.
func checkModule(module string, refs, prefixes, needed []string) []string {
	var out []string
	quoted := make([]string, len(prefixes))
	for i, p := range prefixes {
		quoted[i] = regexp.QuoteMeta(p)
	}
	re := regexp.MustCompile(`^(` + strings.Join(quoted, "|") + `)/[A-Za-z0-9._-]+:[A-Za-z0-9._-]+$`)
	want := strings.Join(prefixes, "/ 또는 ") + "/<이름>:<태그>"
	for _, r := range refs {
		if !re.MatchString(r) {
			out = append(out, fmt.Sprintf("module[%s]: %s 는 %s 형식이 아님", module, r, want))
		}
	}

	count := map[string]int{}
	for _, r := range refs {
		count[imageName(r)]++
	}
	for _, n := range needed {
		if count[n] == 0 {
			line := fmt.Sprintf("module[%s]: %s 없음", module, n)
			if module == "monitoring" && n == "busybox" {
				line += " (monitoring 의 busybox, 또는 aipub 모듈의 aipub/busybox)"
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
		out = append(out, fmt.Sprintf("module[%s]: %s 이름이 두 번 이상 나옴", module, n))
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

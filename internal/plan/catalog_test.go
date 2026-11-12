package plan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// the installer root of the package the fixture new-site-helmfile stands for: a
// complete catalog, aipub-installer.yml, and a package record without aipub-bin
const fixtureRoot = "../../test/simulate/fixtures/new-site-helmfile/installer-root"

func readFixture(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// catalogRoot is an installer root holding the catalog of the fixture after edit,
// and the aipub-installer.yml of the fixture after editCfg (either may be nil).
func catalogRoot(t *testing.T, edit, editCfg func(string) string) Options {
	t.Helper()
	o := testOptions()
	o.Root = t.TempDir()
	cat, cfg := readFixture(t, "ansible/aipub_vars/catalog.yml"), readFixture(t, "aipub-installer.yml")
	if edit != nil {
		cat = edit(cat)
	}
	if editCfg != nil {
		cfg = editCfg(cfg)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Dir(o.CatalogPath()), 0755))
	must(os.WriteFile(o.CatalogPath(), []byte(cat), 0644))
	must(os.WriteFile(o.InstallerConfigPath(), []byte(cfg), 0644))
	return o
}

// without takes the lines holding sub out of the catalog.
func without(sub string) func(string) string {
	return func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.Contains(l, sub) {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
}

func problems(t *testing.T, o Options) string {
	t.Helper()
	r := CheckCatalog(o)
	if r.OK {
		t.Fatalf("passed, want a failure: %v", r.Lines)
	}
	return strings.Join(r.Lines, "\n")
}

func TestCatalogComplete(t *testing.T) {
	o := catalogRoot(t, nil, nil)
	r := CheckCatalog(o)
	if !r.OK || r.Skipped || r.Warn {
		t.Fatalf("complete catalog: %+v", r)
	}
	if want := "network 3 · base 8 · aipub 12 · monitoring 29"; !strings.Contains(r.Lines[0], want) {
		t.Errorf("summary %q lacks %q", r.Lines[0], want)
	}
	if line, ok, found := CatalogSummary(o); !ok || !found || !strings.Contains(line, "이상 없음") {
		t.Errorf("summary: %q %v %v", line, ok, found)
	}
}

func TestCatalogMissingName(t *testing.T) {
	got := problems(t, catalogRoot(t, without("/aipub-web:"), nil))
	if got != "module[aipub]: aipub-web 없음" {
		t.Errorf("got %q", got)
	}
	got = problems(t, catalogRoot(t, without("opensearch-postinstall-job"), nil))
	if got != "module[monitoring]: opensearch-postinstall-job 없음" {
		t.Errorf("got %q", got)
	}
	got = problems(t, catalogRoot(t, without("goharbor/harbor-db"), nil))
	if got != "module[base]: harbor-db 없음" {
		t.Errorf("got %q", got)
	}
	got = problems(t, catalogRoot(t, without("cilium/cilium-envoy"), nil))
	if got != "module[network]: cilium-envoy 없음" {
		t.Errorf("got %q", got)
	}
	// every missing name is a line of its own
	got = problems(t, catalogRoot(t, without("/aipub-backend-"), nil))
	for _, n := range []string{"aipub-backend-api", "aipub-backend-gateway", "aipub-backend-web", "aipub-backend-informer", "aipub-backend-estimate"} {
		if !strings.Contains(got, "module[aipub]: "+n+" 없음") {
			t.Errorf("%s not named:\n%s", n, got)
		}
	}
	// the name counts, not the tag: another tag of a name that is there is fine
	o := catalogRoot(t, func(s string) string { return strings.Replace(s, "aipub-web:main-a472a5f", "aipub-web:other-tag", 1) }, nil)
	if r := CheckCatalog(o); !r.OK {
		t.Errorf("another tag refused: %v", r.Lines)
	}
}

// busybox is pulled from aipub/: taken from the aipub module when monitoring has
// none, and refused when neither has it.
func TestCatalogBusybox(t *testing.T) {
	got := problems(t, catalogRoot(t, without("busybox"), nil))
	if !strings.HasPrefix(got, "module[monitoring]: busybox 없음") || strings.Count(got, "\n") != 0 {
		t.Errorf("no busybox anywhere: %q", got)
	}
	// a catalog written by hand, with busybox under monitoring, is read as before
	moved := func(s string) string {
		s = without("aipub/busybox")(s)
		return strings.Replace(s, "      - aipub/monitoring/report-dispatcher:v1.1.10", "      - aipub/monitoring/report-dispatcher:v1.1.10\n      - aipub/busybox:1.36.1", 1)
	}
	if r := CheckCatalog(catalogRoot(t, moved, nil)); !r.OK {
		t.Errorf("busybox under monitoring refused: %v", r.Lines)
	}
	// both: no name twice in monitoring
	both := func(s string) string {
		return strings.Replace(s, "      - aipub/monitoring/report-dispatcher:v1.1.10", "      - aipub/monitoring/report-dispatcher:v1.1.10\n      - aipub/busybox:1.36.1", 1)
	}
	if r := CheckCatalog(catalogRoot(t, both, nil)); !r.OK {
		t.Errorf("busybox in both refused: %v", r.Lines)
	}
}

func TestCatalogPrefixAndDuplicates(t *testing.T) {
	got := problems(t, catalogRoot(t, func(s string) string {
		return strings.Replace(s, "aipub/aipub-docs:20260527-r4", "other/aipub-docs:20260527-r4", 1)
	}, nil))
	if got != "module[aipub]: other/aipub-docs:20260527-r4 는 aipub/<이름>:<태그> 형식이 아님" {
		t.Errorf("prefix: %q", got)
	}
	// a monitoring image in the wrong place: the aipub module takes only aipub/<name>
	got = problems(t, catalogRoot(t, func(s string) string {
		return strings.Replace(s, "      - aipub/busybox:1.36.1", "      - aipub/busybox:1.36.1\n      - aipub/monitoring/extra:1", 1)
	}, nil))
	if !strings.Contains(got, "module[aipub]: aipub/monitoring/extra:1 는 aipub/<이름>:<태그> 형식이 아님") {
		t.Errorf("a path of another module: %q", got)
	}
	// a digest is not a tag
	got = problems(t, catalogRoot(t, func(s string) string {
		return strings.Replace(s, "goharbor/harbor-db:v2.14.0", "goharbor/harbor-db@sha256:abc", 1)
	}, nil))
	if !strings.Contains(got, "module[base]: goharbor/harbor-db@sha256:abc") {
		t.Errorf("digest: %q", got)
	}
	got = problems(t, catalogRoot(t, func(s string) string {
		return strings.Replace(s, "      - aipub/busybox:1.36.1", "      - aipub/busybox:1.36.1\n      - aipub/aipub-web:other", 1)
	}, nil))
	if got != "module[aipub]: aipub-web 이름이 두 번 이상 나옴" {
		t.Errorf("duplicate: %q", got)
	}
}

// catalog_modules of aipub-installer.yml says which path is which module.
func TestCatalogModulesFromInstallerConfig(t *testing.T) {
	o := catalogRoot(t, nil, func(s string) string { return strings.Replace(s, `"aipub -> aipub"`, `"myaipub -> aipub"`, 1) })
	got := problems(t, o)
	if !strings.Contains(got, "module[aipub]: aipub/aipub-web:main-a472a5f 는 myaipub/<이름>:<태그> 형식이 아님") {
		t.Errorf("catalog_modules not followed: %q", got)
	}
	// no aipub-installer.yml: the paths of the playbooks
	if err := os.Remove(o.InstallerConfigPath()); err != nil {
		t.Fatal(err)
	}
	if r := CheckCatalog(o); !r.OK {
		t.Errorf("without aipub-installer.yml: %v", r.Lines)
	}
}

func TestCatalogOptions(t *testing.T) {
	// monitoring not installed: its module is not looked at
	o := catalogRoot(t, without("opensearch-postinstall-job"), nil)
	o.Monitoring = false
	r := CheckCatalog(o)
	if !r.OK || strings.Contains(r.Lines[0], "monitoring") {
		t.Errorf("monitoring off: %+v", r)
	}

	// the old key java is read when aipub is empty; both and different is refused
	java := func(s string) string { return strings.Replace(s, "    aipub:\n", "    java:\n", 1) }
	if r := CheckCatalog(catalogRoot(t, java, nil)); !r.OK {
		t.Errorf("images.java: %v", r.Lines)
	}
	both := func(s string) string {
		return strings.Replace(s, "    aipub:\n", "    java:\n      - aipub/aipub-web:old\n    aipub:\n", 1)
	}
	if got := problems(t, catalogRoot(t, both, nil)); !strings.Contains(got, "images.aipub 와 images.java 가 다름") {
		t.Errorf("both keys: %q", got)
	}

	// empty, as in the repository: refused like the playbooks do
	empty := func(string) string { return "aipub_catalog: null\n" }
	if got := problems(t, catalogRoot(t, empty, nil)); !strings.Contains(got, "카탈로그가 비어 있습니다") {
		t.Errorf("empty: %q", got)
	}

	// no catalog file: skipped, and the reason names the file (the word
	// "skipped" is the screen's, from Skipped)
	none := testOptions()
	none.Root = t.TempDir()
	r = CheckCatalog(none)
	if !r.OK || !r.Skipped || !strings.Contains(r.Lines[0], "카탈로그 파일이 없음") || !strings.Contains(r.Lines[0], none.CatalogPath()) {
		t.Errorf("no file: %+v", r)
	}
	if _, _, found := CatalogSummary(none); found {
		t.Error("a summary for a catalog that is not there")
	}
}

func TestBuildP5(t *testing.T) {
	steps := Build(aipubState(), testOptions())
	var p5 *Step
	for i := range steps {
		if steps[i].ID == "P5" {
			p5 = &steps[i]
		}
	}
	if p5 == nil || p5.Kind != Check || p5.Title != "카탈로그" {
		t.Fatalf("P5: %+v", p5)
	}
	// after the checks of the machine, before anything runs
	if !strings.HasPrefix(ids(steps), "P1 P2 P5 P6 P7 P8 K1") {
		t.Errorf("order: %s", ids(steps))
	}
}

// --- the names the tables hold, against the installer ---

// installerDir is a checkout of aipub-installer beside this repository, or "".
func installerDir() string {
	candidates := []string{os.Getenv("AIPUB_INSTALLER_DIR"), "../../../staged-inst", "../../../aipub-installer", "../../../../aipub-installer"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "charts/aipub/services.yaml")); err == nil {
			return c
		}
	}
	return ""
}

func sorted(l []string) []string {
	out := append([]string{}, l...)
	sort.Strings(out)
	return out
}

func sameSet(a, b []string) bool { return strings.Join(sorted(a), " ") == strings.Join(sorted(b), " ") }

// The tables are copies of what the playbooks of aipub-installer name. Where the
// installer is at hand they are derived again, from the files they say they came
// from, so that a service added to services.yaml, or an image a playbook starts
// to need, fails here and not on a site.
func TestCatalogNamesMatchInstaller(t *testing.T) {
	dir := installerDir()
	if dir == "" {
		t.Skip("no checkout of aipub-installer beside this repository (set AIPUB_INSTALLER_DIR)")
	}
	read := func(rel string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	// aipub: the images of every release of services.yaml (aipub_app_catalog_names)
	var services struct {
		Components []struct {
			Releases []struct {
				Images []string `yaml:"images"`
			} `yaml:"releases"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(read("charts/aipub/services.yaml"), &services); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var fromServices []string
	for _, c := range services.Components {
		for _, r := range c.Releases {
			for _, img := range r.Images {
				if !seen[img] {
					seen[img] = true
					fromServices = append(fromServices, img)
				}
			}
		}
	}
	if !sameSet(fromServices, catalogAipubNames) {
		t.Errorf("catalogAipubNames drifted from charts/aipub/services.yaml:\n  table:    %v\n  services: %v", sorted(catalogAipubNames), sorted(fromServices))
	}

	// An installer that keeps the list of what each module needs in one file
	// (ansible/aipub_vars/defaults/aipub-catalog.yml, which its playbooks read their
	// catalog_names from) is compared through that file: the tables here are what
	// is used beside an installer from before it, and have to say the same
	if req := readRequiredCatalog(filepath.Join(dir, "ansible/aipub_vars/defaults/aipub-catalog.yml")); req != nil {
		for module, table := range map[string][]string{
			"network": catalogNetworkNames, "base": catalogBaseNames, "monitoring": catalogMonitoringNames,
		} {
			if names := baseNames(req.Required[module]); !sameSet(names, table) {
				t.Errorf("the table of %s drifted from aipub-catalog.yml:\n  table: %v\n  file:  %v", module, sorted(table), sorted(names))
			}
		}
		return
	}

	// base, monitoring: catalog_names of their playbooks; network's is a template
	// that adds the hubble images, of which the three it always has are checked
	for module, table := range map[string][]string{"base": catalogBaseNames, "monitoring": catalogMonitoringNames} {
		var raw []map[string]any
		if err := yaml.Unmarshal(read("ansible/aipub-playbooks/setup-"+module+".yml"), &raw); err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range raw {
			vars, _ := p["vars"].(map[string]any)
			if vars["catalog_module"] != module {
				continue
			}
			list, _ := vars["catalog_names"].([]any)
			for _, n := range list {
				names = append(names, n.(string))
			}
		}
		if len(names) == 0 {
			t.Errorf("no catalog_names for %s in its playbook", module)
		}
		if !sameSet(names, table) {
			t.Errorf("the table of %s drifted from setup-%s.yml:\n  table:    %v\n  playbook: %v", module, module, sorted(table), sorted(names))
		}
	}
	network := string(read("ansible/aipub-playbooks/setup-network.yml"))
	for _, n := range catalogNetworkNames {
		if !strings.Contains(network, "'"+n+"'") {
			t.Errorf("%s is not in catalog_names of setup-network.yml", n)
		}
	}

	// the prefixes: catalog_modules of aipub-installer.yml
	modules := parseCatalogModules(read("aipub-installer.yml"))
	for _, m := range catalogModules {
		if got := modules[m.Name]; len(got) != 1 || got[0] != m.Prefix {
			t.Errorf("module %s: aipub-installer.yml has %v, the table %q", m.Name, got, m.Prefix)
		}
	}
}

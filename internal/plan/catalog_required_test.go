package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An installer that keeps what each module needs in one file
// (ansible/aipub_vars/defaults/aipub-catalog.yml): the names asked of the catalog
// are the ones of that file, and reloader and python are checked too.
//
// testdata/aipub-catalog.yml and testdata/catalog-with-python.yml are that file
// and examples/catalog.yml of aipub-installer feat/IN-70-installer-refactor (26601b5).

func testdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// requiredRoot is an installer root with the catalog of the python example after
// edit and the list of the installer after editRequired (either may be nil).
func requiredRoot(t *testing.T, edit, editRequired func(string) string) Options {
	t.Helper()
	catalog := testdata(t, "catalog-with-python.yml")
	if edit != nil {
		catalog = edit(catalog)
	}
	o := catalogRoot(t, func(string) string { return catalog }, nil)
	required := testdata(t, "aipub-catalog.yml")
	if editRequired != nil {
		required = editRequired(required)
	}
	if err := os.MkdirAll(filepath.Dir(o.RequiredCatalogPath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.RequiredCatalogPath(), []byte(required), 0644); err != nil {
		t.Fatal(err)
	}
	o.Python = true
	return o
}

func TestRequiredCatalogComplete(t *testing.T) {
	r := CheckCatalog(requiredRoot(t, nil, nil))
	if !r.OK || r.Skipped || r.Warn {
		t.Fatalf("the example catalog of the installer should pass: %+v", r)
	}
	for _, want := range []string{"reloader 1", "python 18", "monitoring 30"} {
		if !strings.Contains(r.Lines[0], want) {
			t.Errorf("summary %q lacks %q", r.Lines[0], want)
		}
	}
}

func TestRequiredCatalogPythonImageMissing(t *testing.T) {
	o := requiredRoot(t, without("/usage-manager:"), nil)
	if got := problems(t, o); got != "module[python]: usage-manager 없음" {
		t.Errorf("got %q", got)
	}
	// An image python keeps under a path of its own, and one under agentgateway/
	got := problems(t, requiredRoot(t, without("/openssh-server:"), nil))
	if got != "module[python]: openssh-server 없음" {
		t.Errorf("got %q", got)
	}
	got = problems(t, requiredRoot(t, without("agentgateway/controller:"), nil))
	if got != "module[python]: controller 없음" {
		t.Errorf("got %q", got)
	}

	// A plan that does not install python does not ask for its images
	o.Python = false
	if r := CheckCatalog(o); !r.OK {
		t.Errorf("python is not installed, and its image is asked for: %v", r.Lines)
	}
}

func TestRequiredCatalogNetworkOperator(t *testing.T) {
	// Off, the default: the catalog may go without the operator and its multus
	o := requiredRoot(t, func(s string) string {
		return without("/aipub-network-operator:")(without("/multus-cni:")(s))
	}, nil)
	if r := CheckCatalog(o); !r.OK {
		t.Fatalf("the operator is off, and its images are asked for: %v", r.Lines)
	}
	o.NetworkOperator = true
	got := problems(t, o)
	if !strings.Contains(got, "module[python]: aipub-network-operator 없음") || !strings.Contains(got, "module[python]: multus-cni 없음") {
		t.Errorf("got %q", got)
	}
}

func TestRequiredCatalogReloader(t *testing.T) {
	o := requiredRoot(t, without("stakater/reloader:"), nil)
	if got := problems(t, o); got != "module[reloader]: reloader 없음" {
		t.Errorf("got %q", got)
	}
	// A site that turned reloader off (aipub_reloader_enabled: false)
	o.ReloaderOff = true
	if r := CheckCatalog(o); !r.OK {
		t.Errorf("reloader is off, and its image is asked for: %v", r.Lines)
	}
}

// An image the installer starts to need is asked for without a change here.
func TestRequiredCatalogFollowsTheInstaller(t *testing.T) {
	o := requiredRoot(t, nil, func(s string) string {
		return strings.Replace(s, `    - "goharbor/redis-photon"`, `    - "goharbor/redis-photon"`+"\n"+`    - "goharbor/something-new"`, 1)
	})
	if got := problems(t, o); got != "module[base]: something-new 없음" {
		t.Errorf("got %q", got)
	}
}

// Beside an installer from before the list, nothing changes: the tables of this
// program are used, and reloader and python are not asked for.
func TestCatalogWithoutTheListOfTheInstaller(t *testing.T) {
	o := requiredRoot(t, func(s string) string {
		return without("stakater/reloader:")(without("/usage-manager:")(s))
	}, nil)
	if err := os.Remove(o.RequiredCatalogPath()); err != nil {
		t.Fatal(err)
	}
	if r := CheckCatalog(o); !r.OK {
		t.Errorf("without the list of the installer: %v", r.Lines)
	}
}

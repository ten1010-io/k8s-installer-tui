package fileio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func kinds(t *testing.T, dir, rel string) string {
	t.Helper()
	h, err := ReadHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range h {
		if e.Path == rel {
			out = append(out, e.Kind)
		}
	}
	return strings.Join(out, " ")
}

// Saving appends what it wrote; what was there and not in the history (a file
// edited by hand) is appended before it; saving the same again appends nothing.
func TestWriteAllHistory(t *testing.T) {
	dir := t.TempDir()
	p := DefaultPaths(dir)
	s := state.DefaultState()
	must(t, WriteAll(p, s))
	if got := kinds(t, dir, "group_vars/all/vars.yml"); got != "save" {
		t.Fatalf("first save: %q", got)
	}
	must(t, WriteAll(p, s))
	if got := kinds(t, dir, "group_vars/all/vars.yml"); got != "save" {
		t.Errorf("the same again: %q, want nothing appended", got)
	}

	saved, _ := os.ReadFile(p.Vars)
	edited := string(saved) + "# by hand\nextra_var: 1\n"
	must(t, os.WriteFile(p.Vars, []byte(edited), 0644))
	s.K8sPodSubnet = "10.99.0.0/16"
	must(t, WriteAll(p, s))
	if got := kinds(t, dir, "group_vars/all/vars.yml"); got != "save edit save" {
		t.Fatalf("after a hand edit: %q", got)
	}
	h, _ := ReadHistory(dir)
	var edit HistoryEntry
	for _, e := range h {
		if e.Kind == HistoryEdit {
			edit = e
		}
	}
	if edit.Content != edited || edit.SHA256 != sum([]byte(edited)) {
		t.Errorf("edit entry: %+v", edit)
	}
	// the edit is kept in the file too: saving edits the YAML in place
	if now, _ := os.ReadFile(p.Vars); !strings.Contains(string(now), "extra_var: 1") || !strings.Contains(string(now), "10.99.0.0/16") {
		t.Errorf("vars.yml:\n%s", now)
	}
	if st, err := os.Stat(HistoryPath(dir)); err != nil || (st.Mode().Perm()&0077 != 0 && runtime.GOOS != "windows") {
		t.Errorf("history: %v %v, want private", st, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p.Vars))
	for _, e := range entries {
		if e.Name() != "vars.yml" {
			t.Errorf("left beside vars.yml: %s", e.Name())
		}
	}
}

// A line cut short (a write that did not finish) does not stop the history
// being read.
func TestReadHistorySkipsABrokenLine(t *testing.T) {
	dir := t.TempDir()
	must(t, appendHistory(dir, entry(HistorySave, "inventory.yml", []byte("a\n"))))
	f, _ := os.OpenFile(HistoryPath(dir), os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString(`{"time":"2026-11-10T00:00:00Z","kind":"sa`)
	f.Close()
	h, err := ReadHistory(dir)
	if err != nil || len(h) != 1 || h[0].Content != "a\n" {
		t.Errorf("%+v %v", h, err)
	}
	// what is appended after it starts a line of its own: not lost in it
	must(t, appendHistory(dir, entry(HistoryEdit, "inventory.yml", []byte("b\n"))))
	h, err = ReadHistory(dir)
	if err != nil || len(h) != 2 || h[1].Kind != HistoryEdit || h[1].Content != "b\n" {
		t.Errorf("after the broken line: %+v %v", h, err)
	}
}

// A file that is not UTF-8 (a hand edit in CP949) is kept byte for byte.
func TestHistoryKeepsBytes(t *testing.T) {
	dir := t.TempDir()
	data := []byte("# \xc7\xd1\xb1\xdb comment\nk: v\n")
	must(t, appendHistory(dir, entry(HistoryEdit, "inventory.yml", data)))
	h, err := ReadHistory(dir)
	if err != nil || len(h) != 1 || h[0].Encoding != "base64" || string(h[0].Data()) != string(data) || h[0].SHA256 != sum(data) {
		t.Errorf("%+v %v", h, err)
	}
}

// The copy stops only on content the history does not have: a copy cut short,
// a copy made outside the run, or an edit put back, all go on.
func TestGuardCopyStopsOnlyOnUnknownContent(t *testing.T) {
	site, installer := t.TempDir(), t.TempDir()
	write := func(dir, data string) {
		must(t, os.WriteFile(filepath.Join(dir, "inventory.yml"), []byte(data), 0644))
	}
	stops := func() bool {
		changed, _, err := GuardCopy(site, installer)
		must(t, err)
		return len(changed) > 0
	}
	write(site, "v1\n")
	must(t, appendHistory(site, entry(HistorySave, "inventory.yml", []byte("v1\n"))))
	write(installer, "v1\n")
	must(t, RecordCopy(site, installer))
	// saved twice since; the installer has the save between (a copy that did
	// not record itself): known content, no stop
	write(site, "v2\n")
	must(t, appendHistory(site, entry(HistorySave, "inventory.yml", []byte("v2\n"))))
	write(installer, "v2\n")
	write(site, "v3\n")
	must(t, appendHistory(site, entry(HistorySave, "inventory.yml", []byte("v3\n"))))
	if stops() {
		t.Error("an earlier save on the installer side stopped the copy")
	}
	// edited by hand: stops; put back to what was copied: goes on
	write(installer, "v1\nby hand\n")
	if !stops() {
		t.Error("a hand edit did not stop the copy")
	}
	write(installer, "v1\n")
	if stops() {
		t.Error("an edit put back stopped the copy")
	}
}

// The copy to the installer: the first keeps what the installer had and goes
// on; after a copy, an edit on the installer side stops the next copy once,
// kept in the history; the one after goes on.
func TestGuardCopy(t *testing.T) {
	site, installer := t.TempDir(), t.TempDir()
	write := func(dir, rel, data string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		must(t, os.MkdirAll(filepath.Dir(p), 0755))
		must(t, os.WriteFile(p, []byte(data), 0644))
	}
	copyAll := func() {
		for _, rel := range SiteFiles() {
			if data, err := os.ReadFile(filepath.Join(site, filepath.FromSlash(rel))); err == nil {
				write(installer, rel, string(data))
			}
		}
	}
	write(site, "inventory.yml", "site: 1\n")
	write(site, "group_vars/all/vars.yml", "v: site\n")
	write(installer, "group_vars/all/vars.yml", "v: template\n")
	write(installer, "files/aipub/vars.yml", "not in the site\n")

	changed, _, err := GuardCopy(site, installer)
	if err != nil || len(changed) != 0 {
		t.Fatalf("first copy: %v %v", changed, err)
	}
	if got := kinds(t, site, "group_vars/all/vars.yml"); got != "installer" {
		t.Errorf("the installer's vars.yml before the first copy: %q", got)
	}
	if got := kinds(t, site, "files/aipub/vars.yml"); got != "" {
		t.Errorf("a file the site does not have is not copied, nor kept: %q", got)
	}
	copyAll()
	must(t, RecordCopy(site, installer))
	if got := kinds(t, site, "inventory.yml"); got != "copy" {
		t.Errorf("inventory.yml after the copy: %q", got)
	}
	// copying the same again: nothing to keep, nothing appended
	if changed, _, err := GuardCopy(site, installer); err != nil || len(changed) != 0 {
		t.Fatalf("again: %v %v", changed, err)
	}
	must(t, RecordCopy(site, installer))
	if got := kinds(t, site, "inventory.yml"); got != "copy" {
		t.Errorf("inventory.yml copied twice the same: %q", got)
	}

	// the site saved again: copying it is no edit on the installer side
	write(site, "inventory.yml", "site: 2\n")
	if changed, _, _ := GuardCopy(site, installer); len(changed) != 0 {
		t.Errorf("a new save is not an installer edit: %v", changed)
	}
	copyAll()
	must(t, RecordCopy(site, installer))

	// edited by hand on the installer side
	write(installer, "inventory.yml", "site: 2\nby_hand: true\n")
	changed, _, err = GuardCopy(site, installer)
	if err != nil || strings.Join(changed, ",") != "inventory.yml" {
		t.Fatalf("installer edit: %v %v", changed, err)
	}
	h, _ := ReadHistory(site)
	if e := h[len(h)-1]; e.Kind != HistoryInstaller || e.Content != "site: 2\nby_hand: true\n" {
		t.Errorf("the edit is not kept: %+v", e)
	}
	// seen once: the next copy goes on
	if changed, _, err := GuardCopy(site, installer); err != nil || len(changed) != 0 {
		t.Errorf("after it was kept: %v %v", changed, err)
	}
}

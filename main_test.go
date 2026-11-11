package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// --render writes the default audit policy with the other files, where saving
// would, when the audit preset is on.
func TestRenderWritesAuditPolicy(t *testing.T) {
	site := fileio.DefaultPaths(t.TempDir()) // a new site: nothing there
	s := state.DefaultState()
	s.ControlPlane.Audit = true
	dir := t.TempDir()
	render(site, s, dir) // the validation of an empty site fails; the files are written all the same
	data, err := os.ReadFile(filepath.Join(dir, "files", "aipub", "audit-policy.yaml"))
	if err != nil || !fileio.IsWizardPolicy(data) {
		t.Fatalf("audit policy not rendered: %v\n%s", err, data)
	}
	if _, err := os.Stat(site.AuditPolicy()); err == nil {
		t.Error("render wrote into the site")
	}

	s.ControlPlane.Audit = false
	off := t.TempDir()
	render(site, s, off)
	if _, err := os.Stat(filepath.Join(off, "files", "aipub", "audit-policy.yaml")); err == nil {
		t.Error("audit policy rendered with the preset off")
	}
}

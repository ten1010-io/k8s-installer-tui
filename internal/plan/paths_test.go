package plan

import (
	"os"
	"path/filepath"
	"testing"
)

// C1 is left out when the site is the installer's ansible directory, however
// the root is written (a / at the end, a // inside) or reached (a symlink): cp
// would refuse to copy the files onto themselves, on every run.
func TestCopyCommandsSameDirectory(t *testing.T) {
	root := t.TempDir()
	ansible := filepath.Join(root, "ansible")
	if err := os.Mkdir(ansible, 0o755); err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	for _, r := range []string{root, root + "/", root + "//"} {
		o.Root = r
		if c := CopyCommands(o, ansible); c != nil {
			t.Errorf("root %q, site %q: %d copies, want none: %q", r, ansible, len(c), c[0])
		}
	}
	link := filepath.Join(t.TempDir(), "installer")
	if err := os.Symlink(root, link); err == nil {
		o.Root = link
		if c := CopyCommands(o, ansible); c != nil {
			t.Errorf("root through a symlink: %d copies, want none: %q", len(c), c[0])
		}
	} else {
		t.Logf("no symlink here: %v", err)
	}
	o.Root = root
	if c := CopyCommands(o, t.TempDir()); len(c) == 0 {
		t.Error("a site saved elsewhere is not copied")
	}
}

// The archive path K4 takes is absolute: K4 runs in <root>/ansible, not where
// the program was started.
func TestAbsArchive(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"":                                "",
		"  ":                              "",
		"./" + ArchiveName:                filepath.Join(wd, ArchiveName),
		" " + ArchiveName + " ":           filepath.Join(wd, ArchiveName),
		filepath.Join(wd, "a", "..", "x"): filepath.Join(wd, "x"),
	} {
		if got := AbsArchive(in); got != want {
			t.Errorf("AbsArchive(%q) = %q, want %q", in, got, want)
		}
	}
}

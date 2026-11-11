package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The archive of the user registry (K4, push-user-registry-artifacts.yml). On a
// new site the registry is made by K2 and is empty, so without the archive every
// image the AIPub steps pull from it is missing (A1 waits on Cilium pods that
// stay in ImagePullBackOff until helm gives up). The archive is part of the
// package, so the screen looks for it where a package is usually unpacked.

// ArchiveName is the file of the package that K4 pushes.
const ArchiveName = "ki-cp-user-registry-artifacts.tgz"

// pushStepID is K4, the step that pushes the archive.
const pushStepID = "K4"

// NeedsArchive reports whether a run of this plan cannot get past A1 without
// the archive: a new site (the registry K2 makes is empty) with AIPub on. An
// existing site, or one without AIPub, may leave it out.
func NeedsArchive(aipubEnabled bool, o Options) bool {
	return aipubEnabled && !o.Existing
}

// AbsArchive is the archive path as K4 takes it: absolute. K4 runs in
// <root>/ansible, where a path relative to the directory the program was started
// in names another file or none, after CheckArchive found it here. Empty stays
// empty.
func AbsArchive(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// CheckArchive refuses an archive path that is not a regular file this user
// can read, before anything runs: K4 would otherwise fail after the steps
// before it, or push nothing.
func CheckArchive(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return errors.New("아카이브 " + path + " 를 찾을 수 없습니다: " + err.Error())
	}
	if !fi.Mode().IsRegular() {
		return errors.New("아카이브 " + path + " 는 일반 파일이 아닙니다")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("아카이브 " + path + " 를 읽을 수 없습니다: " + err.Error())
	}
	f.Close()
	return nil
}

// ArchiveDirs are where FindArchive looks by default: the directory of this
// program, the working directory, and every directory one level under the home
// directory (the package unpacked as ~/<package>/).
func ArchiveDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if entries, err := os.ReadDir(home); err == nil {
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, filepath.Join(home, e.Name()))
				}
			}
		}
	}
	return dirs
}

// FindArchive is every readable ArchiveName directly in one of dirs, each path
// once, in the order of dirs.
func FindArchive(dirs []string) []string {
	var found []string
	seen := map[string]bool{}
	for _, d := range dirs {
		p := filepath.Join(d, ArchiveName)
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if seen[p] || CheckArchive(p) != nil {
			continue
		}
		seen[p] = true
		found = append(found, p)
	}
	return found
}

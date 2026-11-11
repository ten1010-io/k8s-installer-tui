package fileio

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The history of the settings is history.jsonl beside the saved files: one JSON
// object a line, only ever appended to. Saving writes every file whole (atomic,
// by rename) and appends what it wrote; what was there before is appended first
// when the history does not have it (a file written by hand, or by a version of
// this tool without the history), so that nothing a save replaces is lost. It
// takes the place of <file>.bak, which kept one version.
//
// The copy to the installer (C1 of the run) uses it too: a file there that the
// history does not have was changed on the installer side, by hand, since the
// last copy; the copy appends it and stops once.
//
// There is no lock: two writers at once (two wizards, a save beside
// --fill-auth-config) may append an entry twice or call a save an edit. Each
// append is one write of whole lines, so nothing is lost.
//
// Read it with jq (fromjson? passes over a line cut short), e.g. the last saved
// vars.yml:
//
//	jq -nrR '[inputs | fromjson? | select(.kind=="save" and .path=="group_vars/all/vars.yml")] | last | .content' history.jsonl
const HistoryFile = "history.jsonl"

// The kinds of a history entry.
const (
	HistorySave      = "save"      // written by saving, Path in the site
	HistoryEdit      = "edit"      // in the site before a save, not written by it
	HistoryInstaller = "installer" // in the installer before a copy, not copied there
	HistoryCopy      = "copy"      // copied to the installer, Path in its ansible directory
)

// HistoryEntry is a line of history.jsonl. Content is the file as text, or in
// base64 when it is not UTF-8 (Encoding "base64"): JSON would replace the bytes
// it can not hold.
type HistoryEntry struct {
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Path     string    `json:"path"` // relative, with /
	SHA256   string    `json:"sha256"`
	Encoding string    `json:"encoding,omitempty"`
	Content  string    `json:"content"`
}

// Data is the content of the file the entry holds.
func (e HistoryEntry) Data() []byte {
	if e.Encoding == "base64" {
		data, _ := base64.StdEncoding.DecodeString(e.Content)
		return data
	}
	return []byte(e.Content)
}

// SiteFiles are the files saving writes and the run copies to the installer,
// relative to the site (and to the ansible directory of the installer).
func SiteFiles() []string {
	return []string{
		"inventory.yml",
		"group_vars/all/vars.yml",
		"files/aipub/vars.yml",
		"files/" + state.WebhookConfigFile,
		"files/" + state.JWKSConfigFile,
		"files/" + state.AuditPolicyFile,
	}
}

// HistoryPath is the history of the site saved in dir.
func HistoryPath(dir string) string { return filepath.Join(dir, HistoryFile) }

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func entry(kind, rel string, data []byte) HistoryEntry {
	e := HistoryEntry{Time: time.Now().UTC(), Kind: kind, Path: rel, SHA256: sum(data), Content: string(data)}
	if !utf8.Valid(data) {
		e.Encoding, e.Content = "base64", base64.StdEncoding.EncodeToString(data)
	}
	return e
}

// ReadHistory is the history of the site saved in dir, oldest first; none when
// there is no file. A line that does not parse (the end of a write cut short) is
// left out.
func ReadHistory(dir string) ([]HistoryEntry, error) {
	f, err := os.Open(HistoryPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []HistoryEntry
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		var e HistoryEntry
		if len(line) > 0 && json.Unmarshal(line, &e) == nil && e.Kind != "" {
			out = append(out, e)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// appendHistory appends entries to the history of dir, private (it holds the
// settings whole). After a line cut short it starts a new line, so that the
// entries appended are not lost in it.
func appendHistory(dir string, entries ...HistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var b bytes.Buffer
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	_, statErr := os.Stat(HistoryPath(dir))
	f, err := os.OpenFile(HistoryPath(dir), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		ownLikeDir(HistoryPath(dir), dir)
	}
	out := b.Bytes()
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err == nil && last[0] != '\n' {
			out = append([]byte{'\n'}, out...)
		}
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// last is the hash of the latest entry of rel among the kinds given, "" when
// there is none.
func last(h []HistoryEntry, rel string, kinds ...string) string {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].Path != rel {
			continue
		}
		for _, k := range kinds {
			if h[i].Kind == k {
				return h[i].SHA256
			}
		}
	}
	return ""
}

// known reports whether the history holds rel with that hash, in any entry.
func known(h []HistoryEntry, rel, sha string) bool {
	for _, e := range h {
		if e.Path == rel && e.SHA256 == sha {
			return true
		}
	}
	return false
}

// relTo is path relative to dir with /, or path itself (with /) when it is not
// under dir.
func relTo(dir, path string) string {
	if r, err := filepath.Rel(dir, path); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(path)
}

// writeAtomic writes data to path through a file beside it renamed over it: a
// write cut short leaves the old file, never half of the new one.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // gone after the rename; left only when something failed
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeWithHistory writes data to path of the site saved in dir. The file there
// is appended to the history first when the history does not have it as it is;
// then what is written. Nothing is written, nor appended, when the file has
// data already.
func writeWithHistory(dir, path string, data []byte, h []HistoryEntry) ([]HistoryEntry, error) {
	rel := relTo(dir, path)
	var before []HistoryEntry
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, data):
		return h, nil
	case err == nil && last(h, rel, HistorySave, HistoryEdit) != sum(old):
		before = append(before, entry(HistoryEdit, rel, old))
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return h, err
	}
	if err := appendHistory(dir, before...); err != nil {
		return h, err
	}
	h = append(h, before...)
	if err := writeAtomic(path, data); err != nil {
		return h, err
	}
	saved := entry(HistorySave, rel, data)
	if err := appendHistory(dir, saved); err != nil {
		return h, err
	}
	return append(h, saved), nil
}

// keepInstaller is the entry that keeps dst, the installer's file rel, in the
// history before something replaces it, when the history does not have it.
// changed: something was copied there before, and dst is none of the contents
// the history has of rel - it was edited on the installer side since.
func keepInstaller(h []HistoryEntry, rel string, dst []byte) (keep *HistoryEntry, changed bool) {
	sha := sum(dst)
	if known(h, rel, sha) {
		return nil, false
	}
	e := entry(HistoryInstaller, rel, dst)
	return &e, last(h, rel, HistoryCopy, HistoryInstaller) != ""
}

// GuardCopy is what the run checks before it copies the site saved in siteDir
// to the ansible directory of the installer. A file there that is not the
// site's and that the history does not have is appended to it, so that nothing
// is lost by the copy. When something was copied there before, such a file was
// edited on the installer side since: it is in changed, for the run to stop
// once and say so; the next copy, the history having it now, goes on. Before
// the first copy, what the installer has (its templates, or a site from before
// this tool) is kept without stopping. A file this user can not read (the copy
// runs with sudo) is in unchecked, and copied over as before.
func GuardCopy(siteDir, ansibleDir string) (changed, unchecked []string, err error) {
	h, err := ReadHistory(siteDir)
	if err != nil {
		return nil, nil, err
	}
	var add []HistoryEntry
	for _, rel := range SiteFiles() {
		src, err := os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue // not copied
		}
		if err != nil {
			return nil, nil, err
		}
		dst, err := os.ReadFile(filepath.Join(ansibleDir, filepath.FromSlash(rel)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case errors.Is(err, fs.ErrPermission):
			unchecked = append(unchecked, rel)
			continue
		case err != nil:
			return nil, nil, err
		}
		if bytes.Equal(src, dst) {
			continue
		}
		if keep, edited := keepInstaller(h, rel, dst); keep != nil {
			add = append(add, *keep)
			h = append(h, *keep)
			if edited {
				changed = append(changed, rel)
			}
		}
	}
	return changed, unchecked, appendHistory(siteDir, add...)
}

// RecordCopy appends to the history of the site what the run copied to the
// installer: each file of the site that is not what the history says is there,
// and that the installer has now (the copy of an optional file may have failed
// without failing the step).
func RecordCopy(siteDir, ansibleDir string) error {
	h, err := ReadHistory(siteDir)
	if err != nil {
		return err
	}
	var add []HistoryEntry
	for _, rel := range SiteFiles() {
		src, err := os.ReadFile(filepath.Join(siteDir, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if dst, err := os.ReadFile(filepath.Join(ansibleDir, filepath.FromSlash(rel))); err == nil && !bytes.Equal(dst, src) {
			continue // not copied
		}
		if last(h, rel, HistoryCopy, HistoryInstaller) != sum(src) {
			add = append(add, entry(HistoryCopy, rel, src))
		}
	}
	return appendHistory(siteDir, add...)
}

package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func TestCheckStorageClass(t *testing.T) {
	s := aipubState() // nfs-client, node1 first
	o := testOptions()
	o.Existing = true
	var scripts []string
	answer := func(out string) RunFunc {
		return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
			scripts = append(scripts, args[len(args)-1])
			return []byte(out), nil
		}
	}

	r := CheckStorageClass(context.Background(), o, s, answer("node1 | CHANGED | rc=0 | (stdout) storageclass.storage.k8s.io/nfs-client\n"))
	if !r.OK || r.Warn || r.Skipped || r.Lines[0] != "node1: StorageClass nfs-client 있음" {
		t.Errorf("present: %+v", r)
	}
	// read only, on the first control plane node, as root
	if got := scripts[0]; !strings.Contains(got, "ansible node1 -i inventory.yml -b -o -m command") ||
		!strings.Contains(got, "kubectl --kubeconfig /etc/kubernetes/admin.conf get storageclass nfs-client -o name") {
		t.Errorf("command: %s", got)
	}

	r = CheckStorageClass(context.Background(), o, s, answer(
		"node1 | FAILED | rc=1 | (stdout) (stderr) Error from server (NotFound): storageclasses.storage.k8s.io \"nfs-client\" not found\n"))
	if !r.OK || !r.Warn || !strings.Contains(r.Lines[0], "node1: StorageClass nfs-client 가 없습니다") {
		t.Errorf("absent: %+v", r)
	}

	// kubectl could not ask: not the same as absent
	r = CheckStorageClass(context.Background(), o, s, answer("node1 | UNREACHABLE! => {\"msg\": \"x\"}\n"))
	if !r.OK || !r.Warn || strings.Contains(r.Lines[0], "가 없습니다") || !strings.Contains(r.Lines[0], "확인하지 못했습니다") {
		t.Errorf("unreachable: %+v", r)
	}
	r = CheckStorageClass(context.Background(), o, s, answer("bash: ansible: command not found\n"))
	if !r.Warn || !strings.Contains(r.Lines[0], "command not found") {
		t.Errorf("no ansible: %+v", r)
	}

	// the shell P6 chose is used by this become too
	o.ShellExecutable = "/usr/bin/dash"
	scripts = nil
	CheckStorageClass(context.Background(), o, s, answer(""))
	if !strings.Contains(scripts[0], "-b -e ansible_shell_executable=/usr/bin/dash -o") {
		t.Errorf("shell: %s", scripts[0])
	}
}

func TestCheckStorageClassSkipped(t *testing.T) {
	called := false
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	off := aipubState()
	off.AipubEnabled = false
	noClass := aipubState()
	noClass.AipubHaModeStorageClass = ""
	cases := []struct {
		name     string
		existing bool
		s        *state.AppState
		want     string
	}{
		{"new site", false, aipubState(), "새 사이트"},
		{"AIPub off", true, off, "AIPub 을 설치하지 않음"},
		{"no class", true, noClass, "정해지지 않음"},
	}
	for _, c := range cases {
		o := testOptions()
		o.Existing = c.existing
		r := CheckStorageClass(context.Background(), o, c.s, run)
		if !r.OK || !r.Skipped || !strings.Contains(r.Lines[0], c.want) {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
	if called {
		t.Error("ran a command although skipped")
	}
}

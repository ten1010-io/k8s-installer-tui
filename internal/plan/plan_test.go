package plan

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func ids(steps []Step) string {
	var out []string
	for _, s := range steps {
		id := s.ID
		if s.Skip {
			id = "(" + id + ")"
		}
		out = append(out, id)
	}
	return strings.Join(out, " ")
}

func aipubState() *state.AppState {
	s := state.DefaultState()
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.K8sIngressVIP = "192.168.0.100"
	return s
}

func TestOrder(t *testing.T) {
	o := DefaultOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	if got, want := ids(Build(aipubState(), o)), "P1 P2 K1 K2 K3 K4 A1 A2 A3 A4 A5"; got != want {
		t.Errorf("with AIPub: %s, want %s", got, want)
	}
	if got, want := ids(Build(state.DefaultState(), o)), "P1 P2 K1 K2 K3 K4"; got != want {
		t.Errorf("without AIPub: %s, want %s", got, want)
	}
	o.ArchivePath, o.Monitoring = "", false
	if got, want := ids(Build(aipubState(), o)), "P1 P2 K1 K2 K3 (K4) A1 A2 A3 (A4) A5"; got != want {
		t.Errorf("no archive, no monitoring: %s, want %s", got, want)
	}
}

func TestScript(t *testing.T) {
	o := DefaultOptions()
	o.ArchivePath = "/media/usb dir/a.tgz"
	steps := Build(aipubState(), o)

	inPlace := Script(steps, o, "/opt/k8s-installer/ansible")
	if strings.Contains(inPlace, "sudo cp") {
		t.Error("copies the files although they were saved where k8s-installer reads them")
	}
	for _, want := range []string{
		"set -euo pipefail",
		"source /opt/k8s-installer/venv/bin/activate",
		"cd /opt/k8s-installer/ansible",
		"ansible-playbook -i inventory.yml playbooks/setup-cluster.yml",
		"ansible-playbook -i inventory.yml playbooks/push-user-registry-artifacts.yml -e 'archive=/media/usb dir/a.tgz'",
		"ansible-playbook -i inventory.yml aipub-playbooks/setup-aipub.yml",
	} {
		if !strings.Contains(inPlace, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	// network before cert before base before monitoring before aipub
	last := -1
	for _, pb := range []string{"setup-network", "setup-cert", "setup-base", "setup-monitoring", "setup-aipub"} {
		i := strings.Index(inPlace, "aipub-playbooks/"+pb+".yml")
		if i < last {
			t.Errorf("%s out of order", pb)
		}
		last = i
	}

	elsewhere := Script(steps, o, "/home/ops/site-a")
	if !strings.Contains(elsewhere, "sudo cp /home/ops/site-a/inventory.yml /opt/k8s-installer/ansible/inventory.yml") {
		t.Errorf("no copy of files saved elsewhere:\n%s", elsewhere)
	}

	o.ArchivePath = ""
	skipped := Script(Build(aipubState(), o), o, "/opt/k8s-installer/ansible")
	if !strings.Contains(skipped, "# K4 push-user-registry-artifacts: 건너뜀") {
		t.Error("a skipped step is not left as a comment")
	}
}

func fakeRun(out string, err error) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte(out), err
	}
}

func TestCheckSSH(t *testing.T) {
	o := DefaultOptions()
	ok := CheckSSH(context.Background(), o, fakeRun(
		"node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \n", nil))
	if !ok.OK || ok.Lines[0] != "정상: node1 node2" {
		t.Errorf("all reachable: %+v", ok)
	}

	bad := CheckSSH(context.Background(), o, fakeRun(
		"node1 | CHANGED | rc=0 | (stdout) \n"+
			"node2 | UNREACHABLE! => {\"msg\": \"ssh: connect to host 10.0.0.2 port 22: No route to host\"}\n"+
			"node3 | FAILED | rc=1 | (stdout) (stderr) sudo: a password is required\n", errors.New("exit status 4")))
	if bad.OK || len(bad.Lines) != 3 ||
		!strings.Contains(bad.Lines[1], "node2: 접속 안 됨") || !strings.Contains(bad.Lines[2], "node3: sudo") {
		t.Errorf("one unreachable, one without sudo: %+v", bad)
	}

	none := CheckSSH(context.Background(), o, fakeRun("bash: ansible: command not found\n", errors.New("exit status 127")))
	if none.OK || !strings.Contains(none.Lines[0], "command not found") {
		t.Errorf("ansible missing: %+v", none)
	}
}

func TestCheckInstalled(t *testing.T) {
	o := DefaultOptions()
	o.Root = t.TempDir()
	if CheckInstalled(o).OK {
		t.Error("ok without a release file")
	}
	if err := os.WriteFile(o.Root+"/release", []byte("1.36.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if r := CheckInstalled(o); !r.OK || r.Lines[0] != "설치된 릴리스 1.36.0" {
		t.Errorf("%+v", r)
	}
}

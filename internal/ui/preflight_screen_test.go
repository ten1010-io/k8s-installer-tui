package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

// The plan screen groups the checks, says in a line what each looks at (with the
// Preflight hint of the variable behind it), and shows a warning apart from a
// failure and from a skip. The commands of the plan take the shell P6 found.
func TestPlanScreenPreflightPresentation(t *testing.T) {
	d, paths := newDriver(t)
	s := d.app.appState
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.K8sIngressVIP = "192.168.0.100"

	opts := plan.DefaultOptions()
	opts.Root = t.TempDir() // no release file: P1 fails
	opts.Existing = true
	s9 := screens.NewS9Install(s, paths, opts)
	s9.SetSize(110, 120)
	s9.SyncFromState(s)

	// a site whose sudoers forbids /bin/sh on node2, a DNS that answers another
	// address, and a cluster without the StorageClass
	ok := "node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \nnode3 | CHANGED | rc=0 | (stdout) \n"
	s9.SetRunner(func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if name == "getent" {
			return []byte("10.0.0.5   " + args[1] + "\n"), nil
		}
		script := args[len(args)-1]
		switch {
		case strings.Contains(script, "command -v dash"):
			return []byte("node2 | CHANGED | rc=0 | (stdout) /usr/bin/dash\n"), nil
		case strings.Contains(script, "sudo -n /bin/sh -c true"):
			return []byte("node1 | CHANGED | rc=0 | (stdout) \nnode2 | FAILED | rc=1 | (stdout) (stderr) not allowed\nnode3 | CHANGED | rc=0 | (stdout) \n"), nil
		case strings.Contains(script, "sudo -n true"):
			return []byte(ok), nil
		case strings.Contains(script, "get storageclass"):
			return []byte("node1 | FAILED | rc=1 | (stdout) (stderr) Error from server (NotFound): storageclasses \"nfs-client\" not found\n"), nil
		}
		return nil, nil // slurp of P3: nothing readable
	})
	s9.Update(tea.KeyMsg{Type: tea.KeyDown}) // off the archive path
	s9.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := s9.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	s9.Update(cmd())
	view := ansi.ReplaceAllString(s9.View(), "")
	t.Logf("plan screen:\n%s", view)

	for _, want := range []string{
		"사전 확인", "실행 순서", "✓ 통과   ! 경고",
		"보는 것: 모든 노드에서 sudo -n /bin/sh -c true",
		"힌트 (ansible_user): 모든 노드에서 `sudo -n /bin/sh -c true` 가 통과해야 함",
		"힌트 (aipub_ingress_zone): `*.<이 도메인>` DNS 가 인그레스 VIP 를 가리켜야",
		"힌트 (aipub_ha_mode_storage_class): `kubectl get storageclass <값>` 이 있어야 함",
		"✗ P1 ", // failure
		"! P6 ", // warnings
		"! P7 ",
		"! P8 ",
		"sudo 가 /bin/sh 를 거부하는 노드: node2",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("plan screen lacks %q", want)
		}
	}
	// the long note of a check stays out of the screen (it is in the script)
	if strings.Contains(view, "configure-control-node-ssh.sh") || strings.Contains(view, "slurp") {
		t.Error("the long notes of the checks are still on the screen")
	}

	// the warning of P6 changed the commands: both the screen's and the script's
	s9.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmds := ansi.ReplaceAllString(s9.View(), ""); !strings.Contains(cmds, "ansible_shell_executable=/usr/bin/dash") {
		t.Errorf("the commands lack the shell executable:\n%s", cmds)
	}
}

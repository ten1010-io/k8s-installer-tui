package plan

import (
	"context"
	"strings"
	"testing"

)

// becomeNodes answers the three ansible runs of P6 by what the script runs:
// plain sudo, sudo with /bin/sh, and the dash probe. Each is the output of
// ansible -o as the node lines it prints.
func becomeNodes(plain, viaSh, probe string) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		script := args[len(args)-1]
		switch {
		case strings.Contains(script, "command -v dash"):
			return []byte(probe), nil
		case strings.Contains(script, "sudo -n /bin/sh -c true"):
			return []byte(viaSh), nil
		case strings.Contains(script, "sudo -n true"):
			return []byte(plain), nil
		}
		return nil, nil
	}
}

const (
	nodeOK      = " | CHANGED | rc=0 | (stdout) \n"
	nodeRefused = " | FAILED | rc=1 | (stdout) (stderr) Sorry, user ops is not allowed to execute '/bin/sh -c true' as root\n"
)

func TestCheckBecomeShellAllOK(t *testing.T) {
	out := "node1" + nodeOK + "node2" + nodeOK
	r, shell := CheckBecomeShell(context.Background(), testOptions(), becomeNodes(out, out, ""))
	if !r.OK || r.Warn || shell != "" || !strings.Contains(r.Lines[0], "node1 node2") {
		t.Errorf("%+v shell=%q", r, shell)
	}
	// nothing asks for the flag
	for _, st := range Build(aipubState(), testOptions()) {
		if strings.Contains(strings.Join(st.Args, " "), "-e ansible_shell_executable") {
			t.Errorf("%s has the flag without it being needed: %v", st.ID, st.Args)
		}
	}
}

func TestCheckBecomeShellRefusedWithDash(t *testing.T) {
	plain := "node1" + nodeOK + "node2" + nodeOK + "node3" + nodeOK
	viaSh := "node1" + nodeOK + "node2" + nodeRefused + "node3" + nodeRefused
	probe := "node2 | CHANGED | rc=0 | (stdout) /usr/bin/dash\nnode3 | CHANGED | rc=0 | (stdout) /usr/bin/dash\n"
	var scripts []string
	run := becomeNodes(plain, viaSh, probe)
	r, shell := CheckBecomeShell(context.Background(), testOptions(), func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		scripts = append(scripts, args[len(args)-1])
		return run(ctx, dir, name, args...)
	})
	if !r.OK || !r.Warn || shell != "/usr/bin/dash" {
		t.Fatalf("%+v shell=%q", r, shell)
	}
	all := strings.Join(r.Lines, "\n")
	for _, want := range []string{"정상 (sudo -n /bin/sh -c true): node1", "sudo 가 /bin/sh 를 거부하는 노드: node2 node3",
		"-e ansible_shell_executable=/usr/bin/dash"} {
		if !strings.Contains(all, want) {
			t.Errorf("lacks %q:\n%s", want, all)
		}
	}
	// the probe goes to the refused nodes only, and nothing here asks for root
	last := scripts[len(scripts)-1]
	if !strings.Contains(last, "ansible node2,node3 ") || strings.Contains(last, " -b ") {
		t.Errorf("probe: %s", last)
	}

	// every playbook of the plan, the screen's and the saved script's, takes the
	// flag next to the -e that leaves JWKS out; the control node commands do not
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	o.ShellExecutable = shell
	s := aipubState()
	s.ControlPlane.Audit = true
	steps := Build(s, o)
	flags := 0
	for _, st := range steps {
		has := strings.Contains(strings.Join(st.Args, " "), "-e ansible_shell_executable=/usr/bin/dash") ||
			strings.HasSuffix(strings.Join(st.Args, " "), "-e ansible_shell_executable=/usr/bin/dash")
		switch st.Kind {
		case Playbook:
			flags++
			if !has {
				t.Errorf("%s lacks the flag: %v", st.ID, st.Args)
			}
		default:
			if has {
				t.Errorf("%s (not a playbook) has the flag", st.ID)
			}
		}
		if st.ID == "K3" && (len(st.Args) != 4 || !strings.Contains(st.Args[1], "audit-policy-file")) {
			t.Errorf("K3 lost its own -e: %v", st.Args)
		}
	}
	script := Script(steps, o, "/opt/k8s-installer/ansible")
	if n := strings.Count(script, "ansible_shell_executable=/usr/bin/dash"); n != flags {
		t.Errorf("script has the flag %d times, want %d:\n%s", n, flags, script)
	}
	if !strings.Contains(script, "ansible-playbook -i inventory.yml aipub-playbooks/setup-aipub.yml -e ansible_shell_executable=/usr/bin/dash") {
		t.Errorf("setup-aipub line:\n%s", script)
	}
	// Build leaves the steps of an Options without it alone
	o.ShellExecutable = ""
	if strings.Contains(Script(Build(s, o), o, "/opt/k8s-installer/ansible"), "ansible_shell_executable=/usr") {
		t.Error("flag without ShellExecutable")
	}
}

func TestCheckBecomeShellRefusedWithoutDash(t *testing.T) {
	plain := "node1" + nodeOK + "node2" + nodeOK
	viaSh := "node1" + nodeRefused + "node2" + nodeRefused
	cases := map[string]struct{ probe, want string }{
		"no dash":        {"node1 | CHANGED | rc=0 | (stdout) /usr/bin/dash\nnode2 | FAILED | rc=3 | (stdout) missing\n", "node2: dash 가 없습니다"},
		"sudo refuses":   {"node1 | FAILED | rc=4 | (stdout) /usr/bin/dash\nnode2 | CHANGED | rc=0 | (stdout) /usr/bin/dash\n", "node1: dash (/usr/bin/dash) 가 있으나 sudo 가 거부합니다"},
		"unreachable":    {"node1 | UNREACHABLE! => {\"msg\": \"x\"}\nnode2 | CHANGED | rc=0 | (stdout) /usr/bin/dash\n", "node1: dash 를 확인하지 못했습니다"},
		"different dash": {"node1 | CHANGED | rc=0 | (stdout) /usr/bin/dash\nnode2 | CHANGED | rc=0 | (stdout) /bin/dash\n", "노드마다 dash 경로가 다릅니다"},
	}
	for name, c := range cases {
		r, shell := CheckBecomeShell(context.Background(), testOptions(), becomeNodes(plain, viaSh, c.probe))
		all := strings.Join(r.Lines, "\n")
		if r.OK || shell != "" || !strings.Contains(all, c.want) || !strings.Contains(all, "sudoers 가 /bin/sh 를 허용하게") {
			t.Errorf("%s: %+v shell=%q", name, r, shell)
		}
	}
}

// A node that fails plain sudo is P2's: it is not a refusal of /bin/sh.
func TestCheckBecomeShellLeavesNoSudoToP2(t *testing.T) {
	plain := "node1" + nodeOK + "node2 | FAILED | rc=1 | (stdout) (stderr) sudo: a password is required\n"
	viaSh := "node1" + nodeOK + "node2 | FAILED | rc=1 | (stdout) (stderr) sudo: a password is required\n"
	r, shell := CheckBecomeShell(context.Background(), testOptions(), becomeNodes(plain, viaSh, ""))
	if !r.OK || r.Warn || shell != "" || !strings.Contains(strings.Join(r.Lines, "\n"), "P2 에서 봅니다: node2") {
		t.Errorf("%+v", r)
	}
	r, _ = CheckBecomeShell(context.Background(), testOptions(), fakeRun("bash: ansible: command not found\n", nil))
	if r.OK || !strings.Contains(r.Lines[0], "command not found") {
		t.Errorf("ansible missing: %+v", r)
	}
}

// The slurp of P3 is a become: it takes the shell too.
func TestSlurpTakesShellExecutable(t *testing.T) {
	o := testOptions()
	o.ShellExecutable = "/usr/bin/dash"
	var calls []string
	_, _ = ReadLegacy(context.Background(), o, aipubState(), fakeSlurp("node1", nil, &calls))
	if len(calls) == 0 || !strings.Contains(calls[0], "-b -e ansible_shell_executable=/usr/bin/dash -o -m slurp") {
		t.Errorf("slurp: %v", calls)
	}
}

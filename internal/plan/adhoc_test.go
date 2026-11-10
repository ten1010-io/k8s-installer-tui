package plan

import (
	"context"
	"strings"
	"testing"
)

// The lines are what ansible-core 2.19 printed against nodes in containers: a
// failed command comes twice (the rc line, then the JSON line), and an
// unreachable node as "UNREACHABLE!:" (older releases: "UNREACHABLE! =>").
const adhoc219 = `n-nodash | FAILED | rc=4 | (stdout) /usr/bin/dash (stderr) Sorry, user tester is not allowed to execute '/usr/bin/dash -c true' as root on n-nodash.
n-nodash | FAILED! => {"ansible_facts": {"discovered_interpreter_python": "/usr/bin/python3.12"},"changed": true,"msg": "non-zero return code","rc": 4}
n-ok1 | CHANGED | rc=0 | (stdout) /usr/bin/dash
n-down | UNREACHABLE!: Task failed: Failed to connect to the host via ssh: ssh: Could not resolve hostname n-down: Name or service not known
n-old | UNREACHABLE! => {"changed": false,"msg": "Failed to connect to the host via ssh","unreachable": true}
n-json | FAILED! => {"changed": false,"msg": "file not found"}
`

func TestParseAdhocFormats(t *testing.T) {
	res := parseAdhoc([]byte(adhoc219))
	if a := res["n-nodash"]; a.Status != "FAILED" || a.RC != "4" || a.Stdout != "/usr/bin/dash" {
		t.Errorf("the JSON line must not replace the rc line: %+v", a)
	}
	if a := res["n-ok1"]; !a.ok() || a.Stdout != "/usr/bin/dash" {
		t.Errorf("%+v", a)
	}
	for _, n := range []string{"n-down", "n-old"} {
		if a := res[n]; a.Status != "UNREACHABLE" || a.ok() {
			t.Errorf("%s: %+v", n, a)
		}
	}
	if a := res["n-json"]; a.Status != "FAILED" || a.RC != "" || a.ok() {
		t.Errorf("%+v", a)
	}
}

// ansible-core 2.17 prints a failed command as the JSON line only.
const adhoc217 = `n-nodash | FAILED! => {"ansible_facts": {"discovered_interpreter_python": "/usr/bin/python3.12"},"changed": true,"cmd": "p=$(command -v dash)","msg": "non-zero return code","rc": 4,"stderr": "sudo: a password is required","stdout": "/usr/bin/dash","stdout_lines": ["/usr/bin/dash"]}
n-nodash2 | FAILED! => {"changed": true,"msg": "non-zero return code","rc": 3,"stderr": "","stdout": "missing"}
n-ok1 | CHANGED | rc=0 | (stdout) /usr/bin/dash
n-down | UNREACHABLE!: Failed to connect to the host via ssh: ssh: Could not resolve hostname n-down
`

func TestParseAdhoc217ReadsTheJSONLine(t *testing.T) {
	res := parseAdhoc([]byte(adhoc217))
	if a := res["n-nodash"]; a.Status != "FAILED" || a.RC != "4" || a.Stdout != "/usr/bin/dash" || !strings.Contains(a.Raw, "a password is required") {
		t.Errorf("%+v", a)
	}
	if a := res["n-nodash2"]; a.RC != "3" || a.Stdout != "missing" {
		t.Errorf("%+v", a)
	}
	if a := res["n-down"]; a.Status != "UNREACHABLE" {
		t.Errorf("%+v", a)
	}
}

func TestCheckSSHReadsBothUnreachableFormats(t *testing.T) {
	out := "n-ok1 | CHANGED | rc=0 | (stdout) \n" +
		"n-nopw | FAILED | rc=1 | (stdout)  (stderr) sudo: a password is required\n" +
		`n-nopw | FAILED! => {"changed": true,"msg": "non-zero return code","rc": 1}` + "\n" +
		"n-down | UNREACHABLE!: Task failed: Failed to connect to the host via ssh\n" +
		`n-old | UNREACHABLE! => {"msg": "x","unreachable": true}` + "\n"
	r := CheckSSH(context.Background(), testOptions(), func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(out), nil
	})
	got := strings.Join(r.Lines, "\n")
	for _, want := range []string{"정상: n-ok1", "n-down: 접속 안 됨", "n-old: 접속 안 됨", "n-nopw: sudo 비밀번호 필요"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if r.OK {
		t.Error("must fail")
	}
}

func TestProbeKeepsRefusedDash(t *testing.T) {
	plain := "n-nodash | CHANGED | rc=0 | (stdout) \n"
	viaSh := "n-nodash | FAILED | rc=1 | (stdout)  (stderr) Sorry, user tester is not allowed to execute '/bin/sh -c true' as root on n-nodash.\n" +
		`n-nodash | FAILED! => {"changed": true,"msg": "non-zero return code","rc": 1}` + "\n"
	r, shell := CheckBecomeShell(context.Background(), testOptions(), becomeNodes(plain, viaSh, adhoc219))
	if r.OK || shell != "" || !strings.Contains(strings.Join(r.Lines, "\n"), "n-nodash: dash (/usr/bin/dash) 가 있으나 sudo 가 거부합니다") {
		t.Errorf("%+v shell=%q", r, shell)
	}
}

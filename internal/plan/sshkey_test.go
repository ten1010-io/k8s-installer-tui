package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// What ansible-inventory --list printed for an inventory of k8s-installer, after a
// warning ansible writes before the JSON.
const inventoryList = `[WARNING]: Invalid characters were found in group names but not replaced
{
    "_meta": {
        "hostvars": {
            "cp1": {"ansible_host": "10.0.0.11", "ansible_user": "ops"},
            "gpu1": {"ansible_host": "10.0.0.21", "ansible_port": 2222, "ansible_ssh_user": "alice"},
            "gpu2": {"ansible_host": "10.0.0.22", "ansible_ssh_port": "2200", "ansible_ssh_private_key_file": "/k", "ansible_become_pass": "x"},
            "localhost": {"ansible_connection": "local"}
        }
    },
    "all": {"children": ["ungrouped", "k8s_node"]},
    "k8s_node": {"hosts": ["cp1", "gpu1", "gpu2", "bare"]}
}
`

func TestParseInventory(t *testing.T) {
	nodes, warnings, err := parseInventory([]byte(inventoryList), "me")
	if err != nil {
		t.Fatal(err)
	}
	want := []SSHNode{
		{"bare", "bare", "22", "me"}, // no host vars: its name, port 22, the user running this
		{"cp1", "10.0.0.11", "22", "ops"},
		{"gpu1", "10.0.0.21", "2222", "alice"},
		{"gpu2", "10.0.0.22", "2200", "me"},
	}
	if len(nodes) != len(want) {
		t.Fatalf("nodes %+v, want %+v", nodes, want)
	}
	for i := range want {
		if nodes[i] != want[i] {
			t.Errorf("node %d: %+v, want %+v", i, nodes[i], want[i])
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "노드 gpu2 ") ||
		!strings.Contains(warnings[0], "ansible_ssh_private_key_file, ansible_become_pass") {
		t.Errorf("warnings %q", warnings)
	}

	if _, _, err := parseInventory([]byte(`{"_meta": {"hostvars": {"n": {"ansible_user": "a b"}}}}`), "me"); err == nil {
		t.Error("a user with a space is taken")
	}
	if _, _, err := parseInventory([]byte("ERROR! the inventory is broken\n"), "me"); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("no JSON: %v", err)
	}
}

// The last lines OpenSSH 9.6 and 8.0 printed against the nodes of the container
// test, and the warning of a changed host key.
func TestClassifySSH(t *testing.T) {
	changed := `@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
Host key for [127.0.0.1]:2222 has changed and you have requested strict checking.
Host key verification failed.
`
	for _, c := range []struct {
		out  string
		want KeyState
	}{
		{"Warning: Permanently added '[127.0.0.1]:2222' (ED25519) to the list of known hosts.\r\nbob@127.0.0.1: Permission denied (publickey,password).\r\n", KeyRefused},
		{"Received disconnect from 10.0.0.1 port 22:2: Too many authentication failures\n", KeyRefused},
		{"ssh: connect to host 127.0.0.1 port 2223: Connection refused\n", KeyUnreachable},
		{"ssh: connect to host 10.0.0.9 port 22: Connection timed out\n", KeyUnreachable},
		{"ssh: Could not resolve hostname n-down: Name or service not known\n", KeyUnreachable},
		{changed, KeyHostChanged},
	} {
		got, detail := classifySSH([]byte(c.out))
		if got != c.want || detail == "" || strings.ContainsAny(detail, "\r\n") {
			t.Errorf("%q: %v %q, want %v", c.out, got, detail, c.want)
		}
	}
}

// sshNodes fakes the nodes: by user@host, what ssh prints for the key and for
// sudo, and a failure when the text is not empty.
type fakeNode struct{ key, sudo string }

type call struct {
	name string
	args []string
	env  []string
}

type recorder struct {
	mu    sync.Mutex
	calls []call
}

func (r *recorder) add(ctx context.Context, name string, args []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call{name, append([]string{}, args...), EnvOf(ctx)})
}

func sshNodes(rec *recorder, inventory, ansible string, nodes map[string]*fakeNode) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		rec.add(ctx, name, args)
		switch name {
		case "bash":
			script := args[len(args)-1]
			switch {
			case strings.Contains(script, "ansible-inventory"):
				return []byte(inventory), nil
			case strings.Contains(script, "ansible all"):
				return []byte(ansible), errors.New("exit status 4")
			}
			return nil, nil
		case "ssh":
			n := nodes[args[len(args)-2]]
			out := n.key
			if args[len(args)-1] == "sudo -n true" && out == "" {
				out = n.sudo
			}
			if out != "" {
				return []byte(out), errors.New("exit status 255")
			}
			return nil, nil
		case "setsid":
			n := nodes[args[len(args)-1]]
			if n.key == "" || strings.Contains(n.key, "Permission denied") {
				n.key = "" // the key is there now
				return []byte("Number of key(s) added: 1\n"), nil
			}
			return []byte("ssh: connect to host x port 22: Connection refused\n"), errors.New("exit status 1")
		}
		return nil, errors.New("unexpected " + name)
	}
}

const p2Inventory = `{"_meta": {"hostvars": {
  "n-ok": {"ansible_host": "10.0.0.1", "ansible_user": "ops"},
  "n-key": {"ansible_host": "10.0.0.2", "ansible_user": "ops"},
  "n-down": {"ansible_host": "10.0.0.3", "ansible_user": "ops"},
  "n-moved": {"ansible_host": "10.0.0.4", "ansible_port": 2222, "ansible_user": "ops"},
  "n-sudo": {"ansible_host": "10.0.0.5", "ansible_user": "ops"}}},
  "all": {"hosts": ["n-ok", "n-key", "n-down", "n-moved", "n-sudo"]}}`

func p2Nodes() map[string]*fakeNode {
	return map[string]*fakeNode{
		"ops@10.0.0.1": {},
		"ops@10.0.0.2": {key: "ops@10.0.0.2: Permission denied (publickey,password).\n"},
		"ops@10.0.0.3": {key: "ssh: connect to host 10.0.0.3 port 22: No route to host\n"},
		"ops@10.0.0.4": {key: "@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\nHost key verification failed.\n"},
		"ops@10.0.0.5": {sudo: "sudo: a password is required\n"},
	}
}

// P2 keeps its ansible run and says, for each node ansible could not reach, why
// ssh could not: the fix of each is on its line.
func TestCheckSSHSaysWhyANodeIsUnreachable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ansible := "n-ok | CHANGED | rc=0 | (stdout) \n" +
		"n-key | UNREACHABLE!: Failed to connect to the host via ssh: ops@10.0.0.2: Permission denied (publickey,password).\n" +
		"n-down | UNREACHABLE!: Failed to connect to the host via ssh\n" +
		"n-moved | UNREACHABLE!: Failed to connect to the host via ssh\n" +
		"n-sudo | FAILED | rc=1 | (stdout) (stderr) sudo: a password is required\n"
	rec := &recorder{}
	r, probed := CheckSSHNodes(context.Background(), testOptions(), sshNodes(rec, p2Inventory, ansible, p2Nodes()))
	got := strings.Join(r.Lines, "\n")
	for _, want := range []string{
		"정상: n-ok",
		"n-key: 접속 안 됨 — 키 거부: 제어 노드의 키 (~/.ssh/id_rsa) 를 받지 않음",
		"n-down: 접속 안 됨 — 접속 불가: 접속하지 못함. 주소·포트·방화벽·sshd 를 확인하세요 (ssh: connect to host 10.0.0.3 port 22: No route to host)",
		"n-moved: 접속 안 됨 — 호스트 키 변경: known_hosts 의 호스트 키와 다름. 노드를 다시 설치한 것이 맞으면 ssh-keygen -R '[10.0.0.4]:2222'",
		"n-sudo: sudo 비밀번호 필요 또는 실패",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("P2 lacks %q:\n%s", want, got)
		}
	}
	if r.OK {
		t.Error("P2 passes")
	}
	if refusing := Refusing(probed); len(refusing) != 1 || refusing[0].Name != "n-key" {
		t.Errorf("refusing %+v", refusing)
	}
	// only the unreachable ones are probed, read only
	for _, c := range rec.calls {
		if c.name != "ssh" {
			continue
		}
		target := c.args[len(c.args)-2]
		if target == "ops@10.0.0.1" || target == "ops@10.0.0.5" {
			t.Errorf("probed a node ansible reached: %v", c.args)
		}
		if !strings.Contains(strings.Join(c.args, " "), "UserKnownHostsFile=") {
			t.Errorf("a look writes to known_hosts: %v", c.args)
		}
	}
}

// The probe logs in with the key of the control node only, and runs sudo after
// the key got in.
func TestProbeNode(t *testing.T) {
	rec := &recorder{}
	run := sshNodes(rec, "", "", p2Nodes())
	k := SSHKey{Private: "/home/ops/.ssh/id_rsa", Public: "/home/ops/.ssh/id_rsa.pub"}
	ok := ProbeNode(context.Background(), run, k, SSHNode{"n", "10.0.0.1", "22", "ops"}, "")
	if !ok.Ready() || ok.Label() != "" {
		t.Errorf("%+v", ok)
	}
	args := strings.Join(rec.calls[0].args, " ")
	for _, want := range []string{"-n -i /home/ops/.ssh/id_rsa", "BatchMode=yes", "IdentitiesOnly=yes", "IdentityAgent=none",
		"ConnectTimeout=10", "StrictHostKeyChecking=accept-new", "-p 22 ops@10.0.0.1 true"} {
		if !strings.Contains(args, want) {
			t.Errorf("ssh %s lacks %q", args, want)
		}
	}
	if strings.Contains(args, "UserKnownHostsFile") {
		t.Error("a send writes the real known_hosts, a probe after it as well")
	}
	if got := strings.Join(rec.calls[1].args, " "); !strings.HasSuffix(got, "ops@10.0.0.1 sudo -n true") {
		t.Errorf("sudo: %s", got)
	}

	sudo := ProbeNode(context.Background(), run, k, SSHNode{"n", "10.0.0.5", "22", "ops"}, "")
	if sudo.Key != KeyOK || sudo.Sudo != SudoNeedsPassword || sudo.Label() != "sudo 비밀번호 필요" ||
		!strings.Contains(sudo.Note(), "NOPASSWD") || !strings.Contains(sudo.Note(), "a password is required") {
		t.Errorf("%+v %s", sudo, sudo.Note())
	}
	refused := ProbeNode(context.Background(), run, k, SSHNode{"n", "10.0.0.2", "22", "ops"}, "")
	if refused.Key != KeyRefused || refused.Sudo != SudoNotChecked {
		t.Errorf("%+v", refused)
	}
}

// A look copies known_hosts and leaves it as it was.
func TestProbeNodesReadOnly(t *testing.T) {
	home := t.TempDir()
	k := SSHKey{Private: filepath.Join(home, ".ssh", "id_rsa"), Public: filepath.Join(home, ".ssh", "id_rsa.pub")}
	os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
	real := filepath.Join(home, ".ssh", "known_hosts")
	os.WriteFile(real, []byte("10.0.0.1 ssh-ed25519 AAAA\n"), 0600)

	var copied string
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if p, ok := strings.CutPrefix(a, "UserKnownHostsFile="); ok {
				if data, _ := os.ReadFile(p); copied == "" {
					copied = string(data) // the first ssh, before it added a host
				}
				os.WriteFile(p, []byte("changed\n"), 0600) // ssh adds a host
				if p == real {
					t.Error("the real known_hosts is given")
				}
			}
		}
		return nil, nil
	}
	if _, err := ProbeNodes(context.Background(), run, k, []SSHNode{{"n", "10.0.0.1", "22", "ops"}}, true); err != nil {
		t.Fatal(err)
	}
	if copied != "10.0.0.1 ssh-ed25519 AAAA\n" {
		t.Errorf("the copy holds %q", copied)
	}
	if data, _ := os.ReadFile(real); string(data) != "10.0.0.1 ssh-ed25519 AAAA\n" {
		t.Errorf("known_hosts changed: %q", data)
	}
}

// The password goes to ssh-copy-id in its environment only: no argument of any
// command has it, and the helper ssh runs holds only the name of the variable.
// The directory of the helper is gone afterwards.
func TestSendKeyWithPasswordKeepsThePasswordOutOfArguments(t *testing.T) {
	const password = ` p@ss w0rd$1 'x' `
	nodes := p2Nodes()
	rec := &recorder{}
	inner := sshNodes(rec, "", "", nodes)
	var helper string
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if name == "setsid" {
			for _, e := range EnvOf(ctx) {
				if p, ok := strings.CutPrefix(e, "SSH_ASKPASS="); ok {
					helper = p
					data, err := os.ReadFile(p)
					if err != nil || strings.Contains(string(data), "p@ss") || !strings.Contains(string(data), askPassVar) {
						t.Errorf("helper %q: %v", data, err)
					}
					if fi, _ := os.Stat(filepath.Dir(p)); fi == nil || fi.Mode().Perm() != 0700 {
						t.Errorf("the helper's directory is not 0700: %v", fi)
					}
				}
			}
		}
		return inner(ctx, dir, name, args...)
	}
	k := SSHKey{Private: "/home/ops/.ssh/id_rsa", Public: "/home/ops/.ssh/id_rsa.pub"}
	send := []SSHNode{{"n-key", "10.0.0.2", "22", "ops"}, {"n-down", "10.0.0.3", "22", "ops"}}
	var got []NodeSSH
	err := SendKeyWithPassword(context.Background(), run, k, send, []byte(password), func(r NodeSSH) { got = append(got, r) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Ready() || got[1].Key != KeyUnreachable || !strings.Contains(got[1].Note(), "키 보내기 실패 (ssh: connect") {
		t.Errorf("results %+v", got)
	}
	var sends int
	for _, c := range rec.calls {
		if strings.Contains(strings.Join(c.args, "\x00"), "p@ss") {
			t.Errorf("an argument has the password: %s %q", c.name, c.args)
		}
		if c.name != "setsid" {
			if len(c.env) > 0 {
				t.Errorf("%s got the environment of the send", c.name)
			}
			continue
		}
		sends++
		if a := strings.Join(c.args, " "); !strings.HasPrefix(a, "-w ssh-copy-id -i /home/ops/.ssh/id_rsa.pub ") ||
			!strings.Contains(a, "NumberOfPasswordPrompts=1") {
			t.Errorf("send: %s", a)
		}
		env := strings.Join(c.env, "\n")
		for _, want := range []string{"SSH_ASKPASS_REQUIRE=force", "DISPLAY=", askPassVar + "=" + password} {
			if !strings.Contains(env, want) {
				t.Errorf("env lacks %q", want)
			}
		}
	}
	if sends != 2 {
		t.Errorf("%d sends", sends)
	}
	if _, err := os.Stat(filepath.Dir(helper)); helper == "" || !os.IsNotExist(err) {
		t.Errorf("the helper's directory %q is left: %v", helper, err)
	}
}

// Ctrl-C: no node after it, and the helper's directory is removed.
func TestSendKeyWithPasswordStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var helper string
	var sends int
	run := func(c context.Context, dir, name string, args ...string) ([]byte, error) {
		if name == "setsid" {
			sends++
			for _, e := range EnvOf(c) {
				if p, ok := strings.CutPrefix(e, "SSH_ASKPASS="); ok {
					helper = p
				}
			}
			cancel() // interrupted during the first send
			return nil, errors.New("signal: killed")
		}
		return nil, nil
	}
	nodes := []SSHNode{{"a", "10.0.0.2", "22", "ops"}, {"b", "10.0.0.3", "22", "ops"}}
	err := SendKeyWithPassword(ctx, run, DefaultSSHKey(), nodes, []byte("pw"), func(NodeSSH) { t.Error("a result after the interrupt") })
	if !errors.Is(err, context.Canceled) || sends != 1 {
		t.Errorf("err %v, %d sends", err, sends)
	}
	if _, err := os.Stat(filepath.Dir(helper)); !os.IsNotExist(err) {
		t.Errorf("the helper's directory is left: %v", err)
	}
}

// The ssh prompt mode: one ssh-copy-id for each node given, quoted.
func TestAskSendScript(t *testing.T) {
	k := SSHKey{Private: "/home/o p/.ssh/id_rsa", Public: "/home/o p/.ssh/id_rsa.pub"}
	s := AskSendScript(k, []SSHNode{{"n-key", "10.0.0.2", "2222", "ops"}}, true)
	for _, want := range []string{
		"ssh-copy-id -i '/home/o p/.ssh/id_rsa.pub' -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -p 2222 ops@10.0.0.2 || echo",
		"read -r -p",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "ssh-copy-id") != 1 || strings.Contains(AskSendScript(k, nil, false), "read") {
		t.Errorf("script:\n%s", s)
	}
}

func TestSSHTable(t *testing.T) {
	table := SSHTable([]NodeSSH{
		{Node: SSHNode{"node-sudo", "127.0.0.1", "22", "alice"}, Key: KeyOK, Sudo: SudoOK},
		{Node: SSHNode{"node-down", "127.0.0.1", "2223", "alice"}, Key: KeyUnreachable, Detail: "ssh: connect to host 127.0.0.1 port 2223: Connection refused"},
	})
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "노드") ||
		!strings.HasPrefix(lines[1], "node-down  alice@127.0.0.1:2223  접속불가  -") || !strings.Contains(lines[1], "Connection refused") ||
		strings.TrimRight(lines[2], " ") != lines[2] || !strings.HasPrefix(lines[2], "node-sudo  alice@127.0.0.1       ok        ok") {
		t.Errorf("table:\n%s", table)
	}
}

// configure-control-node-ssh.sh of the installer makes the key; its
// instructions are dropped.
func TestPrepareKey(t *testing.T) {
	home := t.TempDir()
	k := SSHKey{Private: filepath.Join(home, "id_rsa"), Public: filepath.Join(home, "id_rsa.pub")}
	var script string
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		script = args[len(args)-1]
		os.WriteFile(k.Public, []byte("ssh-rsa AAAA"), 0644)
		return nil, nil
	}
	if err := PrepareKey(context.Background(), testOptions(), run, k); err != nil {
		t.Fatal(err)
	}
	if script != "/opt/k8s-installer/ansible/configure-control-node-ssh.sh --no-color > /dev/null" {
		t.Errorf("ran %q", script)
	}
	failing := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte("[ERROR] File[\"~/.ssh/id_rsa.pub\"] is not a regular file\n"), errors.New("exit status 1")
	}
	if err := PrepareKey(context.Background(), testOptions(), failing, k); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("%v", err)
	}
}

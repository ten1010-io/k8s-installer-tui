package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The SSH key of the control node on the nodes, which P2 and every playbook
// need. A node is ready when it takes the control node's own key
// (~/.ssh/id_rsa, made by configure-control-node-ssh.sh of k8s-installer) and
// runs sudo without a password. The key is sent with ssh-copy-id, the password
// asked by ssh itself on the terminal, or asked once and handed to ssh through
// SSH_ASKPASS: the password goes to the child's environment only, never to an
// argument, a file or a message.

// SSHNode is a node as ansible-inventory resolves it: what a playbook logs in to.
type SSHNode struct {
	Name, Host, Port, User string
}

// SSHKey is the key pair of the control node.
type SSHKey struct {
	Private, Public string
}

// DefaultSSHKey is ~/.ssh/id_rsa of the user running this, the key
// configure-control-node-ssh.sh makes and the playbooks log in with.
func DefaultSSHKey() SSHKey {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}
	p := filepath.Join(home, ".ssh", "id_rsa")
	return SSHKey{Private: p, Public: p + ".pub"}
}

// unusedHostVars are what a playbook uses to reach a node and this path does
// not: what it reports for such a node may differ from what a playbook meets.
var unusedHostVars = []string{
	"ansible_ssh_private_key_file", "ansible_private_key_file",
	"ansible_ssh_common_args", "ansible_ssh_extra_args",
	"ansible_ssh_pass", "ansible_password",
	"ansible_become_password", "ansible_become_pass", "ansible_become_method",
}

// ListSSHNodes reads the nodes of inventory (relative to the ansible directory)
// with ansible-inventory of the venv of k8s-installer, group_vars included, as
// a playbook sees them. A node of ansible_connection local is left out. The
// warnings name the nodes that set what this path does not use.
func ListSSHNodes(ctx context.Context, o Options, run RunFunc, inventory string) ([]SSHNode, []string, error) {
	script := "source " + shellQuote(o.Root+"/venv/bin/activate") + " && ansible-inventory -i " + shellQuote(inventory) + " --list"
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", script)
	if err != nil {
		return nil, nil, fmt.Errorf("ansible-inventory 실패: %v — %s", err, lastLine(out))
	}
	return parseInventory(out, currentUser())
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// parseInventory reads the JSON of ansible-inventory --list, after whatever
// warnings ansible printed before it.
func parseInventory(out []byte, defaultUser string) ([]SSHNode, []string, error) {
	start := bytes.Index(out, []byte("\n{"))
	if bytes.HasPrefix(out, []byte("{")) {
		start = -1
	} else if start < 0 {
		return nil, nil, fmt.Errorf("ansible-inventory 출력을 읽지 못했습니다 — %s", lastLine(out))
	}
	var inv map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(out[start+1:])).Decode(&inv); err != nil {
		return nil, nil, fmt.Errorf("ansible-inventory 출력을 읽지 못했습니다: %v", err)
	}
	var meta struct {
		HostVars map[string]map[string]any `json:"hostvars"`
	}
	names := map[string]bool{}
	for group, raw := range inv {
		if group == "_meta" {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, nil, fmt.Errorf("ansible-inventory 의 _meta 를 읽지 못했습니다: %v", err)
			}
			continue
		}
		var g struct {
			Hosts []string `json:"hosts"`
		}
		if json.Unmarshal(raw, &g) == nil {
			for _, h := range g.Hosts {
				names[h] = true
			}
		}
	}
	for h := range meta.HostVars {
		names[h] = true
	}

	var nodes []SSHNode
	var warnings []string
	for _, name := range sortedKeys(names) {
		v := meta.HostVars[name]
		if str(v["ansible_connection"]) == "local" {
			continue
		}
		var found []string
		for _, u := range unusedHostVars {
			if _, set := v[u]; set {
				found = append(found, u)
			}
		}
		if len(found) > 0 {
			warnings = append(warnings, "노드 "+name+" 가 "+strings.Join(found, ", ")+
				" 를 설정했습니다. 이 확인과 키 보내기는 그 값을 쓰지 않으므로 플레이북이 겪는 것과 다를 수 있습니다")
		}
		n := SSHNode{Name: name, Host: firstOf(v, name, "ansible_host"), Port: firstOf(v, "22", "ansible_port", "ansible_ssh_port"),
			User: firstOf(v, defaultUser, "ansible_user", "ansible_ssh_user")}
		for _, f := range []string{n.Name, n.Host, n.Port, n.User} {
			if f == "" || strings.ContainsAny(f, " \t\r\n") {
				return nil, nil, fmt.Errorf("노드 %s 의 이름·주소·포트·사용자 중 비었거나 공백이 든 값이 있습니다", name)
			}
		}
		nodes = append(nodes, n)
	}
	return nodes, warnings, nil
}

// firstOf is the first of keys that vars sets, as text, else def.
func firstOf(vars map[string]any, def string, keys ...string) string {
	for _, k := range keys {
		if v, ok := vars[k]; ok && v != nil {
			return str(v)
		}
	}
	return def
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// KeyState is whether a node takes the key of the control node.
type KeyState int

const (
	KeyOK KeyState = iota
	KeyRefused
	KeyUnreachable
	KeyHostChanged
)

// SudoState is whether the user of a node runs sudo without a password.
type SudoState int

const (
	SudoNotChecked SudoState = iota // the key did not get in
	SudoOK
	SudoNeedsPassword
)

// NodeSSH is what ssh found on a node.
type NodeSSH struct {
	Node SSHNode
	Key  KeyState
	Sudo SudoState
	// Detail is the last line ssh (or sudo) said when it failed
	Detail string
	// SendErr is the last line of ssh-copy-id when sending the key failed
	SendErr string
}

func (r NodeSSH) Ready() bool { return r.Key == KeyOK && r.Sudo == SudoOK }

// Label says in a few words what is wrong, "" when the node is ready.
func (r NodeSSH) Label() string {
	switch {
	case r.Key == KeyRefused:
		return "키 거부"
	case r.Key == KeyUnreachable:
		return "접속 불가"
	case r.Key == KeyHostChanged:
		return "호스트 키 변경"
	case r.Sudo == SudoNeedsPassword:
		return "sudo 비밀번호 필요"
	}
	return ""
}

// Note says how to fix the node, with what ssh said.
func (r NodeSSH) Note() string {
	var fix string
	switch {
	case r.Key == KeyRefused:
		fix = "제어 노드의 키 (~/.ssh/id_rsa) 를 받지 않음. 키를 보내세요 (설치 화면 k·p, 또는 --send-ssh-key)"
		if r.Node.User == "root" {
			fix += ". root 로 비밀번호 로그인을 sshd 가 막으면 키를 노드에 직접 넣어야 함"
		}
	case r.Key == KeyUnreachable:
		fix = "접속하지 못함. 주소·포트·방화벽·sshd 를 확인하세요"
	case r.Key == KeyHostChanged:
		fix = "known_hosts 의 호스트 키와 다름. 노드를 다시 설치한 것이 맞으면 ssh-keygen -R " + shellQuote(knownHostsName(r.Node)) + " 뒤 다시 확인하세요"
	case r.Sudo == SudoNeedsPassword:
		fix = "사용자 " + r.Node.User + " 에 비밀번호 없는 sudo (NOPASSWD) 가 필요합니다"
	default:
		fix = "제어 노드 키로 접속·sudo 가 됨. ansible 만 접속하지 못하면 inventory 의 ansible_ssh_* 설정을 확인하세요"
	}
	if r.SendErr != "" {
		fix = "키 보내기 실패 (" + r.SendErr + "). " + fix
	}
	if r.Detail != "" {
		fix += " (" + r.Detail + ")"
	}
	return fix
}

// knownHostsName is how known_hosts names the node.
func knownHostsName(n SSHNode) string {
	if n.Port == "22" {
		return n.Host
	}
	return "[" + n.Host + "]:" + n.Port
}

// sshConnOpts: accept-new records the host key of a node seen for the first time,
// which the playbooks need in known_hosts anyway, and still refuses a key that
// changed. knownHosts, when set, is a copy that a read-only look writes to.
func sshConnOpts(knownHosts string) []string {
	opts := []string{"-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new"}
	if knownHosts != "" {
		opts = append(opts, "-o", "UserKnownHostsFile="+knownHosts)
	}
	return opts
}

// keyArgs log in with the key of the control node only: a key of an agent or
// another key of the user getting in says nothing about what the playbooks meet.
func keyArgs(k SSHKey, n SSHNode, knownHosts, command string) []string {
	args := []string{"-n", "-i", k.Private, "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none"}
	args = append(args, sshConnOpts(knownHosts)...)
	return append(args, "-p", n.Port, n.User+"@"+n.Host, command)
}

// ProbeNode looks whether n takes the key and, when it does, runs sudo without a
// password. knownHosts: see sshConnOpts.
func ProbeNode(ctx context.Context, run RunFunc, k SSHKey, n SSHNode, knownHosts string) NodeSSH {
	r := NodeSSH{Node: n}
	out, err := run(ctx, "", "ssh", keyArgs(k, n, knownHosts, "true")...)
	if err != nil {
		r.Key, r.Detail = classifySSH(out)
		return r
	}
	out, err = run(ctx, "", "ssh", keyArgs(k, n, knownHosts, "sudo -n true")...)
	if err != nil {
		r.Sudo, r.Detail = SudoNeedsPassword, lastLine(out)
		return r
	}
	r.Sudo = SudoOK
	return r
}

// classifySSH reads why ssh failed from what it printed.
func classifySSH(out []byte) (KeyState, string) {
	last := lastLine(out)
	switch {
	case bytes.Contains(out, []byte("REMOTE HOST IDENTIFICATION HAS CHANGED")) || strings.Contains(last, "Host key verification failed"):
		return KeyHostChanged, last
	case strings.Contains(last, "Permission denied") || strings.Contains(last, "Too many authentication failures"):
		return KeyRefused, last
	}
	return KeyUnreachable, last
}

// lastLine is the last line that says something, without what would break a
// line of a table.
func lastLine(out []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return strings.ReplaceAll(l, "|", "/")
		}
	}
	return ""
}

// ProbeNodes probes every node. readOnly: the host keys met go to a copy of
// known_hosts that is thrown away, so a look changes nothing.
func ProbeNodes(ctx context.Context, run RunFunc, k SSHKey, nodes []SSHNode, readOnly bool) ([]NodeSSH, error) {
	knownHosts := ""
	if readOnly {
		dir, err := os.MkdirTemp("", "k8s-installer-tui-ssh-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		knownHosts = filepath.Join(dir, "known_hosts")
		data, err := os.ReadFile(filepath.Join(filepath.Dir(k.Private), "known_hosts"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := os.WriteFile(knownHosts, data, 0600); err != nil {
			return nil, err
		}
	}
	res := make([]NodeSSH, 0, len(nodes))
	for _, n := range nodes {
		res = append(res, ProbeNode(ctx, run, k, n, knownHosts))
	}
	return res, nil
}

// Refusing are the nodes that refuse the key: the ones sending it would help.
// A node that is down, whose host key changed or that only lacks sudo is not
// among them; its note says what to do instead.
func Refusing(rs []NodeSSH) []SSHNode {
	var nodes []SSHNode
	for _, r := range rs {
		if r.Key == KeyRefused {
			nodes = append(nodes, r.Node)
		}
	}
	return nodes
}

// PrepareKey makes the key pair of the control node when there is none, by
// running configure-control-node-ssh.sh of k8s-installer (which also puts it in
// the control node's own authorized_keys). What it prints for the operator to
// do by hand is dropped: sending the key does it.
func PrepareKey(ctx context.Context, o Options, run RunFunc, k SSHKey) error {
	script := o.AnsibleDir() + "/configure-control-node-ssh.sh"
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", shellQuote(script)+" --no-color > /dev/null")
	if err != nil {
		return fmt.Errorf("%s 실패: %v — %s", script, err, lastLine(out))
	}
	if _, err := os.Stat(k.Public); err != nil {
		return fmt.Errorf("%s 뒤에도 %s 가 없습니다", script, k.Public)
	}
	return nil
}

// AskSendScript is the bash script that sends the key to nodes with ssh-copy-id
// on the terminal, where ssh asks the password of each node itself. pause: wait
// for Enter at the end, before the screen comes back.
func AskSendScript(k SSHKey, nodes []SSHNode, pause bool) string {
	var b strings.Builder
	for _, n := range nodes {
		args := append([]string{"ssh-copy-id", "-i", k.Public}, sshConnOpts("")...)
		args = append(args, "-p", n.Port, n.User+"@"+n.Host)
		for i, a := range args {
			args[i] = shellQuote(a)
		}
		fmt.Fprintf(&b, "echo %s\n", shellQuote(fmt.Sprintf("── %s (%s@%s, 포트 %s): 키를 보냅니다. 비밀번호는 ssh 가 묻습니다", n.Name, n.User, n.Host, n.Port)))
		fmt.Fprintf(&b, "%s || echo %s\n", strings.Join(args, " "), shellQuote("✗ "+n.Name+": 키를 보내지 못했습니다 (위의 ssh 메시지 참고)"))
	}
	if pause {
		b.WriteString("echo; read -r -p 'Enter 를 누르면 화면으로 돌아갑니다 ' _ || true\n")
	}
	return b.String()
}

// askPassVar holds the password in the environment of ssh-copy-id, and of the
// ssh it runs, which runs the helper.
const askPassVar = "K8S_INSTALLER_TUI_SSH_PASSWORD"

// SendKeyWithPassword sends the key to each node with one password, then probes
// the node again and gives what it found to done. ssh takes the password from
// SSH_ASKPASS when it has no terminal: setsid takes the terminal away, which
// works on OpenSSH before 8.4 too, where SSH_ASKPASS_REQUIRE is not known. One
// prompt per node, so a wrong password costs each node one failed login. The
// helper holds no password, only the name of the variable; it lives in a 0700
// directory removed at the end. Stops at the next node when ctx is done.
func SendKeyWithPassword(ctx context.Context, run RunFunc, k SSHKey, nodes []SSHNode, password []byte, done func(NodeSSH)) error {
	dir, err := os.MkdirTemp("", "k8s-installer-tui-ssh-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	helper := filepath.Join(dir, "askpass.sh")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\\n' \"$"+askPassVar+"\"\n"), 0700); err != nil {
		return err
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = "none"
	}
	env := WithEnv(ctx, "SSH_ASKPASS="+helper, "SSH_ASKPASS_REQUIRE=force", "DISPLAY="+display, askPassVar+"="+string(password))
	for _, n := range nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		args := append([]string{"-w", "ssh-copy-id", "-i", k.Public}, sshConnOpts("")...)
		args = append(args, "-o", "NumberOfPasswordPrompts=1", "-p", n.Port, n.User+"@"+n.Host)
		out, sendErr := run(env, "", "setsid", args...)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r := ProbeNode(ctx, run, k, n, "")
		if sendErr != nil && r.Key != KeyOK {
			r.SendErr = lastLine(out)
		}
		done(r)
	}
	return ctx.Err()
}

// SSHTable is the table of the nodes: name, address, key, sudo, note.
func SSHTable(rs []NodeSSH) string {
	rows := [][]string{{"노드", "주소", "키", "sudo", "비고"}}
	sorted := append([]NodeSSH{}, rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Node.Name < sorted[j].Node.Name })
	for _, r := range sorted {
		addr := r.Node.Host
		if r.Node.Port != "22" {
			addr += ":" + r.Node.Port
		}
		key := map[KeyState]string{KeyOK: "ok", KeyRefused: "거부", KeyUnreachable: "접속불가", KeyHostChanged: "호스트키변경"}[r.Key]
		sudo := map[SudoState]string{SudoNotChecked: "-", SudoOK: "ok", SudoNeedsPassword: "비밀번호필요"}[r.Sudo]
		note := ""
		if !r.Ready() {
			note = r.Note()
		}
		rows = append(rows, []string{r.Node.Name, r.Node.User + "@" + addr, key, sudo, note})
	}
	widths := make([]int, 4)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], lipgloss.Width(row[i]))
		}
	}
	var b strings.Builder
	for _, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(widths) {
				line.WriteString(strings.Repeat(" ", widths[i]-lipgloss.Width(cell)+2))
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}
	return b.String()
}

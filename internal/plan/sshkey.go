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
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
	nodes, warnings, err := listSSHNodes(ctx, o, run, inventory)
	return nodes, warningList(warnings), err
}

// listSSHNodes is ListSSHNodes with each warning by the node it is about.
func listSSHNodes(ctx context.Context, o Options, run RunFunc, inventory string) ([]SSHNode, map[string]string, error) {
	script := "source " + shellQuote(o.Root+"/venv/bin/activate") + " && ansible-inventory -i " + shellQuote(inventory) + " --list"
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", script)
	if err != nil {
		return nil, nil, errors.New(i18n.T("ansible-inventory failed: %v — %s", err, lastLine(out)))
	}
	return inventoryNodes(out, currentUser())
}

// warningList are the warnings by node in the order of the nodes' names.
func warningList(byNode map[string]string) []string {
	var out []string
	for _, n := range sortedKeys(byNode) {
		out = append(out, byNode[n])
	}
	return out
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
	nodes, warnings, err := inventoryNodes(out, defaultUser)
	return nodes, warningList(warnings), err
}

// inventoryNodes is parseInventory with each warning by the node it is about,
// for the probe of P2 to pick those of its nodes.
func inventoryNodes(out []byte, defaultUser string) ([]SSHNode, map[string]string, error) {
	start := bytes.Index(out, []byte("\n{"))
	if bytes.HasPrefix(out, []byte("{")) {
		start = -1
	} else if start < 0 {
		return nil, nil, errors.New(i18n.T("could not read the output of ansible-inventory — %s", lastLine(out)))
	}
	var inv map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(out[start+1:])).Decode(&inv); err != nil {
		return nil, nil, errors.New(i18n.T("could not read the output of ansible-inventory: %v", err))
	}
	var meta struct {
		HostVars map[string]map[string]any `json:"hostvars"`
	}
	names := map[string]bool{}
	for group, raw := range inv {
		if group == "_meta" {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, nil, errors.New(i18n.T("could not read _meta of ansible-inventory: %v", err))
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
	warnings := map[string]string{}
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
			warnings[name] = i18n.T("Node %s sets %s. This check and sending the key do not use them, "+
				"so what they find may differ from what a playbook sees", name, strings.Join(found, ", "))
		}
		n := SSHNode{Name: name, Host: firstOf(v, name, "ansible_host"), Port: firstOf(v, "22", "ansible_port", "ansible_ssh_port"),
			User: firstOf(v, defaultUser, "ansible_user", "ansible_ssh_user")}
		for _, f := range []string{n.Name, n.Host, n.Port, n.User} {
			if f == "" || strings.ContainsAny(f, " \t\r\n") {
				return nil, nil, errors.New(i18n.T("node %s has a name, address, port or user that is empty or has a space in it", name))
			}
		}
		// ssh would read them as options
		if strings.HasPrefix(n.Host, "-") || strings.HasPrefix(n.User, "-") {
			return nil, nil, errors.New(i18n.T("the address (%s) or the user (%s) of node %s starts with '-', which ssh reads as an option: fix the inventory", n.Host, n.User, name))
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
	// KeyNoPassword: refused, and sshd offers no password login, so sending the
	// key cannot work; it has to be put on the node by hand
	KeyNoPassword
	// KeyNotProbed: the probe did not run (or was cut short) in the time it had
	KeyNotProbed
	// KeyUnreadable: ssh could not load the key of the control node itself
	// (permissions, owner, format): the same for every node, fixed here
	KeyUnreadable
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
		return i18n.T("key refused")
	case r.Key == KeyUnreachable:
		return i18n.T("cannot connect")
	case r.Key == KeyHostChanged:
		return i18n.T("host key changed")
	case r.Key == KeyNoPassword:
		return i18n.T("password login off")
	case r.Key == KeyNotProbed:
		return i18n.T("could not check")
	case r.Key == KeyUnreadable:
		return i18n.T("control node key unreadable")
	case r.Sudo == SudoNotChecked:
		return i18n.T("sudo not checked")
	case r.Sudo == SudoNeedsPassword:
		return i18n.T("sudo needs a password")
	}
	return ""
}

// Note says how to fix the node, with what ssh said.
func (r NodeSSH) Note() string {
	var fix string
	switch {
	case r.Key == KeyRefused:
		fix = i18n.T("Does not take the key of the control node (~/.ssh/id_rsa). Send the key (k or p on the install screen, or --send-ssh-key)")
		if r.Node.User == "root" {
			fix += ". " + i18n.T("When sshd refuses password login for root, the key has to be put on the node by hand")
		}
	case r.Key == KeyUnreachable:
		fix = i18n.T("Could not connect. Check the address, port, firewall and sshd")
	case r.Key == KeyHostChanged:
		fix = i18n.T("The host key differs from known_hosts. If the node really was reinstalled, run ssh-keygen -R %s, then check again", shellQuote(knownHostsName(r.Node)))
	case r.Key == KeyNoPassword:
		fix = i18n.T("Does not take the key of the control node, and password login is off — the key has to be put there by hand (as configure-control-node-ssh.sh prints)")
	case r.Key == KeyNotProbed:
		fix = i18n.T("Not checked in time. Check again with --check-ssh")
	case r.Key == KeyUnreadable:
		fix = i18n.T("ssh cannot read the key of this control node (~/.ssh/id_rsa). Check the permissions and owner of the file (this user's, chmod 600)")
	case r.Sudo == SudoNotChecked:
		fix = i18n.T("The key of the control node gets in. sudo was not checked in time. Check again (c on the install screen, or --check-ssh)")
	case r.Sudo == SudoNeedsPassword:
		fix = i18n.T("User %s needs sudo without a password (NOPASSWD)", r.Node.User)
	default:
		fix = i18n.T("The key of the control node gets in and runs sudo. When only ansible cannot connect, check the ansible_ssh_* settings of the inventory")
	}
	if r.SendErr != "" {
		fix = i18n.T("Sending the key failed (%s).", r.SendErr) + " " + fix
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
	return append(args, "-p", n.Port, "--", n.User+"@"+n.Host, command)
}

// probeTimeout bounds one probe: two ssh runs of ConnectTimeout=10 each.
const probeTimeout = 45 * time.Second

// ProbeNode looks whether n takes the key and, when it does, runs sudo without a
// password. knownHosts: see sshConnOpts. When ctx ends first the node is
// KeyNotProbed, not classified from what a killed ssh printed.
func ProbeNode(ctx context.Context, run RunFunc, k SSHKey, n SSHNode, knownHosts string) NodeSSH {
	r := NodeSSH{Node: n}
	if ctx.Err() != nil {
		r.Key = KeyNotProbed
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := run(ctx, "", "ssh", keyArgs(k, n, knownHosts, "true")...)
	if err != nil && ctx.Err() != nil {
		r.Key = KeyNotProbed
		return r
	}
	if err != nil {
		r.Key, r.Detail = classifySSH(out)
		return r
	}
	out, err = run(ctx, "", "ssh", keyArgs(k, n, knownHosts, "sudo -n true")...)
	if err != nil && ctx.Err() != nil {
		r.Sudo = SudoNotChecked
		return r
	}
	if err != nil {
		r.Sudo, r.Detail = SudoNeedsPassword, lastLine(out)
		return r
	}
	r.Sudo = SudoOK
	return r
}

// classifySSH reads why ssh failed from everything it printed: OpenSSH puts the
// cause before other lines ("Too many authentication failures", then
// "Disconnected from ..."). The detail is the line that says it.
func classifySSH(out []byte) (KeyState, string) {
	text := strings.ReplaceAll(string(out), "\r", "")
	line := func(subs ...string) string {
		for _, l := range strings.Split(text, "\n") {
			for _, s := range subs {
				if strings.Contains(l, s) {
					return strings.ReplaceAll(strings.TrimSpace(l), "|", "/")
				}
			}
		}
		return lastLine(out)
	}
	switch {
	case strings.Contains(text, "REMOTE HOST IDENTIFICATION HAS CHANGED") || strings.Contains(text, "Host key verification failed"):
		return KeyHostChanged, line("Host key verification failed", "IDENTIFICATION HAS CHANGED")
	case strings.Contains(text, "Load key "):
		// the key of the control node could not be read (`Load key "...":
		// Permission denied`, `bad permissions`, `invalid format`), before any
		// node had a say
		return KeyUnreadable, line("Load key ")
	case strings.Contains(text, "Too many authentication failures"):
		return KeyRefused, line("Too many authentication failures")
	}
	// the methods sshd offers, as ssh lists them on its last word:
	// "Permission denied (publickey,password)."
	denied := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "Permission denied (") {
			denied = strings.ReplaceAll(strings.TrimSpace(l), "|", "/")
		}
	}
	if denied != "" {
		methods := denied[strings.LastIndex(denied, "("):]
		if !strings.Contains(methods, "password") && !strings.Contains(methods, "keyboard-interactive") {
			return KeyNoPassword, denied
		}
		return KeyRefused, denied
	}
	if strings.Contains(text, "Permission denied") {
		return KeyRefused, line("Permission denied")
	}
	return KeyUnreachable, lastLine(out)
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

// probeParallel bounds how many nodes are probed at once.
const probeParallel = 8

// ProbeNodes probes every node, probeParallel at a time, each in a time of its
// own (ProbeNode). readOnly: the host keys met go to a copy of known_hosts that
// is thrown away, so a look changes nothing.
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
	res := make([]NodeSSH, len(nodes))
	sem := make(chan struct{}, probeParallel)
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n SSHNode) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				res[i] = ProbeNode(ctx, run, k, n, knownHosts)
			case <-ctx.Done():
				res[i] = NodeSSH{Node: n, Key: KeyNotProbed}
			}
		}(i, n)
	}
	wg.Wait()
	return res, nil
}

// Refusing are the nodes that refuse the key: the ones sending it would help.
// A node that is down, whose host key changed, whose sshd takes no password or
// that only lacks sudo is not among them; its note says what to do instead.
func Refusing(rs []NodeSSH) []SSHNode {
	var nodes []SSHNode
	for _, r := range rs {
		if r.Key == KeyRefused {
			nodes = append(nodes, r.Node)
		}
	}
	return nodes
}

// PrepareKey makes sure the key pair of the control node is there before a send.
// With id_rsa.pub there it does nothing. With only id_rsa it stops: the public
// key has to be restored from it (ssh-keygen -y), since the script of the
// installer would make a new pair over the private key. With neither it runs
// configure-control-node-ssh.sh of k8s-installer, which makes the pair and puts
// it in the control node's own authorized_keys; what it prints for the operator
// to do by hand is dropped, sending the key does it.
func PrepareKey(ctx context.Context, o Options, run RunFunc, k SSHKey) error {
	if _, err := os.Stat(k.Public); err == nil {
		return nil
	}
	if _, err := os.Stat(k.Private); err == nil {
		return errors.New(i18n.T("%s is there but %s is not. Restore the public key from the private key, then run again: ssh-keygen -y -f %s > %s",
			k.Private, k.Public, shellQuote(k.Private), shellQuote(k.Public)))
	}
	script := o.AnsibleDir() + "/configure-control-node-ssh.sh"
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", shellQuote(script)+" --no-color > /dev/null")
	if err != nil {
		return errors.New(i18n.T("%s failed: %v — %s", script, err, lastLine(out)))
	}
	if _, err := os.Stat(k.Public); err != nil {
		return errors.New(i18n.T("%s is still not there after %s", k.Public, script))
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
		args = append(args, "-p", n.Port, "--", n.User+"@"+n.Host)
		for i, a := range args {
			args[i] = shellQuote(a)
		}
		fmt.Fprintf(&b, "echo %s\n", shellQuote(i18n.T("── %s (%s@%s, port %s): sending the key. ssh asks for the password", n.Name, n.User, n.Host, n.Port)))
		fmt.Fprintf(&b, "%s || echo %s\n", strings.Join(args, " "), shellQuote(i18n.T("✗ %s: the key was not sent (see what ssh said above)", n.Name)))
	}
	if pause {
		b.WriteString("echo; read -r -p " + shellQuote(i18n.T("Enter to go back to the screen")+" ") + " _ || true\n")
	}
	return b.String()
}

// askPassVar holds the password in the environment of ssh-copy-id, and of the
// ssh it runs, which runs the helper.
const askPassVar = "K8S_INSTALLER_TUI_SSH_PASSWORD"

// askPassHelper writes the helper ssh runs for the password into a new 0700
// directory where the user can run it: $XDG_RUNTIME_DIR when set, else
// ~/.cache/k8s-installer-tui (/tmp is noexec on hardened sites). The helper
// gets the password in its environment, so whoever could swap it would get the
// password: every directory on the way that this user owns is checked first
// (ownDir), and a place that fails is skipped, never used. The helper is run
// once, without the password, to see that it can be; the directory is removed
// when it cannot.
func askPassHelper(ctx context.Context, run RunFunc) (dir, helper string, err error) {
	var why []string
	var bases []func() (string, error)
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		bases = append(bases, func() (string, error) { return x, ownDir(x, false) })
	}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, func() (string, error) {
			base := filepath.Join(home, ".cache", "k8s-installer-tui")
			for _, d := range []struct {
				path   string
				create bool
			}{{home, false}, {filepath.Join(home, ".cache"), true}, {base, true}} {
				if err := ownDir(d.path, d.create); err != nil {
					return base, err
				}
			}
			return base, nil
		})
	}
	for _, place := range bases {
		base, err := place()
		if err != nil {
			why = append(why, err.Error())
			continue
		}
		dir, err := os.MkdirTemp(base, "ssh-askpass-")
		if err == nil {
			err = ownDir(dir, false)
		}
		if err != nil {
			if dir != "" {
				os.RemoveAll(dir)
			}
			why = append(why, err.Error())
			continue
		}
		helper := filepath.Join(dir, "askpass.sh")
		err = os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\n' \"$"+askPassVar+"\"\n"), 0700)
		if err == nil {
			var out []byte
			if out, err = run(ctx, "", helper); err != nil {
				err = errors.New(i18n.T("%s cannot be run: %v %s", helper, err, lastLine(out)))
			}
		}
		if err != nil {
			os.RemoveAll(dir)
			why = append(why, err.Error())
			continue
		}
		return dir, helper, nil
	}
	return "", "", errors.New(i18n.T("no directory where the SSH_ASKPASS helper can run safely, so the key was not sent (check noexec mounts, owners and permissions): %s",
		strings.Join(why, "; ")))
}

// ownDir checks that path is a directory of this user that nobody else can write
// to, and no symbolic link; create: make it (0700) when it is not there.
func ownDir(path string, create bool) error {
	fi, err := os.Lstat(path)
	if create && errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0700); err != nil {
			return err
		}
		fi, err = os.Lstat(path)
	}
	switch {
	case err != nil:
		return err
	case fi.Mode()&os.ModeSymlink != 0:
		return errors.New(i18n.T("%s is a symbolic link", path))
	case !fi.IsDir():
		return errors.New(i18n.T("%s is not a directory", path))
	case fi.Mode().Perm()&0o022 != 0:
		return errors.New(i18n.T("%s can be written by its group or other users (%v)", path, fi.Mode().Perm()))
	}
	if uid, ok := fileOwner(fi); ok && uid != os.Geteuid() {
		return errors.New(i18n.T("the owner of %s (uid %d) is not this user (uid %d)", path, uid, os.Geteuid()))
	}
	return nil
}

// SendKeyWithPassword sends the key to each node with one password, then probes
// the node again and gives what it found to done. ssh takes the password from
// SSH_ASKPASS when it has no terminal: setsid takes the terminal away, which
// works on OpenSSH before 8.4 too, where SSH_ASKPASS_REQUIRE is not known. One
// prompt per node, so a wrong password costs each node one failed login. The
// helper holds no password, only the name of the variable (askPassHelper); its
// directory is removed at the end. Stops at the next node when ctx is done.
func SendKeyWithPassword(ctx context.Context, run RunFunc, k SSHKey, nodes []SSHNode, password []byte, done func(NodeSSH)) error {
	dir, helper, err := askPassHelper(ctx, run)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
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
		args = append(args, "-o", "NumberOfPasswordPrompts=1", "-p", n.Port, "--", n.User+"@"+n.Host)
		out, sendErr := run(env, "", "setsid", args...)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sendErr != nil && bytes.Contains(out, []byte("ssh_askpass")) {
			// not the password: ssh could not run the helper
			return errors.New(i18n.T("ssh could not run the SSH_ASKPASS helper: %s", lastLine(out)))
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
	rows := [][]string{{i18n.T("NODE"), i18n.T("ADDRESS"), i18n.T("KEY"), "sudo", i18n.T("NOTE")}}
	sorted := append([]NodeSSH{}, rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Node.Name < sorted[j].Node.Name })
	for _, r := range sorted {
		addr := r.Node.Host
		if r.Node.Port != "22" {
			addr += ":" + r.Node.Port
		}
		// one word a cell, the words of the table
		key := map[KeyState]string{KeyOK: "ok", KeyRefused: i18n.T("refused"), KeyUnreachable: i18n.T("unreachable"),
			KeyHostChanged: i18n.T("host-key-changed"), KeyNoPassword: i18n.T("no-password-login"), KeyNotProbed: i18n.T("unchecked"),
			KeyUnreadable: i18n.T("key-unreadable")}[r.Key]
		sudo := map[SudoState]string{SudoNotChecked: "-", SudoOK: "ok", SudoNeedsPassword: i18n.T("needs-password")}[r.Sudo]
		if r.Key == KeyOK && r.Sudo == SudoNotChecked {
			sudo = i18n.T("unchecked")
		}
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

package plan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// CheckResult is what a check found: OK, and one line per thing it looked at.
type CheckResult struct {
	OK bool
	// Warn: OK, but worth a look; the lines say what. Skipped: not run, the
	// first line says why
	Warn    bool
	Skipped bool
	Lines   []string
}

// RunFunc runs a command in dir and returns its combined output. Tests replace it.
// The command gets the environment of this process plus EnvOf(ctx).
type RunFunc func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

func ExecRun(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// cancelled: kill what the command started too (ssh under ssh-copy-id holds
	// the output pipe), and stop waiting for the pipe after a while regardless
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	if env := EnvOf(ctx); len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

type envKey struct{}

// WithEnv adds variables (NAME=value) for the commands a RunFunc runs with ctx:
// a value that must stay out of the arguments, which every user of the machine
// can read, goes to the child only this way.
func WithEnv(ctx context.Context, env ...string) context.Context {
	return context.WithValue(ctx, envKey{}, append(append([]string{}, EnvOf(ctx)...), env...))
}

// EnvOf is what WithEnv added to ctx.
func EnvOf(ctx context.Context) []string {
	env, _ := ctx.Value(envKey{}).([]string)
	return env
}

// CheckInstalled is P1: setup.sh has installed k8s-installer under the root,
// which records the release it installed in <root>/release.
func CheckInstalled(o Options) CheckResult {
	data, err := os.ReadFile(o.Root + "/release")
	if err != nil {
		return CheckResult{Lines: []string{
			o.Root + "/release 가 없습니다. 패키지 디렉터리에서 sudo ./setup.sh 를 먼저 실행하세요",
		}}
	}
	return CheckResult{OK: true, Lines: []string{"설치된 릴리스 " + strings.TrimSpace(string(data))}}
}

// CheckSSH is P2: every node of the inventory is reached as ansible_user and runs
// sudo without a password, which the playbooks need (k8s-installer checks
// `sudo -n true` the same way as its first task).
func CheckSSH(ctx context.Context, o Options, run RunFunc) CheckResult {
	r, _ := CheckSSHNodes(ctx, o, run)
	return r
}

// CheckSSHNodes is CheckSSH, which also looks with ssh itself at the nodes
// ansible could not reach (ProbeNodes, read only) and says why: the key refused,
// no connection, a changed host key, or sudo. It returns what it found for those
// nodes, for the screen to offer sending the key to the ones that refuse it.
func CheckSSHNodes(ctx context.Context, o Options, run RunFunc) (CheckResult, []NodeSSH) {
	// the probes below get a time of their own (ProbeNodes), not what is left of this
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := fmt.Sprintf("source %s && ansible all -i inventory.yml -o -m raw -a 'sudo -n true'",
		shellQuote(o.Root+"/venv/bin/activate"))
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", script)

	results := map[string]string{}
	var unreachable []string
	for node, a := range parseAdhoc(out) {
		switch {
		case a.ok():
			results[node] = "ok"
		case a.Status == "UNREACHABLE":
			results[node] = "접속 안 됨 (SSH 키, 주소, 포트 확인)"
			unreachable = append(unreachable, node)
		default:
			results[node] = "sudo 비밀번호 필요 또는 실패 (NOPASSWD 확인)"
		}
	}
	var probed []NodeSSH
	var warnings []string
	if len(unreachable) > 0 {
		probed, warnings = probeUnreachable(parent, o, run, unreachable)
		for _, p := range probed {
			label := p.Label()
			if label == "" {
				label = "ssh 로는 준비됨" // ansible alone fails: its note says where to look
			}
			results[p.Node.Name] = "접속 안 됨 — " + label + ": " + p.Note()
		}
	}
	if len(results) == 0 {
		msg := "ansible을 실행하지 못했습니다"
		if err != nil {
			msg += ": " + err.Error()
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			msg += " — " + firstLine(s)
		}
		return CheckResult{Lines: []string{msg}}, nil
	}

	nodes := make([]string, 0, len(results))
	for n := range results {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	r := CheckResult{OK: true}
	var good []string
	for _, n := range nodes {
		if results[n] == "ok" {
			good = append(good, n)
			continue
		}
		r.OK = false
		r.Lines = append(r.Lines, n+": "+results[n])
	}
	if len(good) > 0 {
		r.Lines = append([]string{"정상: " + strings.Join(good, " ")}, r.Lines...)
		// Probe firewall on reachable nodes: k8s needs ports like 6443, 2379, 10250 open
		fwScript := fmt.Sprintf("source %s && ansible all -i inventory.yml -o -m raw -a 'systemctl is-active firewalld 2>/dev/null || systemctl is-active ufw 2>/dev/null || echo inactive'",
			shellQuote(o.Root+"/venv/bin/activate"))
		fwOut, _ := run(ctx, o.AnsibleDir(), "bash", "-c", fwScript)
		var fwActive []string
		for node, a := range parseAdhoc(fwOut) {
			// a line a unit: firewalld absent or inactive prints its own line
			// ("inactive") before the one of ufw
			for _, l := range strings.Split(a.Stdout, "\n") {
				if strings.TrimSpace(l) == "active" {
					fwActive = append(fwActive, node)
					break
				}
			}
		}
		if len(fwActive) > 0 {
			sort.Strings(fwActive)
			r.Warn = true
			r.Lines = append(r.Lines, "방화벽(firewalld/ufw) 활성 노드: "+strings.Join(fwActive, " ")+" (포트 차단 방지를 위해 비활성화 권장)")
		}
	}
	r.Lines = append(r.Lines, warnings...)
	return r, probed
}

// probeUnreachable looks with ssh at the nodes of names, as the inventory of the
// ansible directory gives them. Nothing when the inventory cannot be listed: P2
// says what ansible said.
func probeUnreachable(ctx context.Context, o Options, run RunFunc, names []string) ([]NodeSSH, []string) {
	lctx, lcancel := context.WithTimeout(ctx, time.Minute)
	defer lcancel()
	all, warnings, err := ListSSHNodes(lctx, o, run, "inventory.yml")
	if err != nil {
		return nil, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var nodes []SSHNode
	var used []string
	for _, n := range all {
		if want[n.Name] {
			nodes = append(nodes, n)
		}
	}
	for _, w := range warnings {
		for _, n := range nodes {
			if strings.HasPrefix(w, "노드 "+n.Name+" ") {
				used = append(used, w)
			}
		}
	}
	probed, err := ProbeNodes(ctx, run, DefaultSSHKey(), nodes, true)
	if err != nil {
		return nil, nil
	}
	return probed, used
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

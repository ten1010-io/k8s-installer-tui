package plan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CheckResult is what a check found: OK, and one line per thing it looked at.
type CheckResult struct {
	OK    bool
	Lines []string
}

// RunFunc runs a command in dir and returns its combined output. Tests replace it.
type RunFunc func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

func ExecRun(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
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

// "node1 | CHANGED | rc=0 | (stdout) ", "node2 | UNREACHABLE! => {...", "node3 | FAILED | rc=1 | ..."
var adhocLine = regexp.MustCompile(`^(\S+) \| (\S+?)!? (?:\| rc=(\d+) )?.*$`)

// CheckSSH is P2: every node of the inventory is reached as ansible_user and runs
// sudo without a password, which the playbooks need (k8s-installer checks
// `sudo -n true` the same way as its first task).
func CheckSSH(ctx context.Context, o Options, run RunFunc) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := fmt.Sprintf("source %s && ansible all -i inventory.yml -o -m command -a 'sudo -n true'",
		shellQuote(o.Root+"/venv/bin/activate"))
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", script)

	results := map[string]string{}
	for _, l := range strings.Split(string(out), "\n") {
		m := adhocLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		node, status := m[1], m[2]
		switch {
		case (status == "CHANGED" || status == "SUCCESS") && (m[3] == "" || m[3] == "0"):
			results[node] = "ok"
		case status == "UNREACHABLE":
			results[node] = "접속 안 됨 (SSH 키, 주소, 포트 확인)"
		default:
			results[node] = "sudo 비밀번호 필요 또는 실패 (NOPASSWD 확인)"
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
		return CheckResult{Lines: []string{msg}}
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
	}
	return r
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

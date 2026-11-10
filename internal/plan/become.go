package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// P6: Ansible runs its become command through a shell (/bin/sh -c ...), so a
// sudoers that allows `sudo -n true` but forbids /bin/sh (`!/bin/sh`) lets P2
// pass and then fails every playbook with "Shared connection closed". The check
// runs `sudo -n /bin/sh -c true` on every node, read only. A node that refuses it
// is probed for dash (read only, too: `command -v dash`, then `sudo -n dash -c
// true`); when every refused node has the same dash the plan takes
// -e ansible_shell_executable=<dash> on every playbook (Options.ShellExecutable).

// sudoShellProbe finds dash and runs `true` through sudo with it, a line of
// stdout and a code the check reads: 0 usable (stdout: the path), 3 no dash,
// 4 dash is there but sudo refuses it.
const sudoShellProbe = `p=$(command -v dash) || { echo missing; exit 3; }; ` +
	`sudo -n "$p" -c true || { echo "$p"; exit 4; }; echo "$p"`

// CheckBecomeShell is P6. It returns the shell executable the plan should pass
// to ansible, empty when none is needed (or when the check failed).
func CheckBecomeShell(ctx context.Context, o Options, run RunFunc) (CheckResult, string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	plain, _, _ := adhoc(ctx, o, run, "all", false, "command", "sudo -n true")
	viaSh, out, err := adhoc(ctx, o, run, "all", false, "command", "sudo -n /bin/sh -c true")
	if len(viaSh) == 0 {
		return CheckResult{Lines: []string{couldNotRun(out, err)}}, ""
	}

	var good, noSudo, refused []string
	for _, n := range sortedKeys(viaSh) {
		switch {
		case viaSh[n].ok():
			good = append(good, n)
		case !plain[n].ok():
			noSudo = append(noSudo, n) // P2 reports it
		default:
			refused = append(refused, n)
		}
	}
	var lines []string
	if len(good) > 0 {
		lines = append(lines, "정상 (sudo -n /bin/sh -c true): "+strings.Join(good, " "))
	}
	if len(noSudo) > 0 {
		lines = append(lines, "sudo 자체가 안 되는 노드는 P2 에서 봅니다: "+strings.Join(noSudo, " "))
	}
	if len(refused) == 0 {
		return CheckResult{OK: true, Lines: lines}, ""
	}

	head := "sudo 가 /bin/sh 를 거부하는 노드: " + strings.Join(refused, " ") +
		" (sudoers 의 !/bin/sh 로 보임). 그대로 두면 Ansible become 이 'Shared connection closed' 로 실패합니다"
	probe, _, _ := adhoc(ctx, o, run, strings.Join(refused, ","), false, "shell", sudoShellProbe)
	paths := map[string][]string{} // dash path -> nodes
	var problems []string
	for _, n := range refused {
		p := probe[n]
		switch {
		case p.ok() && p.Stdout != "":
			paths[p.Stdout] = append(paths[p.Stdout], n)
		case p.RC == "3":
			problems = append(problems, n+": dash 가 없습니다")
		case p.RC == "4":
			problems = append(problems, n+": dash ("+p.Stdout+") 가 있으나 sudo 가 거부합니다")
		default:
			problems = append(problems, n+": dash 를 확인하지 못했습니다 (접속·sudo 확인)")
		}
	}
	if len(problems) == 0 && len(paths) > 1 {
		var parts []string
		for _, p := range sortedKeys(paths) {
			parts = append(parts, strings.Join(paths[p], " ")+": "+p)
		}
		problems = append(problems, "노드마다 dash 경로가 다릅니다 ("+strings.Join(parts, ", ")+
			"): ansible_shell_executable 은 하나만 쓸 수 있으니 경로를 맞추세요")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		lines = append(lines, head)
		lines = append(lines, problems...)
		lines = append(lines, "sudoers 가 /bin/sh 를 허용하게 하거나, sudo 로 실행할 수 있는 셸 (dash) 을 이 노드들에 두세요")
		return CheckResult{Lines: lines}, ""
	}
	dash := sortedKeys(paths)[0]
	lines = append(lines, head,
		fmt.Sprintf("모두 dash (%s) 가 있고 sudo 로 실행됩니다: 계획의 ansible-playbook 에 -e ansible_shell_executable=%s 를 붙입니다", dash, dash))
	return CheckResult{OK: true, Warn: true, Lines: lines}, dash
}

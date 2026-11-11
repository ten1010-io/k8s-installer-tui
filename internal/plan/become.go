package plan

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
	plain, _, _ := adhoc(ctx, o, run, "all", false, "raw", "sudo -n true")
	viaSh, out, err := adhoc(ctx, o, run, "all", false, "raw", "sudo -n /bin/sh -c true")
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
		lines = append(lines, i18n.T("OK (sudo -n /bin/sh -c true): %s", strings.Join(good, " ")))
	}
	if len(noSudo) > 0 {
		lines = append(lines, i18n.T("Nodes where sudo itself fails, which P2 reports: %s", strings.Join(noSudo, " ")))
	}
	if len(refused) == 0 {
		return CheckResult{OK: true, Lines: lines}, ""
	}

	head := i18n.T("Nodes where sudo refuses /bin/sh: %s (seemingly !/bin/sh in sudoers). If left as is, Ansible become fails with 'Shared connection closed'",
		strings.Join(refused, " "))
	probe, _, _ := adhoc(ctx, o, run, strings.Join(refused, ","), false, "raw", sudoShellProbe)
	paths := map[string][]string{} // dash path -> nodes
	var problems []string
	for _, n := range refused {
		p := probe[n]
		// the probe's own line is its last: the raw module runs under a
		// terminal, where what sudo says on stderr comes out on stdout before it
		if i := strings.LastIndexByte(p.Stdout, '\n'); i >= 0 {
			p.Stdout = strings.TrimSpace(p.Stdout[i+1:])
		}
		switch {
		case p.ok() && p.Stdout != "":
			paths[p.Stdout] = append(paths[p.Stdout], n)
		case p.RC == "3":
			problems = append(problems, i18n.T("%s: no dash", n))
		case p.RC == "4":
			problems = append(problems, i18n.T("%s: dash (%s) is there, but sudo refuses it", n, p.Stdout))
		default:
			problems = append(problems, i18n.T("%s: could not look for dash (check the connection and sudo)", n))
		}
	}
	if len(problems) == 0 && len(paths) > 1 {
		var parts []string
		for _, p := range sortedKeys(paths) {
			parts = append(parts, strings.Join(paths[p], " ")+": "+p)
		}
		problems = append(problems, i18n.T("The path of dash differs between the nodes (%s): ansible_shell_executable takes one only, so make them the same",
			strings.Join(parts, ", ")))
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		lines = append(lines, head)
		lines = append(lines, problems...)
		lines = append(lines, i18n.T("Let sudoers allow /bin/sh, or put a shell sudo may run (dash) on these nodes"))
		return CheckResult{Lines: lines}, ""
	}
	dash := sortedKeys(paths)[0]
	lines = append(lines, head,
		i18n.T("All have dash (%s) and sudo runs it: the ansible-playbook lines of the plan get -e ansible_shell_executable=%s", dash, dash))
	return CheckResult{OK: true, Warn: true, Lines: lines}, dash
}

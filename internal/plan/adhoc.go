package plan

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// adhocOut is one node's line of an `ansible -o` run.
type adhocOut struct {
	Status string // CHANGED, SUCCESS, FAILED, UNREACHABLE
	RC     string
	Stdout string
}

func (a adhocOut) ok() bool {
	return (a.Status == "CHANGED" || a.Status == "SUCCESS") && (a.RC == "" || a.RC == "0")
}

// "node | CHANGED | rc=0 | (stdout) /usr/bin/dash", "node | FAILED | rc=3 | (stdout) missing",
// "node | UNREACHABLE! => {...}"
var adhocFull = regexp.MustCompile(`^(\S+) \| (\S+?)!? (?:\| rc=(\d+) )?(.*)$`)

func parseAdhoc(out []byte) map[string]adhocOut {
	res := map[string]adhocOut{}
	for _, l := range strings.Split(string(out), "\n") {
		m := adhocFull.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		stdout := m[4]
		if i := strings.Index(stdout, "(stdout) "); i >= 0 {
			stdout = stdout[i+len("(stdout) "):]
			if j := strings.Index(stdout, " (stderr)"); j >= 0 {
				stdout = stdout[:j]
			}
		} else {
			stdout = ""
		}
		res[m[1]] = adhocOut{Status: m[2], RC: m[3], Stdout: strings.TrimSpace(stdout)}
	}
	return res
}

// adhoc runs one ansible module on the nodes of pattern from the ansible
// directory with the venv active, and returns what each node answered. With
// become the run is as root (and takes the shell executable the plan chose).
// The raw output and the error are for the message when nothing was parsed.
func adhoc(ctx context.Context, o Options, run RunFunc, pattern string, become bool, module, args string) (map[string]adhocOut, []byte, error) {
	script := "source " + shellQuote(o.Root+"/venv/bin/activate") + " && ansible " + shellQuote(pattern) + " -i inventory.yml"
	if become {
		script += " -b"
		if o.ShellExecutable != "" {
			script += " -e " + shellQuote("ansible_shell_executable="+o.ShellExecutable)
		}
	}
	script += " -o -m " + module + " -a " + shellQuote(args)
	out, err := run(ctx, o.AnsibleDir(), "bash", "-c", script)
	return parseAdhoc(out), out, err
}

// couldNotRun is the line of a check whose ansible run gave nothing to read.
func couldNotRun(out []byte, err error) string {
	msg := "ansible을 실행하지 못했습니다"
	if err != nil {
		msg += ": " + err.Error()
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		msg += " — " + firstLine(s)
	}
	return msg
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

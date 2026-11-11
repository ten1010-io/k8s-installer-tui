package plan

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// adhocOut is one node's line of an `ansible -o` run.
type adhocOut struct {
	Status string // CHANGED, SUCCESS, FAILED, UNREACHABLE
	RC     string
	Stdout string
	Raw    string // what follows the return code: stdout and stderr, as printed
}

func (a adhocOut) ok() bool {
	return (a.Status == "CHANGED" || a.Status == "SUCCESS") && (a.RC == "" || a.RC == "0")
}

// What `ansible -o` prints, by ansible-core version (k8s-installer ships 2.17 and 2.19):
//
//	node | CHANGED | rc=0 | (stdout) /usr/bin/dash
//	node | FAILED | rc=3 | (stdout) missing             2.19, then the JSON line below
//	node | FAILED! => {"rc": 3, "stdout": "missing"}    2.17: a failed command is this line only
//	node | UNREACHABLE!: Failed to connect ...          (older: "UNREACHABLE! => {...}")
var adhocFull = regexp.MustCompile(`^(\S+) \| ([A-Z]+)!?:? (?:\| rc=(\d+) )?(.*)$`)

// onelineText undoes what -o does to the output of a command to keep it on one
// line: a newline printed as \n, a carriage return as \r. The raw module (P2,
// P6: before K1 the nodes have no python of the installer for the command
// module) runs under a terminal (ssh -tt), so its stdout ends in \r\n: without
// this "active\r\n" is not "active", and a dash path would carry it into
// ansible_shell_executable.
var onelineText = strings.NewReplacer(`\r\n`, "\n", `\n`, "\n", `\r`, "")

func parseAdhoc(out []byte) map[string]adhocOut {
	res := map[string]adhocOut{}
	for _, l := range strings.Split(string(out), "\n") {
		m := adhocFull.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		if prev, seen := res[m[1]]; seen && prev.RC != "" && m[3] == "" {
			continue // the JSON line after the rc line of the same node: the rc line says more
		}
		a := adhocOut{Status: m[2], RC: m[3], Raw: m[4]}
		if i := strings.Index(a.Raw, "(stdout) "); i >= 0 {
			a.Stdout = a.Raw[i+len("(stdout) "):]
			if j := strings.Index(a.Stdout, " (stderr)"); j >= 0 {
				a.Stdout = a.Stdout[:j]
			}
			a.Stdout = strings.TrimSpace(onelineText.Replace(a.Stdout))
		} else if js, isJSON := strings.CutPrefix(a.Raw, "=> "); isJSON {
			var r struct {
				RC     *int   `json:"rc"`
				Stdout string `json:"stdout"`
				Stderr string `json:"stderr"`
			}
			if json.Unmarshal([]byte(js), &r) == nil && r.RC != nil {
				a.RC, a.Stdout = strconv.Itoa(*r.RC), strings.TrimSpace(r.Stdout)
				a.Raw = strings.TrimSpace(r.Stdout + " " + r.Stderr)
			}
		}
		res[m[1]] = a
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
	msg := i18n.T("ansible could not be run")
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

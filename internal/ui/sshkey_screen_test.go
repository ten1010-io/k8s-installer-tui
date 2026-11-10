package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

const sshInventory = `{"_meta": {"hostvars": {
  "node1": {"ansible_host": "192.168.0.11", "ansible_user": "ops"},
  "node2": {"ansible_host": "192.168.0.12", "ansible_user": "ops"},
  "node3": {"ansible_host": "192.168.0.21", "ansible_user": "ops"}}},
  "all": {"hosts": ["node1", "node2", "node3"]}}`

// sshSite fakes the nodes of the plan screen for P2 and for sending the key:
// what ssh says to the key of each node (by user@host, "" takes it), and records
// every command with the environment it got.
type sshSite struct {
	mu    sync.Mutex
	key   map[string]string
	calls []string // name, arguments and environment of every command
	p2    int      // ansible runs of `sudo -n true`: P2, and P6 first
	block bool     // a send waits for Ctrl+C
}

func (f *sshSite) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("%s %q env=%q", name, args, plan.EnvOf(ctx)))
	switch name {
	case "bash":
		script := args[len(args)-1]
		switch {
		case strings.Contains(script, "configure-control-node-ssh.sh"):
			k := plan.DefaultSSHKey()
			os.MkdirAll(filepath.Dir(k.Public), 0700)
			return nil, os.WriteFile(k.Public, []byte("ssh-rsa AAAA ops@cp"), 0644)
		case strings.Contains(script, "ansible-inventory"):
			return []byte(sshInventory), nil
		case strings.Contains(script, "sudo -n /bin/sh -c true"):
			return []byte("node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \nnode3 | CHANGED | rc=0 | (stdout) \n"), nil
		case strings.Contains(script, "'sudo -n true'"):
			f.p2++
			var out string
			for i, host := range []string{"192.168.0.11", "192.168.0.12", "192.168.0.21"} {
				if f.key["ops@"+host] == "" {
					out += fmt.Sprintf("node%d | CHANGED | rc=0 | (stdout) \n", i+1)
				} else {
					out += fmt.Sprintf("node%d | UNREACHABLE!: Failed to connect to the host via ssh\n", i+1)
				}
			}
			return []byte(out), nil
		}
	case "ssh":
		if out := f.key[args[len(args)-2]]; out != "" {
			return []byte(out), errors.New("exit status 255")
		}
		return nil, nil
	case "setsid":
		if f.block {
			f.mu.Unlock()
			<-ctx.Done()
			f.mu.Lock()
			return nil, ctx.Err()
		}
		f.key[args[len(args)-1]] = "" // the key is there now
		return []byte("Number of key(s) added: 1\n"), nil
	}
	return nil, nil
}

func newSSHScreen(t *testing.T, key map[string]string) (*screens.S9Install, *sshSite) {
	t.Setenv("HOME", t.TempDir())
	d, paths := newDriver(t)
	s := d.app.appState
	s.AipubEnabled = false
	opts := plan.DefaultOptions()
	opts.Root = t.TempDir()
	s9 := screens.NewS9Install(s, paths, opts)
	s9.SetSize(110, 120)
	s9.SyncFromState(s)
	site := &sshSite{key: key}
	s9.SetRunner(site.run)
	s9.Update(tea.KeyMsg{Type: tea.KeyDown}) // off the archive path
	return s9, site
}

// drain feeds the screen what its commands return until none is left, as
// bubbletea would, and returns every message.
func drain(t *testing.T, s9 *screens.S9Install, cmd tea.Cmd) []tea.Msg {
	var msgs []tea.Msg
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		msgs = append(msgs, msg)
		_, cmd = s9.Update(msg)
	}
	return msgs
}

func key(r string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

func view(s9 *screens.S9Install) string { return ansi.ReplaceAllString(s9.View(), "") }

const refused = "ops@192.168.0.12: Permission denied (publickey,password).\n"

// The offer to send the key is there only when a node refuses it; a node that is
// down gets its fix, not a key.
func TestPlanScreenOffersTheKeyOnlyToNodesRefusingIt(t *testing.T) {
	for _, c := range []struct {
		name  string
		key   map[string]string
		offer bool
		want  string
	}{
		{"refused", map[string]string{"ops@192.168.0.12": refused}, true, "키를 받지 않는 노드: node2 — k 키 보내기"},
		{"down", map[string]string{"ops@192.168.0.12": "ssh: connect to host 192.168.0.12 port 22: Connection refused\n"}, false,
			"node2: 접속 안 됨 — 접속 불가: 접속하지 못함"},
		{"all ready", map[string]string{}, false, "정상: node1 node2 node3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s9, _ := newSSHScreen(t, c.key)
			_, cmd := s9.Update(key("c"))
			drain(t, s9, cmd)
			v := view(s9)
			if !strings.Contains(v, c.want) {
				t.Errorf("lacks %q:\n%s", c.want, v)
			}
			if got := strings.Contains(v, "k·p 키 보내기"); got != c.offer {
				t.Errorf("offer shown %v, want %v:\n%s", got, c.offer, v)
			}
			if _, cmd := s9.Update(key("p")); !c.offer && (cmd != nil || strings.Contains(view(s9), "SSH 비밀번호")) {
				t.Error("p asks a password with nothing to send")
			}
		})
	}
}

// p: the password is typed into a masked input, gone from the screen as soon as
// the send starts, in no message and in no argument; P2 runs again at the end.
func TestPlanScreenSendsTheKeyWithOnePassword(t *testing.T) {
	const password = " s3cret pw$ "
	s9, site := newSSHScreen(t, map[string]string{"ops@192.168.0.12": refused, "ops@192.168.0.21": refused})
	_, cmd := s9.Update(key("c"))
	drain(t, s9, cmd)
	checked := site.p2

	s9.Update(key("p"))
	for _, r := range password {
		s9.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	v := view(s9)
	if !strings.Contains(v, "SSH 비밀번호 (node2 node3 공통): ") || !strings.Contains(v, strings.Repeat("*", len(password))) || strings.Contains(v, "s3cret") {
		t.Fatalf("the input is not masked:\n%s", v)
	}

	_, cmd = s9.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if v := view(s9); strings.Contains(v, "s3cret") || strings.Contains(v, "SSH 비밀번호") || !strings.Contains(v, "키 보내는 중 (2대") {
		t.Errorf("the input is not cleared when the send starts:\n%s", v)
	}
	msgs := drain(t, s9, cmd)
	for _, m := range msgs {
		if strings.Contains(fmt.Sprintf("%#v", m), "s3cret") {
			t.Errorf("a message has the password: %T", m)
		}
	}
	if site.p2 != checked+1 {
		t.Errorf("P2 ran %d times after the send, want 1", site.p2-checked)
	}
	var sends int
	for _, c := range site.calls {
		before, env, _ := strings.Cut(c, " env=")
		if strings.Contains(before, "s3cret") {
			t.Errorf("an argument has the password: %s", before)
		}
		if strings.HasPrefix(c, "setsid ") {
			sends++
			if !strings.Contains(env, "_PASSWORD= s3cret pw$ \"") {
				t.Errorf("the send lacks the password in its environment: %s", env)
			}
		} else if strings.Contains(env, "s3cret") {
			t.Errorf("not a send, with the password: %s", c)
		}
	}
	if sends != 2 {
		t.Errorf("%d sends, want 2", sends)
	}
	v = view(s9)
	if !strings.Contains(v, "✓ P2") || !strings.Contains(v, "정상: node1 node2 node3") || strings.Contains(v, "k·p 키 보내기") ||
		!strings.Contains(v, "키 보내기를 마쳤습니다") || strings.Contains(v, "s3cret") {
		t.Errorf("after the send:\n%s", v)
	}
}

// k: the key pair is made, then the terminal goes to ssh-copy-id (P2 running
// again when it is back is TestS9RunsP2AfterTheTerminal of the screens).
func TestPlanScreenSendsTheKeyOnTheTerminal(t *testing.T) {
	s9, _ := newSSHScreen(t, map[string]string{"ops@192.168.0.12": refused})
	_, cmd := s9.Update(key("c"))
	drain(t, s9, cmd)

	_, cmd = s9.Update(key("k"))
	if cmd == nil {
		t.Fatal("k does nothing")
	}
	ready := cmd()
	if fmt.Sprintf("%T", ready) != "screens.keyReadyMsg" {
		t.Fatalf("%T", ready)
	}
	if _, err := os.Stat(plan.DefaultSSHKey().Public); err != nil {
		t.Errorf("no key made: %v", err)
	}
	if _, exec := s9.Update(ready); exec == nil {
		t.Fatal("no terminal handed to ssh-copy-id")
	}
}

// Ctrl+C during a send stops it and stays on the screen; P2 runs again.
func TestPlanScreenStopsASend(t *testing.T) {
	d, _ := newDriver(t)
	s9, site := newSSHScreen(t, map[string]string{"ops@192.168.0.12": refused})
	_, cmd := s9.Update(key("c"))
	drain(t, s9, cmd)
	checked := site.p2
	site.block = true
	s9.Update(key("p"))
	s9.Update(key("pw"))
	_, cmd = s9.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// the App gives Ctrl+C to the screen while it sends
	d.app.screenList[d.app.current] = s9
	if _, quit := d.app.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); quit != nil {
		t.Fatal("Ctrl+C quits during a send")
	}
	drain(t, s9, cmd)
	if v := view(s9); !strings.Contains(v, "키 보내기를 중단했습니다") || site.p2 != checked+1 {
		t.Errorf("P2 ran %d times after the stop:\n%s", site.p2-checked, v)
	}
	// nothing running: Ctrl+C quits
	if _, quit := d.app.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); quit == nil {
		t.Error("Ctrl+C does not quit")
	}
}

// On a screen too low for the plan the password input stays in sight while it
// is asked, and the keys that scroll the plan or move the focus go to the input.
func TestPlanScreenKeepsThePasswordInputInSight(t *testing.T) {
	s9, _ := newSSHScreen(t, map[string]string{"ops@192.168.0.12": refused})
	_, cmd := s9.Update(key("c"))
	drain(t, s9, cmd)
	s9.SetSize(110, 22) // the plan is taller than that
	s9.Update(key("p"))
	before := view(s9)
	if !strings.Contains(before, "── 계획 ") {
		t.Fatalf("the plan fits; the test needs a lower screen:\n%s", before)
	}
	if !strings.Contains(before, "SSH 비밀번호 (node2 공통): ") {
		t.Fatalf("the input is out of sight:\n%s", before)
	}
	for _, k := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyUp}, {Type: tea.KeyTab}} {
		s9.Update(k)
	}
	if v := view(s9); v != before {
		t.Errorf("a scroll or focus key moved the screen while asking:\n%s", v)
	}
	s9.Update(key("x"))
	if v := view(s9); !strings.Contains(v, "SSH 비밀번호 (node2 공통): *") {
		t.Errorf("typed text not in the input:\n%s", v)
	}
}

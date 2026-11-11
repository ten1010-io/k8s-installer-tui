package screens

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The install and preview screens, and the load balancer and certificate mode
// screens, in English (the default): no Hangul. What these screens show of
// other packages (the plan's steps and checks, the schema's descriptions, the
// validator's errors) is made English apart; until it is, its words are let
// through (installForeignHangul), and once it is, nothing is.

// installHangulRuns are the runs of Hangul letters in s.
func installHangulRuns(s string) []string {
	var runs []string
	var cur []rune
	for _, r := range s + " " {
		if unicode.Is(unicode.Hangul, r) {
			cur = append(cur, r)
			continue
		}
		if len(cur) > 0 {
			runs = append(runs, string(cur))
			cur = nil
		}
	}
	return runs
}

var (
	installForeignOnce  sync.Once
	installForeignWords string
	installForeignErr   error
)

// installOwnFiles are the files of this area (with every non-test file of
// internal/ui): their words are not foreign.
var installOwnFiles = map[string]bool{
	"internal/ui/screens/common.go": true, "internal/ui/screens/s6_aipub_lb.go": true, "internal/ui/screens/s6_cert_mode.go": true,
	"internal/ui/screens/s7_preview.go": true, "internal/ui/screens/s9_install.go": true, "internal/ui/screens/s9_run.go": true,
}

// installForeignHangul are the Hangul words the program says outside the
// Korean catalogs and the files of this area, one per line: in the string
// literals of its Go files (not the tests) and in the templates fileio writes.
// These are the other packages' and screens' words, which a screen here may
// show until they are English, wrapped anywhere (a part of one is one of them).
func installForeignHangul(t *testing.T) string {
	t.Helper()
	installForeignOnce.Do(func() {
		root, err := filepath.Abs("../../..")
		if err != nil {
			installForeignErr = err
			return
		}
		var words strings.Builder
		add := func(s string) {
			for _, r := range installHangulRuns(s) {
				words.WriteString(r + "\n")
			}
		}
		fset := token.NewFileSet()
		installForeignErr = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if name == "testdata" || name == "test" || (strings.HasPrefix(name, ".") && path != root) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			switch {
			case strings.HasSuffix(name, ".go"):
				if strings.HasSuffix(name, "_test.go") || name == "ko.go" || strings.HasPrefix(name, "ko_") ||
					installOwnFiles[rel] || filepath.Dir(rel) == "internal/ui" {
					return nil
				}
				f, err := parser.ParseFile(fset, path, nil, 0)
				if err != nil {
					return err
				}
				ast.Inspect(f, func(n ast.Node) bool {
					if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if s, err := strconv.Unquote(lit.Value); err == nil {
							add(s)
						}
					}
					return true
				})
			case strings.HasPrefix(rel, "internal/fileio/templates/"):
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				add(string(data))
			}
			return nil
		})
		installForeignWords = words.String()
	})
	if installForeignErr != nil {
		t.Fatal(installForeignErr)
	}
	return installForeignWords
}

// installEnglish fails on a Hangul word in view that is none of the other
// packages' (installForeignHangul).
func installEnglish(t *testing.T, what, view string) {
	t.Helper()
	view = ansiCodes.ReplaceAllString(view, "")
	foreign := installForeignHangul(t)
	var own []string
	for _, w := range installHangulRuns(view) {
		if !strings.Contains(foreign, w) {
			own = append(own, w)
		}
	}
	if len(own) > 0 {
		t.Errorf("%s says Korean in English (%s):\n%s", what, strings.Join(own, " "), view)
	}
}

// installNoHangul fails on any Hangul in s: text that is this area's alone.
func installNoHangul(t *testing.T, what string, s ...string) {
	t.Helper()
	for _, l := range s {
		if runs := installHangulRuns(l); len(runs) > 0 {
			t.Errorf("%s says Korean in English: %q", what, l)
		}
	}
}

// installEnglishSize is the room a screen has in a terminal of 80x24 (App.dims
// with the header of the English step names on three lines).
const installEnglishW, installEnglishH = 66, 10

// installLastLine is the last line of a view, where the buttons stay on a low
// screen.
func installLastLine(view string) string {
	lines := strings.Split(ansiCodes.ReplaceAllString(view, ""), "\n")
	return lines[len(lines)-1]
}

// installTail is the end of a view (its last three lines, where FitView keeps
// the buttons or the keys of a run, wrapped) as one line with single spaces, as
// installWords puts what is looked for there.
func installTail(view string) string {
	lines := strings.Split(ansiCodes.ReplaceAllString(view, ""), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return installWords(strings.Join(lines, " "))
}

func installWords(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestInstallScreensSayEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)

	t.Run("certificate mode", func(t *testing.T) {
		st := state.DefaultState()
		s := NewS6CertMode()
		s.SyncFromState(st)
		s.SetSize(100, 40)
		v := s.View()
		installEnglish(t, "the certificate mode", v)
		for _, want := range []string{"Choose the certificate mode", "Creates a CA and issues the TLS certificates.", "< Back >", "< Next >"} {
			if !strings.Contains(ansiCodes.ReplaceAllString(v, ""), want) {
				t.Errorf("no %q:\n%s", want, v)
			}
		}
		installNoHangul(t, "the title", s.Title())
		s.SetSize(installEnglishW, installEnglishH)
		if l := installLastLine(s.View()); !strings.Contains(l, "< Back >") || !strings.Contains(l, "< Next >") {
			t.Errorf("at 80x24 the buttons are cut: %q", l)
		}
	})

	t.Run("load balancers", func(t *testing.T) {
		st := state.DefaultState()
		s := NewS6AipubLB()
		s.SyncFromState(st)
		s.SetSize(100, 40)
		installNoHangul(t, "the load balancers, shared IP, no nodes", s.View(), s.Title())
		if v := s.View(); !strings.Contains(v, "(set the CP nodes in step 2)") || !strings.Contains(v, "Ingress/gateway shared VIP") {
			t.Errorf("load balancers:\n%s", v)
		}
		// separate IPs, every field focused in turn, each refusal
		st.K8sCpNodes = []string{"node1", "node2"}
		st.K8sIngressMode = 1
		s.SyncFromState(st)
		for i := 0; i <= s6FocusMax; i++ {
			s.focusIdx = i
			s.syncFocus()
			installNoHangul(t, "the load balancers, separate IPs", s.View())
		}
		refusals := []func(){
			func() {},
			func() { s.ingressLBName.SetValue("lb1") },
			func() { s.ingressVIP.SetValue("192.168.0.100") },
			func() { s.gatewayLBName.SetValue("lb1") },
			func() { s.gatewayLBName.SetValue("lb2") },
			func() { s.gatewayVIP.SetValue("192.168.0.100") },
			func() { s.gatewayVIP.SetValue("192.168.0.101"); s.hostPort.SetValue("0") },
		}
		for _, set := range refusals {
			set()
			errs := s.Validate()
			if len(errs) == 0 {
				t.Fatalf("not refused: %+v", s)
			}
			installNoHangul(t, "a refusal of the load balancers", errs...)
		}
		s.hostPort.SetValue("443")
		if errs := s.Validate(); len(errs) != 0 {
			t.Errorf("refused: %v", errs)
		}
		s.focusIdx = s6FocusNav
		s.SetSize(installEnglishW, installEnglishH)
		if l := installLastLine(s.View()); !strings.Contains(l, "< Back >") || !strings.Contains(l, "< Next >") {
			t.Errorf("at 80x24 the buttons are cut: %q", l)
		}
	})

	t.Run("preview", func(t *testing.T) {
		// AIPub with JWKS waiting for its CA, the audit policy on, and nodes the
		// validator refuses: errors, warnings and notes
		st := state.DefaultState()
		st.AipubEnabled = true
		st.AipubHaModeStorageClass = "nfs-client"
		st.K8sIngressVIP = "10.1.0.5"
		st.ControlPlane.Audit = true
		st.ControlPlane.JWKS = true
		paths := fileio.DefaultPaths(t.TempDir())
		s := NewS7Preview(st, paths, t.TempDir())
		s.SetSize(110, 60)
		s.SyncFromState(st)
		if len(s.tabs) == 0 || !s.tabs[0].isNotes || s.tabs[0].name != "Notes" {
			t.Fatalf("the preview does not open on its notes: %+v", s.tabs)
		}
		for i := range s.tabs {
			s.activeTab = i
			s.refreshViewport()
			installEnglish(t, "the preview, tab "+s.tabs[i].name, s.View())
		}
		installNoHangul(t, "the preview", s.Title())
		installEnglish(t, "the preview refuses", strings.Join(s.Validate(), "\n")) // the validator's errors
		s.doSave()                                                                 // refused: the errors
		if !strings.Contains(s.saveErr, "Cannot save") {
			t.Errorf("save error %q", s.saveErr)
		}
		installEnglish(t, "the preview refusing to save", s.View())

		// a site the validator takes: saved, and the files said
		st = state.DefaultState()
		st.Nodes = []state.NodeConfig{{Name: "node1", AnsibleHost: "192.168.0.11"}}
		st.KiCpNodes, st.K8sNodes, st.K8sCpNodes = []string{"node1"}, []string{"node1"}, []string{"node1"}
		st.KiCpHaMode = false
		s = NewS7Preview(st, paths, t.TempDir())
		s.SetSize(110, 60)
		s.SyncFromState(st)
		if len(s.errors) != 0 {
			t.Fatalf("refused: %v", s.errors)
		}
		installNoHangul(t, "the preview before saving", s.Validate()...)
		s.doSave()
		if !s.saved {
			t.Fatalf("not saved: %s", s.saveErr)
		}
		s.activeTab = 0
		s.refreshViewport()
		v := ansiCodes.ReplaceAllString(s.View(), "")
		installEnglish(t, "the preview saved", v)
		for _, want := range []string{"✓ Saved files (the previous content kept in", "✓ Saved — the previous content kept in", "< Next >"} {
			if !strings.Contains(v, want) {
				t.Errorf("no %q:\n%s", want, v)
			}
		}
		s.SetSize(installEnglishW, installEnglishH)
		if l := installLastLine(s.View()); !strings.Contains(l, "< Back >") || !strings.Contains(l, "< Next >") {
			t.Errorf("at 80x24 the buttons are cut: %q", l)
		}
	})
}

// installState checks the install screen in one state: its view (at its size,
// and at 80x24 where the buttons or the keys of a run stay at the end), and
// what the wizard and the footer say of it.
func installState(t *testing.T, s9 *S9Install, what string, end ...string) {
	t.Helper()
	installEnglish(t, what, s9.View())
	installNoHangul(t, what+": the reason the wizard stays", s9.BusyReason())
	installNoHangul(t, what+": the footer", s9.KeyHints()...)
	if s9.confirm != "" {
		installNoHangul(t, what+": the question", s9.confirmQuestion())
	}
	w, h := s9.width, s9.height
	s9.SetSize(installEnglishW, installEnglishH)
	defer s9.SetSize(w, h)
	v := s9.View()
	installEnglish(t, what+" at 80x24", v)
	tail := installTail(v)
	for _, want := range end {
		if !strings.Contains(tail, installWords(want)) {
			t.Errorf("%s at 80x24: no %q at the end %q:\n%s", what, want, tail, ansiCodes.ReplaceAllString(v, ""))
		}
	}
}

func TestInstallScreenSaysEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)

	s9, f := newRunScreen(t)
	installNoHangul(t, "the install screen", s9.Title(), s9.archive.Placeholder)
	installState(t, s9, "the plan", "< Back >", "< Run >")
	if v := plainView(s9); !strings.Contains(v, "Install plan") || !strings.Contains(v, "Run order") {
		t.Errorf("the plan:\n%s", v)
	}
	p := newPump(s9)

	// the checks under way, then a node refusing the key
	p.key(tea.KeyMsg{Type: tea.KeyTab}) // off the archive path
	_, checks := s9.Update(runeKey("c"))
	if !s9.checking {
		t.Fatal("the checks do not run")
	}
	installState(t, s9, "the checks running", "< Back >", "< Run >")
	if !strings.Contains(plainView(s9), "Checking... (ansible all -m raw -a 'sudo -n true')") {
		t.Errorf("the checks running:\n%s", plainView(s9))
	}
	p.run(checks)
	p.until(t, "the checks end", func() bool { return !s9.checking })
	s9.ssh = []plan.NodeSSH{{Node: plan.SSHNode{Name: "node1", Host: "192.168.0.11"}, Key: plan.KeyRefused}}
	installState(t, s9, "a node refusing the key", "< Back >", "< Run >")
	if !strings.Contains(plainView(s9), "Nodes refusing the key: node1") {
		t.Errorf("no offer of the key:\n%s", plainView(s9))
	}

	// p: the password asked; k: the key prepared, then sent on the terminal
	s9.Update(runeKey("p")) // the blink of its input is not run
	if !s9.asking {
		t.Fatal("p does not ask the password")
	}
	installState(t, s9, "the password asked")
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	s9.Update(runeKey("k"))
	if !s9.preparing {
		t.Fatal("k does not prepare the key")
	}
	installState(t, s9, "the key prepared", "< Back >", "< Run >")
	s9.Update(keyReadyMsg{id: s9.prepID, nodes: plan.Refusing(s9.ssh)}) // its ssh-copy-id is not run
	installState(t, s9, "the key sent on the terminal", "< Back >", "< Run >")
	s9.send = &sendRun{ch: make(chan tea.Msg, 1), done: make(chan struct{})}
	s9.Update(sendProgressMsg{run: s9.send, result: plan.NodeSSH{Node: plan.SSHNode{Name: "node1"}}})
	if !strings.Contains(plainView(s9), "node1: key accepted") {
		t.Errorf("no node done:\n%s", plainView(s9))
	}
	s9.send = nil
	_, p2 := s9.Update(sendDoneMsg{err: errors.New("exit status 1")})
	installState(t, s9, "the key send ended", "< Back >", "< Run >")
	if !strings.Contains(s9.message, "Sending the key did not finish (exit status 1)") {
		t.Errorf("message %q", s9.message)
	}
	p.run(p2)
	p.until(t, "P2 ends", func() bool { return !s9.checking })

	// the commands
	s9.Update(runeKey("d"))
	installState(t, s9, "the commands")
	if !strings.Contains(plainView(s9), "Install commands") {
		t.Errorf("the commands:\n%s", plainView(s9))
	}
	s9.Update(runeKey("d"))

	// the questions asked before a run
	for _, c := range []string{"resume", "fresh", "no-archive", "reset-cluster"} {
		s9.confirm = c
		installState(t, s9, "the question "+c, "< Back >", "< Run >")
	}
	s9.confirm = ""

	// a run: the log of the step that runs, the StorageClass stop, the end
	release := make(chan struct{})
	f.block["setup-k8s-installer"] = release
	p.pressRun()
	p.until(t, "K1 runs", func() bool { return s9.stepStatus("K1") == plan.StatusRunning })
	p.until(t, "the log of K1 is shown", func() bool { return strings.Contains(plainView(s9), "TASK [fake task of setup-k8s-installer]") })
	installState(t, s9, "a run", "Ctrl+C stop")
	if v := plainView(s9); !strings.Contains(v, "Install run") || !strings.Contains(v, "── log K1 (last 10 lines) ──") {
		t.Errorf("a run:\n%s", v)
	}
	close(release)
	p.until(t, "M1 waits", func() bool { return s9.stepStatus("M1") == plan.StatusWaiting })
	installState(t, s9, "the StorageClass stop", "Enter installed, continue")
	f.mu.Lock()
	f.classReady = true
	f.mu.Unlock()
	p.key(tea.KeyMsg{Type: tea.KeyEnter})
	if s9.message != "Checking the StorageClass again..." || !s9.rechecking {
		t.Errorf("Enter at the stop: %q", s9.message)
	}
	p.until(t, "the run ends", func() bool { return s9.ex.done })
	if s9.message != "" || s9.rechecking {
		t.Errorf("the word of Enter at the stop outlived it: %q", s9.message)
	}
	installState(t, s9, "a run that ended", "Esc to the plan")
	if !strings.Contains(plainView(s9), "✓ The install finished") {
		t.Errorf("the end:\n%s", plainView(s9))
	}
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	s9.SyncFromState(s9.appState)
	installState(t, s9, "the plan after the run", "< Back >", "< Run >")
	if !strings.Contains(plainView(s9), "All done. f runs it again from the start") {
		t.Errorf("the last run:\n%s", plainView(s9))
	}
}

func TestInstallFailedRunSaysEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)

	// K3 failed on a new site: the summary, its log, the reset offered
	s9, f := newRunScreen(t)
	f.fail["setup-cluster"] = 2
	f.say["setup-cluster"] = "TASK [kubeadm init]\n"
	resetRecorder(s9)
	p := newPump(s9)
	p.pressRun()
	p.until(t, "the run ends", func() bool { return s9.ex != nil && s9.ex.done })
	installState(t, s9, "a failed run", "r retry the failed step", "x reset the cluster")
	v := plainView(s9)
	for _, want := range []string{"✗ Failed. Once fixed, Run resumes from the failed step", "✗ K3 setup-cluster (exit code 2)  log: "} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	p.key(runeKey("l"))
	installState(t, s9, "the log of the failed step")
	if !strings.Contains(plainView(s9), "Log K3 setup-cluster") {
		t.Errorf("the log:\n%s", plainView(s9))
	}
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	p.key(runeKey("x"))
	installState(t, s9, "the reset asked")
	if !strings.Contains(plainView(s9), "This cannot be undone. To reset, type reset and press Enter") {
		t.Errorf("the reset question:\n%s", plainView(s9))
	}
	p.key(tea.KeyMsg{Type: tea.KeyEsc})
	installNoHangul(t, "the reset cancelled", s9.message)
	p.key(tea.KeyMsg{Type: tea.KeyEsc}) // the plan
	s9.SyncFromState(s9.appState)
	installState(t, s9, "the plan after a failed run", "< Back >", "< Resume >")
	if v := plainView(s9); !strings.Contains(v, "left: ✗K3") || !strings.Contains(v, "x reset the cluster") {
		t.Errorf("the plan after a failed run:\n%s", v)
	}
	// Ctrl+C during a run, twice
	release := make(chan struct{})
	f.mu.Lock()
	f.fail = map[string]int{}
	f.block["setup-cluster"] = release
	f.mu.Unlock()
	p.pressRun()
	p.until(t, "K3 runs", func() bool { return s9.stepStatus("K3") == plan.StatusRunning })
	for i := 0; i < 2; i++ {
		s9.Interrupt()
		installNoHangul(t, "Ctrl+C during a run", s9.message)
	}
	p.until(t, "the run stops", func() bool { return s9.ex.done })
	installState(t, s9, "a run stopped")
}

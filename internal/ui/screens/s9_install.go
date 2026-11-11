package screens

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/styles"
)

// The installation plan of the saved settings (internal/plan): the checks to make
// and the playbooks to run, in order. It shows the plan, its commands and a
// script of them, runs the checks, and runs the plan (실행, s9_run.go): the
// checks first, then the stages, stopping once for the StorageClass driver, and
// going on from the last run (state.json) when there is one.
//
// Focus layout:
//   0  : 아카이브 경로 (K4)
//   1  : 모니터링 설치 (AIPub only)
//   2  : the plan, when it is taller than the room it has (↑/↓ scroll; ↓ at
//        its bottom goes on to the buttons)
//   3  : nav slot (←/→: 이전/실행)
//
// Off the text input: c 사전 확인, d 명령 보기, w 스크립트 저장, f 처음부터
// (when there was a run), and when P2
// found nodes that refuse the key of the control node: k 키 보내기 (ssh asks
// the password of each node on the terminal), p 비밀번호 한 번 (a masked input,
// handed to ssh through SSH_ASKPASS). Both run P2 again at the end.

const (
	s9FocusArchive = iota
	s9FocusMonitoring
	s9FocusPlan
	s9FocusNav
)

// checkDoneMsg carries the results of the checks that run ansible and take a
// while: P2, P6, and P3 on an existing site, by step id. shell is what P6 found
// for plan.Options.ShellExecutable (empty: none needed); set only when P6 ran
type checkDoneMsg struct {
	results  map[string]plan.CheckResult
	shell    string
	shellSet bool
	// what P2 found with ssh on the nodes ansible could not reach; set when P2 ran
	ssh    []plan.NodeSSH
	sshSet bool
}

// keyReadyMsg: the key pair is there (or err), the terminal can go to ssh-copy-id
// for nodes.
type keyReadyMsg struct {
	id    int // of the k it answers (S9Install.prepID)
	err   error
	nodes []plan.SSHNode
}

// sendProgressMsg is a node done by the one-password send. Neither it nor
// sendDoneMsg carries the password.
type sendProgressMsg struct {
	run    *sendRun
	result plan.NodeSSH
}

// sendDoneMsg ends a send: of run for the one-password send, nil for the
// terminal (k).
type sendDoneMsg struct {
	run *sendRun
	err error
}

type S9Install struct {
	appState *state.AppState
	paths    fileio.Paths
	opts     plan.Options
	base     plan.Options // as the wizard gave them; opts may take the last run's
	run      plan.RunFunc
	exec     plan.StreamFunc // the steps of a run; nil: plan.ExecStream

	archive textinput.Model
	steps   []plan.Step
	checks  map[string]plan.CheckResult
	// checksFor is checkKey when the checks ran: a gate on the checks of another
	// site (saved again since, or other options) runs them again
	checksFor string
	checking  bool
	message   string

	// sending the key (k, p): ssh holds what P2 found, pass the masked input of
	// p while asking, and while sending cancel stops it and progress gives each
	// node as it is done
	ssh       []plan.NodeSSH
	pass      textinput.Model
	asking    bool
	sending   bool
	preparing bool // k: the key pair is being made, prepID is its answer
	prepID    int
	send      *sendRun // the one-password send under way, nil for k
	sendLines []string

	showScript bool
	vp         viewport.Model
	planVP     viewport.Model // the plan, cut to the room under the inputs

	// the run (s9_run.go): ex while it runs and after, until Esc; last the last
	// run (state.json); confirm the y/n asked before one ("resume", "fresh")
	ex *execRun
	// active is the last run started, for stopRun, which runs off the update
	// loop (Ctrl+C, quitting) and may not read ex
	active         atomic.Pointer[execRun]
	last           *plan.RunState
	inputHash      string // of the site files when the screen was entered (viewPrev)
	confirm        string
	runAfterChecks bool // 실행 (or f) waits for the checks, then goes on (gate)
	fresh          bool // the run that waits is from the first step (f)
	showLog        bool
	logVP          viewport.Model
	logStep        plan.StepState
	selMoved       bool

	focusIdx int
	navIdx   int
	width    int
	height   int
}

func NewS9Install(s *state.AppState, paths fileio.Paths, opts plan.Options) *S9Install {
	a := textinput.New()
	a.Placeholder = "예: /media/usb/ki-cp-user-registry-artifacts.tgz (이미 넣었으면 비움)"
	a.CharLimit = 256
	p := textinput.New()
	p.EchoMode = textinput.EchoPassword
	p.CharLimit = 256
	p.Prompt = ""
	return &S9Install{
		appState: s, paths: paths, opts: opts, base: opts, run: plan.ExecRun,
		archive: a, pass: p, checks: map[string]plan.CheckResult{}, vp: viewport.New(80, 20), planVP: viewport.New(80, 20),
	}
}

// SetRunner replaces how the checks run commands (tests).
func (s *S9Install) SetRunner(r plan.RunFunc) { s.run = r }

func (s *S9Install) Title() string { return "설치" }
func (s *S9Install) SetSize(w, h int) {
	s.width, s.height = w, h
	s.vp.Width = w
	s.vp.Height = max(5, h-8)
	s.layout()
}

// layout gives the plan the room left by what is above and below it, as
// rendered at the width of the screen; the whole plan when it fits.
func (s *S9Install) layout() {
	measure := lipgloss.NewStyle().Width(max(1, s.width))
	// wrapped here, so that the viewport counts the lines it shows
	content := measure.Render(s.planText())
	s.planVP.Width = s.width
	s.planVP.SetContent(content)
	room := s.height - lipgloss.Height(measure.Render(s.top())) - lipgloss.Height(measure.Render(s.bottom()))
	if n := lipgloss.Height(content); n <= room {
		s.planVP.Height = n
	} else {
		// and a line that says where in the plan it is; on a low screen as little
		// as a line of the plan, the buttons kept in sight
		s.planVP.Height = max(1, room-1)
	}
	s.planVP.SetYOffset(s.planVP.YOffset)
	// the plan fits now (a larger screen, shorter results): no stop there any more
	if s.focusIdx == s9FocusPlan && s.planFits() {
		s.focusIdx = s9FocusNav
	}
}

// planFits is whether the whole plan is in sight; if not, it is a stop of the
// focus, where ↑/↓ scroll it.
func (s *S9Install) planFits() bool {
	return s.planVP.TotalLineCount() <= s.planVP.Height
}

func (s *S9Install) SyncFromState(st *state.AppState) {
	if s.asking {
		s.closePass()
	}
	s.archive.SetValue(s.opts.ArchivePath)
	s.focusIdx, s.navIdx = s9FocusArchive, 0
	s.showScript = false
	s.message = ""
	s.confirm = ""
	if !s.running() {
		s.ex, s.selMoved = nil, false
		s.loadPrev()
	}
	// hashed once here, not on every frame
	s.inputHash = plan.InputHash(s.inputs())
	if s.checksFor != s.checkKey() && !s.checking {
		// of another site, or other options: not shown, and run again before a run
		s.checks, s.ssh = map[string]plan.CheckResult{}, nil
	}
	s.rebuild()
	s.syncFocus()
	s.planVP.GotoTop()
}

func (s *S9Install) SyncToState(_ *state.AppState) {}
func (s *S9Install) Validate() []string            { return nil }
func (s *S9Install) Init() tea.Cmd                 { return nil }

func (s *S9Install) rebuild() {
	s.opts.ArchivePath = strings.TrimSpace(s.archive.Value())
	s.opts.SiteDir = s.savedIn()
	// the plan of what was saved: whether the authentication config of JWKS went
	// out with its CA
	s.opts.JWKSCAReady = fileio.LookupCA(s.appState).PEM != ""
	s.opts.StorageVars = plan.ReadStorageVars(s.paths.AipubVars)
	s.steps = plan.Build(s.appState, s.opts)
	// wrapped here, so that the viewport counts the lines it shows and scrolls to
	// the last of them (it would wrap a long line itself and cut what overflows)
	s.vp.SetContent(lipgloss.NewStyle().Width(max(1, s.vp.Width)).Render(s.script()))
}

// savedIn is the directory the settings were saved in, the ansible directory when
// the tool runs where k8s-installer reads them.
func (s *S9Install) savedIn() string { return filepath.Dir(s.paths.Inventory) }

func (s *S9Install) script() string { return plan.Script(s.steps, s.opts, s.savedIn()) }

func (s *S9Install) syncFocus() {
	if s.focusIdx == s9FocusArchive && !s.showScript {
		s.archive.Focus()
	} else {
		s.archive.Blur()
	}
}

func (s *S9Install) next(i int) int {
	i++
	if i == s9FocusMonitoring && !s.appState.AipubEnabled {
		i++
	}
	if i == s9FocusPlan && s.planFits() {
		i++
	}
	return min(i, s9FocusNav)
}

func (s *S9Install) prev(i int) int {
	i--
	if i == s9FocusPlan && s.planFits() {
		i--
	}
	if i == s9FocusMonitoring && !s.appState.AipubEnabled {
		i--
	}
	return max(i, 0)
}

func (s *S9Install) runChecks() tea.Cmd {
	s.checks, s.checksFor = map[string]plan.CheckResult{}, s.checkKey()
	s.checks["P1"] = plan.CheckInstalled(s.opts)
	if s.appState.AipubEnabled {
		// local reads, nothing to wait for
		if plan.HelmfileChosen(s.appState) {
			s.checks["P4"] = plan.CheckAipubBin(s.opts, s.appState)
		}
		s.checks["P5"] = plan.CheckCatalog(s.opts)
	}
	p2 := s.runP2()
	if s.appState.AipubEnabled {
		s.checks["P7"] = plan.CheckResult{Lines: []string{"확인 중... (getent hosts " + strings.Join(plan.AccessHosts(s.appState), " ") + ")"}}
	}
	if s.appState.AipubEnabled {
		s.checks["P8"] = plan.CheckResult{Lines: []string{"확인 중... (kubectl get storageclass " + s.appState.AipubHaModeStorageClass + ")"}}
	}
	s.checks["P6"] = plan.CheckResult{Lines: []string{"확인 중... (ansible all -m command -a 'sudo -n /bin/sh -c true')"}}
	opts, run, st := s.opts, s.run, s.appState
	if opts.Existing {
		// read only: the apiserver manifest of the first control plane node
		s.checks["P3"] = plan.CheckResult{Lines: []string{"확인 중... (ansible -m slurp " + plan.APIServerManifest + ")"}}
	}
	return func() tea.Msg {
		done := p2().(checkDoneMsg)
		r := done.results
		var shell string
		r["P6"], shell = plan.CheckBecomeShell(context.Background(), opts, run)
		if st.AipubEnabled {
			r["P7"] = plan.CheckDNS(context.Background(), opts, st, run)
			opts.ShellExecutable = shell // the kubectl of P8 is a become
			r["P8"] = plan.CheckStorageClass(context.Background(), opts, st, run)
		}
		if opts.Existing {
			// after P6: the slurp is a become, too
			opts.ShellExecutable = shell
			r["P3"] = plan.CheckLegacy(context.Background(), opts, st, run)
		}
		return checkDoneMsg{results: r, shell: shell, shellSet: true, ssh: done.ssh, sshSet: true}
	}
}

// runP2 is P2 alone, which sending the key ends with.
func (s *S9Install) runP2() tea.Cmd {
	s.checks["P2"] = plan.CheckResult{Lines: []string{"확인 중... (ansible all -m command -a 'sudo -n true')"}}
	s.checking = true
	opts, run := s.opts, s.run
	return func() tea.Msg {
		r, nodes := plan.CheckSSHNodes(context.Background(), opts, run)
		return checkDoneMsg{results: map[string]plan.CheckResult{"P2": r}, ssh: nodes, sshSet: true}
	}
}

// refusing are the nodes P2 found refusing the key, which k and p send it to.
func (s *S9Install) refusing() []plan.SSHNode {
	if s.checking || s.sending || s.preparing {
		return nil
	}
	return plan.Refusing(s.ssh)
}

// prepareKey makes the key pair when there is none, before the terminal goes to
// ssh-copy-id (k). Until its answer comes nothing else starts (preparing).
func (s *S9Install) prepareKey(nodes []plan.SSHNode) tea.Cmd {
	s.message = "키 준비 중..."
	s.preparing = true
	s.prepID++
	id, opts, run := s.prepID, s.opts, s.run
	return func() tea.Msg {
		return keyReadyMsg{id: id, err: plan.PrepareKey(context.Background(), opts, run, plan.DefaultSSHKey()), nodes: nodes}
	}
}

// sendWithPassword starts the one-password send (p) and clears the input: the
// password lives on only in the goroutine that sends, which zeroes it at the end.
func (s *S9Install) sendWithPassword() tea.Cmd {
	password := []byte(s.pass.Value())
	started := false
	defer func() {
		if !started {
			for i := range password {
				password[i] = 0
			}
		}
	}()
	s.closePass()
	nodes := s.refusing()
	if len(password) == 0 || len(nodes) == 0 {
		s.message = "비밀번호가 비어 있어 보내지 않았습니다"
		return nil
	}
	started = true // the goroutine zeroes it from here
	ctx, cancel := context.WithCancel(context.Background())
	r := &sendRun{ch: make(chan tea.Msg), cancel: cancel, done: make(chan struct{})}
	s.send, s.sending = r, true
	s.message = ""
	s.sendLines = []string{fmt.Sprintf("키 보내는 중 (%d대, Ctrl+C 중단)", len(nodes))}
	opts, run := s.opts, s.run
	go func() {
		defer close(r.done)
		defer close(r.ch)
		defer func() {
			for i := range password {
				password[i] = 0
			}
		}()
		// a screen that no longer reads (cancelled) must not hold the send up
		put := func(m tea.Msg) {
			select {
			case r.ch <- m:
			case <-ctx.Done():
			}
		}
		key := plan.DefaultSSHKey()
		if err := plan.PrepareKey(ctx, opts, run, key); err != nil {
			put(sendDoneMsg{run: r, err: err})
			return
		}
		err := plan.SendKeyWithPassword(ctx, run, key, nodes, password, func(n plan.NodeSSH) { put(sendProgressMsg{run: r, result: n}) })
		put(sendDoneMsg{run: r, err: err})
	}()
	return r.wait()
}

// sendRun is one one-password send: its goroutine gives each node on ch, and
// closes done once it has stopped and the helper directory is gone.
type sendRun struct {
	ch       chan tea.Msg
	cancel   context.CancelFunc
	done     chan struct{}
	stopping bool
}

// wait is the next message of the send; when the goroutine stopped without its
// last one (cancelled), the send is done as cancelled.
func (r *sendRun) wait() tea.Cmd {
	return func() tea.Msg {
		if m, ok := <-r.ch; ok {
			return m
		}
		return sendDoneMsg{run: r, err: context.Canceled}
	}
}

func (s *S9Install) closePass() {
	s.pass.Reset()
	s.pass.Blur()
	s.asking = false
	s.syncFocus()
}

// Busy holds the wizard on this screen while the password is asked or the key
// sent: the messages of the send come to this screen only.
func (s *S9Install) Busy() bool {
	return s.asking || s.sending || s.preparing || s.running() || s.confirm != ""
}

// BusyReason is what the wizard says when it does not leave.
func (s *S9Install) BusyReason() string {
	if s.running() {
		return "설치를 실행하는 중입니다. 끝나거나 Ctrl+C 로 멈춘 뒤 이동하세요"
	}
	if s.confirm != "" {
		return "y 또는 n 으로 답한 뒤 이동하세요"
	}
	if s.preparing {
		return "키를 준비하는 중입니다. 끝난 뒤 이동하세요"
	}
	if s.sending {
		return "키를 보내는 중입니다. 끝나거나 Ctrl+C 로 중단한 뒤 이동하세요"
	}
	return "비밀번호 입력 중입니다. Enter 로 보내거나 Esc 로 취소한 뒤 이동하세요"
}

// KeyHints are the keys of what is going on, for the footer.
func (s *S9Install) KeyHints() []string {
	switch {
	case s.running():
		if _, ok := s.waitingStep(); ok {
			return []string{"Enter 설치함, 계속", "Ctrl+C 멈춤"}
		}
		return []string{"Ctrl+C 멈춤"}
	case s.confirm != "":
		return []string{"y 예", "n 아니오"}
	case s.asking:
		return []string{"Enter 보내기", "Esc 취소"}
	case s.sending:
		return []string{"Ctrl+C 중단"}
	case len(s.refusing()) > 0:
		return []string{"k·p 키 보내기"}
	}
	return nil
}

// Interrupt is Ctrl+C on this screen: it stops a send, or leaves the password
// input, instead of quitting. False when there is nothing to stop, or when a
// stop is under way already: the second Ctrl+C quits (after Stop).
func (s *S9Install) Interrupt() bool {
	switch {
	case s.running():
		return s.interruptRun()
	case s.confirm != "":
		s.confirm, s.message = "", "실행하지 않았습니다"
		return true
	case s.send != nil && !s.send.stopping:
		s.send.stopping = true
		s.send.cancel()
		s.message = "중단하는 중... (한 번 더 Ctrl+C 면 종료)"
		return true
	case s.asking:
		s.closePass()
		return true
	}
	return false
}

// Stop ends a send before the program quits: cancelled, and waited for until
// its helper directory is removed (the commands it runs are killed on cancel).
func (s *S9Install) Stop() {
	s.stopRun()
	if s.asking {
		s.closePass()
	}
	if s.send == nil {
		return
	}
	s.send.cancel()
	select {
	case <-s.send.done:
	case <-time.After(15 * time.Second):
	}
}

func (s *S9Install) writeScript() {
	path := filepath.Join(s.savedIn(), "install-plan.sh")
	if err := os.WriteFile(path, []byte(s.script()), 0755); err != nil {
		s.message = "스크립트 저장 실패: " + err.Error()
		return
	}
	s.message = "스크립트 저장: " + path
}

func (s *S9Install) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	s.layout() // the plan as it is shown: whether it fits, its bottom
	if cmd, ok := s.updateRun(msg); ok {
		return s, cmd
	}
	switch msg := msg.(type) {
	case checkDoneMsg:
		for id, r := range msg.results {
			s.checks[id] = r
		}
		s.checking = false
		if msg.sshSet {
			s.ssh = msg.ssh
		}
		if msg.shellSet {
			s.opts.ShellExecutable = msg.shell
			s.rebuild()
		}
		if s.runAfterChecks {
			// shellSet: all of them ran (runChecks), not P2 alone after a key send
			return s, s.gate(msg.shellSet)
		}
		return s, nil

	case keyReadyMsg:
		if msg.id != s.prepID || !s.preparing || s.sending {
			return s, nil // of a k that is over, or a send runs already
		}
		s.preparing = false
		if msg.err != nil {
			s.message = "키 준비 실패: " + msg.err.Error()
			return s, nil
		}
		s.message = ""
		// the terminal goes to ssh-copy-id, which asks each password itself;
		// sending until it gives the terminal back (sendDoneMsg without a run)
		s.sending = true
		s.sendLines = []string{"터미널에서 키를 보내는 중 (ssh-copy-id)"}
		cmd := exec.Command("bash", "-c", plan.AskSendScript(plan.DefaultSSHKey(), msg.nodes, true))
		return s, tea.ExecProcess(cmd, func(err error) tea.Msg { return sendDoneMsg{err: err} })

	case sendProgressMsg:
		if msg.run != s.send {
			return s, nil // of a send that is over
		}
		r := msg.result
		line := r.Node.Name + ": 키 받음"
		if r.Key != plan.KeyOK {
			line = r.Node.Name + ": " + r.Label() + " — " + r.Note()
		}
		s.sendLines = append(s.sendLines, line)
		return s, s.send.wait()

	case sendDoneMsg:
		if msg.run != s.send {
			return s, nil
		}
		s.sending, s.send = false, nil
		switch {
		case errors.Is(msg.err, context.Canceled):
			s.message = "키 보내기를 중단했습니다. P2 를 다시 확인합니다"
		case msg.err != nil:
			s.message = "키 보내기가 끝나지 않았습니다 (" + msg.err.Error() + "). P2 를 다시 확인합니다"
		default:
			s.message = "키 보내기를 마쳤습니다. P2 를 다시 확인합니다"
		}
		return s, s.runP2()

	case tea.KeyMsg:
		if s.confirm != "" {
			switch msg.String() {
			case "y", "Y":
				return s, s.answer(true)
			case "n", "N", "esc":
				return s, s.answer(false)
			}
			return s, nil
		}
		if s.ex != nil {
			return s, s.runKey(msg)
		}
		if s.asking {
			switch msg.String() {
			case "esc":
				s.closePass()
				return s, nil
			case "enter":
				return s, s.sendWithPassword()
			}
			var cmd tea.Cmd
			s.pass, cmd = s.pass.Update(msg)
			return s, cmd
		}
		if s.sending {
			return s, nil // Ctrl+C stops it (Interrupt)
		}
		typing := s.focusIdx == s9FocusArchive && !s.showScript && msg.Type == tea.KeyRunes
		if s.showScript {
			switch msg.String() {
			case "d", "esc", "q":
				s.showScript = false
				s.syncFocus()
				return s, nil
			}
			var cmd tea.Cmd
			s.vp, cmd = s.vp.Update(msg)
			return s, cmd
		}
		if !typing {
			switch msg.String() {
			case "d":
				s.rebuild()
				s.vp.GotoTop()
				s.showScript = true
				s.syncFocus()
				return s, nil
			case "w":
				s.rebuild()
				s.writeScript()
				return s, nil
			case "c":
				if !s.checking {
					return s, s.runChecks()
				}
				return s, nil
			case "f":
				return s, s.pressFresh()
			case "k":
				if nodes := s.refusing(); len(nodes) > 0 {
					return s, s.prepareKey(nodes)
				}
				return s, nil
			case "p":
				if len(s.refusing()) > 0 {
					s.asking = true
					s.archive.Blur()
					s.pass.Reset()
					return s, s.pass.Focus()
				}
				return s, nil
			}
		}
		switch msg.String() {
		case "up":
			if s.focusIdx == s9FocusPlan && !s.planVP.AtTop() {
				s.planVP.LineUp(1)
				return s, nil
			}
			s.focusIdx = s.prev(s.focusIdx)
			s.syncFocus()
			return s, nil
		case "down", "tab":
			if msg.String() == "down" && s.focusIdx == s9FocusPlan && !s.planVP.AtBottom() {
				s.planVP.LineDown(1)
				return s, nil
			}
			s.rebuild()
			s.focusIdx = s.next(s.focusIdx)
			s.syncFocus()
			return s, nil
		case "left":
			if s.focusIdx == s9FocusNav && s.navIdx > 0 {
				s.navIdx--
			}
		case "right":
			if s.focusIdx == s9FocusNav && s.navIdx < 1 {
				s.navIdx++
			}
		case "enter", " ":
			switch s.focusIdx {
			case s9FocusArchive:
				if msg.String() == "enter" {
					s.rebuild()
					s.focusIdx = s.next(s.focusIdx)
					s.syncFocus()
					return s, nil
				}
			case s9FocusMonitoring:
				s.opts.Monitoring = !s.opts.Monitoring
				s.rebuild()
				return s, nil
			case s9FocusNav:
				if s.navIdx == 0 {
					return s, Prev()
				}
				return s, s.pressRun()
			}
		}
	}
	var cmd tea.Cmd
	if s.focusIdx == s9FocusArchive && !s.showScript {
		s.archive, cmd = s.archive.Update(msg)
		s.rebuild()
	}
	return s, cmd
}

// preflightOf is the variable of the schema named key that carries a Preflight
// hint (ansible_user is in two files, only one has it).
func preflightOf(key string) (schema.Var, bool) {
	for _, v := range schema.All() {
		if v.Key == key && v.Preflight != "" {
			return v, true
		}
	}
	return schema.Var{}, false
}

// viewSend is what goes under P2 about sending the key: the offer when nodes
// refuse it, or the progress of a send. The password input is in bottom(),
// where the plan scrolled anywhere does not hide it.
func (s *S9Install) viewSend() string {
	var b strings.Builder
	switch {
	case s.sending:
		for _, l := range s.sendLines {
			b.WriteString(styles.StylePrimary.Render("        "+l) + "\n")
		}
	case s.asking:
		b.WriteString(styles.StylePrimary.Render("        비밀번호는 아래 입력칸에") + "\n")
	default:
		nodes := s.refusing()
		if len(nodes) == 0 {
			break
		}
		var names []string
		for _, n := range nodes {
			names = append(names, n.Name)
		}
		b.WriteString(styles.StyleWarning.Render("        키를 받지 않는 노드: "+strings.Join(names, " ")+
			" — k 키 보내기 (ssh 가 노드마다 비밀번호를 물음)   p 비밀번호 한 번 입력") + "\n")
	}
	return b.String()
}

func (s *S9Install) View() string {
	if s.ex != nil {
		return s.viewRun()
	}
	var b strings.Builder
	if s.showScript {
		b.WriteString(styles.StyleTitle.Render("설치 명령") + styles.StyleMuted.Render("   ↑/↓ 스크롤   d·Esc 닫기") + "\n\n")
		b.WriteString(s.vp.View() + "\n")
		return b.String()
	}

	s.layout()
	top := s.top()
	b.WriteString(top + "\n" + s.planVP.View() + "\n")
	if !s.planFits() {
		// where in the plan the viewport is
		bar := fmt.Sprintf("── 계획 %d–%d / %d줄", s.planVP.YOffset+1, min(s.planVP.YOffset+s.planVP.Height, s.planVP.TotalLineCount()),
			s.planVP.TotalLineCount())
		if s.focusIdx == s9FocusPlan {
			b.WriteString(styles.StylePrimary.Render("▶ "+bar+" ── ↑/↓ 스크롤, 끝에서 ↓ 버튼으로") + "\n")
		} else {
			b.WriteString(styles.StyleMuted.Render("  "+bar+" ── Tab·↓ 로 와서 ↑/↓ 스크롤") + "\n")
		}
	}
	above := strings.Count(b.String(), "\n") // where bottom() starts
	b.WriteString(s.bottom())

	// a screen too low even for a line of the plan: the buttons stay, the
	// focused part in sight over them. top ends on a blank line after the inputs
	view := b.String()
	if s.asking {
		// the password input, the first lines of bottom()
		return FitView(view, above+1, above+3, s.width, s.height)
	}
	n := strings.Count(top, "\n")
	start, end := n-1, n // 모니터링
	switch s.focusIdx {
	case s9FocusArchive:
		start, end = n-1, n
		if s.appState.AipubEnabled {
			start, end = n-2, n-1
		}
	case s9FocusPlan:
		start, end = n+1, n+2+s.planVP.Height // the plan and the line under it
	case s9FocusNav:
		start = strings.Count(view, "\n")
		end = start + 1
	}
	return FitView(view, start, end, s.width, s.height)
}

// top is what is above the plan: the title, where the settings were saved and
// the inputs.
func (s *S9Install) top() string {
	var b strings.Builder
	b.WriteString(styles.StyleTitle.Render("설치 계획") + "\n")
	b.WriteString(styles.StyleMuted.Render("저장한 설정으로 실행할 순서입니다. 실행을 누르면 사전 확인 뒤 이 순서대로 실행하고, "+
		"StorageClass 드라이버 설치 (M1) 에서 한 번 멈춥니다.") + "\n\n")
	// what the plan takes the site to be and why: it decides whether the
	// cluster is built or updated, and P3
	if s.opts.SiteWhy != "" {
		b.WriteString(styles.StyleMuted.Render("사이트: "+s.opts.SiteWhy) + "\n\n")
	}
	if prev := s.viewPrev(); prev != "" {
		b.WriteString(prev + "\n")
	}

	if copies := plan.CopyCommands(s.opts, s.savedIn()); len(copies) > 0 {
		b.WriteString(styles.StyleWarning.Render("! 설정을 "+s.savedIn()+" 에 저장했습니다. 설치 전에 "+
			s.opts.AnsibleDir()+" 로 복사해야 합니다 (스크립트에 포함)") + "\n\n")
	}

	f := s.focusIdx
	b.WriteString(RenderSectionHeader("아카이브 경로", f == s9FocusArchive) + "  " + s.archive.View() + "\n")
	if s.appState.AipubEnabled {
		mon := styles.RadioOff() + " 설치 안 함"
		if s.opts.Monitoring {
			mon = styles.RadioOn() + " 설치"
		}
		b.WriteString(RenderSectionHeader("모니터링", f == s9FocusMonitoring) + "  " + mon + "\n")
	}
	return b.String() // its last line is blank
}

// planText is the plan: the checks and the steps.
func (s *S9Install) planText() string {
	var b strings.Builder
	b.WriteString(styles.StyleMuted.Render("  ✓ 통과   ! 경고 (계속 가능)   ✗ 실패   - 건너뜀   · 아직 안 함") + "\n\n")
	started, inChecks := false, false
	for _, st := range s.steps {
		// the checks first and the steps that run after them, each under a heading
		if isCheck := st.Kind == plan.Check; !started || isCheck != inChecks {
			if isCheck {
				b.WriteString(styles.StyleTitle.Render("사전 확인") + styles.StyleMuted.Render("  (읽기만 합니다. c 로 실행)") + "\n")
			} else {
				b.WriteString("\n" + styles.StyleTitle.Render("실행 순서") + "\n")
			}
			started, inChecks = true, isCheck
		}
		icon := "·"
		res, checked := s.checks[st.ID]
		switch {
		case st.Skip:
			icon = "-"
		case checked && res.Skipped:
			icon = "-"
		case checked && res.OK && res.Warn:
			icon = "!"
		case checked && res.OK:
			icon = "✓"
		case checked && !res.OK && !((st.ID == "P2" || st.ID == "P3" || st.ID == "P6" || st.ID == "P7" || st.ID == "P8") && s.checking):
			icon = "✗"
		}
		line := fmt.Sprintf("  %s %-3s %-30s", icon, st.ID, st.Title)
		switch {
		case st.Skip:
			b.WriteString(styles.StyleMuted.Render(line+"건너뜀: "+st.SkipWhy) + "\n")
		case icon == "✗":
			b.WriteString(styles.StyleError.Render(line) + "\n")
		case icon == "!":
			b.WriteString(styles.StyleWarning.Render(line) + "\n")
		case icon == "✓":
			b.WriteString(styles.StyleSuccess.Render(line) + "\n")
		default:
			b.WriteString(line + "\n")
		}
		switch {
		case st.Skip:
		case st.Kind == plan.Check && st.Sees != "":
			b.WriteString(styles.StyleMuted.Render("        보는 것: "+st.Sees) + "\n")
			if v, ok := preflightOf(st.Var); ok {
				b.WriteString(styles.StyleMuted.Render("        힌트 ("+v.Key+"): "+v.Preflight) + "\n")
			}
		default:
			b.WriteString(styles.StyleMuted.Render("        "+st.Note) + "\n")
		}
		if checked {
			// what it found, in the colour of the verdict
			style := lipgloss.NewStyle()
			switch icon {
			case "✗":
				style = styles.StyleError
			case "!":
				style = styles.StyleWarning
			case "-":
				style = styles.StyleMuted
			}
			for i, l := range res.Lines {
				if i == 0 && res.Skipped && !strings.HasPrefix(l, "건너뜀") {
					l = "건너뜀: " + l
				}
				b.WriteString(style.Render("        "+l) + "\n")
			}
		}
		if st.ID == "P2" {
			b.WriteString(s.viewSend())
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// bottom is what is under the plan: a message, the keys and the buttons.
func (s *S9Install) bottom() string {
	var b strings.Builder
	if s.asking {
		// first, for View to keep it in sight (askLines)
		var names []string
		for _, n := range s.refusing() {
			names = append(names, n.Name)
		}
		b.WriteString("\n" + styles.StylePrimary.Render("SSH 비밀번호 ("+strings.Join(names, " ")+" 공통): ") + s.pass.View() + "\n")
		b.WriteString(styles.StyleMuted.Render("Enter 보내기   Esc 취소   (저장하지 않으며 보낸 뒤 지웁니다)") + "\n")
	}
	if s.confirm != "" {
		b.WriteString("\n" + styles.StyleWarning.Render(s.confirmQuestion()) + "\n")
	}
	if s.message != "" {
		b.WriteString("\n" + styles.StylePrimary.Render(s.message) + "\n")
	}
	keys := "  c 사전 확인   d 명령 보기   w 스크립트 저장 (install-plan.sh)"
	if s.last != nil {
		keys += "   f 처음부터"
	}
	if len(s.refusing()) > 0 {
		keys += "   k·p 키 보내기"
	}
	b.WriteString("\n" + styles.StyleMuted.Render(keys+"   — 입력 칸 밖에서") + "\n")

	prevFocused := s.focusIdx == s9FocusNav && s.navIdx == 0
	runFocused := s.focusIdx == s9FocusNav && s.navIdx == 1
	run := "실행"
	if s.last != nil && len(s.last.Remaining()) > 0 {
		run = "이어서 실행"
	}
	b.WriteString("\n" + RenderNavButtons("이전", run, prevFocused, runFocused, s.width))
	return b.String()
}

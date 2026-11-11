package plan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The runner runs the stages of the plan (Stages) on the control node, each
// playbook and shell step with ansible-playbook (or bash) of the installer venv,
// its output to a log of the step. What ran is kept in state.json beside the
// logs, so that a run that failed or was stopped goes on from the first step that
// did not succeed (every setup playbook is safe to run again), across a restart
// of the program. The run does not outlive the program (no detach yet): quitting
// stops it.
//
// The directory is <installer root>/tui-install: beside ansible/, not in it -
// every playbook tars the ansible directory (sync-ansible.yml), and a log being
// written there would be in that archive and changing under tar; and upgrade.sh
// replaces ansible/, scripts/, bundle/ and venv/ but not this.

// StateDir is where a run keeps state.json, its lock and the logs.
func StateDir(o Options) string { return strings.TrimRight(o.Root, "/") + "/tui-install" }

type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusRunning   StepStatus = "running"
	StatusOK        StepStatus = "ok"
	StatusFailed    StepStatus = "failed"
	StatusSkipped   StepStatus = "skipped"
	StatusWaiting   StepStatus = "manual-waiting"
	StatusCancelled StepStatus = "cancelled"
)

// Icon is how the screen and --install show a status.
func (s StepStatus) Icon() string {
	switch s {
	case StatusRunning:
		return "▶"
	case StatusOK:
		return "✓"
	case StatusFailed, StatusCancelled:
		return "✗"
	case StatusSkipped:
		return "-"
	case StatusWaiting:
		return "⏸"
	}
	return "·"
}

type StepState struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Stage    int        `json:"stage"`
	Status   StepStatus `json:"status"`
	Start    *time.Time `json:"start,omitempty"`
	End      *time.Time `json:"end,omitempty"`
	ExitCode *int       `json:"exit_code,omitempty"`
	Log      string     `json:"log,omitempty"`
	Note     string     `json:"note,omitempty"`
	// Continued: the step failed on something that does not hold the steps
	// after it back (informerOnly), and the run went on. It is still failed: a
	// resume runs it again
	Continued bool `json:"continued,omitempty"`
	// PGID is the process group of the command while it runs (its session
	// leads it): a program killed outright leaves it behind as running, and
	// the next run refuses to start while that group is alive
	PGID int `json:"pgid,omitempty"`
}

// Elapsed is how long the step ran, or runs so far.
func (s StepState) Elapsed(now time.Time) time.Duration {
	switch {
	case s.Start == nil:
		return 0
	case s.End != nil:
		return s.End.Sub(*s.Start)
	}
	return now.Sub(*s.Start)
}

type Outcome string

const (
	OutcomeOK      Outcome = "ok"
	OutcomeFailed  Outcome = "failed"
	OutcomeStopped Outcome = "stopped"
)

// RunState is state.json.
type RunState struct {
	// InputHash is of the site files when the run started (InputHash)
	InputHash string `json:"input_hash"`
	// Existing and AipubInstalled are Options of the plan: what was there when
	// the installation started. A resume plans with them, not with what the
	// files say now (the run itself saved them)
	Existing       bool `json:"existing"`
	AipubInstalled bool `json:"aipub_installed"`
	// Monitoring and ArchivePath are what the operator chose for the run; a
	// resume keeps them too, so that it runs the plan it went on from
	Monitoring  bool        `json:"monitoring"`
	ArchivePath string      `json:"archive_path,omitempty"`
	Started     time.Time   `json:"started"`
	Updated     time.Time   `json:"updated"`
	Outcome     Outcome     `json:"outcome,omitempty"` // empty while it runs, or when the program died
	Steps       []StepState `json:"steps"`
}

// Step is the state of step id, nil when the run has none.
func (r *RunState) Step(id string) *StepState {
	for i := range r.Steps {
		if r.Steps[i].ID == id {
			return &r.Steps[i]
		}
	}
	return nil
}

// Remaining are the steps a resume runs: neither ok nor skipped.
func (r *RunState) Remaining() []StepState {
	var out []StepState
	for _, s := range r.Steps {
		if s.Status != StatusOK && s.Status != StatusSkipped {
			out = append(out, s)
		}
	}
	return out
}

// WentOn reports whether the run got to its end though steps failed: every
// step that is not ok or skipped is one the run went on past (Continued).
// Such a run is failed, and nothing but those steps is left of it.
func (r *RunState) WentOn() bool {
	left := r.Remaining()
	for _, s := range left {
		if s.Status != StatusFailed || !s.Continued {
			return false
		}
	}
	return len(left) > 0
}

const stateFile = "state.json"

// LoadRunState reads state.json of dir; nil without an error when there is none.
func LoadRunState(dir string) (*RunState, error) {
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st RunState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, stateFile), err)
	}
	return &st, nil
}

// SetAsideRun moves state.json of dir to state.json.reset, after reset-cluster.yml
// has taken apart what the run built: the steps it records as ok are not done
// any more (the ki cp services of K2 are gone), so a resume would skip them. The
// next run starts from the first step; the record stays to be read.
func SetAsideRun(dir string) error {
	err := os.Rename(filepath.Join(dir, stateFile), filepath.Join(dir, stateFile+".reset"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// writeAtomic writes path whole or not at all: a reader never sees half of it.
func writeAtomic(p string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return err
	}
	// the rename itself survives a crash once the directory is on disk
	return syncDir(filepath.Dir(p))
}

// InputHash is of the site files a run is made from: a resume compares it to
// see whether they changed since. A missing file counts as missing.
func InputHash(files []string) string {
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		fmt.Fprintf(h, "%s\x00%t\x00%d\x00", filepath.ToSlash(f), err == nil, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// StreamFunc runs a command in dir with its output (stdout and stderr) to out;
// the error of a command that failed is an *exec.ExitError. Tests replace it.
type StreamFunc func(ctx context.Context, dir string, out io.Writer, name string, args ...string) error

// killGrace is how long a cancelled step has after SIGINT before SIGKILL.
var killGrace = 10 * time.Second

type startedKey struct{}

// withStarted has ExecStream tell started the pid of the command once it runs:
// the leader of its session, so its process group too.
func withStarted(ctx context.Context, started func(pid int)) context.Context {
	return context.WithValue(ctx, startedKey{}, started)
}

// ExecStream is StreamFunc of the machine: the command leads a session of its
// own, with no terminal (a sudo that would ask a password fails, not waits),
// and a cancel sends SIGINT to its whole process group (ansible-playbook and
// its ssh), then SIGKILL after killGrace.
func ExecStream(ctx context.Context, dir string, out io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	setSession(cmd)
	var mu sync.Mutex
	var kill *time.Timer
	cmd.Cancel = func() error {
		mu.Lock()
		kill = time.AfterFunc(killGrace, func() { _ = killTree(cmd) })
		mu.Unlock()
		return interruptTree(cmd)
	}
	cmd.WaitDelay = killGrace + 5*time.Second
	if env := EnvOf(ctx); len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	err := cmd.Start()
	if err == nil {
		if started, ok := ctx.Value(startedKey{}).(func(int)); ok {
			started(cmd.Process.Pid)
		}
		err = cmd.Wait()
	}
	mu.Lock()
	if kill != nil {
		kill.Stop()
	}
	mu.Unlock()
	return err
}

// StepScript is what bash runs for a playbook or shell step: in the ansible
// directory with the venv active, as install-plan.sh does.
func StepScript(o Options, st Step) string {
	return "set -e\n" +
		"source " + shellQuote(o.Root+"/venv/bin/activate") + "\n" +
		"cd " + shellQuote(o.AnsibleDir()) + "\n" +
		"export ANSIBLE_DISPLAY_SKIPPED_HOSTS=false ANSIBLE_FORCE_COLOR=0 PYTHONUNBUFFERED=1\n" +
		Command(st) + "\n"
}

// removing is a reset-/remove- playbook named in a shell line.
var removing = regexp.MustCompile(`(^|[\s/'"])(reset|remove)-[\w.-]*\.ya?ml\b`)

// forbidden: the runner never runs what removes things, whatever the plan says.
func forbidden(st Step) bool {
	base := path.Base(st.Playbook)
	if strings.HasPrefix(base, "reset-") || strings.HasPrefix(base, "remove-") {
		return true
	}
	for _, a := range append(append([]string{}, st.Args...), st.Shell...) {
		if strings.Contains(a, "delete_data") || strings.Contains(a, "delete_namespace") {
			return true
		}
	}
	for _, l := range st.Shell {
		if removing.MatchString(l) {
			return true
		}
	}
	return false
}

// LockedError: another program runs this installation.
type LockedError struct{ Path, PID string }

func (e *LockedError) Error() string {
	pid := e.PID
	if pid == "" {
		pid = "?"
	}
	return i18n.T("another k8s-installer-tui (pid %s) is running this install (%s)", pid, e.Path)
}

// Runner runs the stages once (Run). Continue, Stop and Kill may be called from
// any goroutine while it runs.
type Runner struct {
	Opts   Options
	State  *state.AppState // the nodes the StorageClass check asks
	Dir    string
	Stages [][]Step
	Inputs []string   // the site files (InputHash)
	Exec   StreamFunc // playbook and shell steps
	Cmd    RunFunc    // the StorageClass check, and making Dir with sudo
	// Notify gets every change of a step, from the goroutine that runs it
	Notify func(StepState)

	mu       sync.Mutex
	st       RunState
	cont     chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
	kill     context.CancelFunc
	killed   bool
	saveErr  error // the first state.json that could not be written
}

func NewRunner(o Options, s *state.AppState, stages [][]Step, inputs []string) *Runner {
	return &Runner{Opts: o, State: s, Dir: StateDir(o), Stages: stages, Inputs: inputs,
		Exec: ExecStream, Cmd: ExecRun, cont: make(chan struct{}, 1), stopped: make(chan struct{})}
}

// Continue answers the Manual step that waits: the operator says it is done.
func (r *Runner) Continue() {
	select {
	case r.cont <- struct{}{}:
	default:
	}
}

// Stop starts no new step: the running ones finish, a Manual step stops waiting.
func (r *Runner) Stop() { r.stopOnce.Do(func() { close(r.stopped) }) }

// Kill stops the running steps now (SIGINT to their process groups, then SIGKILL).
func (r *Runner) Kill() {
	r.Stop()
	r.mu.Lock()
	r.killed = true
	k := r.kill
	r.mu.Unlock()
	if k != nil {
		k()
	}
}

func (r *Runner) stopping() bool {
	select {
	case <-r.stopped:
		return true
	default:
		return false
	}
}

// SaveErr is the first error writing state.json during the run, nil when every
// write went through: with one, state.json may be behind what ran.
func (r *Runner) SaveErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveErr
}

// Snapshot is the state as it is now.
func (r *Runner) Snapshot() RunState {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.st
	st.Steps = append([]StepState{}, r.st.Steps...)
	return st
}

// Run runs the stages: with resume the steps the last run (state.json) finished
// ok are not run again. The error is for a run that could not start: another
// program holds the lock, the directory could not be made.
func (r *Runner) Run(ctx context.Context, resume bool) (Outcome, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r.mu.Lock()
	r.kill = cancel
	killed := r.killed
	r.mu.Unlock()
	// stopped before it started: state.json stays as the last run left it
	if killed || r.stopping() || ctx.Err() != nil {
		return OutcomeStopped, nil
	}

	if err := ensureDir(ctx, r.Dir, r.Cmd); err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T("state directory %s", r.Dir), err)
	}
	lock, err := lockFile(filepath.Join(r.Dir, "lock"))
	if err != nil {
		return "", err
	}
	defer lock.Close()

	prev, err := LoadRunState(r.Dir)
	if err != nil && resume {
		return "", err
	}
	// the lock went with a program killed outright, its step did not
	if err := leftRunning(prev); err != nil {
		return "", err
	}
	if !resume {
		prev = nil
	}
	r.mu.Lock()
	r.init(prev)
	r.mu.Unlock()
	if err := r.sudoReady(ctx); err != nil {
		return "", err
	}
	r.mu.Lock()
	err = r.save()
	r.mu.Unlock()
	if err != nil {
		return "", err
	}
	outcome := r.stages(ctx)
	r.mu.Lock()
	r.st.Outcome = outcome
	r.st.Updated = time.Now()
	r.persist()
	r.mu.Unlock()
	return outcome, nil
}

// StillRunningError: a step of the last run still runs, left by a program that
// was killed outright (SIGKILL, out of memory) with the step in a session of
// its own. Running it again beside itself would overwrite its files.
type StillRunningError struct {
	Step string
	PGID int
}

func (e *StillRunningError) Error() string {
	return i18n.T("%s of the last run is still running (process group %d, pid %d). Wait for it to end, or stop it with kill -INT -- -%d, then run again",
		e.Step, e.PGID, e.PGID, e.PGID)
}

// leftRunning is a StillRunningError for a step of prev recorded as running
// whose process group is still alive.
func leftRunning(prev *RunState) error {
	if prev == nil {
		return nil
	}
	for _, s := range prev.Steps {
		if s.Status == StatusRunning && s.PGID > 0 && groupAlive(s.PGID) {
			return &StillRunningError{Step: s.ID, PGID: s.PGID}
		}
	}
	return nil
}

// sudoReady: the shell steps that run sudo on this control node (C1, A5.1) run
// it as sudo -n in a session with no terminal, so a sudo that wants a password
// fails there. It is found before the first step: sudo -n true here.
func (r *Runner) sudoReady(ctx context.Context) error {
	var ids []string
	for _, stage := range r.Stages {
		for _, st := range stage {
			if usesSudo(st) && r.st.Step(st.ID).Status == StatusPending {
				ids = append(ids, st.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	out, err := r.Cmd(ctx, "/", "sudo", "-n", "true")
	if err != nil {
		return errors.New(i18n.T("Steps %s run with sudo on this control node, and sudo asks for a password (sudo -n true: %s). "+
			"Give this user sudo without a password (NOPASSWD), then run again",
			strings.Join(ids, ", "), firstLine(strings.TrimSpace(string(out)+" "+err.Error()))))
	}
	return nil
}

func (r *Runner) init(prev *RunState) {
	now := time.Now()
	r.st = RunState{InputHash: InputHash(r.Inputs), Existing: r.Opts.Existing, AipubInstalled: r.Opts.AipubInstalled,
		Monitoring: r.Opts.Monitoring, ArchivePath: r.Opts.ArchivePath, Started: now, Updated: now}
	// again: a step the last run went on past (Continued) runs again now, and
	// so do the steps after it, though they were ok. They ran after it for a
	// reason: setup-aipub's k8s-jwks writes the authentication config on the
	// nodes by hand, and A5.1 and A5.2 put the installer's own back over it
	again := false
	for i, stage := range r.Stages {
		for _, st := range stage {
			s := StepState{ID: st.ID, Title: st.Title, Stage: i, Status: StatusPending}
			p := (*StepState)(nil)
			if prev != nil {
				p = prev.Step(st.ID)
			}
			if st.Skip {
				s.Status, s.Note = StatusSkipped, st.SkipWhy
			} else if p != nil && !st.AlwaysRun && !again && p.Status == StatusOK {
				s = *p
				// the title in the language of this run, which may not be the last one's
				s.Stage, s.Title = i, st.Title
			}
			if p != nil && p.Status == StatusFailed && p.Continued {
				again = true
			}
			r.st.Steps = append(r.st.Steps, s)
		}
	}
}

// save writes state.json; r.mu is held.
func (r *Runner) save() error {
	data, err := json.MarshalIndent(r.st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(r.Dir, stateFile), data)
}

// persist saves once the run is under way, where a failure stops nothing: the
// first is kept (SaveErr) for the end of the run to say; r.mu is held.
func (r *Runner) persist() {
	if err := r.save(); err != nil && r.saveErr == nil {
		r.saveErr = err
	}
}

// set changes step id and saves the state.
func (r *Runner) set(id string, change func(*StepState)) {
	r.mu.Lock()
	s := r.st.Step(id)
	change(s)
	r.st.Updated = time.Now()
	r.persist()
	got := *s
	r.mu.Unlock()
	if r.Notify != nil {
		r.Notify(got)
	}
}

func (r *Runner) status(id string) StepStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st.Step(id).Status
}

// stages runs one stage after another. A step that fails ends the run once the
// other steps of its stage have finished: a playbook is not cut off half way
// for another's failure (it runs to its end, safe to run again), and a Manual
// step that waits stops waiting.
func (r *Runner) stages(ctx context.Context) Outcome {
	left := false // a step failed and the run went on past it
	for _, stage := range r.Stages {
		if r.stopping() || ctx.Err() != nil {
			return OutcomeStopped
		}
		// a failure in the stage stops the waits of the Manual steps beside it
		waits, failed := context.WithCancel(ctx)
		var wg sync.WaitGroup
		for _, st := range stage {
			if s := r.status(st.ID); s == StatusOK || s == StatusSkipped {
				continue
			}
			wg.Add(1)
			go func(st Step) {
				defer wg.Done()
				if st.Kind == Manual {
					r.manual(ctx, waits, st)
					return
				}
				if !r.command(ctx, st) {
					failed()
				}
			}(st)
		}
		wg.Wait()
		failed()
		var fail, stop bool
		for _, st := range stage {
			switch r.status(st.ID) {
			case StatusFailed:
				// a failure the later steps do not depend on (Continued) is kept
				// as the outcome of the run, but does not end it here
				if r.continued(st.ID) {
					left = true
				} else {
					fail = true
				}
			case StatusOK, StatusSkipped:
			default:
				stop = true
			}
		}
		switch {
		case fail:
			return OutcomeFailed
		case stop:
			return OutcomeStopped
		}
	}
	if left {
		return OutcomeFailed
	}
	return OutcomeOK
}

// continued reports whether the failed step id let the run go on.
func (r *Runner) continued(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.st.Step(id)
	return st != nil && st.Continued
}

func now() *time.Time {
	t := time.Now()
	return &t
}

// command runs a playbook or shell step; false when it did not succeed.
func (r *Runner) command(ctx context.Context, st Step) bool {
	logPath := filepath.Join(r.Dir, "logs", st.ID+".log")
	if forbidden(st) {
		r.set(st.ID, func(s *StepState) {
			s.Status, s.Note = StatusFailed, i18n.T("Playbooks that delete or reset are never run: %s", Command(st))
		})
		return false
	}
	// appended: the log of every attempt is kept
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		r.set(st.ID, func(s *StepState) { s.Status, s.Note = StatusFailed, i18n.T("Log file: %v", err) })
		return false
	}
	defer f.Close()
	// the header of the attempt (attemptLines): its time, the ID and the title,
	// then the command
	fmt.Fprintf(f, "── %s %s %s\n── %s\n", time.Now().Format(time.RFC3339), st.ID, st.Title, strings.ReplaceAll(Command(st), "\n", "\n── "))
	copying := st.ID == copyStepID && r.Opts.SiteDir != ""
	if copying {
		// what the copy replaces goes to the history of the site first; a file
		// edited on the installer side since the last copy stops it once
		changed, unchecked, err := fileio.GuardCopy(r.Opts.SiteDir, r.Opts.AnsibleDir())
		if len(unchecked) > 0 {
			fmt.Fprintf(f, "── %s\n", i18n.T("Overwritten unchecked, as they could not be read (not in the history): %s", strings.Join(unchecked, ", ")))
		}
		note := ""
		switch {
		case err != nil:
			note = i18n.T("The check before the copy failed: %v", err)
		case len(changed) > 0:
			note = i18n.T("Files on the installer side were edited by hand since the last copy: %s. "+
				"What they held is kept in %s. Look at it (and bring it into the site if needed), then run again to overwrite them",
				strings.Join(changed, ", "), filepath.Join(r.Opts.SiteDir, fileio.HistoryFile))
		}
		if note != "" {
			fmt.Fprintf(f, "── %s\n", note)
			code := 1
			r.set(st.ID, func(s *StepState) {
				s.Status, s.Start, s.End, s.ExitCode, s.Log, s.Note = StatusFailed, now(), now(), &code, logPath, note
			})
			return false
		}
	}
	r.set(st.ID, func(s *StepState) {
		s.Status, s.Start, s.End, s.ExitCode, s.Log, s.Note = StatusRunning, now(), nil, nil, logPath, ""
		s.Continued = false
	})
	// its process group is recorded while it runs (leftRunning); no Notify, the
	// step shows nothing new
	started := withStarted(ctx, func(pid int) {
		r.mu.Lock()
		r.st.Step(st.ID).PGID = pid
		r.persist()
		r.mu.Unlock()
	})
	err = r.Exec(started, r.Opts.AnsibleDir(), f, "bash", "-c", StepScript(r.Opts, st))
	code := 0
	if err != nil {
		code = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
	}
	if err == nil && copying {
		// not a failure of the copy, which is done: the next one says it again
		if rerr := fileio.RecordCopy(r.Opts.SiteDir, r.Opts.AnsibleDir()); rerr != nil {
			fmt.Fprintf(f, "── %s\n", i18n.T("The copy could not be recorded in the history: %v", rerr))
		}
	}
	fmt.Fprintf(f, "── %s %s %s\n", time.Now().Format(time.RFC3339), st.ID, i18n.T("end: exit code %d", code))
	var note string
	goesOn := false
	if err != nil && ctx.Err() == nil {
		note = knownFailure(st, logPath, r.failureContext(st))
		if goesOn = informerOnly(st, attemptLines(logPath, st)); goesOn {
			note += ". " + i18n.T("Only backend-informer failed, and the steps after this one do not depend on it: this step stays failed and the run goes on")
		}
	}
	r.set(st.ID, func(s *StepState) {
		s.End, s.ExitCode, s.PGID = now(), &code, 0
		switch {
		case err == nil:
			s.Status = StatusOK
		case ctx.Err() != nil:
			s.Status, s.Note = StatusCancelled, i18n.T("Stopped (Ctrl+C twice)")
		default:
			s.Status, s.Note, s.Continued = StatusFailed, note, goesOn
		}
	})
	// to the stage, a failure the run goes on past is no failure: the steps
	// beside it are not told to stop
	return err == nil || goesOn
}

// The stop of setup-aipub that the installers' README says to expect.
const (
	setupAipubPlaybook = "aipub-playbooks/setup-aipub.yml"
	workspaceCRDGone   = "CRD 'workspaces.aipub.ten1010.io' does not exist"
)

// componentFailed is the result line install.sh of aipub-installer prints for
// a component that failed.
var componentFailed = regexp.MustCompile(`AIPUB(?:_JAVA)?_RESULT .*\bcomponent=(\S+) result=FAIL`)

// attemptLines are the lines of the last attempt of a step: its log keeps
// every attempt, each under the header command() writes ("── <time> <ID>
// <title>", then "── <command>"). What an earlier attempt said must not decide
// this one.
func attemptLines(logPath string, st Step) []string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	// the header is known by its time, the ID and the command echoed under it,
	// not by the title: that is in the language of the run that wrote it, and a
	// resume may speak the other. The line that ends an attempt ("── <time>
	// <ID> ...") has no command under it
	start, echo := 0, "── "+firstLine(Command(st))
	for i := 0; i+1 < len(lines); i++ {
		if lines[i+1] == echo && attemptHeader(lines[i], st.ID) {
			start = i
		}
	}
	return lines[start:]
}

// attemptHeader reports whether l is "── <time> <id>", with anything after.
func attemptHeader(l, id string) bool {
	rest, ok := strings.CutPrefix(l, "── ")
	if !ok {
		return false
	}
	at, rest, _ := strings.Cut(rest, " ")
	if _, err := time.Parse(time.RFC3339, at); err != nil {
		return false
	}
	return rest == id || strings.HasPrefix(rest, id+" ")
}

// failedTasks are the tasks of an ansible log that failed and were not
// ignored, by name: a "fatal: [host]" line under its "TASK [name]" line, unless
// "...ignoring" follows before the next task.
func failedTasks(lines []string) []string {
	var out []string
	task, open := "", false // open: the last failure may still be ignored
	for _, l := range lines {
		switch t := strings.TrimSpace(l); {
		case strings.HasPrefix(t, "TASK ["):
			task, open = strings.TrimRight(strings.TrimPrefix(t, "TASK ["), "* "), false
			task = strings.TrimSuffix(task, "]")
		case strings.HasPrefix(t, "fatal: ["):
			out, open = append(out, task), true
		case t == "...ignoring" && open:
			out, open = out[:len(out)-1], false
		}
	}
	return out
}

// informerOnly reports whether setup-aipub stopped on nothing but
// backend-informer finding no Workspace CRD, which the python services make
// and aipub-installer does not install (its README: "backend-informer fails
// until the python services have made the Workspace CRD"). backend-informer is
// the last component of the module, so everything else of it is deployed, the
// backend that issues the tokens among them; the steps after setup-aipub (the
// JWKS config and its update-cluster) need that backend and the CA, not the
// informer. So the run goes on, and the step stays failed for the resume that
// follows the python services. Any other component failing, or a node out of
// reach, is not this: the run stops there as on any failure.
//
// The informer's own error is in the log of every new cluster, so it proves
// nothing alone. The playbook must also have failed on one task only, and that
// task the one that reports the component: in helmfile mode the stop after
// --emit-values (tasks/aipub/install-aipub-2-apiserver.yml), which comes after
// helmfile sync, k8s-jwks and --after-helmfile have all succeeded; with the
// installer or dag, the result of the install.sh call that had
// backend-informer in it (tasks/aipub/run-installer.yml). A helmfile sync that
// failed, or an install.sh that died without a result line, fails on another
// task and prints no line of a component: without this the run would go on to
// the apiserver with a backend that is not deployed.
func informerOnly(st Step, lines []string) bool {
	if st.Playbook != setupAipubPlaybook {
		return false
	}
	full := strings.Join(lines, "\n")
	if !strings.Contains(full, workspaceCRDGone) || strings.Contains(full, "UNREACHABLE") {
		return false
	}
	fails := componentFailed.FindAllStringSubmatch(full, -1)
	if len(fails) == 0 {
		return false
	}
	for _, m := range fails {
		if m[1] != "backend-informer" {
			return false
		}
	}
	failed := failedTasks(lines)
	if len(failed) != 1 {
		return false
	}
	task := failed[0]
	return strings.Contains(task, "Stop: install.sh --emit-values failed") ||
		(strings.Contains(task, "Result of ") && strings.Contains(task, "backend-informer"))
}

// failureContext is what knownFailure needs of the run besides the log; r.mu
// is not held.
func (r *Runner) failureContext(st Step) failureContext {
	r.mu.Lock()
	defer r.mu.Unlock()
	fc := failureContext{resettable: ResetHelps(r.Opts.Existing, st.ID)}
	if k4 := r.st.Step(pushStepID); k4 != nil {
		fc.k4 = k4.Status
	}
	return fc
}

type failureContext struct {
	// resettable: a new-site run failing while the cluster is still being built,
	// where reset-cluster.yml is what clears the leftovers (ResetHelps)
	resettable bool
	// k4 is the status of K4 in this run: skipped or never run (pending) means
	// the user registry may be empty
	k4 StepStatus
}

// ResetHelps reports whether reset-cluster.yml is a way out of a failure of
// step id: only on a new site (not existing) and only in K1..K3, where the
// cluster is still being built and what stops the run is what an earlier
// install left (packages, kubelet enabled). After K3 the cluster stands and
// holds what the later steps put in it, and on an existing site it is the
// site's own cluster: reset-cluster.yml destroys either.
func ResetHelps(existing bool, id string) bool {
	return !existing && (id == "K1" || id == "K2" || id == "K3")
}

// ResetScript is what the install screen runs for x (ResetHelps), as the steps
// run (StepScript: this user, the venv, the ansible directory), its output also
// to logs/reset-cluster.log beside the steps' logs. The runner itself never runs
// it (forbidden): the screen does, only after the operator typed the word.
//
// The two -e are those of reset-cluster.yml of k8s-installer for a cluster that
// never finished coming up. sdfc= ("skip deleting from cluster",
// tasks/preflight-playbook.yml) skips delete-all-nodes.sh
// (tasks/reset-k8s-cluster.yml), the one step that asks the apiserver to delete
// the node objects: after a failure in K1..K3 there may be no apiserver to ask,
// and its rescue says to run again with -e sdfc=; everything after it works on
// the nodes themselves, and the node objects go with the cluster anyway.
// k8s_cp_extra_files=[] keeps validate-hostvars.py (prepare-vars.yml) from
// refusing the run over a file of the list not written yet (the JWKS config,
// made by A5.1); what is taken off the nodes is read from each node's own
// record (tasks/reset-k8s-cp-extra-files.yml reads applied-vars.yml), not from
// this variable, so the empty list removes nothing less.
func ResetScript(o Options) string {
	st := Step{ID: "reset", Title: "reset-cluster", Kind: Playbook, Playbook: "playbooks/reset-cluster.yml",
		Args: []string{"-e", "sdfc=", "-e", `{"k8s_cp_extra_files":[]}`}}
	if o.ShellExecutable != "" {
		st.Args = append(st.Args, "-e", "ansible_shell_executable="+o.ShellExecutable)
	}
	logs := StateDir(o) + "/logs"
	return "set -o pipefail\nmkdir -p " + shellQuote(logs) + "\n(\n" + StepScript(o, st) + ") 2>&1 | tee -a " + shellQuote(logs+"/reset-cluster.log") + "\n"
}

// helmNamespaces are the steps that install with helm and wait for the pods
// (helm --wait), by playbook, with the namespace to look at: Cilium in
// kube-system, Harbor and the AIPub services in aipub (aipub_namespace), the
// monitoring module in namespaces of its own.
var helmNamespaces = map[string]string{
	"aipub-playbooks/setup-network.yml":    "-n kube-system",
	"aipub-playbooks/setup-base.yml":       "-n aipub",
	"aipub-playbooks/setup-monitoring.yml": "-A",
	"aipub-playbooks/setup-aipub.yml":      "-n aipub",
}

// podsNotStarted are what a helm --wait that ran out of time leaves in the log.
var podsNotStarted = []string{"ImagePullBackOff", "ErrImagePull", "context deadline exceeded", "timed out waiting for the condition", "not ready"}

// knownFailure returns a human diagnosis for known failure patterns, or extracts
// the raw failure message as a fallback so unexpected errors are still explained.
func knownFailure(st Step, logPath string, fc failureContext) string {
	lines := TailLines(logPath, 200)
	full := strings.Join(lines, "\n")

	// 1. Check known specific patterns
	// the whole attempt, not its tail: helmfile mode prints a long result after
	// the component's error. And the informer alone (informerOnly), not its error
	// alone: that is in the log of every new cluster, and would say "the services
	// before it are installed" of a setup-aipub that failed on something else
	if informerOnly(st, attemptLines(logPath, st)) {
		// deterministic until then, so no retry: aipub-installer README "What to expect on a new cluster"
		return i18n.T("Known issue: backend-informer waits for the Workspace CRD the Python services make. The services before it are installed. " +
			"Install the Python services, then resume (aipub-installer README)")
	}
	if ns, helm := helmNamespaces[st.Playbook]; helm && containsAny(full, podsNotStarted) {
		note := i18n.T("Diagnosis: pods did not start, and helm stopped waiting for them. The usual cause is images missing from the user registry (ImagePullBackOff). "+
			"Look with kubectl %s get pods on a control plane node", ns)
		if fc.k4 == StatusSkipped || fc.k4 == StatusPending {
			note += ". " + i18n.T("This run skipped K4 push-user-registry-artifacts: give the path of %s as the archive path and resume "+
				"(--install --resume --archive <file>). K4 runs first, then this step", ArchiveName)
		}
		return note
	}
	if strings.Contains(full, "error syncing endpoints with etcd") || (strings.Contains(full, "check-etcd") && strings.Contains(full, "context deadline exceeded")) {
		return i18n.T("Diagnosis: a firewall blocks etcd between the control plane nodes (ports 2379/2380). Fix: open ports 2379 and 2380/tcp on every node, or check the state of the firewall.")
	}
	// the leftovers of an earlier install: only where a reset is a way out
	// (ResetHelps), else the raw error below says what failed
	if fc.resettable && (strings.Contains(full, "package[\"containerd\"] already installed") || (strings.Contains(full, "already installed") && strings.Contains(full, "containerd"))) {
		return i18n.T("Diagnosis: an earlier install left containerd on the nodes. Fix: reset the nodes with reset-cluster.yml, then run again from the first step " +
			"(x on the install screen when it offers x, then type reset)")
	}
	if strings.Contains(full, "Failed dependencies") && strings.Contains(full, "libcrypto.so") {
		return i18n.T("Diagnosis: the openssl-devel package conflicts with the dependencies of python3.12-libs. Fix: run 'sudo dnf remove -y openssl-devel' on the node.")
	}
	if fc.resettable && strings.Contains(full, "Kubelet already enabled") {
		return i18n.T("Diagnosis: the kubelet service is enabled already: left by an earlier install, or a cluster already runs on this node. " +
			"A cluster in service is not to be reset: start again with --existing. When it is a leftover, reset the nodes with reset-cluster.yml, then " +
			"run again from the first step (x on the install screen when it offers x, then type reset)")
	}
	if (strings.Contains(full, "k8s_cp_extra_files") || strings.Contains(full, "authentication-config.yaml")) && strings.Contains(full, "not there") {
		// Every playbook of the plan carries the -e of its stage (withStages),
		// which leaves out a file not written yet: this refusal comes from a
		// playbook run by hand. An empty file is no way past it: copied to the
		// control plane nodes, an empty authentication config keeps the
		// apiserver from starting (fileio.AuthConfig)
		return i18n.T("Diagnosis: a file that k8s_cp_extra_files names (the JWKS authentication config or another, made by A5.1) is not in the install directory. " +
			"At each step the wizard leaves out, with -e, the files not yet written, so this refusal comes from a playbook run by hand. " +
			"Fix: run it with the wizard (or --install), or, when running it by hand, add the -e from the ── line of this step's log as it is. " +
			"Do not make an empty file: an empty authentication config copied to the control plane nodes keeps the apiserver from starting")
	}

	// the informer's error, the failure not shown to be it alone and none of the
	// above: said, but not that the services before it are there; with it the
	// raw error of the log without that line
	if st.Playbook == setupAipubPlaybook && strings.Contains(strings.Join(attemptLines(logPath, st), "\n"), workspaceCRDGone) {
		note := i18n.T("The Workspace CRD error of backend-informer is in the log (on a new cluster it always is, until the Python services are installed). " +
			"But it is not shown to be the only failure, so the services before it are not known to be installed. Look for other failures in the log first; " +
			"when the informer is the only one, install the Python services, then resume")
		var others []string
		for _, l := range lines {
			if !strings.Contains(l, workspaceCRDGone) {
				others = append(others, l)
			}
		}
		if raw := extractRawError(others); raw != "" {
			note += ". " + i18n.T("Raw failure: %s", raw)
		}
		return note
	}

	// 2. Fallback: extract the raw failure summary from the Ansible log
	if raw := extractRawError(lines); raw != "" {
		return i18n.T("Raw failure: %s", raw)
	}
	return ""
}

func extractRawError(lines []string) string {
	var candidate string
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "TASK [") || strings.HasPrefix(l, "PLAY [") || strings.HasPrefix(l, "PLAY RECAP") ||
			strings.HasPrefix(l, "── ") || strings.HasPrefix(l, "changed:") || strings.HasPrefix(l, "skipping:") || strings.HasPrefix(l, "ok:") {
			continue
		}
		if strings.HasPrefix(l, "[ERROR]") || strings.HasPrefix(l, "error:") || strings.HasPrefix(l, "Error:") {
			return cleanErrorLine(l)
		}
		if strings.HasPrefix(l, "msg: ") && candidate == "" {
			candidate = strings.TrimPrefix(l, "msg: ")
		}
		if strings.HasPrefix(l, "fatal: ") && candidate == "" {
			candidate = l
		}
	}
	if candidate != "" {
		return cleanErrorLine(candidate)
	}
	return ""
}

func cleanErrorLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|-")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "'\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// cut on runes, not bytes: the messages are often Korean, and a byte cut
	// splits a character
	if r := []rune(s); len(r) > 140 {
		return string(r[:137]) + "..."
	}
	return s
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// manual is a Manual step: it checks first (a class may be there already),
// then waits for Continue and checks again, until every class is there. It
// stops waiting on Stop, on Kill, and when a step beside it failed (waits).
func (r *Runner) manual(ctx, waits context.Context, st Step) {
	logPath := filepath.Join(r.Dir, "logs", st.ID+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err == nil {
		defer f.Close()
	}
	record := func(lines ...string) {
		if f != nil {
			for _, l := range lines {
				fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), l)
			}
		}
	}
	record("── " + st.ID + " " + ManualPrompt(st))
	r.set(st.ID, func(s *StepState) {
		s.Status, s.Start, s.End, s.Log, s.Note = StatusRunning, now(), nil, logPath, i18n.T("Checking (kubectl get storageclass)")
	})
	for {
		missing, lines := CheckClasses(ctx, r.Opts, r.State, r.Cmd, st.Classes)
		record(lines...)
		if ctx.Err() != nil {
			r.set(st.ID, func(s *StepState) { s.Status, s.End, s.Note = StatusCancelled, now(), i18n.T("Stopped") })
			return
		}
		if len(missing) == 0 {
			r.set(st.ID, func(s *StepState) { s.Status, s.End, s.Note = StatusOK, now(), strings.Join(lines, "; ") })
			return
		}
		// an Enter pressed before this question does not answer it
		select {
		case <-r.cont:
		default:
		}
		r.set(st.ID, func(s *StepState) {
			s.Status, s.Note = StatusWaiting, i18n.T("Missing: %s — %s", strings.Join(classNames(missing), ", "), strings.Join(lines, "; "))
		})
		select {
		case <-r.cont:
			record(i18n.T("Installed (by the operator), checking again"))
			r.set(st.ID, func(s *StepState) { s.Status, s.Note = StatusRunning, i18n.T("Checking (kubectl get storageclass)") })
		case <-r.stopped:
			r.set(st.ID, func(s *StepState) {
				s.Status, s.End, s.Note = StatusCancelled, now(), i18n.T("Stopped while waiting")
			})
			return
		case <-waits.Done():
			note := i18n.T("Stopped: another step of the same stage failed")
			if ctx.Err() != nil || r.stopping() {
				note = i18n.T("Stopped while waiting")
			}
			r.set(st.ID, func(s *StepState) { s.Status, s.End, s.Note = StatusCancelled, now(), note })
			return
		}
	}
}

// TailLines are the last n lines of the file at p (a log), nothing when it
// cannot be read. Only its end is read.
func TailLines(p string, n int) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	const most = 64 << 10
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	off := max(0, fi.Size()-most)
	data := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(data, off); err != nil && !errors.Is(err, io.EOF) {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if off > 0 && len(lines) > 1 {
		lines = lines[1:] // the first is cut
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// ensureDir makes dir and its logs, this user's alone (the logs hold what the
// playbooks print). The installer root is root's (setup.sh): when this user may
// not write there, dir is made with sudo (NOPASSWD, P2) and given to this user,
// once.
func ensureDir(ctx context.Context, dir string, run RunFunc) error {
	logs := filepath.Join(dir, "logs")
	err := os.MkdirAll(logs, 0700)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	out, serr := run(ctx, "/", "sudo", "-n", "install", "-d", "-m", "0700",
		"-o", strconv.Itoa(os.Getuid()), "-g", strconv.Itoa(os.Getgid()), dir)
	if serr != nil {
		return fmt.Errorf("%w (sudo: %s)", err, firstLine(strings.TrimSpace(string(out))+" "+serr.Error()))
	}
	return os.MkdirAll(logs, 0700)
}

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
	return "다른 k8s-installer-tui (pid " + pid + ") 가 이 설치를 실행 중입니다 (" + e.Path + ")"
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
		return "", fmt.Errorf("상태 디렉터리 %s: %w", r.Dir, err)
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
	return fmt.Sprintf("지난 실행의 %s 가 아직 실행 중입니다 (프로세스 그룹 %d, pid %d). 끝나기를 기다리거나 kill -INT -- -%d 로 멈춘 뒤 다시 실행하세요",
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
		return fmt.Errorf("%s 는 이 제어 노드에서 sudo 로 실행하는데, sudo 가 비밀번호를 묻습니다 (sudo -n true: %s). "+
			"이 사용자에게 비밀번호 없는 sudo (NOPASSWD) 를 준 뒤 다시 실행하세요",
			strings.Join(ids, ", "), firstLine(strings.TrimSpace(string(out)+" "+err.Error())))
	}
	return nil
}

func (r *Runner) init(prev *RunState) {
	now := time.Now()
	r.st = RunState{InputHash: InputHash(r.Inputs), Existing: r.Opts.Existing, AipubInstalled: r.Opts.AipubInstalled,
		Monitoring: r.Opts.Monitoring, ArchivePath: r.Opts.ArchivePath, Started: now, Updated: now}
	for i, stage := range r.Stages {
		for _, st := range stage {
			s := StepState{ID: st.ID, Title: st.Title, Stage: i, Status: StatusPending}
			if st.Skip {
				s.Status, s.Note = StatusSkipped, st.SkipWhy
			} else if prev != nil && !st.AlwaysRun {
				if p := prev.Step(st.ID); p != nil && p.Status == StatusOK {
					s = *p
					s.Stage = i
				}
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
				fail = true
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
	return OutcomeOK
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
			s.Status, s.Note = StatusFailed, "삭제·초기화 플레이북은 실행하지 않습니다: "+Command(st)
		})
		return false
	}
	// appended: the log of every attempt is kept
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		r.set(st.ID, func(s *StepState) { s.Status, s.Note = StatusFailed, "로그 파일: "+err.Error() })
		return false
	}
	defer f.Close()
	fmt.Fprintf(f, "── %s %s %s\n── %s\n", time.Now().Format(time.RFC3339), st.ID, st.Title, strings.ReplaceAll(Command(st), "\n", "\n── "))
	r.set(st.ID, func(s *StepState) {
		s.Status, s.Start, s.End, s.ExitCode, s.Log, s.Note = StatusRunning, now(), nil, nil, logPath, ""
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
	fmt.Fprintf(f, "── %s %s 끝: 종료 코드 %d\n", time.Now().Format(time.RFC3339), st.ID, code)
	r.set(st.ID, func(s *StepState) {
		s.End, s.ExitCode, s.PGID = now(), &code, 0
		switch {
		case err == nil:
			s.Status = StatusOK
		case ctx.Err() != nil:
			s.Status, s.Note = StatusCancelled, "중단함 (Ctrl+C 두 번)"
		default:
			s.Status, s.Note = StatusFailed, knownFailure(st, logPath)
		}
	})
	return err == nil
}

// knownFailure is what the installers' README says about a failure the log
// shows, empty when it is none of them.
func knownFailure(st Step, logPath string) string {
	if st.Playbook != "aipub-playbooks/setup-aipub.yml" {
		return ""
	}
	if strings.Contains(strings.Join(TailLines(logPath, 200), "\n"), "workspaces.aipub.ten1010.io") {
		// deterministic until then, so no retry: aipub-installer README "What to expect on a new cluster"
		return "알려진 일: backend-informer 는 Python 서비스가 만드는 Workspace CRD 를 기다립니다. 그 앞의 서비스는 설치됐습니다. " +
			"Python 서비스를 설치한 뒤 이어서 실행하세요 (aipub-installer README)"
	}
	return ""
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
		s.Status, s.Start, s.End, s.Log, s.Note = StatusRunning, now(), nil, logPath, "확인 중 (kubectl get storageclass)"
	})
	for {
		missing, lines := CheckClasses(ctx, r.Opts, r.State, r.Cmd, st.Classes)
		record(lines...)
		if ctx.Err() != nil {
			r.set(st.ID, func(s *StepState) { s.Status, s.End, s.Note = StatusCancelled, now(), "중단함" })
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
			s.Status, s.Note = StatusWaiting, "없음: "+strings.Join(missing, ", ")+" — "+strings.Join(lines, "; ")
		})
		select {
		case <-r.cont:
			record("설치함 (운영자), 다시 확인")
			r.set(st.ID, func(s *StepState) { s.Status, s.Note = StatusRunning, "확인 중 (kubectl get storageclass)" })
		case <-r.stopped:
			r.set(st.ID, func(s *StepState) {
				s.Status, s.End, s.Note = StatusCancelled, now(), "멈춤: 기다리던 중 중단"
			})
			return
		case <-waits.Done():
			note := "같은 단계의 다른 단계가 실패해 멈춤"
			if ctx.Err() != nil || r.stopping() {
				note = "멈춤: 기다리던 중 중단"
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

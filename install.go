package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// installHeadless is --install: the stages of the plan with no screen, the
// progress a line per change of a step, the StorageClass stop asked on the
// terminal (Enter). 0 when every step succeeded, 1 when one failed (or the
// stop was left without an answer), 2 when it could not start (a check ✗, the
// lock, a run left unfinished without --resume or --fresh), 130 on Ctrl-C (129
// SIGHUP, 143 SIGTERM). A new site with AIPub does not start without --archive
// (exit 2) unless noArchive says the operator means it: its user registry is
// empty, and A1 would wait for images that are not there.
func installHeadless(paths fileio.Paths, s *state.AppState, opts plan.Options, resume, fresh, noArchive bool) int {
	in := bufio.NewReader(os.Stdin)
	savedIn := filepath.Dir(paths.Inventory)
	inputs := []string{paths.Inventory, paths.Vars, paths.AipubVars, paths.WebhookConfig()}
	dir := plan.StateDir(opts)

	prev, err := plan.LoadRunState(dir)
	if !resume && !fresh {
		// a run from K1 over one that did not finish is said, not taken
		switch {
		case err != nil:
			fmt.Fprintln(os.Stderr, "state.json:", err, "— 처음부터 다시 하려면 --fresh")
			return 2
		case prev != nil && len(prev.Remaining()) > 0:
			var left []string
			for _, st := range prev.Remaining() {
				left = append(left, st.ID)
			}
			fmt.Fprintf(os.Stderr, "지난 실행 (%s 에 시작) 이 끝나지 않았습니다. 남은 단계: %s\n"+
				"이어서 하려면 --resume, 끝난 단계까지 처음부터 다시 하려면 --fresh 를 붙이세요\n",
				prev.Started.Format("2006-01-02 15:04"), strings.Join(left, " "))
			return 2
		}
	}
	// k4Kept: the run goes on with a K4 that succeeded, and its archive path
	k4Kept := false
	if resume {
		switch {
		case err != nil:
			fmt.Fprintln(os.Stderr, "state.json:", err)
			return 2
		case prev == nil:
			fmt.Fprintln(os.Stderr, dir+"/state.json 이 없어 이어서 할 실행이 없습니다. --resume 없이 실행하세요")
			return 2
		}
		if k4 := prev.Step("K4"); k4 != nil && k4.Status == plan.StatusOK {
			k4Kept = true
		}
		// the plan of the run it goes on with, whatever the flags say now
		if differ := keptFromLast(prev, opts, k4Kept); len(differ) > 0 {
			fmt.Println("! 이어서 실행은 지난 실행의 설정으로 계획합니다 (이번 플래그와 다름): " + strings.Join(differ, ", ") +
				". 바꾸려면 --fresh 로 처음부터")
		}
		opts.Existing, opts.AipubInstalled = prev.Existing, prev.AipubInstalled
		opts.SiteWhy = plan.ResumedSite(prev, opts.SiteWhy)
		opts.Monitoring = prev.Monitoring
		// a K4 that did not succeed runs with the --archive of this start, before
		// the step the last run stopped at; one that did keeps its archive
		if k4Kept {
			opts.ArchivePath = prev.ArchivePath
		} else if opts.ArchivePath != "" {
			fmt.Println("── K4 가 끝나지 않은 실행: 이번 --archive 로 K4 부터 실행합니다: " + opts.ArchivePath)
		}
		var done []string
		for _, st := range prev.Steps {
			if st.Status == plan.StatusOK {
				done = append(done, st.ID)
			}
		}
		fmt.Printf("── 이어서 실행 (%s 에 시작한 실행). 이미 끝난 단계: %s\n", prev.Started.Format("2006-01-02 15:04"), strings.Join(done, " "))
		if prev.InputHash != plan.InputHash(inputs) {
			fmt.Print("! 지난 실행 뒤에 사이트 파일이 바뀌었습니다. 끝난 단계는 다시 실행하지 않습니다. 계속할까요? [y/N] ")
			answer, _ := in.ReadString('\n')
			if a := strings.TrimSpace(strings.ToLower(answer)); a != "y" && a != "yes" {
				fmt.Println("시작하지 않았습니다")
				return 2
			}
		}
	}
	if !k4Kept {
		switch {
		case opts.ArchivePath != "":
			if err := plan.CheckArchive(opts.ArchivePath); err != nil {
				fmt.Fprintln(os.Stderr, "✗ "+err.Error()+". --archive 를 고친 뒤 다시 실행하세요")
				return 2
			}
		case plan.NeedsArchive(s.AipubEnabled, opts) && !noArchive:
			fmt.Fprintln(os.Stderr, "✗ 새 사이트인데 --archive 가 없습니다. K2 가 만드는 사용자 레지스트리는 비어 있어, K4 (push-user-registry-artifacts) 없이는 "+
				"A1 setup-network 부터 이미지를 받지 못해 실패합니다.\n"+
				"  --archive <"+plan.ArchiveName+" 의 경로> 를 붙이세요. 알고도 K4 없이 하려면 --no-archive")
			for _, f := range plan.FindArchive(plan.ArchiveDirs()) {
				fmt.Fprintln(os.Stderr, "  찾은 파일: "+f)
			}
			return 2
		}
	}
	opts.SiteDir = savedIn
	opts.JWKSCAReady = fileio.LookupCA(s).PEM != ""
	opts.StorageVars = plan.ReadStorageVars(paths.AipubVars)

	// Ctrl-C: the first stops after the running steps, the second now; SIGTERM
	// and SIGHUP (the ssh session dropped) now. Nothing runs past this program
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	var got atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var runner atomic.Pointer[plan.Runner]
	go func() {
		ints := 0
		for sig := range sigs {
			got.CompareAndSwap(0, int32(sig.(syscall.Signal)))
			r := runner.Load()
			if sig == os.Interrupt && ints == 0 && r != nil {
				ints++
				fmt.Fprintln(os.Stderr, "\n── 실행 중인 단계가 끝나면 멈춥니다 (한 번 더 Ctrl-C 면 바로 멈춤)")
				r.Stop()
				continue
			}
			if r != nil {
				fmt.Fprintln(os.Stderr, "\n── 실행 중인 단계를 멈춥니다")
				r.Kill()
			}
			cancel()
		}
	}()
	signalled := func() int { return 128 + int(got.Load()) }

	steps := plan.Build(s, opts)
	fmt.Println("── 사이트: " + opts.SiteWhy)
	fmt.Println("── 사전 확인")
	results, shell := plan.Prechecks(ctx, s, opts, plan.ExecRun, steps)
	for _, st := range steps {
		if r, ok := results[st.ID]; ok && st.Kind == plan.Check {
			icon := "✓"
			switch {
			case r.Skipped:
				icon = "-"
			case !r.OK:
				icon = "✗"
			case r.Warn:
				icon = "!"
			}
			fmt.Printf("%s %-4s %s: %s\n", icon, st.ID, st.Title, strings.Join(r.Lines, " / "))
		}
	}
	if ctx.Err() != nil {
		return signalled()
	}
	if _, failed := plan.CheckVerdict(steps, results); len(failed) > 0 {
		fmt.Fprintln(os.Stderr, "✗ 사전 확인 실패: "+strings.Join(failed, " ")+". 고친 뒤 다시 실행하세요")
		return 2
	}
	opts.ShellExecutable = shell
	stages := plan.Stages(plan.Build(s, opts), opts, savedIn)
	fmt.Println("── 단계 (| 다음 단계, + 함께 실행): " + plan.StageIDs(stages))

	r := plan.NewRunner(opts, s, stages, inputs)
	byID := map[string]plan.Step{}
	for _, stage := range stages {
		for _, st := range stage {
			byID[st.ID] = st
		}
	}
	// Enter answers the StorageClass stop. Once the input has ended (no
	// terminal) nothing can answer it: the run stops there, not before
	var eof atomic.Bool
	noAnswer := func() {
		fmt.Fprintln(os.Stderr, "── 입력이 없어 멈춥니다 (StorageClass 단계는 Enter 로 답합니다)")
		r.Stop()
	}
	var mu sync.Mutex // the lines of steps running together
	r.Notify = func(st plan.StepState) {
		mu.Lock()
		printStep(st, byID[st.ID])
		mu.Unlock()
		if st.Status == plan.StatusWaiting && eof.Load() {
			noAnswer()
		}
	}
	runner.Store(r)
	go func() {
		for {
			if _, err := in.ReadString('\n'); err != nil {
				eof.Store(true)
				if waiting(r.Snapshot()) {
					noAnswer()
				}
				return
			}
			r.Continue()
		}
	}()

	start := time.Now()
	outcome, err := r.Run(ctx, resume)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ 시작하지 못했습니다:", err)
		var locked *plan.LockedError
		if errors.As(err, &locked) || ctx.Err() == nil {
			return 2
		}
		return signalled()
	}
	snap := r.Snapshot()
	printSummary(os.Stdout, snap, outcome, time.Since(start))
	if err := r.SaveErr(); err != nil {
		fmt.Fprintln(os.Stderr, "! "+dir+"/state.json 을 쓰지 못했습니다 ("+err.Error()+"). 기록이 실행보다 뒤처졌을 수 있습니다")
	}
	switch {
	case outcome == plan.OutcomeOK:
		return 0
	case outcome == plan.OutcomeStopped && got.Load() != 0:
		return signalled()
	}
	return 1
}

// keptFromLast are the flags of this start that differ from the run a resume
// goes on with, as that run had them. The archive is kept only with a K4 that
// succeeded (k4Kept); otherwise the --archive of this start is used.
func keptFromLast(prev *plan.RunState, o plan.Options, k4Kept bool) []string {
	var out []string
	if o.Existing != prev.Existing {
		out = append(out, fmt.Sprintf("--existing=%t", prev.Existing))
	}
	if o.AipubInstalled != prev.AipubInstalled {
		out = append(out, fmt.Sprintf("--aipub-installed=%t", prev.AipubInstalled))
	}
	if o.Monitoring != prev.Monitoring {
		out = append(out, fmt.Sprintf("--no-monitoring=%t", !prev.Monitoring))
	}
	if k4Kept && o.ArchivePath != prev.ArchivePath {
		out = append(out, "--archive "+prev.ArchivePath+" (K4 끝남)")
	}
	return out
}

func waiting(st plan.RunState) bool {
	for _, s := range st.Steps {
		if s.Status == plan.StatusWaiting {
			return true
		}
	}
	return false
}

func printStep(st plan.StepState, step plan.Step) {
	at := time.Now().Format("15:04:05")
	switch st.Status {
	case plan.StatusRunning:
		if step.Kind == plan.Manual {
			fmt.Printf("%s ▶ %s %s: %s\n", at, st.ID, st.Title, st.Note)
			return
		}
		fmt.Printf("%s ▶ %s %s  (로그 %s)\n", at, st.ID, st.Title, st.Log)
	case plan.StatusWaiting:
		fmt.Printf("%s ⏸ %s %s\n         %s\n         %s\n         설치했으면 Enter (설치함, 계속). Ctrl-C 멈춤\n",
			at, st.ID, plan.ManualPrompt(step), st.Title, st.Note)
	case plan.StatusOK:
		fmt.Printf("%s ✓ %s %s  %s\n", at, st.ID, st.Title, elapsed(st.Elapsed(time.Now())))
	case plan.StatusFailed:
		code := ""
		if st.ExitCode != nil {
			code = fmt.Sprintf(" (종료 코드 %d)", *st.ExitCode)
		}
		fmt.Printf("%s ✗ %s %s  실패%s  %s\n", at, st.ID, st.Title, code, elapsed(st.Elapsed(time.Now())))
	case plan.StatusCancelled:
		fmt.Printf("%s ✗ %s %s  %s\n", at, st.ID, st.Title, st.Note)
	}
}

// printSummary is the end of a run: what failed with its log and last lines,
// or what is left.
func printSummary(w io.Writer, st plan.RunState, outcome plan.Outcome, took time.Duration) {
	fmt.Fprintln(w, "──")
	switch {
	case outcome == plan.OutcomeOK:
		fmt.Fprintf(w, "✓ 설치를 마쳤습니다 (%s)\n", elapsed(took))
		return
	case outcome == plan.OutcomeStopped:
		fmt.Fprintln(w, "■ 멈췄습니다. --install --resume 로 이어서 실행합니다")
	case st.WentOn():
		// to its end, past a step that stays failed: still exit code 1
		fmt.Fprintln(w, "■ 끝까지 실행했지만 아래 단계는 실패로 남았습니다. 원인을 푼 뒤 --install --resume 로 그 단계만 다시 실행합니다")
	default:
		fmt.Fprintln(w, "✗ 실패했습니다. 고친 뒤 --install --resume 로 실패한 단계부터 이어서 실행합니다")
	}
	for _, s := range st.Steps {
		if s.Status != plan.StatusFailed {
			continue
		}
		fmt.Fprintf(w, "✗ %s %s  로그: %s\n", s.ID, s.Title, s.Log)
		if s.Note != "" {
			fmt.Fprintln(w, "  "+s.Note)
		}
		for _, l := range plan.TailLines(s.Log, 10) {
			fmt.Fprintln(w, "  │ "+l)
		}
	}
	var left []string
	for _, s := range st.Remaining() {
		left = append(left, s.ID)
	}
	if len(left) > 0 {
		fmt.Fprintln(w, "남은 단계: "+strings.Join(left, " "))
	}
}

func elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

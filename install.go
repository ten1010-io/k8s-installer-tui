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
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
			fmt.Fprintln(os.Stderr, i18n.T("state.json: %v — to start over, use --fresh", err))
			return 2
		case prev != nil && len(prev.Remaining()) > 0:
			var left []string
			for _, st := range prev.Remaining() {
				left = append(left, st.ID)
			}
			fmt.Fprintln(os.Stderr, i18n.T("the last run (started at %s) did not finish. Steps left: %s\n"+
				"To resume, add --resume; to run every step again from the start, add --fresh",
				prev.Started.Format("2006-01-02 15:04"), strings.Join(left, " ")))
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
			fmt.Fprintln(os.Stderr, i18n.T("%s/state.json is not there, so there is no run to resume. Run without --resume", dir))
			return 2
		}
		if k4 := prev.Step("K4"); k4 != nil && k4.Status == plan.StatusOK {
			k4Kept = true
		}
		// the plan of the run it goes on with, whatever the flags say now
		if differ := keptFromLast(prev, opts, k4Kept); len(differ) > 0 {
			fmt.Println(i18n.T("! a resumed run plans with the settings of the last run (different from the flags now): %s. To change them, start over with --fresh",
				strings.Join(differ, ", ")))
		}
		opts.Existing, opts.AipubInstalled = prev.Existing, prev.AipubInstalled
		opts.SiteWhy = plan.ResumedSite(prev, opts.SiteWhy)
		opts.Monitoring = prev.Monitoring
		// a K4 that did not succeed runs with the --archive of this start, before
		// the step the last run stopped at; one that did keeps its archive
		if k4Kept {
			opts.ArchivePath = prev.ArchivePath
		} else if opts.ArchivePath != "" {
			fmt.Println(i18n.T("── K4 did not finish in the last run: running from K4 with this run's --archive: %s", opts.ArchivePath))
		}
		var done []string
		for _, st := range prev.Steps {
			if st.Status == plan.StatusOK {
				done = append(done, st.ID)
			}
		}
		fmt.Println(i18n.T("── resuming (the run started at %s). Steps already done: %s", prev.Started.Format("2006-01-02 15:04"), strings.Join(done, " ")))
		if prev.InputHash != plan.InputHash(inputs) {
			fmt.Print(i18n.T("! the site files changed since the last run. Steps already done are not run again. Continue? [y/N] "))
			answer, _ := in.ReadString('\n')
			if a := strings.TrimSpace(strings.ToLower(answer)); a != "y" && a != "yes" {
				fmt.Println(i18n.T("not started"))
				return 2
			}
		}
	}
	if !k4Kept {
		switch {
		case opts.ArchivePath != "":
			if err := plan.CheckArchive(opts.ArchivePath); err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("✗ %v. Fix --archive and run again", err))
				return 2
			}
		case plan.NeedsArchive(s.AipubEnabled, opts) && !noArchive:
			fmt.Fprintln(os.Stderr, i18n.T("✗ this is a new site but there is no --archive. The user registry K2 makes is empty, so without K4 (push-user-registry-artifacts) "+
				"the images cannot be pulled and the steps from A1 setup-network on fail.\n"+
				"  Add --archive <path of %s>. To run without K4 anyway, use --no-archive", plan.ArchiveName))
			for _, f := range plan.FindArchive(plan.ArchiveDirs()) {
				fmt.Fprintln(os.Stderr, i18n.T("  found: %s", f))
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
				fmt.Fprintln(os.Stderr, "\n"+i18n.T("── stopping when the running step ends (Ctrl-C once more stops at once)"))
				r.Stop()
				continue
			}
			if r != nil {
				fmt.Fprintln(os.Stderr, "\n"+i18n.T("── stopping the running step"))
				r.Kill()
			}
			cancel()
		}
	}()
	signalled := func() int { return 128 + int(got.Load()) }

	steps := plan.Build(s, opts)
	fmt.Println(i18n.T("── site: %s", opts.SiteWhy))
	fmt.Println(i18n.T("── checks"))
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
			lines := append([]string{}, r.Lines...)
			if r.Skipped && len(lines) > 0 {
				// as the install screen says it: the check gives the reason only
				lines[0] = i18n.T("skipped: %s", lines[0])
			}
			fmt.Printf("%s %-4s %s: %s\n", icon, st.ID, st.Title, strings.Join(lines, " / "))
		}
	}
	if ctx.Err() != nil {
		return signalled()
	}
	if _, failed := plan.CheckVerdict(steps, results); len(failed) > 0 {
		fmt.Fprintln(os.Stderr, i18n.T("✗ checks failed: %s. Fix them and run again", strings.Join(failed, " ")))
		return 2
	}
	opts.ShellExecutable = shell
	stages := plan.Stages(plan.Build(s, opts), opts, savedIn)
	fmt.Println(i18n.T("── steps (| next step, + run together): %s", plan.StageIDs(stages)))

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
		fmt.Fprintln(os.Stderr, i18n.T("── no input, stopping (the StorageClass step is answered with Enter)"))
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
		fmt.Fprintln(os.Stderr, i18n.T("✗ could not start: %v", err))
		var locked *plan.LockedError
		if errors.As(err, &locked) || ctx.Err() == nil {
			return 2
		}
		return signalled()
	}
	snap := r.Snapshot()
	printSummary(os.Stdout, snap, outcome, time.Since(start))
	if err := r.SaveErr(); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("! could not write %s/state.json (%v). The record may be behind the run", dir, err))
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
		out = append(out, i18n.T("--archive %s (K4 done)", prev.ArchivePath))
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
		fmt.Printf("%s ▶ %s %s  %s\n", at, st.ID, st.Title, i18n.T("(log %s)", st.Log))
	case plan.StatusWaiting:
		fmt.Printf("%s ⏸ %s %s\n         %s\n         %s\n         %s\n",
			at, st.ID, plan.ManualPrompt(step), st.Title, st.Note, i18n.T("When installed, press Enter (installed, continue). Ctrl-C stops"))
	case plan.StatusOK:
		fmt.Printf("%s ✓ %s %s  %s\n", at, st.ID, st.Title, elapsed(st.Elapsed(time.Now())))
	case plan.StatusFailed:
		code := ""
		if st.ExitCode != nil {
			code = i18n.T(" (exit code %d)", *st.ExitCode)
		}
		fmt.Printf("%s ✗ %s %s  %s%s  %s\n", at, st.ID, st.Title, i18n.T("failed"), code, elapsed(st.Elapsed(time.Now())))
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
		fmt.Fprintln(w, i18n.T("✓ install finished (%s)", elapsed(took)))
		return
	case outcome == plan.OutcomeStopped:
		fmt.Fprintln(w, i18n.T("■ stopped. --install --resume resumes from here"))
	case st.WentOn():
		// to its end, past a step that stays failed: still exit code 1
		fmt.Fprintln(w, i18n.T("■ ran to the end, but the steps below stay failed. After fixing the cause, --install --resume runs only those steps again"))
	default:
		fmt.Fprintln(w, i18n.T("✗ failed. After fixing it, --install --resume resumes from the failed step"))
	}
	for _, s := range st.Steps {
		if s.Status != plan.StatusFailed {
			continue
		}
		fmt.Fprintln(w, i18n.T("✗ %s %s  log: %s", s.ID, s.Title, s.Log))
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
		fmt.Fprintln(w, i18n.T("Steps left: %s", strings.Join(left, " ")))
	}
}

func elapsed(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	// termenv (lipgloss) detects color support from COLORTERM/TERM env vars.
	// SSH sessions often don't forward COLORTERM, causing lipgloss to fall back
	// to 16-color mode even on 256-color terminals. Force truecolor so lipgloss
	// always outputs 256-color escape codes, matching what nmtui/k9s see.
	if os.Getenv("COLORTERM") == "" {
		os.Setenv("COLORTERM", "truecolor")
	}
	// the language first: the help of the flags is in it
	if err := i18n.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	flag.String("lang", string(i18n.Current()), i18n.T("language of the screens and messages: en (default) or ko; %s=ko does the same", i18n.EnvVar))
	inventoryPath := flag.String("inventory", "inventory.yml", i18n.T("path of inventory.yml"))
	varsPath := flag.String("vars", "group_vars/all/vars.yml", i18n.T("path of vars.yml (k8s-installer)"))
	aipubVarsPath := flag.String("aipub-vars", "files/aipub/vars.yml", i18n.T("path of the AIPub site values (aipub-installer)"))
	renderDir := flag.String("render", "", i18n.T("headless: render the three files and the overview site-values.yml into this directory and print the validation result (existing files are left as they are)"))
	installerRoot := flag.String("installer-root", plan.DefaultOptions().Root, i18n.T("directory k8s-installer is installed in (setup.sh)"))
	fillAuth := flag.Bool("fill-auth-config", false, i18n.T("headless: rebuild files/aipub/authentication-config.yaml of the AIPub authentication (JWKS) from the issuer and the CA and write it "+
		"to both the saved site and <installer-root>/ansible/files (running it again changes nothing). Fails when there is no CA"))
	jwksCA := flag.String("jwks-ca", "", i18n.T("JWKS CA file (default %s, put there by aipub-installer setup-cert)", state.JWKSCAPath))
	checkSSH := flag.Bool("check-ssh", false, i18n.T("headless: print a table of whether every node of --inventory accepts the key (~/.ssh/id_rsa) of this control node and runs sudo without a password "+
		"(read only, known_hosts untouched). 0 all ready, 1 a node is not ready, 2 could not run"))
	sendSSHKey := flag.Bool("send-ssh-key", false, i18n.T("headless: send the key with ssh-copy-id to the nodes that do not accept it (ssh asks for each node's password) "+
		"and print the table of --check-ssh. If there is no key, it is made with <installer-root>/ansible/configure-control-node-ssh.sh. Ctrl-C 130, SIGTERM 143, SIGHUP 129"))
	askPassOnce := flag.Bool("ask-pass-once", false, i18n.T("with --send-ssh-key, ask for the password once on the terminal (not shown) and use it for every node"))
	install := flag.Bool("install", false, i18n.T("headless: run the install. After the checks (P1..P8, it does not start on a ✗) run the steps of the plan in turn and "+
		"wait for Enter at the StorageClass step (M1). State and logs go to <installer-root>/tui-install. "+
		"Ctrl-C once stops after the running step, twice stops at once. Exit code 0 done, 1 failed, 2 could not start, 130 interrupted"))
	resume := flag.Bool("resume", false, i18n.T("with --install, skip the steps that succeeded in the last run (state.json) and resume "+
		"(the plan uses --existing, --aipub-installed and --no-monitoring of the last run. --archive is the last run's if its K4 succeeded; "+
		"otherwise this run's --archive is used, starting again from K4)"))
	fresh := flag.Bool("fresh", false, i18n.T("--install: run every step from the start even if a last run did not finish (without it, --install does not start). "+
		"The site is judged by its state now (%s)", plan.ClusterRecord))
	existing := flag.Bool("existing", false, i18n.T("plan for a site that already has a cluster (wizard and --install). Without it, the site is an existing one only when %s "+
		"(the record k8s-installer leaves after it creates the cluster) is there. --resume uses the last run's", plan.ClusterRecord))
	aipubInstalled := flag.Bool("aipub-installed", false, i18n.T("plan for an existing site that has AIPub installed too (wizard and --install. "+
		"The CA of setup-cert is on the nodes and the JWKS issuer answers: the control plane entries are applied in one update-cluster). "+
		"aipub-installer leaves no install record, so only this flag decides. Default is no AIPub"))
	archive := flag.String("archive", "", i18n.T("--install: path of the user registry archive (%s) for K4. "+
		"Needed for a new site that installs AIPub (it does not start without it, --no-archive). On an existing site, leaving it empty skips K4", plan.ArchiveName))
	noArchive := flag.Bool("no-archive", false, i18n.T("--install: start a new site that installs AIPub without --archive (skips K4: the registry is empty and the steps from A1 on may fail)"))
	noMonitoring := flag.Bool("no-monitoring", false, i18n.T("--install: do not install monitoring (A4)"))
	noPython := flag.Bool("no-python", false, i18n.T("--install: do not install the python services (A6, A7)"))
	showVersion := flag.Bool("version", false, i18n.T("print the version"))
	flag.Parse()

	if *showVersion {
		fmt.Println("k8s-installer-tui", version)
		os.Exit(0)
	}
	// absolute and clean, as the site paths are: the steps run in <root>/ansible,
	// and C1 is left out only when the site is that directory (a / at the end,
	// from tab completion, would have it copy the files onto themselves)
	*installerRoot = abs(*installerRoot)

	if *checkSSH || *sendSSHKey {
		// the nodes as ansible-inventory gives them; the settings are not read
		opts := plan.DefaultOptions()
		opts.Root = *installerRoot
		os.Exit(sshKeys(opts, abs(*inventoryPath), *sendSSHKey, *askPassOnce))
	}

	paths := fileio.Paths{Inventory: abs(*inventoryPath), Vars: abs(*varsPath), AipubVars: abs(*aipubVarsPath)}
	if *jwksCA != "" {
		fileio.DefaultCAPath = abs(*jwksCA)
	}

	// Start from defaults, then overlay existing files
	s := state.DefaultState()
	if err := fileio.LoadAll(paths, s); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("load error: %v", err))
		os.Exit(1)
	}

	if *fillAuth {
		os.Exit(fillAuthConfig(paths, s, filepath.Join(*installerRoot, "ansible", "files")))
	}
	// whether the cluster is there: said (--existing) or recorded on this control
	// node, never taken from the files of the site (DetectSite), for the wizard,
	// --install and --render alike
	site := plan.DetectSite(*existing, *aipubInstalled)
	if *renderDir != "" {
		// an existing site keeps the audit policy of its nodes: the default is
		// not rendered for it until the wizard has read them (PlanAuditPolicy)
		s.Existing = site.Existing
		fmt.Println("site:", site.Why)
		os.Exit(render(paths, s, *renderDir))
	}
	if *aipubInstalled && !site.Existing {
		fmt.Fprintln(os.Stderr, i18n.T("--aipub-installed is for an existing site only (AIPub cannot be there without a cluster): %s", site.Why))
		os.Exit(2)
	}
	if *install {
		if *resume && *fresh {
			fmt.Fprintln(os.Stderr, i18n.T("--resume and --fresh cannot be used together"))
			os.Exit(2)
		}
		opts := plan.DefaultOptions()
		opts.Root, opts.ArchivePath, opts.Monitoring = *installerRoot, plan.AbsArchive(*archive), !*noMonitoring
		opts.Python = !*noPython
		opts.Existing, opts.AipubInstalled, opts.SiteWhy = site.Existing, site.AipubInstalled, site.Why
		// as the wizard does: P3 of an existing site keeps its nodes' audit policy
		// under files/aipub where none is, so that C1 copies it to the installer
		opts.AuditPolicy = paths.AuditPolicy()
		os.Exit(installHeadless(paths, s, opts, *resume, *fresh, *noArchive))
	}

	opts := plan.DefaultOptions()
	opts.Root = *installerRoot
	opts.Existing, opts.AipubInstalled, opts.SiteWhy = site.Existing, site.AipubInstalled, site.Why
	// a run from the first step decides the site again (the cluster may have
	// been built or reset since the program started), with the same flags
	opts.Detect = func() plan.Site { return plan.DetectSite(*existing, *aipubInstalled) }
	app := ui.NewApp(s, paths, opts, version)
	p := tea.NewProgram(app, tea.WithAltScreen())
	// SIGTERM and SIGHUP (the ssh session dropped) end the program too, by way
	// of the cleanup below; bubbletea alone would quit on SIGTERM without it and
	// die on SIGHUP
	// not Ctrl-C: it is a key here, and a signal only while ssh-copy-id has the
	// terminal (k), where it stops that and comes back
	ctx, code, release := stopSignals(syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err := p.Run()
	// whatever ended it: a key send still running stops, its commands are
	// killed and its helper directory is removed, in a bounded time
	app.Stop()
	stopped := ctx.Err() != nil
	release()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("error: %v", err))
		os.Exit(1)
	}
	if stopped {
		os.Exit(code())
	}
}

// render writes what saving would write, under dir in the layout of the ansible
// directory of k8s-installer, and reports the validation. 0 when it is valid.
func render(from fileio.Paths, s *state.AppState, dir string) int {
	r, err := fileio.Render(from, s)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("render error: %v", err))
		return 1
	}
	to := fileio.DefaultPaths(dir)
	// the audit policy saving would write (the default), or which one it keeps
	s.ControlPlane.AuditNote = r.Audit.Note
	if r.Audit.Note != "" {
		fmt.Println("audit policy:", r.Audit.Note)
	}
	for _, f := range []struct {
		path string
		data []byte
	}{{to.Inventory, r.Inventory}, {to.Vars, r.Vars}, {to.AipubVars, r.AipubVars}, {to.WebhookConfig(), r.WebhookConfig}, {to.AuthConfig(), r.AuthConfig},
		{to.AuditPolicy(), r.AuditPolicy}} {
		if f.data == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(f.path, f.data, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("written:", f.path)
	}
	// the view of all of it, for a person to read; nothing reads it back
	view := filepath.Join(dir, schema.SiteValuesFile)
	if err := os.WriteFile(view, schema.SiteValues(s), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("written:", view)
	if r.JWKSPending(s) {
		fmt.Println(i18n.T("JWKS: waiting for the CA (%s is not there) — %s is not written, --fill-auth-config of the install plan makes it after setup-cert", r.CA.Path, state.JWKSConfigFile))
	}
	if r.CA.Err != nil && s.AppliedControlPlane().JWKS {
		fmt.Println(i18n.T("JWKS: CA file error: %v", r.CA.Err))
	}
	errs := validator.Validate(s)
	for _, e := range errs {
		fmt.Println("✗", e.Error())
	}
	if len(errs) > 0 {
		return 2
	}
	fmt.Println(i18n.T("✓ validation passed"))
	return 0
}

// fillAuthConfig is --fill-auth-config (fileio.FillAuthConfig), the step of the
// plan that puts the CA into the authentication config of the JWKS preset after
// setup-cert. 0 when both copies hold it.
func fillAuthConfig(p fileio.Paths, s *state.AppState, installerFiles string) int {
	lines, err := fileio.FillAuthConfig(p, s, installerFiles)
	for _, l := range lines {
		fmt.Println(l)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "--fill-auth-config:", err)
		return 1
	}
	return 0
}

// sshKeys is --check-ssh, and --send-ssh-key (send) with or without
// --ask-pass-once (once): the key of the control node on the nodes of inventory,
// then the table of every node. 0 when every node is ready, 1 when one is not,
// 2 when it could not run, 128 + the signal when stopped (130 Ctrl-C, 143
// SIGTERM, 129 SIGHUP: the ssh session dropped).
func sshKeys(o plan.Options, inventory string, send, once bool) int {
	ctx, code, stop := stopSignals(os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	run, key := plan.ExecRun, plan.DefaultSSHKey()

	if send {
		if err := plan.PrepareKey(ctx, o, run, key); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("preparing the key failed: %v", err))
			return interrupted(ctx, code, 2)
		}
	} else if _, err := os.Stat(key.Private); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("! %s is not there: no node is ready. --send-ssh-key makes it and sends it", key.Private))
	}
	nodes, warnings, err := plan.ListSSHNodes(ctx, o, run, inventory)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("listing the nodes failed: %v", err))
		return interrupted(ctx, code, 2)
	}
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, i18n.T("no nodes in %s", inventory))
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "!", w)
	}

	results, err := plan.ProbeNodes(ctx, run, key, nodes, !send)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("the check failed: %v", err))
		return interrupted(ctx, code, 2)
	}
	if ctx.Err() != nil {
		return interrupted(ctx, code, 130)
	}
	if refusing := plan.Refusing(results); send && len(refusing) > 0 {
		byName := map[string]int{}
		for i, r := range results {
			byName[r.Node.Name] = i
		}
		if once {
			password, err := readPassword(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "--ask-pass-once:", err)
				return interrupted(ctx, code, 2)
			}
			fmt.Fprintln(os.Stderr, i18n.T("── sending the key (%d nodes)", len(refusing)))
			err = plan.SendKeyWithPassword(ctx, run, key, refusing, password, func(r plan.NodeSSH) {
				results[byName[r.Node.Name]] = r
				verdict := i18n.T("key accepted")
				if r.Key != plan.KeyOK {
					verdict = r.Label() + ": " + r.Note()
				}
				fmt.Fprintf(os.Stderr, "── %s: %s\n", r.Node.Name, verdict)
			})
			for i := range password {
				password[i] = 0
			}
			if err != nil {
				if ctx.Err() == nil {
					fmt.Fprintln(os.Stderr, i18n.T("sending the key failed: %v", err))
				}
				return interrupted(ctx, code, 2)
			}
		} else {
			// ssh asks each password on this terminal; Ctrl-C reaches it and ends
			// the script, and this process sees it through ctx. A SIGTERM or
			// SIGHUP to this process alone is passed on to the job
			cmd := exec.Command("bash", "-c", plan.AskSendScript(key, refusing, false))
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
			ran := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					plan.TerminateJob()
				case <-ran:
				}
			}()
			_ = cmd.Run()
			close(ran)
			if ctx.Err() != nil {
				return interrupted(ctx, code, 130)
			}
			for _, n := range refusing {
				results[byName[n.Name]] = plan.ProbeNode(ctx, run, key, n, "")
			}
		}
	}
	if ctx.Err() != nil {
		return interrupted(ctx, code, 130)
	}

	fmt.Print("\n" + plan.SSHTable(results))
	for _, r := range results {
		if !r.Ready() {
			fmt.Println(i18n.T("✗ some nodes are not ready. Fix them as the notes say and run again"))
			return 1
		}
	}
	fmt.Println(i18n.T("✓ every node accepts the key of this control node and runs sudo without a password"))
	return 0
}

// interrupted is the exit status of the signal that stopped ctx, else def.
func interrupted(ctx context.Context, code func() int, def int) int {
	if ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, i18n.T("stopped (exit code %d)", code()))
		return code()
	}
	return def
}

// stopSignals: ctx ends on one of sigs (Ctrl-C, SIGTERM, SIGHUP: the ssh session dropped);
// code is then the exit status, 128 + the first signal: 130, 143 or 129.
func stopSignals(sigs ...os.Signal) (ctx context.Context, code func() int, release func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, sigs...)
	var got atomic.Int32
	go func() {
		for s := range ch {
			if sig, ok := s.(syscall.Signal); ok {
				got.CompareAndSwap(0, int32(sig))
			}
			cancel()
		}
	}()
	return ctx, func() int { return 128 + int(got.Load()) }, func() {
		signal.Stop(ch)
		close(ch)
		cancel()
	}
}

// readPassword asks the password on the terminal without echo. On Ctrl-C the
// terminal gets its echo back.
func readPassword(ctx context.Context) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New(i18n.T("a terminal is needed to ask for the password"))
	}
	saved, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stderr, i18n.T("SSH password of the nodes (not shown): "))
	type read struct {
		pw  []byte
		err error
	}
	ch := make(chan read, 1)
	go func() {
		pw, err := term.ReadPassword(fd)
		ch <- read{pw, err}
	}()
	select {
	case r := <-ch:
		fmt.Fprintln(os.Stderr)
		if r.err == nil && len(r.pw) == 0 {
			r.err = errors.New(i18n.T("the password is empty"))
		}
		return r.pw, r.err
	case <-ctx.Done():
		_ = term.Restore(fd, saved)
		fmt.Fprintln(os.Stderr)
		return nil, ctx.Err()
	}
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("path error: %v", err))
		os.Exit(1)
	}
	return a
}

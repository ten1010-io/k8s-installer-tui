package main

import (
	"context"
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
	inventoryPath := flag.String("inventory", "inventory.yml", "inventory.yml 경로")
	varsPath := flag.String("vars", "group_vars/all/vars.yml", "vars.yml 경로 (k8s-installer)")
	aipubVarsPath := flag.String("aipub-vars", "files/aipub/vars.yml", "AIPub 사이트 값 경로 (aipub-installer)")
	renderDir := flag.String("render", "", "화면 없이 세 파일과 한눈에 보는 site-values.yml 을 이 디렉터리에 렌더링하고 검증 결과를 출력 (기존 파일은 그대로)")
	installerRoot := flag.String("installer-root", plan.DefaultOptions().Root, "k8s-installer가 설치된 디렉터리 (setup.sh)")
	fillAuth := flag.Bool("fill-auth-config", false, "화면 없이 AIPub 인증 (JWKS) 의 files/aipub/authentication-config.yaml 을 issuer 와 CA 로 다시 만들어 "+
		"저장한 사이트와 <installer-root>/ansible/files 양쪽에 씀 (다시 실행해도 같음). CA 가 없으면 실패")
	jwksCA := flag.String("jwks-ca", "", "JWKS CA 파일 (기본 "+state.JWKSCAPath+", aipub-installer setup-cert 가 둠)")
	checkSSH := flag.Bool("check-ssh", false, "화면 없이 --inventory 의 모든 노드가 이 제어 노드의 키 (~/.ssh/id_rsa) 를 받고 비밀번호 없이 sudo 하는지 "+
		"표로 출력 (읽기만, known_hosts 도 그대로). 0 모두 준비됨, 1 준비 안 된 노드 있음, 2 실행 못 함")
	sendSSHKey := flag.Bool("send-ssh-key", false, "화면 없이 키를 받지 않는 노드에 ssh-copy-id 로 키를 보내고 (비밀번호는 노드마다 ssh 가 물음) "+
		"--check-ssh 의 표를 출력. 키가 없으면 <installer-root>/ansible/configure-control-node-ssh.sh 로 만듦. Ctrl-C 130, SIGTERM 143, SIGHUP 129")
	askPassOnce := flag.Bool("ask-pass-once", false, "--send-ssh-key 에서 비밀번호를 터미널에서 한 번만 물어 (화면에 보이지 않음) 모든 노드에 씀")
	showVersion := flag.Bool("version", false, "버전 출력")
	flag.Parse()

	if *showVersion {
		fmt.Println("k8s-installer-tui", version)
		os.Exit(0)
	}

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
		fmt.Fprintf(os.Stderr, "로드 오류: %v\n", err)
		os.Exit(1)
	}

	if *fillAuth {
		os.Exit(fillAuthConfig(paths, s, filepath.Join(*installerRoot, "ansible", "files")))
	}
	if *renderDir != "" {
		// an existing site keeps the audit policy of its nodes: the default is
		// not rendered for it until the wizard has read them (PlanAuditPolicy)
		if loaded, _ := fileio.LoadSite(paths); loaded != nil {
			s.Existing = true
		}
		os.Exit(render(paths, s, *renderDir))
	}

	opts := plan.DefaultOptions()
	opts.Root = *installerRoot
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
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "렌더링 오류: %v\n", err)
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
		fmt.Println("JWKS: CA 대기 (" + r.CA.Path + " 없음) — " + state.JWKSConfigFile + " 는 쓰지 않음, 설치 계획의 --fill-auth-config 가 setup-cert 뒤에 만듦")
	}
	if r.CA.Err != nil && s.AppliedControlPlane().JWKS {
		fmt.Println("JWKS: CA 파일 오류:", r.CA.Err)
	}
	errs := validator.Validate(s)
	for _, e := range errs {
		fmt.Println("✗", e.Error())
	}
	if len(errs) > 0 {
		return 2
	}
	fmt.Println("✓ 검증 통과")
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
			fmt.Fprintln(os.Stderr, "키 준비 실패:", err)
			return interrupted(ctx, code, 2)
		}
	} else if _, err := os.Stat(key.Private); err != nil {
		fmt.Fprintln(os.Stderr, "!", key.Private, "가 없습니다: 어느 노드도 준비되지 않았습니다. --send-ssh-key 가 만들고 보냅니다")
	}
	nodes, warnings, err := plan.ListSSHNodes(ctx, o, run, inventory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "노드 목록 실패:", err)
		return interrupted(ctx, code, 2)
	}
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, inventory, "에 노드가 없습니다")
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "!", w)
	}

	results, err := plan.ProbeNodes(ctx, run, key, nodes, !send)
	if err != nil {
		fmt.Fprintln(os.Stderr, "확인 실패:", err)
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
			fmt.Fprintf(os.Stderr, "── 키 보내는 중 (%d대)\n", len(refusing))
			err = plan.SendKeyWithPassword(ctx, run, key, refusing, password, func(r plan.NodeSSH) {
				results[byName[r.Node.Name]] = r
				verdict := "키 받음"
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
					fmt.Fprintln(os.Stderr, "키 보내기 실패:", err)
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
			fmt.Println("✗ 준비되지 않은 노드가 있습니다. 비고대로 고친 뒤 다시 실행하세요")
			return 1
		}
	}
	fmt.Println("✓ 모든 노드가 이 제어 노드의 키를 받고 비밀번호 없이 sudo 합니다")
	return 0
}

// interrupted is the exit status of the signal that stopped ctx, else def.
func interrupted(ctx context.Context, code func() int, def int) int {
	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "중단했습니다 (종료 코드 %d)\n", code())
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
		return nil, fmt.Errorf("비밀번호를 물을 터미널이 필요합니다")
	}
	saved, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(os.Stderr, "노드의 SSH 비밀번호 (화면에 보이지 않음): ")
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
			r.err = fmt.Errorf("비밀번호가 비어 있습니다")
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
		fmt.Fprintf(os.Stderr, "경로 오류: %v\n", err)
		os.Exit(1)
	}
	return a
}

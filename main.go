package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
	"github.com/ten1010-io/k8s-installer-tui/internal/validator"
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
	showVersion := flag.Bool("version", false, "버전 출력")
	flag.Parse()

	if *showVersion {
		fmt.Println("k8s-installer-tui", version)
		os.Exit(0)
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
		os.Exit(render(paths, s, *renderDir))
	}

	opts := plan.DefaultOptions()
	opts.Root = *installerRoot
	app := ui.NewApp(s, paths, opts, version)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
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
	for _, f := range []struct {
		path string
		data []byte
	}{{to.Inventory, r.Inventory}, {to.Vars, r.Vars}, {to.AipubVars, r.AipubVars}, {to.WebhookConfig(), r.WebhookConfig}, {to.AuthConfig(), r.AuthConfig}} {
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

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경로 오류: %v\n", err)
		os.Exit(1)
	}
	return a
}

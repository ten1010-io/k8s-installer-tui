package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// childEnv makes the test binary run main() with the arguments in it, as the
// program does (-h exits the process, so it cannot run in the test itself).
const childEnv = "K8S_INSTALLER_TUI_TEST_MAIN_ARGS"

func TestMain(m *testing.M) {
	if args := os.Getenv(childEnv); args != "" {
		os.Args = append([]string{"k8s-installer-tui"}, strings.Fields(args)...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// captured runs f and returns what it printed to stdout and stderr.
func captured(t *testing.T, f func()) string {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	f()
	_ = w.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	return <-done
}

// noHangul fails for each line of out that has Hangul. The lines the validator
// says (starting with a cross) are in the words of its own package, which has
// its own test.
func noHangul(t *testing.T, what, out string) {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "✗ ") && what == "render" {
			continue
		}
		if hasHangul(l) {
			t.Errorf("%s says Korean: %q", what, l)
		}
	}
}

// In English, --render, --fill-auth-config and the refusals of --install say
// nothing in Korean.
func TestCLIIsEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)

	// an existing site with the audit preset on (its policy is held) and the
	// JWKS preset waiting for its CA
	site := fileio.DefaultPaths(t.TempDir())
	old := fileio.DefaultCAPath
	fileio.DefaultCAPath = filepath.Join(t.TempDir(), "ca.crt")
	defer func() { fileio.DefaultCAPath = old }()
	s := state.DefaultState()
	s.Existing = true
	s.AipubEnabled = true
	s.AipubIngressZone = "example.com"
	s.ControlPlane.Audit, s.ControlPlane.JWKS = true, true
	var code int
	out := captured(t, func() { code = render(site, s, t.TempDir()) })
	if code == 0 {
		t.Errorf("an empty site rendered as valid:\n%s", out)
	}
	for _, want := range []string{"audit policy: ", "JWKS: waiting for the CA"} {
		if !strings.Contains(out, want) {
			t.Errorf("render did not say %q:\n%s", want, out)
		}
	}
	noHangul(t, "render", out)

	// --fill-auth-config with no CA
	out = captured(t, func() { code = fillAuthConfig(site, s, filepath.Join(t.TempDir(), "files")) })
	if code != 1 || !strings.Contains(out, "there is no CA") {
		t.Errorf("fill-auth-config: %d\n%s", code, out)
	}
	noHangul(t, "fill-auth-config", out)

	// --install --resume with no run to resume
	root := t.TempDir()
	opts := plan.DefaultOptions()
	opts.Root = root
	paths := fileio.DefaultPaths(filepath.Join(root, "site"))
	out = captured(t, func() { code = installHeadless(paths, state.DefaultState(), opts, true, false, false) })
	if code != 2 || !strings.Contains(out, "no run to resume") {
		t.Errorf("--resume with no run: %d\n%s", code, out)
	}
	noHangul(t, "--install --resume", out)

	// the end of a run
	out = captured(t, func() {
		printSummary(os.Stdout, plan.RunState{}, plan.OutcomeOK, 0)
	})
	noHangul(t, "summary", out)
}

// -h in English has no Korean, and in Korean it has the help in Korean. The
// program runs as a child, as -h exits it.
func TestHelpIsEnglish(t *testing.T) {
	run := func(args string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), childEnv+"="+args, i18n.EnvVar+"=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", args, err, out)
		}
		return string(out)
	}
	en := run("-h")
	if !strings.Contains(en, "-install") {
		t.Fatalf("no help:\n%s", en)
	}
	noHangul(t, "-h", en)
	if ko := run("--lang ko -h"); !hasHangul(ko) {
		t.Errorf("--lang ko -h has no Korean:\n%s", ko)
	}
}

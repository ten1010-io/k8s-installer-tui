package plan

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func ids(steps []Step) string {
	var out []string
	for _, s := range steps {
		id := s.ID
		if s.Skip {
			id = "(" + id + ")"
		}
		out = append(out, id)
	}
	return strings.Join(out, " ")
}

// self is the binary the plan runs for --fill-auth-config, the same in every test
const self = "/usr/local/bin/k8s-installer-tui"

func testOptions() Options {
	o := DefaultOptions()
	o.Self = self
	return o
}

func aipubState() *state.AppState {
	s := state.DefaultState()
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.K8sIngressVIP = "192.168.0.100"
	s.K8sCpNodes = []string{"node1", "node2"}
	return s
}

func TestOrder(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	// JWKS is on for a new site, and applied after setup-aipub
	if got, want := ids(Build(aipubState(), o)), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("with AIPub: %s, want %s", got, want)
	}
	if got, want := ids(Build(state.DefaultState(), o)), "P1 P2 P6 K1 K2 K3 K4"; got != want {
		t.Errorf("without AIPub: %s, want %s", got, want)
	}
	o.ArchivePath, o.Monitoring = "", false
	if got, want := ids(Build(aipubState(), o)), "P1 P2 P5 P6 P7 K1 K2 K3 (K4) A1 A2 A3 (A4) A5 A5.1 A5.2"; got != want {
		t.Errorf("no archive, no monitoring: %s, want %s", got, want)
	}
}

func TestScript(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb dir/a.tgz"
	steps := Build(aipubState(), o)

	inPlace := Script(steps, o, "/opt/k8s-installer/ansible")
	if strings.Contains(inPlace, "sudo cp") {
		t.Error("copies the files although they were saved where k8s-installer reads them")
	}
	for _, want := range []string{
		"set -euo pipefail",
		"source /opt/k8s-installer/venv/bin/activate",
		"cd /opt/k8s-installer/ansible",
		"ansible-playbook -i inventory.yml playbooks/setup-cluster.yml",
		"ansible-playbook -i inventory.yml playbooks/push-user-registry-artifacts.yml -e 'archive=/media/usb dir/a.tgz'",
		"ansible-playbook -i inventory.yml aipub-playbooks/setup-aipub.yml",
	} {
		if !strings.Contains(inPlace, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	// network before cert before base before monitoring before aipub
	last := -1
	for _, pb := range []string{"setup-network", "setup-cert", "setup-base", "setup-monitoring", "setup-aipub"} {
		i := strings.Index(inPlace, "aipub-playbooks/"+pb+".yml")
		if i < last {
			t.Errorf("%s out of order", pb)
		}
		last = i
	}

	elsewhere := Script(steps, o, "/home/ops/site-a")
	if !strings.Contains(elsewhere, "sudo cp /home/ops/site-a/inventory.yml /opt/k8s-installer/ansible/inventory.yml") {
		t.Errorf("no copy of files saved elsewhere:\n%s", elsewhere)
	}

	o.ArchivePath = ""
	skipped := Script(Build(aipubState(), o), o, "/opt/k8s-installer/ansible")
	if !strings.Contains(skipped, "# K4 push-user-registry-artifacts: 건너뜀") {
		t.Error("a skipped step is not left as a comment")
	}
}

func fakeRun(out string, err error) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte(out), err
	}
}

func TestCheckSSH(t *testing.T) {
	o := testOptions()
	ok := CheckSSH(context.Background(), o, fakeRun(
		"node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \n", nil))
	if !ok.OK || ok.Lines[0] != "정상: node1 node2" {
		t.Errorf("all reachable: %+v", ok)
	}

	bad := CheckSSH(context.Background(), o, fakeRun(
		"node1 | CHANGED | rc=0 | (stdout) \n"+
			"node2 | UNREACHABLE! => {\"msg\": \"ssh: connect to host 10.0.0.2 port 22: No route to host\"}\n"+
			"node3 | FAILED | rc=1 | (stdout) (stderr) sudo: a password is required\n", errors.New("exit status 4")))
	if bad.OK || len(bad.Lines) != 3 ||
		!strings.Contains(bad.Lines[1], "node2: 접속 안 됨") || !strings.Contains(bad.Lines[2], "node3: sudo") {
		t.Errorf("one unreachable, one without sudo: %+v", bad)
	}

	none := CheckSSH(context.Background(), o, fakeRun("bash: ansible: command not found\n", errors.New("exit status 127")))
	if none.OK || !strings.Contains(none.Lines[0], "command not found") {
		t.Errorf("ansible missing: %+v", none)
	}
}

func TestCheckInstalled(t *testing.T) {
	o := testOptions()
	o.Root = t.TempDir()
	if CheckInstalled(o).OK {
		t.Error("ok without a release file")
	}
	if err := os.WriteFile(o.Root+"/release", []byte("1.36.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if r := CheckInstalled(o); !r.OK || r.Lines[0] != "설치된 릴리스 1.36.0" {
		t.Errorf("%+v", r)
	}
}

// The webhook preset reads the CA setup-cert makes: setup-cluster builds the
// cluster without its entries, and update-cluster.yml applies them right after
// setup-cert. The other presets stay in setup-cluster.
func TestWebhookAppliedAfterCert(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	s := aipubState()
	s.ControlPlane.Audit, s.ControlPlane.RuntimeConfig, s.ControlPlane.JWKS = true, true, false
	if got, want := ids(Build(s, o)), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A3 A4 A5"; got != want {
		t.Errorf("audit and runtime-config only: %s, want %s", got, want)
	}
	for _, st := range Build(s, o) {
		if st.ID == "K3" && len(st.Args) != 0 {
			t.Errorf("K3 given %v without the webhook", st.Args)
		}
	}

	s.ControlPlane.Webhook = true
	s.ControlPlane.WebhookURL = "https://aipub.example.com/token-review"
	steps := Build(s, o)
	if got, want := ids(steps), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5"; got != want {
		t.Errorf("with the webhook: %s, want %s", got, want)
	}
	var k3, a25 Step
	for _, st := range steps {
		switch st.ID {
		case "K3":
			k3 = st
		case "A2.5":
			a25 = st
		}
	}
	if a25.Playbook != "playbooks/update-cluster.yml" || !strings.Contains(a25.Note, "한 대씩") {
		t.Errorf("A2.5: %+v", a25)
	}
	if len(k3.Args) != 2 || k3.Args[0] != "-e" {
		t.Fatalf("K3 args: %v", k3.Args)
	}
	extra := k3.Args[1]
	for _, want := range []string{`"k8s_apiserver_extra_args":[`, `"name":"audit-policy-file"`,
		`"name":"runtime-config"`, `"k8s_cp_extra_files":[{"src":"aipub/audit-policy.yaml"`} {
		if !strings.Contains(extra, want) {
			t.Errorf("K3 extra vars lack %s: %s", want, extra)
		}
	}
	if strings.Contains(extra, "webhook") || strings.Contains(extra, "k8s-installer-certs") {
		t.Errorf("K3 extra vars keep the webhook: %s", extra)
	}
	if !strings.Contains(k3.Note, "A2.5") {
		t.Errorf("K3 note: %s", k3.Note)
	}
	script := Script(steps, o, "/home/ops/site-a")
	for _, want := range []string{"ansible-playbook -i inventory.yml playbooks/setup-cluster.yml -e '{",
		"ansible-playbook -i inventory.yml playbooks/update-cluster.yml",
		"/home/ops/site-a/files/aipub/webhook-token-auth.yaml"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if strings.Index(script, "setup-cert.yml") > strings.Index(script, "update-cluster.yml") {
		t.Error("update-cluster before setup-cert")
	}
}

// The JWKS preset names the issuer of AIPub and carries the CA of setup-cert:
// aipub-installer runs k8s-jwks inside setup-aipub, after the backend. The
// cluster is built without its entries (K3 -e), the webhook step leaves them out
// too (A2.5 -e), and after setup-aipub the CA is copied in (A5.1) and
// update-cluster.yml applies them (A5.2).
func TestJWKSAppliedAfterAipub(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	s := aipubState()
	s.ControlPlane.Audit = true
	steps := Build(s, o)
	if got, want := ids(steps), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("JWKS: %s, want %s", got, want)
	}
	byID := map[string]Step{}
	for _, st := range steps {
		byID[st.ID] = st
	}
	k3 := byID["K3"]
	if len(k3.Args) != 2 || !strings.Contains(k3.Args[1], `"name":"audit-policy-file"`) ||
		strings.Contains(k3.Args[1], "authentication-config") || !strings.Contains(k3.Note, "JWKS 는 A5.2") {
		t.Errorf("K3 does not leave JWKS out: %+v", k3)
	}
	fill, apply := byID["A5.1"], byID["A5.2"]
	if want := "sudo /usr/local/bin/k8s-installer-tui --fill-auth-config --inventory /opt/k8s-installer/ansible/inventory.yml " +
		"--vars /opt/k8s-installer/ansible/group_vars/all/vars.yml --aipub-vars /opt/k8s-installer/ansible/files/aipub/vars.yml " +
		"--installer-root /opt/k8s-installer"; fill.Kind != Shell || len(fill.Shell) != 1 || fill.Shell[0] != want {
		t.Errorf("A5.1: %+v\nwant %s", fill, want)
	}
	if apply.Playbook != "playbooks/update-cluster.yml" || len(apply.Args) != 0 || !strings.Contains(apply.Note, "한 대씩") {
		t.Errorf("A5.2: %+v", apply)
	}

	// with the legacy webhook as well: A2.5 applies the webhook and still leaves JWKS out
	s.ControlPlane.Webhook = true
	s.ControlPlane.WebhookURL = "https://aipub.example.com/token-review"
	steps = Build(s, o)
	if got, want := ids(steps), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("JWKS and webhook: %s, want %s", got, want)
	}
	for _, st := range steps {
		byID[st.ID] = st
	}
	if k3 := byID["K3"].Args[1]; strings.Contains(k3, "webhook") || strings.Contains(k3, "authentication-config") {
		t.Errorf("K3 keeps an auth preset: %s", k3)
	}
	a25 := byID["A2.5"]
	if len(a25.Args) != 2 || !strings.Contains(a25.Args[1], "authentication-token-webhook-config-file") ||
		strings.Contains(a25.Args[1], "authentication-config\"") || !strings.Contains(a25.Note, "A5.2") {
		t.Errorf("A2.5 does not apply the webhook without JWKS: %+v", a25)
	}

	o.SiteDir = "/home/ops/site-a"
	s.ControlPlane.JWKSCAFile = "/srv/pki/aipub ca.crt"
	steps = Build(s, o)
	script := Script(steps, o, "/home/ops/site-a")
	order := []string{"setup-cluster.yml", "setup-cert.yml", "echo '── A2.5", "setup-aipub.yml",
		"echo '── A5.1", "--fill-auth-config --inventory /home/ops/site-a/inventory.yml", "echo '── A5.2"}
	last := -1
	for _, want := range order {
		i := strings.Index(script, want)
		if i < 0 || i < last {
			t.Errorf("%q missing or out of order in:\n%s", want, script)
		}
		last = i
	}
	for _, want := range []string{"/home/ops/site-a/files/aipub/authentication-config.yaml",
		"--installer-root /opt/k8s-installer --jwks-ca '/srv/pki/aipub ca.crt'"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	for _, gone := range []string{"python3", "-m fetch", "REPLACE_ME"} {
		if strings.Contains(script, gone) {
			t.Errorf("script still has %q:\n%s", gone, script)
		}
	}

	// the CA no longer comes from a control plane node: nothing skipped without one
	s.K8sCpNodes = nil
	if got := ids(Build(s, o)); !strings.HasSuffix(got, "A5 A5.1 A5.2") {
		t.Errorf("without a control plane node: %s", got)
	}

	// without AIPub there is no issuer: nothing of JWKS
	s = state.DefaultState()
	if got := ids(Build(s, o)); strings.Contains(got, "A5") {
		t.Errorf("without AIPub: %s", got)
	}
}

// A site that existed before: P3 offered, and once AIPub is there with the CA of
// JWKS in its config, one update-cluster.yml applies the control plane, no -e,
// no A5.1. A new site keeps its sequence.
func TestExistingSitePlan(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	s := aipubState()
	s.ControlPlane.Audit, s.ControlPlane.Webhook = true, true
	s.ControlPlane.WebhookURL = "https://aipub.example.com/token-review"

	if got, want := ids(Build(s, o)), "P1 P2 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("new site: %s, want %s", got, want)
	}

	o.Existing, o.AipubInstalled, o.JWKSCAReady = true, true, true
	steps := Build(s, o)
	if got, want := ids(steps), "P1 P2 P3 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5"; got != want {
		t.Errorf("existing site with the CA: %s, want %s", got, want)
	}
	for _, st := range steps {
		if len(st.Args) != 0 && st.ID != "K4" {
			t.Errorf("%s given %v on an existing site", st.ID, st.Args)
		}
		if st.ID == "A2.5" && (st.Playbook != "playbooks/update-cluster.yml" || !strings.Contains(st.Title, "컨트롤 플레인 항목")) {
			t.Errorf("A2.5: %+v", st)
		}
	}

	// the CA not there: as a new site, the CA filled after setup-aipub
	o.JWKSCAReady = false
	if got, want := ids(Build(s, o)), "P1 P2 P3 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("existing site without the CA: %s, want %s", got, want)
	}
	// JWKS off: nothing waits for a CA
	s.ControlPlane.JWKS = false
	if got, want := ids(Build(s, o)), "P1 P2 P3 P5 P6 P7 K1 K2 K3 K4 A1 A2 A2.5 A3 A4 A5"; got != want {
		t.Errorf("existing site without JWKS: %s, want %s", got, want)
	}
	// AIPub not installed yet on the existing cluster: as a new site
	s.ControlPlane.JWKS, o.AipubInstalled, o.JWKSCAReady = true, false, true
	if got := ids(Build(s, o)); !strings.HasSuffix(got, "A5 A5.1 A5.2") {
		t.Errorf("existing cluster, AIPub new: %s", got)
	}
	// without AIPub: the control plane applied by itself
	s = state.DefaultState()
	s.ControlPlane.Audit = true
	if got, want := ids(Build(s, o)), "P1 P2 P3 P6 K1 K2 K3 K4 K5"; got != want {
		t.Errorf("existing site without AIPub: %s, want %s", got, want)
	}
}

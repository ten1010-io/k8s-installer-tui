package plan

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
	if got, want := ids(Build(aipubState(), o)), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("with AIPub: %s, want %s", got, want)
	}
	if got, want := ids(Build(state.DefaultState(), o)), "P1 P2 P6 K1 K2 K3 K4"; got != want {
		t.Errorf("without AIPub: %s, want %s", got, want)
	}
	o.ArchivePath, o.Monitoring = "", false
	if got, want := ids(Build(aipubState(), o)), "P1 P2 P5 P6 P7 P8 K1 K2 K3 (K4) A1 A2 M1 A3 (A4) A5 A5.1 A5.2"; got != want {
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
	// the audit policy goes along, where k8s-installer reads k8s_cp_extra_files
	if want := "[ -f /home/ops/site-a/files/aipub/audit-policy.yaml ] && sudo install -D -m 0644 " +
		"/home/ops/site-a/files/aipub/audit-policy.yaml /opt/k8s-installer/ansible/files/aipub/audit-policy.yaml || true"; !strings.Contains(elsewhere, want) {
		t.Errorf("no copy of the audit policy:\n%s", elsewhere)
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
	if got, want := ids(Build(s, o)), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A3 A4 A5"; got != want {
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
	if got, want := ids(steps), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A2.5 A3 A4 A5"; got != want {
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
	if got, want := ids(steps), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A3 A4 A5 A5.1 A5.2"; got != want {
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
	// the tests run in Korean (TestMain), which A5.1 is told: sudo drops the
	// environment
	fillCmd := "sudo /usr/local/bin/k8s-installer-tui --fill-auth-config --inventory /opt/k8s-installer/ansible/inventory.yml " +
		"--vars /opt/k8s-installer/ansible/group_vars/all/vars.yml --aipub-vars /opt/k8s-installer/ansible/files/aipub/vars.yml " +
		"--installer-root /opt/k8s-installer"
	if want := fillCmd + " --lang ko"; fill.Kind != Shell || len(fill.Shell) != 1 || fill.Shell[0] != want {
		t.Errorf("A5.1: %+v\nwant %s", fill, want)
	}
	i18n.Set(i18n.EN)
	for _, st := range Build(s, o) {
		if st.ID == "A5.1" && st.Shell[0] != fillCmd+" --lang en" {
			t.Errorf("A5.1 in English: %s\nwant %s --lang en", st.Shell[0], fillCmd)
		}
	}
	i18n.Set(i18n.KO)
	if apply.Playbook != "playbooks/update-cluster.yml" || len(apply.Args) != 0 || !strings.Contains(apply.Note, "한 대씩") {
		t.Errorf("A5.2: %+v", apply)
	}

	// with the legacy webhook as well: A2.5 applies the webhook and still leaves JWKS out
	s.ControlPlane.Webhook = true
	s.ControlPlane.WebhookURL = "https://aipub.example.com/token-review"
	steps = Build(s, o)
	if got, want := ids(steps), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
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

	if got, want := ids(Build(s, o)), "P1 P2 P5 P6 P7 P8 K1 K2 K3 K4 A1 A2 M1 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("new site: %s, want %s", got, want)
	}

	o.Existing, o.AipubInstalled, o.JWKSCAReady = true, true, true
	steps := Build(s, o)
	if got, want := ids(steps), "P1 P2 P3 P5 P6 P7 P8 (K1) (K2) (K3) K4 A1 A2 M1 A2.5 A3 A4 A5"; got != want {
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
	if got, want := ids(Build(s, o)), "P1 P2 P3 P5 P6 P7 P8 (K1) (K2) K3 K4 A1 A2 M1 A2.5 A3 A4 A5 A5.1 A5.2"; got != want {
		t.Errorf("existing site without the CA: %s, want %s", got, want)
	}
	// JWKS off: nothing waits for a CA
	s.ControlPlane.JWKS = false
	if got, want := ids(Build(s, o)), "P1 P2 P3 P5 P6 P7 P8 (K1) (K2) (K3) K4 A1 A2 M1 A2.5 A3 A4 A5"; got != want {
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
	if got, want := ids(Build(s, o)), "P1 P2 P3 P6 (K1) (K2) (K3) K4 K5"; got != want {
		t.Errorf("existing site without AIPub: %s, want %s", got, want)
	}
}

// An existing cluster never gets setup-cluster (init-k8s-cluster.sh refuses a
// node whose kubelet is enabled): K1 and K2 are kept and skipped, and K3 is the
// update-cluster.yml of the cluster stage, or skipped when a later step applies
// everything at once.
func TestExistingClusterSteps(t *testing.T) {
	byID := func(steps []Step) map[string]Step {
		m := map[string]Step{}
		for _, st := range steps {
			m[st.ID] = st
		}
		return m
	}
	o := testOptions()
	s := aipubState()
	s.ControlPlane.Audit = true

	// a new site: setup-cluster, with the cluster stage
	k := byID(Build(s, o))
	if k["K1"].Skip || k["K2"].Skip || k["K3"].Skip || k["K3"].Playbook != "playbooks/setup-cluster.yml" {
		t.Errorf("new site: %+v %+v %+v", k["K1"], k["K2"], k["K3"])
	}

	// an existing cluster on which AIPub is installed now: staged, K3 applies
	// the audit log before the AIPub steps and leaves JWKS out
	o.Existing = true
	k = byID(Build(s, o))
	for _, id := range []string{"K1", "K2"} {
		if !k[id].Skip || !strings.Contains(k[id].SkipWhy, "upgrade-k8s-installer.yml") {
			t.Errorf("%s on an existing cluster: %+v", id, k[id])
		}
	}
	k3 := k["K3"]
	if k3.Skip || k3.Playbook != "playbooks/update-cluster.yml" || !strings.Contains(k3.Title, "update-cluster") ||
		len(k3.Args) != 2 || !strings.Contains(k3.Args[1], `"name":"audit-policy-file"`) ||
		strings.Contains(k3.Args[1], "authentication-config") || !strings.Contains(k3.Note, "JWKS 는 A5.2") {
		t.Errorf("K3 of an existing cluster, AIPub new: %+v", k3)
	}
	script := Script(Build(s, o), o, "/opt/k8s-installer/ansible")
	if strings.Contains(script, "setup-cluster.yml") || !strings.Contains(script, "# K1 setup-k8s-installer: 건너뜀 (클러스터가 이미 있음") {
		t.Errorf("script of an existing cluster:\n%s", script)
	}
	// nothing at the cluster stage: K3 has nothing to do
	s.ControlPlane.Audit = false
	if k3 := byID(Build(s, o))["K3"]; !k3.Skip || !strings.Contains(k3.SkipWhy, "적용할 컨트롤 플레인 항목 없음") {
		t.Errorf("K3 with only JWKS on: %+v", k3)
	}
	s.ControlPlane.Audit = true

	// AIPub there with the CA: A2.5 applies everything, K3 skipped
	o.AipubInstalled, o.JWKSCAReady = true, true
	k = byID(Build(s, o))
	if !k["K3"].Skip || !strings.Contains(k["K3"].SkipWhy, "A2.5") || k["A2.5"].Playbook != "playbooks/update-cluster.yml" {
		t.Errorf("direct: K3 %+v, A2.5 %+v", k["K3"], k["A2.5"])
	}

	// without AIPub: K5 applies everything, K3 skipped; nothing set, no K5 and
	// K3 has nothing to do
	d := state.DefaultState()
	d.ControlPlane.Audit = true
	k = byID(Build(d, o))
	if !k["K3"].Skip || !strings.Contains(k["K3"].SkipWhy, "K5") || k["K5"].Playbook != "playbooks/update-cluster.yml" {
		t.Errorf("without AIPub: K3 %+v, K5 %+v", k["K3"], k["K5"])
	}
	d.ControlPlane.Audit = false
	if got, want := ids(Build(d, o)), "P1 P2 P3 P6 (K1) (K2) (K3) (K4)"; got != want {
		t.Errorf("existing cluster, nothing to apply: %s, want %s", got, want)
	}
}

// Every playbook of a staged plan carries the lists applied by then: the
// cluster stage up to A2.5, the stage A2.5 applied up to A5.2, nothing after.
// Not staged (AIPub off, nothing waits, the direct site): no -e at all.
func TestStageArgsFollowThePlan(t *testing.T) {
	o := testOptions()
	o.ArchivePath = "/media/usb/a.tgz"
	s := aipubState()
	s.ControlPlane.Audit, s.ControlPlane.Webhook = true, true
	s.ControlPlane.WebhookURL = "https://aipub.example.com/token-review"
	cluster, cert := stageArgs(s.AppliedControlPlane(), state.StageCluster)[1], stageArgs(s.AppliedControlPlane(), state.StageAfterCert)[1]
	want := map[string]string{"K1": cluster, "K2": cluster, "K3": cluster, "K4": cluster, "A1": cluster, "A2": cluster,
		"A2.5": cert, "A3": cert, "A4": cert, "A5": cert, "A5.2": ""}
	for _, st := range Build(s, o) {
		w, ok := want[st.ID]
		if !ok {
			continue
		}
		var got string
		for i := 0; i+1 < len(st.Args); i++ {
			if st.Args[i] == "-e" && strings.HasPrefix(st.Args[i+1], "{") {
				got = st.Args[i+1]
			}
		}
		if got != w {
			t.Errorf("%s: -e %s, want %s", st.ID, got, w)
		}
	}
	// the script runs what the runner runs (Command), the shell of P6 beside it
	o.ShellExecutable = "/bin/dash"
	script := Script(Build(s, o), o, "/opt/k8s-installer/ansible")
	if want := "playbooks/setup-control-node.yml -e '" + cluster + "' -e ansible_shell_executable=/bin/dash"; !strings.Contains(script, want) {
		t.Errorf("script lacks %q:\n%s", want, script)
	}
	if want := "push-user-registry-artifacts.yml -e archive=/media/usb/a.tgz -e '" + cluster + "'"; !strings.Contains(script, want) {
		t.Errorf("script lacks %q", want)
	}

	for name, s := range map[string]*state.AppState{"AIPub off": func() *state.AppState {
		d := state.DefaultState()
		d.ControlPlane.Audit, d.ControlPlane.Webhook = true, true
		return d
	}(), "nothing waits": func() *state.AppState {
		a := aipubState()
		a.ControlPlane.JWKS, a.ControlPlane.Audit = false, true
		return a
	}()} {
		for _, st := range Build(s, testOptions()) {
			if len(st.Args) != 0 {
				t.Errorf("%s: %s given %v", name, st.ID, st.Args)
			}
		}
	}
}

func TestDetectSite(t *testing.T) {
	dir := t.TempDir()
	old := ClusterRecord
	ClusterRecord = dir + "/applied-release"
	t.Cleanup(func() { ClusterRecord = old })

	if s := DetectSite(false, false); s.Existing || s.AipubInstalled || !strings.HasPrefix(s.Why, "새 사이트: "+ClusterRecord+" 없음") ||
		!strings.Contains(s.Why, "--existing") {
		t.Errorf("no record: %+v", s)
	}
	if s := DetectSite(true, false); !s.Existing || !strings.HasPrefix(s.Why, "기존 사이트: --existing 으로 지정") ||
		!strings.Contains(s.Why, "--aipub-installed") {
		t.Errorf("flag: %+v", s)
	}
	if err := os.WriteFile(ClusterRecord, []byte("1.36.0"), 0644); err != nil {
		t.Fatal(err)
	}
	if s := DetectSite(false, false); !s.Existing || s.AipubInstalled || !strings.HasPrefix(s.Why, "기존 사이트: "+ClusterRecord+" 있음") {
		t.Errorf("record: %+v", s)
	}
	if s := DetectSite(false, true); !s.Existing || !s.AipubInstalled || !strings.Contains(s.Why, "AIPub: --aipub-installed 로 지정") {
		t.Errorf("record and --aipub-installed: %+v", s)
	}
	// a record that cannot be looked at: not guessed existing
	ClusterRecord = dir + "/applied-release/under-a-file"
	if s := DetectSite(false, false); s.Existing || !strings.Contains(s.Why, "확인하지 못함") || !strings.Contains(s.Why, "--existing") {
		t.Errorf("record not readable: %+v", s)
	}
}

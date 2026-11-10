package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
)

// legacyNode answers the commands the checks run on an existing site: sudo for
// P2, and ansible slurp of the files of the first control plane node, whose
// apiserver was given the audit log, the token webhook and JWKS by hand.
func legacyNode(calls *[]string) plan.RunFunc {
	manifest := `apiVersion: v1
kind: Pod
metadata: {name: kube-apiserver, namespace: kube-system}
spec:
  containers:
  - name: kube-apiserver
    command:
    - kube-apiserver
    - --advertise-address=192.168.0.11
    - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
    - --audit-log-path=/var/log/aipub/usage/audit.log
    - --authentication-token-webhook-config-file=/etc/kubernetes/webhook-token-auth.yaml
    - --authentication-config=/etc/kubernetes/authentication-config.yaml
    volumeMounts:
    - {name: audit-policy, mountPath: /etc/kubernetes/audit-policy.yaml, readOnly: true}
    - {name: audit-log, mountPath: /var/log/aipub/usage}
    - {name: webhook-token-auth, mountPath: /etc/kubernetes/webhook-token-auth.yaml, readOnly: true}
    - {name: authentication-config, mountPath: /etc/kubernetes/authentication-config.yaml, readOnly: true}
`
	files := map[string]string{
		plan.APIServerManifest: manifest,
		"/etc/kubernetes/webhook-token-auth.yaml": "apiVersion: v1\nkind: Config\nclusters:\n- name: aipub\n  cluster:\n" +
			"    server: https://aipub.lkjh.local/api/v1/token-review\n    certificate-authority: /etc/k8s-installer/certs/ca.crt\n" +
			"users:\n- name: kube-apiserver\n  user: {}\n",
		"/etc/kubernetes/authentication-config.yaml": "apiVersion: apiserver.config.k8s.io/v1\nkind: AuthenticationConfiguration\n" +
			"jwt:\n  - issuer:\n      url: https://login.lkjh.local\n",
	}
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		script := args[len(args)-1]
		*calls = append(*calls, script)
		if strings.Contains(script, "sudo -n true") {
			return []byte("node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \nnode3 | CHANGED | rc=0 | (stdout) \n"), nil
		}
		for path, content := range files {
			if strings.Contains(script, "src="+path) {
				return []byte(fmt.Sprintf(`node1 | SUCCESS => {"changed": false,"content": "%s","encoding": "base64","source": "%s"}`+"\n",
					base64.StdEncoding.EncodeToString([]byte(content)), path)), nil
			}
		}
		return []byte(`node1 | FAILED! => {"msg": "file not found"}` + "\n"), fmt.Errorf("exit status 2")
	}
}

// An old site: AIPub installed, the apiserver flags only in the manifests edited
// by hand, none in vars.yml. c on the control plane screen reads them, turns the
// presets on and fills the webhook URL and the issuer; saved with the CA on the
// control node, the config is written at once and the plan applies everything
// in one update-cluster.yml.
func TestWizardLegacySite(t *testing.T) {
	d0, paths := newDriver(t)
	old := d0.app.appState
	old.AipubEnabled = true
	old.AipubIngressZone = "lkjh.local"
	old.AipubHaMode = false
	old.AipubHarborLocalPVNode = "node1"
	old.AipubCpNodes = []string{"node1", "node2"}
	old.K8sIngressVIP = "192.168.0.100"
	old.ControlPlane = state.ControlPlane{} // nothing in vars.yml: the flags were added by hand
	if err := fileio.WriteAll(paths, old); err != nil {
		t.Fatal(err)
	}
	// started again on that site, as main does
	ca := newTestCA(t)
	withCA(t, ca)
	s := state.DefaultState()
	if err := fileio.LoadAll(paths, s); err != nil {
		t.Fatal(err)
	}
	if s.ControlPlane.JWKS || s.ControlPlane.Audit || s.ControlPlane.Webhook {
		t.Fatalf("presets on before the check: %+v", s.ControlPlane)
	}
	var calls []string
	d := &driver{t: t, app: NewApp(s, paths, plan.DefaultOptions(), "test")}
	d.app.screenList[4].(*screens.S4bControlPlane).SetRunner(legacyNode(&calls))
	d.app.screenList[10].(*screens.S9Install).SetRunner(legacyNode(&calls))
	d.send(tea.WindowSizeMsg{Width: 110, Height: 80})

	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("4b.컨트롤플레인")
	if !strings.Contains(d.screen(), "c 로 첫 컨트롤 플레인 노드의 kube-apiserver.yaml") {
		t.Errorf("the check is not offered on an existing site:\n%s", d.screen())
	}
	if len(calls) != 0 {
		t.Fatalf("the check ran by itself: %q", calls)
	}
	d.keys("c")
	t.Logf("S4b after c:\n%s", d.screen())
	for _, want := range []string{
		"node1 의 kube-apiserver.yaml 에 손으로 넣은 설정이 있습니다",
		"--audit-policy-file=/etc/kubernetes/audit-policy.yaml → AIPub 감사 로그",
		"--authentication-token-webhook-config-file=/etc/kubernetes/webhook-token-auth.yaml → AIPub 인증 웹훅",
		"--authentication-config=/etc/kubernetes/authentication-config.yaml → AIPub 인증 (JWKS)",
		"update-cluster 가 손으로 고친 것을 설치기가 관리하는 같은 플래그로 바꿉니다",
		"감지된 프리셋을 끄면 update-cluster 뒤 그 기능이 클러스터에서 사라집니다",
		"[x] AIPub 감사 로그  (노드에서 감지됨)",
		"[ ] runtime-config",
		"[x] AIPub 인증 (JWKS)",
		"[x] AIPub 인증 웹훅",
		"https://login.lkjh.local",
		"https://aipub.lkjh.local/api/v1/token-review",
		"✓ CA 있음",
	} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("S4b does not show %q", want)
		}
	}
	for _, c := range calls {
		if !strings.Contains(c, "-m slurp") {
			t.Errorf("the check ran something other than slurp: %s", c)
		}
	}

	// a detected preset turned off is said to take the feature away
	d.keys(" ")
	if !strings.Contains(d.screen(), "[ ] AIPub 감사 로그  ⚠ 노드에서 감지됨 — 끄면 update-cluster 뒤 이 기능이 사라짐") {
		t.Errorf("no warning on a detected preset turned off:\n%s", d.screen())
	}
	d.keys(" ")

	d.keys("ctrl+n")
	d.at("5.AIPub")
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	if strings.Contains(d.screen(), "검증 오류") || strings.Contains(d.screen(), "CA 대기") {
		t.Fatalf("preview:\n%s", d.screen())
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}
	v := readYAML(t, paths.Vars)
	got := fmt.Sprint(v["k8s_apiserver_extra_args"])
	for _, want := range []string{"audit-policy-file", "authentication-config", "authentication-token-webhook-config-file"} {
		if !strings.Contains(got, want) {
			t.Errorf("vars.yml lacks %s: %s", want, got)
		}
	}
	ac, err := os.ReadFile(paths.AuthConfig())
	if err != nil {
		t.Fatalf("authentication config not written with the CA there: %v", err)
	}
	if !strings.Contains(string(ac), "url: https://login.lkjh.local\n") ||
		!strings.Contains(string(ac), "        "+strings.Split(ca, "\n")[1]+"\n") || strings.Contains(string(ac), "REPLACE_ME") {
		t.Errorf("authentication config:\n%s", ac)
	}
	kc, err := os.ReadFile(paths.WebhookConfig())
	if err != nil || !strings.Contains(string(kc), "https://aipub.lkjh.local/api/v1/token-review") {
		t.Errorf("webhook kubeconfig: %v\n%s", err, kc)
	}

	// the plan: P3, one update-cluster for the control plane, no A5.1
	d.keys("enter")
	d.at("9.설치")
	d.keys("down", "down") // off the archive path
	d.keys("c")
	t.Logf("S9:\n%s", d.screen())
	for _, want := range []string{"P3  apiserver 수동 설정 확인", "A2.5 update-cluster (컨트롤 플레인 항목 적용)",
		"모두 켜져 있음: update-cluster 가 손으로 고친 것을"} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("9.설치 does not show %q", want)
		}
	}
	if strings.Contains(d.screen(), "A5.1") {
		t.Errorf("A5.1 planned although the config has its CA:\n%s", d.screen())
	}
}

// A new site never reads an apiserver: c does nothing on the control plane screen.
func TestWizardNewSiteNoLegacyCheck(t *testing.T) {
	d, _ := newDriver(t)
	var calls []string
	d.app.screenList[4].(*screens.S4bControlPlane).SetRunner(legacyNode(&calls))
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("4b.컨트롤플레인")
	d.keys("c")
	if len(calls) != 0 || strings.Contains(d.screen(), "kube-apiserver.yaml 에 손으로") || strings.Contains(d.screen(), "c 로 첫 컨트롤 플레인") {
		t.Errorf("checked a new site: %q\n%s", calls, d.screen())
	}
}

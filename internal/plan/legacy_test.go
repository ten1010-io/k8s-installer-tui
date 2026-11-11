package plan

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// apiserverManifest is a kube-apiserver.yaml of kubeadm with the flags given
// added to its command, the way they were added by hand on old sites.
func apiserverManifest(flags []string, mounts []string) string {
	var b strings.Builder
	b.WriteString(`apiVersion: v1
kind: Pod
metadata:
  name: kube-apiserver
  namespace: kube-system
spec:
  containers:
  - command:
    - kube-apiserver
    - --advertise-address=10.10.0.11
    - --authorization-mode=Node,RBAC
    - --etcd-servers=https://127.0.0.1:2379
`)
	for _, f := range flags {
		b.WriteString("    - " + f + "\n")
	}
	b.WriteString(`    image: registry.k8s.io/kube-apiserver:v1.30.4
    name: kube-apiserver
    volumeMounts:
    - mountPath: /etc/kubernetes/pki
      name: k8s-certs
      readOnly: true
`)
	for i, m := range mounts {
		fmt.Fprintf(&b, "    - mountPath: %s\n      name: extra-%d\n      readOnly: true\n", m, i)
	}
	b.WriteString("  hostNetwork: true\n")
	return b.String()
}

var allLegacyFlags = []string{
	"--audit-policy-file=/etc/kubernetes/audit-policy.yaml",
	"--audit-log-path=/var/log/aipub/usage/audit.log",
	"--audit-log-maxage=7",
	"--runtime-config=api/all=true,admissionregistration.k8s.io/v1beta1=true",
	"--authentication-token-webhook-config-file=/etc/kubernetes/webhook-token-auth.yaml",
	"--authentication-token-webhook-cache-ttl=2m",
	"--authentication-config=/etc/kubernetes/authentication-config.yaml",
}

var allLegacyMounts = []string{"/etc/kubernetes/audit-policy.yaml", "/var/log/aipub/usage",
	"/etc/kubernetes/webhook-token-auth.yaml", "/etc/kubernetes/authentication-config.yaml"}

func TestDetectLegacy(t *testing.T) {
	// every flag
	l, err := DetectLegacy([]byte(apiserverManifest(allLegacyFlags, allLegacyMounts)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{state.PresetAudit, state.PresetRuntimeConfig, state.PresetWebhook, state.PresetJWKS} {
		if !l.Presets[id] {
			t.Errorf("%s not detected: %+v", id, l)
		}
	}
	if len(l.Found) != 4 || !strings.Contains(l.Found[0], "--audit-policy-file=/etc/kubernetes/audit-policy.yaml → AIPub 감사 로그") ||
		strings.Contains(strings.Join(l.Found, "\n"), "마운트 없음") {
		t.Errorf("found: %q", l.Found)
	}
	if len(l.Differs) != 1 || l.Differs[0] != "--audit-log-maxage=7 (프리셋은 30)" {
		t.Errorf("differs: %q", l.Differs)
	}

	// none: a kubeadm manifest as it comes
	l, err = DetectLegacy([]byte(apiserverManifest(nil, nil)))
	if err != nil || l.Any() || len(l.Found) != 0 {
		t.Errorf("nothing added: %+v %v", l, err)
	}

	// each flag alone, and only that preset
	for _, c := range []struct {
		flag, preset string
	}{
		{"--audit-policy-file=/etc/kubernetes/audit-policy.yaml", state.PresetAudit},
		{"--runtime-config=admissionregistration.k8s.io/v1beta1=true", state.PresetRuntimeConfig},
		{"--authentication-token-webhook-config-file=/etc/kubernetes/webhook-token-auth.yaml", state.PresetWebhook},
		{"--authentication-config=/etc/kubernetes/authentication-config.yaml", state.PresetJWKS},
	} {
		l, err := DetectLegacy([]byte(apiserverManifest([]string{c.flag}, nil)))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range state.Presets {
			if l.Presets[p.ID] != (p.ID == c.preset) {
				t.Errorf("%s: preset %s detected %v", c.flag, p.ID, l.Presets[p.ID])
			}
		}
		// a file flag without its mount is said
		if strings.Contains(c.flag, "=/") && !strings.Contains(l.Found[0], "마운트 없음") {
			t.Errorf("%s without a mount: %q", c.flag, l.Found)
		}
	}

	// another runtime-config is not the preset
	l, _ = DetectLegacy([]byte(apiserverManifest([]string{"--runtime-config=api/all=true"}, nil)))
	if l.Any() {
		t.Errorf("another runtime-config: %+v", l)
	}

	if _, err := DetectLegacy([]byte("spec: [")); err == nil {
		t.Error("not YAML taken")
	}
}

func TestLegacyFilesRefuseSecrets(t *testing.T) {
	kc := `apiVersion: v1
kind: Config
clusters:
- name: aipub-token-review
  cluster:
    server: https://aipub.example.com/api/v1/token-review
    certificate-authority: /etc/k8s-installer/certs/ca.crt
users:
- name: kube-apiserver
  user: {}
contexts:
- name: aipub-token-review
  context: {cluster: aipub-token-review, user: kube-apiserver}
current-context: aipub-token-review
`
	if u, why := WebhookURLFrom([]byte(kc)); u != "https://aipub.example.com/api/v1/token-review" || why != "" {
		t.Errorf("plain kubeconfig: %q %q", u, why)
	}
	withToken := strings.Replace(kc, "user: {}", "user:\n    token: abc.def", 1)
	if u, why := WebhookURLFrom([]byte(withToken)); u != "" || why == "" {
		t.Errorf("a token copied: %q", u)
	}
	withKey := strings.Replace(kc, "user: {}", "user:\n    client-key-data: LS0t", 1)
	if u, _ := WebhookURLFrom([]byte(withKey)); u != "" {
		t.Errorf("a key copied: %q", u)
	}
	inURL := strings.Replace(kc, "https://aipub", "https://admin:pw@aipub", 1)
	if u, _ := WebhookURLFrom([]byte(inURL)); u != "" {
		t.Errorf("a credential in the URL copied: %q", u)
	}

	ac := "apiVersion: apiserver.config.k8s.io/v1\nkind: AuthenticationConfiguration\njwt:\n  - issuer:\n" +
		"      url: https://aipub.univ.ac.kr\n      certificateAuthority: |\n        -----BEGIN CERTIFICATE-----\n        AA\n        -----END CERTIFICATE-----\n"
	if u, why := IssuerFrom([]byte(ac)); u != "https://aipub.univ.ac.kr" || why != "" {
		t.Errorf("issuer: %q %q", u, why)
	}
	if u, _ := IssuerFrom([]byte(ac + "# -----BEGIN PRIVATE KEY-----\n")); u != "" {
		t.Errorf("issuer next to a key copied: %q", u)
	}
}

// fakeSlurp answers ansible -m slurp for the files given, by path, as ansible -o
// prints it; a file not given is not there.
func fakeSlurp(node string, files map[string]string, calls *[]string) RunFunc {
	return func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		script := args[len(args)-1]
		*calls = append(*calls, script)
		for path, content := range files {
			if strings.Contains(script, "src="+path) {
				return []byte(fmt.Sprintf(`%s | SUCCESS => {"changed": false,"content": "%s","encoding": "base64","source": "%s"}`+"\n",
					node, base64.StdEncoding.EncodeToString([]byte(content)), path)), nil
			}
		}
		return []byte(node + ` | FAILED! => {"changed": false,"msg": "file not found"}` + "\n"), fmt.Errorf("exit status 2")
	}
}

func TestReadLegacy(t *testing.T) {
	s := aipubState()
	var calls []string
	run := fakeSlurp("node1", map[string]string{
		APIServerManifest: apiserverManifest(allLegacyFlags, allLegacyMounts),
		"/etc/kubernetes/webhook-token-auth.yaml": "clusters:\n- cluster:\n    server: https://aipub.univ.ac.kr/api/v1/token-review\n" +
			"users:\n- name: kube-apiserver\n  user:\n    token: secret-value\n",
		"/etc/kubernetes/authentication-config.yaml": "jwt:\n  - issuer:\n      url: https://aipub.univ.ac.kr\n",
		"/etc/kubernetes/audit-policy.yaml":          oldAuditPolicy,
	}, &calls)
	o := testOptions()
	o.AuditPolicy = filepath.Join(t.TempDir(), "files", "aipub", "audit-policy.yaml")
	l, err := ReadLegacy(context.Background(), o, s, run)
	if err != nil {
		t.Fatal(err)
	}
	if l.Node != "node1" || !l.Presets[state.PresetWebhook] || !l.Presets[state.PresetJWKS] {
		t.Errorf("%+v", l)
	}
	// the kubeconfig holds a token: its URL is not copied either
	if l.WebhookURL != "" || len(l.Refused) != 1 || !strings.Contains(l.Refused[0], "자격 증명") {
		t.Errorf("webhook URL %q, refused %q", l.WebhookURL, l.Refused)
	}
	if l.Issuer != "https://aipub.univ.ac.kr" {
		t.Errorf("issuer %q", l.Issuer)
	}
	// read only: slurp as root, on the first control plane node, nothing else
	for _, c := range calls {
		if !strings.Contains(c, "ansible node1 -i inventory.yml -b -o -m slurp -a src=/etc/kubernetes/") {
			t.Errorf("not a slurp of the node: %s", c)
		}
	}
	if len(calls) != 4 {
		t.Errorf("calls: %q", calls)
	}
	// the node's audit policy, taken as it is where none was
	if got, err := os.ReadFile(o.AuditPolicy); err != nil || string(got) != oldAuditPolicy || !l.Audit.Taken {
		t.Errorf("policy not taken: %v %+v\n%s", err, l.Audit, got)
	}

	// the saved presets against it: the webhook off is a feature taken away
	s.ControlPlane.Audit, s.ControlPlane.RuntimeConfig, s.ControlPlane.Webhook = true, true, false
	r := LegacyResult(l, s)
	if r.OK || !strings.Contains(strings.Join(r.Lines, "\n"), "✗ AIPub 인증 웹훅 (레거시 — JWKS 로 옮기기 전 사이트) 이(가) 꺼져 있음") {
		t.Errorf("webhook off: %+v", r)
	}
	s.ControlPlane.Webhook = true
	if r := LegacyResult(l, s); !r.OK || !strings.Contains(strings.Join(r.Lines, "\n"), "감사 정책: 노드의 감사 정책을 가져와 그대로 씁니다") {
		t.Errorf("all on: %+v", r)
	}

	// the manifest can not be read
	none := fakeSlurp("node1", nil, &calls)
	if r := CheckLegacy(context.Background(), testOptions(), s, none); r.OK || !strings.Contains(r.Lines[0], "file not found") {
		t.Errorf("unreadable: %+v", r)
	}
}

// oldAuditPolicy is the policy sites installed by aipub-installer 4_8_x/5_0_x run
// with: older than the default the wizard writes.
const oldAuditPolicy = `apiVersion: audit.k8s.io/v1
kind: Policy
omitStages:
  - "RequestReceived"
  - "ResponseStarted"
  - "Panic"
omitManagedFields: true
rules:
  - level: RequestResponse
    verbs: ["create", "patch", "update", "delete"]
    resources:
    - group: ""
      resources: ["pods", "pods/status"]
  - level: RequestResponse
    verbs: ["create", "delete"]
    resources:
    - group: "project.aipub.ten1010.io"
      resources: ["projects"]
`

// P3 on a site whose audit policy can not be read: not OK, and nothing written
// in its place, until the operator confirms the default or puts a file there.
func TestCheckLegacyAuditPolicyUnread(t *testing.T) {
	s := aipubState()
	s.ControlPlane.Audit = true
	var calls []string
	run := fakeSlurp("node1", map[string]string{
		APIServerManifest: apiserverManifest(allLegacyFlags[:3], allLegacyMounts[:2]),
	}, &calls)
	o := testOptions()
	o.AuditPolicy = filepath.Join(t.TempDir(), "files", "aipub", "audit-policy.yaml")
	r := CheckLegacy(context.Background(), o, s, run)
	lines := strings.Join(r.Lines, "\n")
	if r.OK || !strings.Contains(lines, "✗ 노드의 감사 정책을 읽지 못했습니다") || !strings.Contains(lines, "file not found") {
		t.Errorf("unread policy passed: %+v", r)
	}
	if _, err := os.Lstat(o.AuditPolicy); err == nil {
		t.Error("a policy was written for a node whose policy was not read")
	}
	// the operator confirms the default (o on 4b)
	s.ControlPlane.AuditDefaultOK = true
	if r := CheckLegacy(context.Background(), o, s, run); !r.OK || !strings.Contains(strings.Join(r.Lines, "\n"), "기본 정책으로 바꾸기로 했습니다") {
		t.Errorf("confirmed: %+v", r)
	}
	// or puts the site's file there
	s.ControlPlane.AuditDefaultOK = false
	site := []byte("apiVersion: audit.k8s.io/v1\nkind: Policy\nrules:\n- level: Metadata\n")
	if err := os.MkdirAll(filepath.Dir(o.AuditPolicy), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.AuditPolicy, site, 0644); err != nil {
		t.Fatal(err)
	}
	if r := CheckLegacy(context.Background(), o, s, run); !r.OK || !strings.Contains(strings.Join(r.Lines, "\n"), "사이트 파일을 씁니다") {
		t.Errorf("site file: %+v", r)
	}
	if got, _ := os.ReadFile(o.AuditPolicy); !bytes.Equal(got, site) {
		t.Errorf("the site's file changed:\n%s", got)
	}
	// a policy that holds a key is not copied
	run = fakeSlurp("node1", map[string]string{
		APIServerManifest:                   apiserverManifest(allLegacyFlags[:3], allLegacyMounts[:2]),
		"/etc/kubernetes/audit-policy.yaml": oldAuditPolicy + "# -----BEGIN PRIVATE KEY-----\n",
	}, &calls)
	l, err := ReadLegacy(context.Background(), testOptions(), s, run)
	if err != nil || l.AuditPolicy != nil || !strings.Contains(l.AuditUnread, "자격 증명") {
		t.Errorf("a key copied: %v %+v", err, l)
	}
	// a policy names the resource secrets: not a credential
	if policyLooksSecret([]byte(oldAuditPolicy + "  - level: Metadata\n    resources:\n    - group: \"\"\n      resources: [\"secrets\"]\n")) {
		t.Error("the resource secrets taken for a credential")
	}
}

// On an existing site planned without --aipub-installed, AIPub's authentication
// put on the apiserver by hand (k8s-jwks, the webhook) means AIPub is
// installed: the staged K3 would take it out until A5.2, so P3 is not OK and
// says to start again with the flag. With the flag, or with neither on the
// node, P3 is as the control plane screen reads it.
func TestCheckLegacyAuthByHandNeedsAipubInstalled(t *testing.T) {
	webhookKC := "apiVersion: v1\nkind: Config\nclusters:\n- name: aipub\n  cluster:\n" +
		"    server: https://aipub.example.com/api/v1/token-review\n    certificate-authority: /etc/k8s-installer/certs/ca.crt\n" +
		"users:\n- name: kube-apiserver\n  user: {}\n"
	authConfig := "apiVersion: apiserver.config.k8s.io/v1\nkind: AuthenticationConfiguration\njwt:\n  - issuer:\n      url: https://aipub.example.com\n"
	node := func(flags []string) RunFunc {
		var calls []string
		return fakeSlurp("node1", map[string]string{
			APIServerManifest:                            apiserverManifest(flags, nil),
			"/etc/kubernetes/webhook-token-auth.yaml":    webhookKC,
			"/etc/kubernetes/authentication-config.yaml": authConfig,
		}, &calls)
	}
	s := aipubState()
	s.ControlPlane.Webhook, s.ControlPlane.WebhookURL = true, "https://aipub.example.com/api/v1/token-review"
	existing := testOptions()
	existing.Existing = true
	installed := existing
	installed.AipubInstalled, installed.JWKSCAReady = true, true

	for _, c := range []struct {
		name  string
		flags []string
	}{
		{"JWKS by hand", allLegacyFlags[6:7]},
		{"webhook by hand", allLegacyFlags[4:6]},
	} {
		r := CheckLegacy(context.Background(), existing, s, node(c.flags))
		lines := strings.Join(r.Lines, "\n")
		if r.OK || !strings.Contains(lines, "AIPub 이 설치된 사이트입니다. --aipub-installed 로 다시 시작하세요") ||
			strings.Contains(lines, "update-cluster 가 손으로 고친 것을") || !strings.Contains(lines, "K3 부터 A2.5·A5.2") {
			t.Errorf("%s, existing without --aipub-installed: %+v", c.name, r)
		}
		r = CheckLegacy(context.Background(), installed, s, node(c.flags))
		lines = strings.Join(r.Lines, "\n")
		if !r.OK || strings.Contains(lines, "--aipub-installed") || !strings.Contains(lines, "update-cluster 가 손으로 고친 것을") {
			t.Errorf("%s, with --aipub-installed: %+v", c.name, r)
		}
	}
	// neither on the node: nothing to guard
	if r := CheckLegacy(context.Background(), existing, s, node(nil)); !r.OK || strings.Contains(strings.Join(r.Lines, "\n"), "--aipub-installed") {
		t.Errorf("no auth by hand: %+v", r)
	}
}

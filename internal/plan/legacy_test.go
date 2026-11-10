package plan

import (
	"context"
	"encoding/base64"
	"fmt"
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
	}, &calls)
	l, err := ReadLegacy(context.Background(), testOptions(), s, run)
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
	if len(calls) != 3 {
		t.Errorf("calls: %q", calls)
	}

	// the saved presets against it: the webhook off is a feature taken away
	s.ControlPlane.Audit, s.ControlPlane.RuntimeConfig, s.ControlPlane.Webhook = true, true, false
	r := LegacyResult(l, s)
	if r.OK || !strings.Contains(strings.Join(r.Lines, "\n"), "✗ AIPub 인증 웹훅 (레거시 — JWKS 로 옮기기 전 사이트) 이(가) 꺼져 있음") {
		t.Errorf("webhook off: %+v", r)
	}
	s.ControlPlane.Webhook = true
	if r := LegacyResult(l, s); !r.OK {
		t.Errorf("all on: %+v", r)
	}

	// the manifest can not be read
	none := fakeSlurp("node1", nil, &calls)
	if r := CheckLegacy(context.Background(), testOptions(), s, none); r.OK || !strings.Contains(r.Lines[0], "file not found") {
		t.Errorf("unreadable: %+v", r)
	}
}

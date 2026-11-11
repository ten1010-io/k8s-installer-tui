package fileio

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// controlPlaneState is sampleState with every preset on and an entry typed in for
// each of the eight variables.
func controlPlaneState() *state.AppState {
	s := sampleState()
	cp := &s.ControlPlane
	cp.Audit, cp.RuntimeConfig, cp.JWKS, cp.Webhook = true, true, true, true
	cp.WebhookURL = "https://aipub.example.com/api/v1/token-review"
	cp.Args = map[string][]state.ExtraArg{
		"apiserver":          {{Name: "feature-gates", Value: "Foo=true"}},
		"controller_manager": {{Name: "node-cidr-mask-size", Value: "22"}},
		"scheduler":          {{Name: "v", Value: "2"}},
		"etcd":               {{Name: "quota-backend-bytes", Value: "8589934592"}},
	}
	cp.Volumes = map[string][]state.ExtraVolume{
		"apiserver":          {{Name: "oidc", HostPath: "/etc/oidc", MountPath: "/etc/oidc", ReadOnly: true}},
		"controller_manager": {{Name: "cm-extra", HostPath: "/etc/cm", MountPath: "/etc/cm", PathType: "Directory"}},
		"scheduler":          {{Name: "sched-config", HostPath: "/etc/sched", MountPath: "/etc/sched"}},
	}
	cp.Files = []state.CpExtraFile{{Src: "site/oidc-ca.crt", Dest: "/etc/oidc/ca.crt", Mode: "0644"}}
	return s
}

// The eight variables are written in the shapes k8s-installer reads, presets
// first, and read back into the same presets and entries.
func TestControlPlaneRoundTrip(t *testing.T) {
	withCA(t, "") // before setup-cert
	p := DefaultPaths(t.TempDir())
	s := controlPlaneState()
	must(t, WriteAll(p, s))

	data, err := os.ReadFile(p.Vars)
	must(t, err)
	v := decode(t, data)
	args := v["k8s_apiserver_extra_args"].([]interface{})
	if len(args) != 10 { // audit 5, runtime-config 1, jwks 1, webhook 2, typed 1
		t.Fatalf("apiserver args: %v", args)
	}
	if a := args[6].(map[string]interface{}); a["name"] != "authentication-config" || a["value"] != "/etc/kubernetes/authentication-config.yaml" {
		t.Errorf("jwks arg %v", a)
	}
	if a := args[0].(map[string]interface{}); a["name"] != "audit-policy-file" || a["value"] != "/etc/kubernetes/audit-policy.yaml" {
		t.Errorf("first arg %v", a)
	}
	if a := args[9].(map[string]interface{}); a["name"] != "feature-gates" {
		t.Errorf("typed arg not last: %v", a)
	}
	vols := v["k8s_apiserver_extra_volumes"].([]interface{})
	if len(vols) != 6 {
		t.Fatalf("apiserver volumes: %v", vols)
	}
	if vol := vols[2].(map[string]interface{}); vol["name"] != "authentication-config" || vol["hostPath"] != "/etc/kubernetes/authentication-config.yaml" ||
		vol["mountPath"] != "/etc/kubernetes/authentication-config.yaml" || vol["readOnly"] != true || vol["pathType"] != "File" {
		t.Errorf("jwks volume: %v", vol)
	}
	if vol := vols[0].(map[string]interface{}); vol["readOnly"] != true || vol["pathType"] != "File" {
		t.Errorf("audit-policy volume: %v", vol)
	}
	if vol := vols[1].(map[string]interface{}); vol["name"] != "audit-log" || vol["pathType"] != "DirectoryOrCreate" {
		t.Errorf("audit-log volume: %v", vol)
	} else if _, ok := vol["readOnly"]; ok {
		t.Errorf("readOnly written while false: %v", vol)
	}
	files := v["k8s_cp_extra_files"].([]interface{})
	if len(files) != 4 || files[2].(map[string]interface{})["src"] != "aipub/webhook-token-auth.yaml" {
		t.Errorf("cp extra files: %v", files)
	}
	if f := files[1].(map[string]interface{}); f["src"] != "aipub/authentication-config.yaml" ||
		f["dest"] != "/etc/kubernetes/authentication-config.yaml" || f["mode"] != "0600" {
		t.Errorf("jwks file: %v", f)
	}
	for _, k := range []string{"k8s_controller_manager_extra_args", "k8s_scheduler_extra_args", "k8s_etcd_extra_args",
		"k8s_controller_manager_extra_volumes", "k8s_scheduler_extra_volumes"} {
		if l, _ := v[k].([]interface{}); len(l) != 1 {
			t.Errorf("%s: %v", k, v[k])
		}
	}
	if !strings.Contains(string(data), "# Flags and host paths added to the kube-apiserver") {
		t.Error("the comment of the template above the apiserver lists is gone")
	}

	// the kubeconfig of the webhook, next to files/aipub/vars.yml
	kc, err := os.ReadFile(p.WebhookConfig())
	must(t, err)
	for _, want := range []string{`server: "https://aipub.example.com/api/v1/token-review"`,
		"certificate-authority: /etc/k8s-installer/certs/ca.crt", "current-context: aipub-token-review"} {
		if !strings.Contains(string(kc), want) {
			t.Errorf("webhook kubeconfig lacks %q:\n%s", want, kc)
		}
	}

	// the authentication config of JWKS: not written while there is no CA, never
	// with a placeholder
	if _, err := os.Stat(p.AuthConfig()); err == nil {
		t.Error("authentication config written without a CA")
	}

	back := state.DefaultState()
	must(t, LoadAll(p, back))
	if !reflect.DeepEqual(back.ControlPlane, s.ControlPlane) {
		t.Errorf("read back\n  %+v\nwant\n  %+v", back.ControlPlane, s.ControlPlane)
	}

	// with the CA of setup-cert: written, the issuer derived from the zone
	ca := newTestCA(t, "aipub-ca")
	withCA(t, ca)
	must(t, WriteAll(p, s))
	ac, err := os.ReadFile(p.AuthConfig())
	must(t, err)
	for _, want := range []string{"url: https://aipub.aipub.example.com\n", "        -----BEGIN CERTIFICATE-----\n"} {
		if !strings.Contains(string(ac), want) {
			t.Errorf("authentication config lacks %q:\n%s", want, ac)
		}
	}

	// an issuer of its own and the CA are read back
	s.ControlPlane.JWKSIssuerURL = "https://login.example.com/aipub"
	s.ControlPlane.JWKSCA = ca
	must(t, WriteAll(p, s))
	back = state.DefaultState()
	must(t, LoadAll(p, back))
	if !reflect.DeepEqual(back.ControlPlane, s.ControlPlane) {
		t.Errorf("read back with issuer and CA\n  %+v\nwant\n  %+v", back.ControlPlane, s.ControlPlane)
	}

	// Everything off: every list is [] again, nothing left over
	back.ControlPlane = state.ControlPlane{}
	must(t, WriteAll(p, back))
	data, err = os.ReadFile(p.Vars)
	must(t, err)
	v = decode(t, data)
	for _, k := range []string{"k8s_apiserver_extra_args", "k8s_apiserver_extra_volumes", "k8s_controller_manager_extra_args",
		"k8s_controller_manager_extra_volumes", "k8s_scheduler_extra_args", "k8s_scheduler_extra_volumes",
		"k8s_etcd_extra_args", "k8s_cp_extra_files"} {
		if l, ok := v[k].([]interface{}); !ok || len(l) != 0 {
			t.Errorf("%s after turning everything off: %#v", k, v[k])
		}
	}
	if strings.Contains(string(data), "/etc/kubernetes/audit-policy.yaml") {
		t.Error("an entry of a preset is left after turning it off")
	}
}

// The authentication config is the template of aipub-installer with the issuer
// and the CA filled in, as k8s-jwks fills it: the same document otherwise.
func TestAuthConfig(t *testing.T) {
	s := controlPlaneState()
	testCA := newTestCA(t, "aipub-ca")
	var tmpl, got map[string]interface{}
	must(t, yaml.Unmarshal(authConfigTemplate, &tmpl))
	out := AuthConfig(s, testCA)
	if out == nil {
		t.Fatal("no authentication config (has the template of aipub-installer changed?)")
	}
	must(t, yaml.Unmarshal(out, &got))
	issuer := tmpl["jwt"].([]interface{})[0].(map[string]interface{})["issuer"].(map[string]interface{})
	issuer["url"] = "https://aipub.aipub.example.com"
	issuer["certificateAuthority"] = testCA + "\n"
	if !reflect.DeepEqual(got, tmpl) {
		t.Errorf("not the template filled in:\n%s", out)
	}
	if !strings.HasPrefix(string(out), "# Written by k8s-installer-tui") ||
		!strings.Contains(string(out), "# - The \"oidc:\" prefix keeps the RBAC bindings") {
		t.Errorf("header or the comments of the template missing:\n%s", out)
	}

	// not written without AIPub (the issuer is AIPub), nor with the preset off
	if strings.Contains(string(out), "REPLACE_ME") {
		t.Errorf("a placeholder left:\n%s", out)
	}

	// not written without a CA, without AIPub (the issuer is AIPub), nor with the
	// preset off
	if AuthConfig(s, "") != nil {
		t.Error("written without a CA")
	}
	s.AipubEnabled = false
	if AuthConfig(s, testCA) != nil {
		t.Error("written without AIPub")
	}
	s.AipubEnabled, s.ControlPlane.JWKS = true, false
	if AuthConfig(s, testCA) != nil {
		t.Error("written with the preset off")
	}
}

// Turning one preset off takes out its entries and nothing else; a preset only
// partly in the file is read as entries typed in.
func TestControlPlanePresetOff(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	s := controlPlaneState()
	must(t, WriteAll(p, s))
	s.ControlPlane.Audit = false
	must(t, WriteAll(p, s))

	back := state.DefaultState()
	must(t, LoadAll(p, back))
	cp := back.ControlPlane
	if cp.Audit || !cp.RuntimeConfig || !cp.JWKS || !cp.Webhook {
		t.Errorf("presets read back: audit %v runtime %v jwks %v webhook %v", cp.Audit, cp.RuntimeConfig, cp.JWKS, cp.Webhook)
	}
	if len(cp.Args["apiserver"]) != 1 || len(cp.Volumes["apiserver"]) != 1 || len(cp.Files) != 1 {
		t.Errorf("typed entries changed: %+v", cp)
	}

	// JWKS off: its three entries go, the legacy webhook and the rest stay
	s.ControlPlane.JWKS = false
	must(t, WriteAll(p, s))
	data, err := os.ReadFile(p.Vars)
	must(t, err)
	if strings.Contains(string(data), "authentication-config") {
		t.Errorf("an entry of JWKS is left after turning it off:\n%s", data)
	}
	back = state.DefaultState()
	must(t, LoadAll(p, back))
	cp = back.ControlPlane
	if cp.JWKS || !cp.Webhook || !cp.RuntimeConfig {
		t.Errorf("presets read back after JWKS off: jwks %v webhook %v runtime %v", cp.JWKS, cp.Webhook, cp.RuntimeConfig)
	}
	if len(cp.Args["apiserver"]) != 1 || len(cp.Volumes["apiserver"]) != 1 || len(cp.Files) != 1 {
		t.Errorf("typed entries changed after JWKS off: %+v", cp)
	}
	v := decode(t, data)
	if n := len(v["k8s_apiserver_extra_args"].([]interface{})); n != 4 { // runtime 1, webhook 2, typed 1
		t.Errorf("apiserver args after JWKS off: %d", n)
	}

	// one entry of the audit preset is not the preset
	partial := state.ControlPlane{}
	partial.Absorb(map[string][]state.ExtraArg{"apiserver": {{Name: "audit-log-maxage", Value: "30"}}}, nil, nil)
	if partial.Audit || len(partial.Args["apiserver"]) != 1 {
		t.Errorf("a partial preset: %+v", partial)
	}
}

// The files the presets copy have to be under ansible/files; the kubeconfig of
// the webhook is not looked for, the wizard writes it, nor the audit policy while
// saving writes the default.
func TestMissingCpFiles(t *testing.T) {
	p := DefaultPaths(t.TempDir())
	installer := t.TempDir()
	s := controlPlaneState()
	var srcs []string
	for _, f := range MissingCpFiles(s, p.FilesDir(), installer) {
		srcs = append(srcs, f.Src)
	}
	if got := strings.Join(srcs, " "); got != "site/oidc-ca.crt" {
		t.Errorf("missing: %s", got)
	}
	// an existing site: the default waits, the policy is missing until it is there
	s.Existing = true
	if got := MissingCpFiles(s, p.FilesDir(), installer); len(got) != 2 || got[0].Src != state.AuditPolicyFile {
		t.Errorf("held policy: %v", got)
	}
	must(t, os.MkdirAll(installer+"/aipub", 0755))
	must(t, os.WriteFile(installer+"/aipub/audit-policy.yaml", []byte("apiVersion: audit.k8s.io/v1\n"), 0644))
	for _, f := range MissingCpFiles(s, p.FilesDir(), installer) {
		if f.Src == state.AuditPolicyFile {
			t.Errorf("the policy is reported missing once it is under the installer: %v", f)
		}
	}
	s.Existing = false
	s.ControlPlane.WebhookURL = "" // nothing to write the kubeconfig from
	if got := MissingCpFiles(s, p.FilesDir(), installer); len(got) != 2 || got[0].Src != state.WebhookConfigFile {
		t.Errorf("webhook without a URL: %v", got)
	}
	s.AipubIngressZone = "" // nothing to derive the issuer from
	if got := MissingCpFiles(s, p.FilesDir(), installer); len(got) != 3 || got[0].Src != state.JWKSConfigFile {
		t.Errorf("JWKS without an issuer: %v", got)
	}
}

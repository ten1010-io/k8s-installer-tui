package fileio

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// controlPlaneState is sampleState with every preset on and an entry typed in for
// each of the eight variables.
func controlPlaneState() *state.AppState {
	s := sampleState()
	cp := &s.ControlPlane
	cp.Audit, cp.RuntimeConfig, cp.Webhook = true, true, true
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
	p := DefaultPaths(t.TempDir())
	s := controlPlaneState()
	must(t, WriteAll(p, s))

	data, err := os.ReadFile(p.Vars)
	must(t, err)
	v := decode(t, data)
	args := v["k8s_apiserver_extra_args"].([]interface{})
	if len(args) != 9 { // audit 5, runtime-config 1, webhook 2, typed 1
		t.Fatalf("apiserver args: %v", args)
	}
	if a := args[0].(map[string]interface{}); a["name"] != "audit-policy-file" || a["value"] != "/etc/kubernetes/audit-policy.yaml" {
		t.Errorf("first arg %v", a)
	}
	if a := args[8].(map[string]interface{}); a["name"] != "feature-gates" {
		t.Errorf("typed arg not last: %v", a)
	}
	vols := v["k8s_apiserver_extra_volumes"].([]interface{})
	if len(vols) != 5 {
		t.Fatalf("apiserver volumes: %v", vols)
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
	if len(files) != 3 || files[1].(map[string]interface{})["src"] != "aipub/webhook-token-auth.yaml" {
		t.Errorf("cp extra files: %v", files)
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

	back := state.DefaultState()
	must(t, LoadAll(p, back))
	if !reflect.DeepEqual(back.ControlPlane, s.ControlPlane) {
		t.Errorf("read back\n  %+v\nwant\n  %+v", back.ControlPlane, s.ControlPlane)
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
	if cp.Audit || !cp.RuntimeConfig || !cp.Webhook {
		t.Errorf("presets read back: audit %v runtime %v webhook %v", cp.Audit, cp.RuntimeConfig, cp.Webhook)
	}
	if len(cp.Args["apiserver"]) != 1 || len(cp.Volumes["apiserver"]) != 1 || len(cp.Files) != 1 {
		t.Errorf("typed entries changed: %+v", cp)
	}

	// one entry of the audit preset is not the preset
	partial := state.ControlPlane{}
	partial.Absorb(map[string][]state.ExtraArg{"apiserver": {{Name: "audit-log-maxage", Value: "30"}}}, nil, nil)
	if partial.Audit || len(partial.Args["apiserver"]) != 1 {
		t.Errorf("a partial preset: %+v", partial)
	}
}

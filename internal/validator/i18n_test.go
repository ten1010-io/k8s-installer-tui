package validator

import (
	"testing"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func hangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// brokenMessages gathers the messages of broken values of every kind the
// validator rejects, in the current language.
func brokenMessages() []string {
	var msgs []string
	add := func(errs []Error) {
		for _, e := range errs {
			msgs = append(msgs, e.Msg)
		}
	}
	for _, t := range []schema.Type{schema.TypeInt, schema.TypePort, schema.TypeCIDR, schema.TypeIP, schema.TypeFQDN,
		schema.TypeSubdomain, schema.TypeMinor, schema.TypeDuration, schema.TypeSize, schema.TypeDays} {
		v := schema.Var{Key: "k", Type: t, Short: "SSH port (22 if empty)", Required: "ki_cp_ha_mode=true"}
		add(checkVar(v, "k", "x y", true))
		add(checkVar(v, "k", nil, true))
	}
	enum := schema.Var{Key: "e", Type: schema.TypeEnum, Enum: []string{"a", "b"}}
	add(checkVar(enum, "e", "c", false))
	list := schema.Var{Key: "l", Type: schema.TypeList, Elem: schema.TypeCIDR, Required: schema.Always, Short: "Service range"}
	add(checkVar(list, "l", []string{}, true))

	// every real variable left empty, and given a value of no format
	s := valid()
	for _, v := range schema.All() {
		add(checkVar(v, v.Key, nil, true))
	}

	add(Validate(state.DefaultState()))
	bad := valid()
	bad.Nodes[0].Name = ""
	bad.Nodes[2].Name = "node2"
	bad.KiCpNodes = []string{"nobody"}
	bad.K8sNodes = []string{"nobody"}
	bad.K8sPodSubnet = "10.0.0.0/8"
	bad.K8sServiceSubnet = "10.1.0.0/16"
	bad.KiCpHaModeVIP = "172.16.0.1"
	bad.K8sIngressMode = 3
	bad.K8sIngressLBName, bad.K8sIngressVIP = "", ""
	add(Validate(bad))
	sep := valid()
	sep.K8sIngressMode = 1
	sep.K8sIngressLBName, sep.K8sGatewayLBName = "lb", "lb"
	sep.K8sIngressVIP, sep.K8sGatewayVIP = "192.168.0.100", "x"
	add(Validate(sep))
	sep.K8sGatewayLBName, sep.K8sGatewayVIP = "", ""
	add(Validate(sep))
	sep.K8sGatewayLBName, sep.K8sGatewayVIP = "gw", "192.168.0.11"
	add(Validate(sep))
	vip := valid()
	vip.K8sIngressVIP = vip.Nodes[0].AnsibleHost
	vip.KiCpHaModeVIP = "192.168.0.100"
	vip.K8sIngressMode = 1
	vip.K8sGatewayLBName, vip.K8sGatewayVIP = "gw", vip.KiCpHaModeVIP
	add(Validate(vip))
	pod := valid()
	pod.K8sCpNodes = nil
	pod.K8sIngressVIP = "10.244.0.5"
	add(Validate(pod))
	hv := valid()
	hv.AipubHaMode = false
	hv.AipubHarborLocalPVNode = "nobody"
	hv.AipubCpNodes = nil
	hv.MonitoringCpNodes = []string{"node3"}
	hv.K8sCpNodes = []string{"node1"}
	add(Validate(hv))
	add(Monitoring(hv))

	out := valid()
	out.K8sIngressMode = 1
	out.K8sGatewayLBName, out.K8sGatewayVIP = "gw", "10.1.0.6"
	out.K8sIngressVIP = "10.1.0.5"
	msgs = append(msgs, Warnings(out)...)

	// the control plane
	add(checkExtraArgs(schema.Var{Key: "k8s_apiserver_extra_args", Decided: []string{"etcd-servers"}}, "f",
		[]state.ExtraArg{{Name: "--x", Value: "1"}, {Name: "Bad_Name", Value: "1"}, {Name: "etcd-servers", Value: "1"}}))
	add(checkExtraVolumes("f", []state.ExtraVolume{
		{Name: "Bad_Name", HostPath: "rel", MountPath: "rel", PathType: "Nope"},
		{Name: "dup", HostPath: "/a", MountPath: "/a"}, {Name: "dup", HostPath: "/a", MountPath: "/a"},
		{Name: "k8s-certs", HostPath: "/a", MountPath: "/a"}}))
	add(checkCpExtraFiles("f", []state.CpExtraFile{
		{Src: "/abs", Dest: "rel", Mode: "99"}, {Src: "../x", Dest: "/a", Mode: "0644"},
		{Src: "a", Dest: "/a", Mode: "0644"}, {Src: "b", Dest: "/a", Mode: "0644"},
		{Src: "c", Dest: "/etc/kubernetes/pki/x", Mode: "0644"}}))
	add(checkIssuerURL("", true))
	add(checkIssuerURL("http://x", true))
	add(checkWebhookURL(&state.ControlPlane{Webhook: true}))
	add(checkWebhookURL(&state.ControlPlane{Webhook: true, WebhookURL: "http://x"}))
	web := valid()
	web.AipubEnabled = false
	web.ControlPlane.Webhook = true
	add(Validate(web))
	_ = s
	return msgs
}

// The English default says no Korean in a message, and the Korean catalog
// still says the same messages in Korean.
func TestMessagesAreEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	defer i18n.Set(i18n.KO)
	en := brokenMessages()
	if len(en) < 60 {
		t.Fatalf("only %d messages gathered", len(en))
	}
	for _, m := range en {
		if hangul(m) {
			t.Errorf("Korean in English: %q", m)
		}
	}
	i18n.Set(i18n.KO)
	ko := brokenMessages()
	if len(ko) != len(en) {
		t.Fatalf("%d messages in English, %d in Korean", len(en), len(ko))
	}
	var korean int
	for i := range ko {
		if hangul(ko[i]) {
			korean++
		}
	}
	if korean < len(ko)-8 { // the ones with no Korean word: the English text of a variable
		t.Errorf("only %d of %d messages in Korean", korean, len(ko))
	}
}

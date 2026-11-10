package plan

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// A site installed before k8s-installer took the apiserver flags in vars.yml has
// them only in the static pod manifest, edited by hand (aipub-installer k8s-jwks,
// the audit policy, the token webhook). update-cluster.yml has kubeadm write that
// manifest again from the cluster configuration, so a flag the wizard does not
// declare is gone after it. The check here reads the manifest of a control plane
// node - read only, with slurp - and says which presets stand for what is there.
// The audit policy the node runs with is read too and kept under ansible/files
// where none is, so that k8s_cp_extra_files copies the node's own policy back
// rather than the wizard's default over it.

// APIServerManifest is the static pod of the apiserver on a control plane node.
const APIServerManifest = "/etc/kubernetes/manifests/kube-apiserver.yaml"

// Legacy is what the apiserver of a node runs with, as the presets see it.
type Legacy struct {
	Node string
	// The presets whose flag is there, by preset id (state.Preset*)
	Presets map[string]bool
	// Found are the lines that say what was found, a flag each, for a person
	Found []string
	// Differs are the flags of a detected preset the node sets to another value
	// than the preset: the preset's value is what update-cluster.yml applies
	Differs []string
	// WebhookURL and Issuer are read from the files the flags name, when they
	// could be read and held nothing that looks like a credential
	WebhookURL string
	Issuer     string
	// Refused says what was not copied and why
	Refused []string
	// AuditPolicy is the policy the node runs with (--audit-policy-file), when it
	// could be read and held nothing that looks like a credential; AuditUnread
	// says why it was not. Audit is what became of it at Options.AuditPolicy
	AuditPolicy []byte
	AuditUnread string
	Audit       fileio.AuditAdoption
}

// Any reports whether a preset was found.
func (l *Legacy) Any() bool {
	for _, on := range l.Presets {
		if on {
			return true
		}
	}
	return false
}

// manifest is the part of a static pod the check reads.
type manifest struct {
	Spec struct {
		Containers []struct {
			Name         string   `yaml:"name"`
			Command      []string `yaml:"command"`
			Args         []string `yaml:"args"`
			VolumeMounts []struct {
				Name      string `yaml:"name"`
				MountPath string `yaml:"mountPath"`
			} `yaml:"volumeMounts"`
		} `yaml:"containers"`
	} `yaml:"spec"`
}

// APIServerFlags are the flags of the apiserver container of a static pod
// manifest, name (no dashes) to value, and the paths it mounts.
func APIServerFlags(data []byte) (map[string]string, []string, error) {
	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("%s 를 읽지 못했습니다: %w", APIServerManifest, err)
	}
	if len(m.Spec.Containers) == 0 {
		return nil, nil, fmt.Errorf("%s 에 컨테이너가 없습니다", APIServerManifest)
	}
	c := m.Spec.Containers[0]
	for _, x := range m.Spec.Containers {
		if x.Name == "kube-apiserver" {
			c = x
			break
		}
	}
	flags := map[string]string{}
	for _, a := range append(append([]string{}, c.Command...), c.Args...) {
		if !strings.HasPrefix(a, "--") {
			continue
		}
		name, value, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		flags[name] = value
	}
	var mounts []string
	for _, v := range c.VolumeMounts {
		mounts = append(mounts, v.MountPath)
	}
	return flags, mounts, nil
}

// legacyFlag is the flag a preset is known by on a node.
var legacyFlag = map[string]string{
	state.PresetAudit:         "audit-policy-file",
	state.PresetRuntimeConfig: "runtime-config",
	state.PresetWebhook:       "authentication-token-webhook-config-file",
	state.PresetJWKS:          "authentication-config",
}

const runtimeConfigValue = "admissionregistration.k8s.io/v1beta1=true"

// DetectLegacy reads a kube-apiserver.yaml: the presets whose flag it has, and
// what of theirs it sets otherwise.
func DetectLegacy(data []byte) (*Legacy, error) {
	flags, mounts, err := APIServerFlags(data)
	if err != nil {
		return nil, err
	}
	l := &Legacy{Presets: map[string]bool{}}
	mounted := func(path string) string {
		for _, m := range mounts {
			if m == path || strings.HasPrefix(path, strings.TrimRight(m, "/")+"/") {
				return ""
			}
		}
		return " (마운트 없음)"
	}
	for _, p := range state.Presets {
		name := legacyFlag[p.ID]
		value, ok := flags[name]
		if !ok {
			continue
		}
		if p.ID == state.PresetRuntimeConfig && !contains(strings.Split(value, ","), runtimeConfigValue) {
			continue
		}
		l.Presets[p.ID] = true
		line := "--" + name + "=" + value + " → " + p.Title
		if strings.HasPrefix(value, "/") {
			line += mounted(value)
		}
		l.Found = append(l.Found, line)
		for _, a := range p.Args {
			if v, ok := flags[a.Name]; ok && v != a.Value && !(p.ID == state.PresetRuntimeConfig) {
				l.Differs = append(l.Differs, fmt.Sprintf("--%s=%s (프리셋은 %s)", a.Name, v, a.Value))
			}
		}
	}
	return l, nil
}

func contains(l []string, x string) bool {
	for _, v := range l {
		if strings.TrimSpace(v) == x {
			return true
		}
	}
	return false
}

// looksSecret reports whether a file holds something that looks like a
// credential: a key, a token, a password, client certificate data.
func looksSecret(data []byte) bool {
	t := strings.ToLower(string(data))
	for _, w := range []string{"private key", "token:", "token-data", "password", "client-key", "client-certificate-data",
		"auth-provider", "exec:", "secret"} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// policyLooksSecret reports whether an audit policy holds something that looks
// like a credential. A policy names the resource secrets, so the word alone is
// not one.
func policyLooksSecret(data []byte) bool {
	t := strings.ToLower(string(data))
	for _, w := range []string{"private key", "token:", "token-data", "password:", "client-key-data", "client-certificate-data"} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// plainURL is a URL fit to copy: https, no credential, no query.
func plainURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// WebhookURLFrom is the server of the kubeconfig of the token webhook, unless the
// file holds anything that looks like a credential: then nothing, and why.
func WebhookURLFrom(data []byte) (string, string) {
	if looksSecret(data) {
		return "", "웹훅 kubeconfig 에 자격 증명(토큰·키)으로 보이는 것이 있어 URL 을 가져오지 않았습니다. 직접 입력하세요"
	}
	var kc struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if yaml.Unmarshal(data, &kc) != nil || len(kc.Clusters) == 0 || kc.Clusters[0].Cluster.Server == "" {
		return "", "웹훅 kubeconfig 에서 server 를 찾지 못했습니다. 직접 입력하세요"
	}
	if s := kc.Clusters[0].Cluster.Server; plainURL(s) {
		return s, ""
	}
	return "", "웹훅 server 가 https URL (자격 증명·쿼리 없이) 이 아니라 가져오지 않았습니다. 직접 입력하세요"
}

// IssuerFrom is the issuer of an authentication config, unless the file holds
// anything that looks like a credential (the CA in it is public).
func IssuerFrom(data []byte) (string, string) {
	if strings.Contains(strings.ToLower(string(data)), "private key") {
		return "", "authentication-config.yaml 에 키로 보이는 것이 있어 issuer 를 가져오지 않았습니다. 직접 확인하세요"
	}
	var ac struct {
		JWT []struct {
			Issuer struct {
				URL string `yaml:"url"`
			} `yaml:"issuer"`
		} `yaml:"jwt"`
	}
	if yaml.Unmarshal(data, &ac) != nil || len(ac.JWT) == 0 || ac.JWT[0].Issuer.URL == "" {
		return "", "authentication-config.yaml 에서 issuer 를 찾지 못했습니다"
	}
	if u := ac.JWT[0].Issuer.URL; plainURL(u) {
		return u, ""
	}
	return "", "issuer 가 https URL 이 아니라 가져오지 않았습니다"
}

// slurp reads a file of node, read only (ansible slurp, as root: the manifests
// are root's). ansible -o prints one line: node | SUCCESS => {"content": ...}.
func slurp(ctx context.Context, o Options, node, path string, run RunFunc) ([]byte, error) {
	_, out, err := adhoc(ctx, o, run, node, true, "slurp", "src="+path)
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, node+" | ") {
			continue
		}
		head, body, ok := strings.Cut(l, "=> ")
		if !ok {
			return nil, errors.New(firstLine(l))
		}
		if !strings.Contains(head, "SUCCESS") {
			var r struct {
				Msg string `json:"msg"`
			}
			_ = json.Unmarshal([]byte(body), &r)
			if r.Msg == "" {
				r.Msg = strings.TrimSpace(head)
			}
			return nil, fmt.Errorf("%s: %s", path, r.Msg)
		}
		var r struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return base64.StdEncoding.DecodeString(r.Content)
	}
	msg := "ansible을 실행하지 못했습니다"
	if err != nil {
		msg += ": " + err.Error()
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		msg += " — " + firstLine(s)
	}
	return nil, errors.New(msg)
}

// ReadLegacy is the check of an existing site: the apiserver of the first k8s
// control plane node, the webhook URL and the issuer from the files its flags
// name, and its audit policy. Nothing is written on the node; the audit policy is
// written to o.AuditPolicy as it is when nothing is there (never over a file).
func ReadLegacy(ctx context.Context, o Options, s *state.AppState, run RunFunc) (*Legacy, error) {
	if len(s.K8sCpNodes) == 0 {
		return nil, errors.New("k8s 컨트롤 플레인 노드가 없습니다")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	node := s.K8sCpNodes[0]
	data, err := slurp(ctx, o, node, APIServerManifest, run)
	if err != nil {
		return nil, err
	}
	l, err := DetectLegacy(data)
	if err != nil {
		return nil, err
	}
	l.Node = node
	flags, _, _ := APIServerFlags(data)
	if l.Presets[state.PresetWebhook] {
		if b, err := slurp(ctx, o, node, flags[legacyFlag[state.PresetWebhook]], run); err != nil {
			l.Refused = append(l.Refused, "웹훅 kubeconfig 를 읽지 못했습니다: "+err.Error())
		} else if u, why := WebhookURLFrom(b); why != "" {
			l.Refused = append(l.Refused, why)
		} else {
			l.WebhookURL = u
		}
	}
	if l.Presets[state.PresetJWKS] {
		if b, err := slurp(ctx, o, node, flags[legacyFlag[state.PresetJWKS]], run); err != nil {
			l.Refused = append(l.Refused, "authentication-config.yaml 을 읽지 못했습니다: "+err.Error())
		} else if u, why := IssuerFrom(b); why != "" {
			l.Refused = append(l.Refused, why)
		} else {
			l.Issuer = u
		}
	}
	if l.Presets[state.PresetAudit] {
		path := flags[legacyFlag[state.PresetAudit]]
		b, err := slurp(ctx, o, node, path, run)
		switch {
		case err != nil:
			l.AuditUnread = err.Error()
		case len(bytes.TrimSpace(b)) == 0:
			l.AuditUnread = path + " 가 비어 있습니다"
		case policyLooksSecret(b):
			l.AuditUnread = "자격 증명(토큰·키)으로 보이는 것이 있어 가져오지 않았습니다"
		default:
			l.AuditPolicy = b
		}
		if o.AuditPolicy != "" {
			a, err := fileio.AdoptAuditPolicy(o.AuditPolicy, l.AuditPolicy)
			if err != nil {
				a.Note = "⚠ 노드의 감사 정책을 " + o.AuditPolicy + " 에 쓰지 못했습니다: " + err.Error()
			}
			l.Audit = a
		}
	}
	return l, nil
}

// CheckLegacy is P3: what ReadLegacy finds, against the presets saved. Not OK
// when a preset the node runs with is off: update-cluster.yml would take that
// feature out of the cluster.
func CheckLegacy(ctx context.Context, o Options, s *state.AppState, run RunFunc) CheckResult {
	l, err := ReadLegacy(ctx, o, s, run)
	if err != nil {
		return CheckResult{Lines: []string{err.Error()}}
	}
	return LegacyResult(l, s)
}

// LegacyResult says what a check found against the presets of s.
func LegacyResult(l *Legacy, s *state.AppState) CheckResult {
	r := CheckResult{OK: true}
	if !l.Any() {
		r.Lines = []string{l.Node + ": 손으로 넣은 AIPub apiserver 설정 없음"}
		return r
	}
	r.Lines = append(r.Lines, l.Node+" 의 "+APIServerManifest+" 에 손으로 넣은 설정:")
	for _, f := range l.Found {
		r.Lines = append(r.Lines, "  "+f)
	}
	var off []string
	for _, p := range state.Presets {
		if l.Presets[p.ID] && !s.ControlPlane.On(p.ID) {
			off = append(off, p.Title)
		}
	}
	sort.Strings(off)
	for _, t := range off {
		r.OK = false
		r.Lines = append(r.Lines, "✗ "+t+" 이(가) 꺼져 있음: update-cluster 뒤 이 기능이 클러스터에서 사라집니다. 4b 에서 켜고 다시 저장하세요")
	}
	if r.OK {
		r.Lines = append(r.Lines, "모두 켜져 있음: update-cluster 가 손으로 고친 것을 설치기가 관리하는 같은 플래그로 바꿉니다")
	}
	if !l.Presets[state.PresetAudit] || !s.ControlPlane.Audit {
		return r
	}
	// the policy k8s_cp_extra_files copies over the node's
	if l.Audit.Note != "" {
		r.Lines = append(r.Lines, "감사 정책: "+l.Audit.Note)
	}
	if l.AuditUnread != "" {
		switch {
		case l.Audit.Kept:
			r.Lines = append(r.Lines, "⚠ 노드의 감사 정책을 읽지 못했습니다 ("+l.AuditUnread+"): ansible/files 의 사이트 파일을 씁니다")
		case s.ControlPlane.AuditDefaultOK:
			r.Lines = append(r.Lines, "⚠ 노드의 감사 정책을 읽지 못했지만 기본 정책으로 바꾸기로 했습니다 (4b 의 o): update-cluster 가 노드의 정책을 바꿉니다")
		default:
			r.OK = false
			r.Lines = append(r.Lines, "✗ 노드의 감사 정책을 읽지 못했습니다 ("+l.AuditUnread+"): 기본 정책이 읽지 못한 정책을 바꾸게 됩니다. "+
				"정책 파일을 ansible/files/"+state.AuditPolicyFile+" 에 직접 두거나, 4b 감사 로그 줄에서 o 로 기본 정책 쓰기를 확인하고 다시 저장하세요")
		}
	}
	return r
}

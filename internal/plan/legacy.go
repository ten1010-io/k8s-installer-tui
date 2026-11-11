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
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
		return nil, nil, fmt.Errorf("%s: %w", i18n.T("could not read %s", APIServerManifest), err)
	}
	if len(m.Spec.Containers) == 0 {
		return nil, nil, errors.New(i18n.T("%s has no container", APIServerManifest))
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
		return " " + i18n.T("(not mounted)")
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
		line := "--" + name + "=" + value + " → " + i18n.T(p.Title)
		if strings.HasPrefix(value, "/") {
			line += mounted(value)
		}
		l.Found = append(l.Found, line)
		for _, a := range p.Args {
			if v, ok := flags[a.Name]; ok && v != a.Value && !(p.ID == state.PresetRuntimeConfig) {
				l.Differs = append(l.Differs, i18n.T("--%s=%s (the preset has %s)", a.Name, v, a.Value))
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
		return "", i18n.T("The webhook kubeconfig holds what looks like a credential (a token or key), so the URL was not taken. Enter it yourself")
	}
	var kc struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if yaml.Unmarshal(data, &kc) != nil || len(kc.Clusters) == 0 || kc.Clusters[0].Cluster.Server == "" {
		return "", i18n.T("No server found in the webhook kubeconfig. Enter it yourself")
	}
	if s := kc.Clusters[0].Cluster.Server; plainURL(s) {
		return s, ""
	}
	return "", i18n.T("The webhook server is not an https URL (with no credential or query), so it was not taken. Enter it yourself")
}

// IssuerFrom is the issuer of an authentication config, unless the file holds
// anything that looks like a credential (the CA in it is public).
func IssuerFrom(data []byte) (string, string) {
	if strings.Contains(strings.ToLower(string(data)), "private key") {
		return "", i18n.T("authentication-config.yaml holds what looks like a key, so the issuer was not taken. Check it yourself")
	}
	var ac struct {
		JWT []struct {
			Issuer struct {
				URL string `yaml:"url"`
			} `yaml:"issuer"`
		} `yaml:"jwt"`
	}
	if yaml.Unmarshal(data, &ac) != nil || len(ac.JWT) == 0 || ac.JWT[0].Issuer.URL == "" {
		return "", i18n.T("No issuer found in authentication-config.yaml")
	}
	if u := ac.JWT[0].Issuer.URL; plainURL(u) {
		return u, ""
	}
	return "", i18n.T("The issuer is not an https URL, so it was not taken")
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
	msg := i18n.T("ansible could not be run")
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
		return nil, errors.New(i18n.T("there is no k8s control plane node"))
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
			l.Refused = append(l.Refused, i18n.T("Could not read the webhook kubeconfig: %v", err))
		} else if u, why := WebhookURLFrom(b); why != "" {
			l.Refused = append(l.Refused, why)
		} else {
			l.WebhookURL = u
		}
	}
	if l.Presets[state.PresetJWKS] {
		if b, err := slurp(ctx, o, node, flags[legacyFlag[state.PresetJWKS]], run); err != nil {
			l.Refused = append(l.Refused, i18n.T("Could not read authentication-config.yaml: %v", err))
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
			l.AuditUnread = i18n.T("%s is an empty file", path)
		case policyLooksSecret(b):
			l.AuditUnread = i18n.T("it holds what looks like a credential (a token or key), so it was not taken")
		default:
			l.AuditPolicy = b
		}
		if o.AuditPolicy != "" {
			a, err := fileio.AdoptAuditPolicy(o.AuditPolicy, l.AuditPolicy)
			if err != nil {
				a.Note = i18n.T("⚠ Could not write the audit policy of the node to %s: %v", o.AuditPolicy, err)
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
	return planLegacy(LegacyResult(l, s), l, s, o)
}

// planLegacy is the part of P3 that depends on the plan, which the control
// plane screen's read (LegacyResult) has no plan for. A staged plan's K3 is
// update-cluster.yml with the cluster stage only: kubeadm writes the manifest
// again without the authentication flags put in by hand (k8s-jwks, the
// webhook), and AIPub's login is gone until A2.5 or A5.2 applies them. On a site
// planned without AIPub (no --aipub-installed, which nothing on the nodes can
// stand in for) those flags say AIPub is installed: not OK, start again with
// the flag. With it, the gap stays only while the JWKS config waits for its
// CA; that is said rather than "update-cluster keeps the same flags".
func planLegacy(r CheckResult, l *Legacy, s *state.AppState, o Options) CheckResult {
	var auth []string
	for _, id := range []string{state.PresetJWKS, state.PresetWebhook} {
		if l.Presets[id] {
			for _, p := range state.Presets {
				if p.ID == id {
					auth = append(auth, i18n.T(p.Title))
				}
			}
		}
	}
	if len(auth) == 0 || !o.Existing {
		return r
	}
	// the line that says every preset found is on, after the header and the
	// flags found (LegacyResult)
	if i := 1 + len(l.Found); staged(s, o) && len(offPresets(l, s)) == 0 && i < len(r.Lines) {
		r.Lines[i] = i18n.T("All on: but K3 update-cluster applies the entries of the cluster stage only, so the authentication flags put in by hand (%s) "+
			"are out of the apiserver from K3 until A2.5 or A5.2 applies them", strings.Join(auth, ", "))
		r.Warn = true
	}
	if !o.AipubInstalled {
		r.OK = false
		r.Lines = append(r.Lines, i18n.T("✗ AIPub authentication (%s) is on in the apiserver already: AIPub is installed on this site. "+
			"Start again with --aipub-installed (run as it is, AIPub login is gone from K3 until A5.2)", strings.Join(auth, ", ")))
	}
	return r
}

// offPresets are the presets the node runs with that s has turned off, by
// title in the language spoken, sorted.
func offPresets(l *Legacy, s *state.AppState) []string {
	var off []string
	for _, p := range state.Presets {
		if l.Presets[p.ID] && !s.ControlPlane.On(p.ID) {
			off = append(off, i18n.T(p.Title))
		}
	}
	sort.Strings(off)
	return off
}

// LegacyResult says what a check found against the presets of s.
func LegacyResult(l *Legacy, s *state.AppState) CheckResult {
	r := CheckResult{OK: true}
	if !l.Any() {
		r.Lines = []string{i18n.T("%s: no AIPub apiserver settings added by hand", l.Node)}
		return r
	}
	r.Lines = append(r.Lines, i18n.T("Put in by hand in %s of %s:", APIServerManifest, l.Node))
	for _, f := range l.Found {
		r.Lines = append(r.Lines, "  "+f)
	}
	for _, t := range offPresets(l, s) {
		r.OK = false
		r.Lines = append(r.Lines, i18n.T("✗ %s is off: after update-cluster this feature is gone from the cluster. Turn it on in 4b and save again", t))
	}
	if r.OK {
		r.Lines = append(r.Lines, i18n.T("All on: update-cluster replaces what was edited by hand with the same flags, managed by the installer"))
	}
	if !l.Presets[state.PresetAudit] || !s.ControlPlane.Audit {
		return r
	}
	// the policy k8s_cp_extra_files copies over the node's
	if l.Audit.Note != "" {
		r.Lines = append(r.Lines, i18n.T("Audit policy: %s", l.Audit.Note))
	}
	if l.AuditUnread != "" {
		switch {
		case l.Audit.Kept:
			r.Lines = append(r.Lines, i18n.T("⚠ Could not read the audit policy of the node (%s): the site's file in ansible/files is used", l.AuditUnread))
		case s.ControlPlane.AuditDefaultOK:
			r.Lines = append(r.Lines, i18n.T("⚠ Could not read the audit policy of the node, but the default policy was chosen to replace it (o in 4b): update-cluster replaces the node's policy"))
		default:
			r.OK = false
			r.Lines = append(r.Lines, i18n.T("✗ Could not read the audit policy of the node (%s): the default policy would replace a policy not read. "+
				"Put the policy file in ansible/files/%s yourself, or confirm writing the default policy with o on the audit log row of 4b, and save again",
				l.AuditUnread, state.AuditPolicyFile))
		}
	}
	return r
}

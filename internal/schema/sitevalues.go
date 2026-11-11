package schema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// SiteValuesFile is the name the site-values view is written under.
const SiteValuesFile = "site-values.yml"

const commentColumn = 38

// SiteValues renders every variable of the schema with the value the state gives
// it, as one annotated document: a section per file, then per group, a variable
// per line. A view for a person to read: no installer reads it, and the wizard
// keeps writing the three files.
//
// ✅ marks a value that has to be given (for this state); ○ one that has a
// default. An optional variable the state leaves at its default, or does not
// manage, is written commented out with the default, visible but inert.
func SiteValues(s *state.AppState) []byte {
	var b strings.Builder
	b.WriteString(i18n.T(siteValuesHeader))

	var checks []string
	for _, f := range Files {
		b.WriteString("\n\n# " + strings.Repeat("─", 60) + "\n")
		fmt.Fprintf(&b, "#  [%s]  →  %s\n", strings.ToUpper(string(f)), f.Path())
		b.WriteString("# " + strings.Repeat("─", 60) + "\n")
		if !Written(s, f) {
			b.WriteString("# " + i18n.T("AIPub is not installed, so this file is not written") + "\n")
			continue
		}
		vars := ByFile(f)
		for _, g := range Groups {
			first := true
			for _, v := range vars {
				if v.Group != g.Name {
					continue
				}
				if first {
					b.WriteString("\n" + groupHeading(i18n.T(g.Title)) + "\n")
					first = false
				}
				line, live := siteValueLine(s, v)
				b.WriteString(line + "\n")
				var hints []string
				if live && v.Preflight != "" {
					hints = append(hints, i18n.T(v.Preflight))
				}
				for _, h := range append(hints, controlPlaneHints(s, v)...) {
					b.WriteString("# preflight: " + h + "\n")
					checks = append(checks, v.Key+": "+h)
				}
			}
		}
	}

	b.WriteString("\n\n# " + strings.Repeat("=", 60) + "\n")
	b.WriteString("#  " + i18n.T("Preflight checklist (the \"# preflight:\" lines above, collected)") + "\n")
	for _, c := range checks {
		b.WriteString("#    [ ] " + c + "\n")
	}
	b.WriteString("# " + strings.Repeat("=", 60) + "\n")
	return []byte(b.String())
}

const siteValuesHeader = `# ============================================================
#  Site settings — all the values in one file (a view made by k8s-installer-tui)
#
#  ✅ = a value that must be given   ○ = has a default. A commented-out line keeps its default
#  This file is for reading; the installer does not read it. Where the values are really saved:
#    [K8S]       → group_vars/all/vars.yml   (k8s-installer)
#    [AIPUB]     → files/aipub/vars.yml      (aipub_* only, kept across upgrades, no secrets)
#    nodes and groups are in [INVENTORY] → inventory.yml
#  constant-vars.yml holds installer constants — never edit it
#
#  "# preflight:" lines say what to check before the run; they are collected at the bottom
# ============================================================
`

func groupHeading(title string) string {
	n := 40 - displayWidth(title)
	if n < 3 {
		n = 3
	}
	return "### " + title + " " + strings.Repeat("─", n)
}

// siteValueLine is the line of one variable, and whether it is live (not
// commented out).
func siteValueLine(s *state.AppState, v Var) (string, bool) {
	required := v.RequiredIn(s)
	mark := "○"
	if required {
		mark = "✅"
	}
	note := i18n.T(v.Short)
	if key, val, ok := v.Condition(); ok {
		note = i18n.T("(required when %s=%s) %s", key, val, note)
	}

	val, managed := Value(s, v)
	if managed && v.Host {
		val = setHosts(v, val.([]HostValue))
	}
	live := required || (managed && !isDefault(v, val))
	if !live {
		return withComment("# "+v.Key+": "+flowYAML(defaultNode(v)), mark+" "+note), false
	}
	if !managed {
		val = defaultNode(v)
	} else if !required && v.Default != "" {
		note = i18n.T("(default %s) %s", flowYAML(defaultNode(v)), note)
	}
	return withComment(v.Key+": "+flowYAML(val), mark+" "+note), true
}

// setHosts keeps the nodes that set the variable to something other than its
// default, as a mapping of node name to value; a required variable keeps every
// node, so that one left empty shows.
func setHosts(v Var, hv []HostValue) yaml.Node {
	m := yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
	def := Normalize(v, v.DefaultValue())
	for _, h := range hv {
		n := Normalize(v, h.Value)
		if v.Required != Always && (n == nil || reflect.DeepEqual(n, def)) {
			continue
		}
		var val yaml.Node
		_ = val.Encode(n)
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: h.Host}, &val)
	}
	return m
}

// defaultNode is the default as written in the schema, so that a mapping keeps
// the order of its keys.
func defaultNode(v Var) any {
	if v.Type == TypeList || v.Type == TypeMap {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(v.Default), &doc); err == nil && len(doc.Content) == 1 {
			return *doc.Content[0]
		}
	}
	return v.DefaultValue()
}

func isDefault(v Var, val any) bool {
	if n, ok := val.(yaml.Node); ok {
		return len(n.Content) == 0
	}
	got := Normalize(v, val)
	return got == nil || reflect.DeepEqual(got, Normalize(v, v.DefaultValue()))
}

func withComment(text, comment string) string {
	pad := commentColumn - displayWidth(text)
	if pad < 1 {
		pad = 1
	}
	return text + strings.Repeat(" ", pad) + "# " + comment
}

// flowYAML writes a value on one line: flow collections, strings quoted.
func flowYAML(val any) string {
	var n yaml.Node
	if yn, ok := val.(yaml.Node); ok {
		n = yn
	} else if err := n.Encode(val); err != nil {
		return fmt.Sprint(val)
	}
	flow(&n, false)
	out, err := yaml.Marshal(&n)
	if err != nil {
		return fmt.Sprint(val)
	}
	return strings.TrimSpace(string(out))
}

func flow(n *yaml.Node, isKey bool) {
	switch n.Kind {
	case yaml.SequenceNode, yaml.MappingNode:
		n.Style = yaml.FlowStyle
		for i, c := range n.Content {
			flow(c, n.Kind == yaml.MappingNode && i%2 == 0)
		}
	case yaml.ScalarNode:
		if !isKey && n.Tag == "!!str" {
			n.Style = yaml.DoubleQuotedStyle
		}
	}
}

// displayWidth counts a wide character (Hangul, ✅) as two columns.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && r != '─' && r != '→' && r != '○' && r != '·' {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// controlPlaneHints are the checks the presets of the control plane leave to the
// site: the files k8s_cp_extra_files copies have to be under ansible/files, the
// issuer of JWKS and the token review server of the webhook have to answer.
func controlPlaneHints(s *state.AppState, v Var) []string {
	cp := s.AppliedControlPlane()
	var out []string
	switch v.Key {
	case state.ArgsVar("apiserver"):
		if cp.JWKS {
			out = append(out, i18n.T("JWKS issuer %q must be reachable from the control plane nodes (AIPub backend-gateway, which comes up after A5 setup-aipub; applied by A5.2 update-cluster)",
				s.JWKSIssuer()))
			out = append(out, i18n.T("CA %s is put on the nodes (the control node included) by A2 setup-cert; authentication-config.yaml is written only when it exists (otherwise A5.1 --fill-auth-config makes it)", state.JWKSCAPath))
			out = append(out, i18n.T("Running update-cluster or adding a node without the CA makes the installer refuse with 'source file missing' (validate-hostvars.py; the nodes are left as they are)"))
		}
		if cp.Webhook {
			out = append(out, i18n.T("Check that the webhook URL %q is reachable from the control plane nodes and verified with %s (applied by A2.5 update-cluster)",
				cp.WebhookURL, state.WebhookCAPath))
		}
	case state.CpExtraFilesVar:
		for _, f := range cp.ExtraFiles(state.StageAll) {
			out = append(out, i18n.T("ansible/files/%s must exist", f.Src)+FileSource(cp, f))
		}
	}
	return out
}

// FileSource says who puts a file of k8s_cp_extra_files under ansible/files; for
// the audit policy, which policy is used (cp.AuditNote, set from
// fileio.PlanAuditPolicy where the files are known).
func FileSource(cp *state.ControlPlane, f state.CpExtraFile) string {
	for _, p := range state.Presets {
		for _, pf := range p.Files {
			if pf != f {
				continue
			}
			if p.ID == state.PresetWebhook {
				return " (" + i18n.T("the wizard makes it from the webhook URL") + ")"
			}
			if p.ID == state.PresetJWKS {
				return " (" + i18n.T("the wizard makes it from the issuer URL and the CA; while there is no CA yet, A5.1 --fill-auth-config makes it") + ")"
			}
			if p.ID == state.PresetAudit {
				if cp.AuditNote != "" {
					return " (" + cp.AuditNote + ")"
				}
				return " (" + i18n.T("if missing the wizard writes the default policy; a file that exists is left as it is") + ")"
			}
		}
	}
	return " (" + i18n.T("the site puts it there") + ")"
}

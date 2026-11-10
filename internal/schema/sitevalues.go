package schema

import (
	"fmt"
	"reflect"
	"strings"

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
	b.WriteString(siteValuesHeader)

	var checks []string
	for _, f := range Files {
		b.WriteString("\n\n# " + strings.Repeat("─", 60) + "\n")
		fmt.Fprintf(&b, "#  [%s]  →  %s\n", strings.ToUpper(string(f)), f.Path())
		b.WriteString("# " + strings.Repeat("─", 60) + "\n")
		if !Written(s, f) {
			b.WriteString("# AIPub 을 설치하지 않으므로 이 파일은 쓰지 않습니다\n")
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
					b.WriteString("\n" + groupHeading(g.Title) + "\n")
					first = false
				}
				line, live := siteValueLine(s, v)
				b.WriteString(line + "\n")
				var hints []string
				if live && v.Preflight != "" {
					hints = append(hints, v.Preflight)
				}
				for _, h := range append(hints, controlPlaneHints(s, v)...) {
					b.WriteString("# preflight: " + h + "\n")
					checks = append(checks, v.Key+": "+h)
				}
			}
		}
	}

	b.WriteString("\n\n# " + strings.Repeat("=", 60) + "\n")
	b.WriteString("#  실행 전 preflight 체크리스트 (위 \"# preflight:\" 를 모은 것)\n")
	for _, c := range checks {
		b.WriteString("#    [ ] " + c + "\n")
	}
	b.WriteString("# " + strings.Repeat("=", 60) + "\n")
	return []byte(b.String())
}

const siteValuesHeader = `# ============================================================
#  사이트 설정 — 한 파일로 보는 값 (k8s-installer-tui 가 만든 보기)
#
#  ✅ = 반드시 채우는 값   ○ = 기본값이 있음. 주석 처리된 줄은 기본값 그대로
#  이 파일은 보기용이고 설치기는 읽지 않는다. 값이 실제로 저장되는 곳:
#    [K8S]       → group_vars/all/vars.yml   (k8s-installer)
#    [AIPUB]     → files/aipub/vars.yml      (aipub_* 만, 업그레이드에도 보존, 비밀 금지)
#    노드와 그룹은 [INVENTORY] → inventory.yml
#  constant-vars.yml 은 설치기 상수 — 절대 편집하지 않는다
#
#  "# preflight:" 는 실행 전에 확인할 것. 맨 아래에 모아 두었다
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
	note := v.Short
	if key, val, ok := v.Condition(); ok {
		note = fmt.Sprintf("(%s=%s 일 때 필수) %s", key, val, note)
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
		note = "(기본 " + flowYAML(defaultNode(v)) + ") " + note
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
			out = append(out, fmt.Sprintf("JWKS issuer %q 가 컨트롤 플레인 노드에서 닿아야 함 (AIPub backend-gateway, A5 setup-aipub 뒤에 뜸; A5.2 update-cluster 에서 적용)",
				s.JWKSIssuer()))
			out = append(out, "CA "+state.JWKSCAPath+" 는 A2 setup-cert 가 모든 노드에 둠; A5.1 이 authentication-config.yaml 에 복사")
		}
		if cp.Webhook {
			out = append(out, fmt.Sprintf("웹훅 URL %q 이 컨트롤 플레인 노드에서 닿고 %s 로 검증되는지 (A2.5 update-cluster 에서 적용)",
				cp.WebhookURL, state.WebhookCAPath))
		}
	case state.CpExtraFilesVar:
		for _, f := range cp.ExtraFiles(state.StageAll) {
			out = append(out, "ansible/files/"+f.Src+" 이 있어야 함"+FileSource(f))
		}
	}
	return out
}

// FileSource says who puts a file of k8s_cp_extra_files under ansible/files.
func FileSource(f state.CpExtraFile) string {
	for _, p := range state.Presets {
		for _, pf := range p.Files {
			if pf != f {
				continue
			}
			if p.ID == state.PresetWebhook {
				return " (위저드가 웹훅 URL 로 만듦)"
			}
			if p.ID == state.PresetJWKS {
				return " (위저드가 issuer URL 로 만듦, CA 는 A5.1 이 노드에서 복사)"
			}
			if p.Provider != "" {
				return " (" + p.Title + " 정책: " + p.Provider + " 제공)"
			}
		}
	}
	return " (사이트가 둠)"
}

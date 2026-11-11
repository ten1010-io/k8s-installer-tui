package state

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what this package says (i18n): keys are the English in the code.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		// Preset.Title, shown through i18n.T
		"AIPub audit log":             "AIPub 감사 로그",
		"AIPub authentication (JWKS)": "AIPub 인증 (JWKS)",
		"AIPub authentication webhook (legacy: for sites not yet moved to JWKS)": "AIPub 인증 웹훅 (레거시 — JWKS 로 옮기기 전 사이트)",
		"runtime-config (admissionregistration v1beta1)":                         "runtime-config (admissionregistration v1beta1)",
	})
}

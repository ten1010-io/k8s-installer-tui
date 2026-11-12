package main

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of the flags of the python services.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"--install: do not install the python services (A6, A7)": "--install: python 서비스를 설치하지 않음 (A6, A7)",
	})
}

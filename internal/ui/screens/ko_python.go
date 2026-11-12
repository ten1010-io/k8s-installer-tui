package screens

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what the screens say about the python services.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"python services: don't install": "python 서비스: 설치 안 함",
		"python services: install":       "python 서비스: 설치",
	})
}

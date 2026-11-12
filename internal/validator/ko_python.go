package validator

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of the checks of the python variables (pythonChecks).
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"Enter the cluster name the coaster license keys were issued for": "coaster 라이선스 키를 발급받은 클러스터 이름을 입력해주세요",
		"'%s' is not in the k8s node group":                               "'%s'는 k8s 노드 그룹에 없습니다",
	})
}

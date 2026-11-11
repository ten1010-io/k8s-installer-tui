package fileio

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what this package says (i18n): keys are the English in the code.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		// the audit policy
		"policy: the wizard writes the default policy":                   "정책: 위저드가 기본 정책을 씀",
		"policy: the site file is kept as it is":                         "정책: 사이트 파일을 그대로 둠",
		"policy: the default policy written before is kept as it is":     "정책: 전에 쓴 기본 정책을 그대로 둠",
		"policy: the one taken from the node is used as it is":           "정책: 노드에서 가져온 것을 그대로 씀",
		"⚠ policy: not written, the policy of the nodes is not read yet": "⚠ 정책: 노드의 정책을 읽기 전이라 쓰지 않음",
		"⚠ policy: the file cannot be used":                              "⚠ 정책 파일을 쓸 수 없음",
		"a symlink whose target does not exist":                          "대상이 없는 심볼릭 링크입니다",
		"unreadable (%v)":                                                "읽을 수 없습니다 (%v)",
		"a directory":                                                    "디렉터리입니다",
		"not a regular file":                                             "일반 파일이 아닙니다",
		"empty (the apiserver does not start with a policy of no rules)": "비어 있습니다 (apiserver 가 규칙 0개 정책으로 뜨지 않음)",
		"%s is not usable: %s. The wizard does not write over it. Fix or remove it, then save again": "%[1]s 이(가) %[2]s: 위저드가 덮어쓰지 않습니다. 고치거나 지운 뒤 다시 저장하세요",
		"existing site: the audit policy of the nodes is not read yet, so the default policy is not written. " +
			"In 4b take the policy of the nodes with c, put a file there yourself, or confirm writing the default policy with o on the 4b audit log row": "기존 사이트: 노드의 감사 정책을 아직 읽지 않아 기본 정책을 쓰지 않습니다. " +
			"4b 에서 c 로 노드의 정책을 가져오거나, 파일을 직접 두거나, 4b 감사 로그 줄에서 o 로 기본 정책 쓰기를 확인하세요",
		"the wizard writes the default AIPub audit policy (aipub-installer-archive AIP-2611)":                                        "위저드가 기본 AIPub 감사 정책을 씁니다 (aipub-installer-archive AIP-2611)",
		"the audit policy of node %s is taken and used as it is":                                                                     "노드 %s 의 감사 정책을 가져와 그대로 씁니다",
		" (same as the default policy of the wizard)":                                                                                " (위저드 기본 정책과 같음)",
		"the default policy the wizard wrote before is kept as it is":                                                                "위저드가 전에 쓴 기본 정책을 그대로 둡니다",
		"the policy file of the site is kept as it is (the wizard does not write over it)":                                           "사이트의 정책 파일을 그대로 둡니다 (위저드가 덮어쓰지 않음)",
		"⚠ %s is not usable: %s. The policy of the node was not written there":                                                       "⚠ %[1]s 이(가) %[2]s: 노드의 정책을 그 자리에 쓰지 않았습니다",
		"the audit policy of the node is taken and used as it is (%s)":                                                               "노드의 감사 정책을 가져와 그대로 씁니다 (%s)",
		", same as the default policy of the wizard":                                                                                 ", 위저드 기본 정책과 같음",
		"the policy file of the site":                                                                                                "사이트의 정책 파일",
		"the default policy the wizard wrote":                                                                                        "위저드가 쓴 기본 정책",
		"%s has %s, so it is used":                                                                                                   "%s 에 %s이 있어 그것을 씁니다",
		"%s: %s is the same as the policy of the node, so it is used as it is":                                                       "%s 의 %s이 노드의 정책과 같습니다: 그대로 씁니다",
		"⚠ %s: %s differs from the policy of the node. It is left as it is, and update-cluster changes the policy of the node to it": "⚠ %s 의 %s이 노드의 정책과 다릅니다: 그대로 두고, update-cluster 가 노드의 정책을 이것으로 바꿉니다",

		// the CA
		"the file has a PEM block %q (only CA certificates go in)":                     "PEM 블록 %q 가 있습니다 (CA 인증서만 넣습니다)",
		"could not read the certificate":                                               "인증서를 읽지 못했습니다",
		"there is no PEM certificate":                                                  "PEM 인증서가 없습니다",
		"a PEM block could not be read to its end":                                     "PEM 블록을 끝까지 읽지 못했습니다",
		"only %[2]d of %[1]d PEM blocks were read as certificates (a block is broken)": "PEM 블록 %d 개 중 %d 개만 인증서로 읽었습니다 (깨진 블록이 있습니다)",

		// --fill-auth-config
		"this site does not install AIPub (the JWKS issuer is AIPub)":                               "AIPub 을 설치하지 않는 사이트입니다 (JWKS issuer 는 AIPub)",
		"the AIPub authentication (JWKS) preset is off":                                             "AIPub 인증 (JWKS) 프리셋이 꺼져 있습니다",
		"there is no JWKS issuer (aipub_ingress_zone or the issuer URL)":                            "JWKS issuer 가 없습니다 (aipub_ingress_zone 또는 issuer URL)",
		"there is no CA: %s (aipub-installer setup-cert puts it there. Run again after setup-cert)": "CA 가 없습니다: %s (aipub-installer setup-cert 가 둡니다. setup-cert 뒤에 다시 실행하세요)",
		"could not read the CA: %v":                                                                 "CA 를 읽지 못했습니다: %v",
		". The CA of the saved authentication config may not be this cluster's, so it is not used":  ". 저장된 인증 설정의 CA 는 이 클러스터의 것인지 알 수 없어 쓰지 않습니다",
		"could not make authentication-config.yaml (check whether the template changed)":            "authentication-config.yaml 을 만들지 못했습니다 (템플릿이 바뀌었는지 확인)",
	})
}

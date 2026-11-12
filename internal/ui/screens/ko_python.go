package screens

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what the screens say about the python services.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		// the install screen
		"python services: don't install": "python 서비스: 설치 안 함",
		"python services: install":       "python 서비스: 설치",

		// s5c python
		"Python services settings": "Python 서비스 설정",
		"coaster, usage-manager, workload-controller-manager, the Gateways (setup-python). " +
			"Whether they are installed is chosen on the install screen": "coaster, usage-manager, workload-controller-manager, Gateway (setup-python). " +
			"설치할지는 설치 화면에서 고릅니다",
		"Cluster name": "클러스터 이름",
		"GPU nodes":    "GPU 노드",
		"(shown after the nodes are set in steps 1 and 2)": "(1·2단계에서 노드를 정하면 여기에 표시됩니다)",
		"Python deploy tool":    "Python 배포 도구",
		"Network operator":      "네트워크 오퍼레이터",
		"InfiniBand sites only": "InfiniBand 사이트만",
		"Set the InfiniBand nodes and ports in files/aipub/vars.yml " +
			"(aipub_python_network_operator_ib_nodes, _ib_pfs): setup-python stops with the value to paste when they are empty": "InfiniBand 노드와 포트는 files/aipub/vars.yml 에 직접 적습니다 " +
			"(aipub_python_network_operator_ib_nodes, _ib_pfs): 비어 있으면 setup-python 이 붙여 넣을 값을 보여 주고 멈춥니다",
		"The coaster license keys (aipub_python_coaster_license_keys) are set in files/aipub/vars.yml, " +
			"or patched by hand after the install; the wizard keeps what is there": "coaster 라이선스 키(aipub_python_coaster_license_keys)는 files/aipub/vars.yml 에 적거나 " +
			"설치 뒤 손으로 넣습니다. 이미 적어 둔 값은 위저드가 그대로 둡니다",
	})
}

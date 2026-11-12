package plan

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of the python steps of the plan (pythonSteps) and of what the
// runner says about them.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"AIPub services. The python services follow in A6": "AIPub 서비스. python 서비스는 A6 에서 이어집니다",
		"The python services: coaster, usage-manager, workload-controller-manager, the inference and workload Gateways. " +
			"After setup-aipub, which makes the usages database": "python 서비스: coaster, usage-manager, workload-controller-manager, 추론·워크로드 Gateway. " +
			"usages 데이터베이스를 만드는 setup-aipub 뒤에 실행합니다",
		"setup-aipub (again)": "setup-aipub (다시)",
		"Once more after the python services: backend-informer needs the Workspace CRD of workload-controller-manager": "python 서비스 뒤에 한 번 더: backend-informer 가 workload-controller-manager 의 Workspace CRD 를 기다립니다",
		"Python services not installed": "python 서비스 설치 안 함",
		"The audit log preset (4b) is off: setup-python puts the audit log usage-manager reads into " +
			"kube-apiserver.yaml by hand, and the next update-cluster or upgrade takes it out again (run setup-python after one). " +
			"With the preset on, kubeadm keeps it": "감사 로그 프리셋(4b)이 꺼져 있습니다: setup-python 이 usage-manager 가 읽는 감사 로그 설정을 " +
			"kube-apiserver.yaml 에 손으로 넣는데, 다음 update-cluster 나 업그레이드 때 다시 사라집니다 (그 뒤에는 setup-python 을 다시 실행). " +
			"프리셋을 켜면 kubeadm 이 유지합니다",
		"Expected on a new cluster: backend-informer waits for the Workspace CRD the python services make. " +
			"The services before it are installed. The run goes on: A6 installs the python services and A7 runs setup-aipub again, which clears this": "새 클러스터에서는 예상된 일입니다: backend-informer 가 python 서비스가 만드는 Workspace CRD 를 기다립니다. " +
			"그 앞의 서비스는 설치됐습니다. 실행은 계속됩니다: A6 이 python 서비스를 설치하고 A7 이 setup-aipub 을 다시 돌려 이 실패를 풉니다",
		"backend-informer came up in A7, setup-aipub run again after the python services": "A7(python 서비스 뒤에 다시 돌린 setup-aipub)에서 backend-informer 가 올라왔습니다",
	})
}

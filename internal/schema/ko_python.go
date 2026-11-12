package schema

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of the python variables (vars_python.go) and of aipub_multus_enabled.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"Python services": "Python 서비스",

		"Cluster name the coaster license keys were issued for":                                                    "coaster 라이선스 키를 발급받은 클러스터 이름",
		"the coaster license keys of this cluster are issued for this name; coaster refuses a key of another name": "이 클러스터의 coaster 라이선스 키가 이 이름으로 발급되어야 함. 다른 이름의 키는 coaster 가 거부",
		"GPU nodes, by kubernetes node name (coaster labels them)":                                                 "GPU 노드 (쿠버네티스 노드 이름, coaster 가 라벨을 붙임)",
		"coaster license keys by node name (unset: patched by hand after the install)":                             "노드별 coaster 라이선스 키 (비우면 설치 뒤 손으로 넣음)",
		"Load balancer of the workload Gateway (unset: the one of the ingress, on 8080/8443)":                      "워크로드 Gateway 의 로드밸런서 (비우면 인그레스 것을 8080/8443 으로 같이 씀)",
		"a name of aipub_load_balancers":                                                                           "aipub_load_balancers 에 있는 이름",
		"Storage classes wcm makes ClusterVolumes on (RWX, Immediate)":                                             "wcm 이 ClusterVolume 을 만드는 StorageClass (RWX, Immediate)",
		"each storage class exists and binds Immediate":                                                            "각 StorageClass 가 있고 volumeBindingMode 가 Immediate",
		"Show the TCP services of a workload to its project":                                                       "워크로드의 TCP 서비스를 프로젝트에 보여 줌",
		"Pass the LLM labels on to the workloads":                                                                  "LLM 라벨을 워크로드에 전달",
		"Extra values files of the python chart, on the node the installer runs on":                                "python 차트에 더할 values 파일 (설치기가 도는 노드에 있는 경로)",
		"python deploy tool": "python 배포 도구",
		"project-controller replicas after the python services (0: scale by hand)":         "python 서비스 설치 뒤 project-controller 복제 수 (0: 손으로 조정)",
		"Install aipub-network-operator (InfiniBand sites), and multus with setup-network": "aipub-network-operator 설치 (InfiniBand 사이트), setup-network 가 multus 도 설치",
		"InfiniBand nodes and their fabric name (required with the network operator)":      "InfiniBand 노드와 fabric 이름 (네트워크 오퍼레이터를 켜면 필수)",
		"search-ib-pfs.sh --all on a node prints the value to paste":                       "노드에서 search-ib-pfs.sh --all 을 돌리면 붙여 넣을 값이 나옴",
		"InfiniBand ports given to the pods, by node (required with the network operator)": "파드에 줄 InfiniBand 포트, 노드별 (네트워크 오퍼레이터를 켜면 필수)",
		"IP range of the pods on InfiniBand (empty: RDMA only)":                            "InfiniBand 위 파드 IP 대역 (비우면 RDMA 만)",
		"does not overlap the pod, service, node or storage ranges of the site":            "사이트의 pod·service·노드·스토리지 대역과 겹치지 않아야 함",
		"Network operator mode (pf, or vf for SR-IOV)":                                     "네트워크 오퍼레이터 모드 (pf, SR-IOV 면 vf)",
		"VFs per PF, as set in the firmware (vf mode)":                                     "PF 하나당 VF 수, 펌웨어에 설정한 값 (vf 모드)",
		"Nodes that run opensm and their priority (vf mode)":                               "opensm 을 돌릴 노드와 우선순위 (vf 모드)",
		"Install multus (unset: follows aipub_python_network_operator_enabled)":            "multus 설치 (비우면 aipub_python_network_operator_enabled 를 따름)",
	})
}

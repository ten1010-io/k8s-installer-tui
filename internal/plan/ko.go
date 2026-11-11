package plan

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what this package says (i18n): keys are the English in the code.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		// plan.go: the steps, their notes and the script
		"setup.sh done": "setup.sh 완료",
		"whether the release file setup.sh leaves in the install directory is there":      "setup.sh 가 설치 디렉터리에 남긴 release 파일이 있는지",
		"Unpacks the bundle and installs it in %s. It needs sudo, so you run it yourself": "번들을 풀고 %s 에 설치하는 단계. sudo가 필요해 직접 실행합니다",
		"SSH and sudo access": "SSH·sudo 접속",
		"whether every node lets ansible_user in and passes sudo -n true": "모든 노드에 ansible_user 로 접속해 sudo -n true 가 통과하는지",
		"Once the key configure-control-node-ssh.sh made is on every node (k or p on the install screen, or --send-ssh-key), " +
			"every node lets you in and runs sudo without a password. For the state of the key alone, --check-ssh": "configure-control-node-ssh.sh 가 만든 키를 모든 노드에 등록한 뒤 (설치 화면 k·p, 또는 --send-ssh-key), 모든 노드에 비밀번호 없이 접속·sudo. " +
			"키 상태만 보려면 --check-ssh",
		"apiserver settings made by hand": "apiserver 수동 설정 확인",
		"whether what was put by hand in kube-apiserver.yaml of the first control plane node matches the presets turned on (read only)": "첫 컨트롤 플레인 노드의 kube-apiserver.yaml 에 손으로 넣은 설정이 켜진 프리셋과 맞는지 (읽기만)",
		"Reads %s of the first k8s control plane node, read only (slurp): whether the audit log, runtime-config and " +
			"authentication flags put in by hand match the presets turned on. A feature found and turned off is gone after update-cluster": "첫 k8s 컨트롤 플레인 노드의 %s 를 읽기만 함 (slurp). 손으로 넣은 감사 로그·runtime-config·" +
			"인증 플래그가 켜진 프리셋과 맞는지. 감지된 것을 끄고 update-cluster 하면 그 기능이 사라짐",
		"whether the package carries helmfile and kustomize (aipub_bin of aipub-package.json)": "패키지가 helmfile·kustomize 를 실었는지 (aipub-package.json 의 aipub_bin)",
		"Deploy tool helmfile: whether the package carries helmfile and kustomize (aipub_bin of %s). " +
			"Without them, the site has to put them on the nodes itself and set their paths as variables": "배포 도구가 helmfile: 패키지가 helmfile·kustomize 를 실었는지 (%s 의 aipub_bin). " +
			"없으면 사이트가 노드에 직접 두고 경로를 변수로 지정해야 함",
		"Catalog": "카탈로그",
		"whether the catalog has every image the aipub modules use, by name": "카탈로그에 aipub 모듈이 쓰는 이미지가 이름으로 모두 있는지",
		"The aipub playbooks refuse to run when an image their module uses is not in the catalog (ansible/aipub_vars/catalog.yml) by name. " +
			"Checked first by the same rules (skipped when there is no catalog file)": "aipub 플레이북은 모듈이 쓰는 이미지가 카탈로그 (ansible/aipub_vars/catalog.yml) 에 이름으로 없으면 실행 전에 거부합니다. " +
			"같은 규칙으로 미리 확인 (카탈로그 파일이 없으면 건너뜀)",
		"become shell": "become 셸",
		"whether sudo -n /bin/sh -c true passes on every node (and when refused, whether dash gets around it)": "모든 노드에서 sudo -n /bin/sh -c true 가 통과하는지 (막히면 dash 로 우회할 수 있는지)",
		"When sudoers refuses /bin/sh, Ansible become fails with 'Shared connection closed'. Checks " +
			"sudo -n /bin/sh -c true on every node (read only). When it is refused and dash is there, the ansible-playbook lines of the plan get " +
			"-e ansible_shell_executable=<dash>": "sudoers 가 /bin/sh 를 막으면 Ansible become 이 'Shared connection closed' 로 실패합니다. 모든 노드에서 " +
			"sudo -n /bin/sh -c true 를 확인 (읽기만). 막혔고 dash 가 있으면 계획의 ansible-playbook 에 " +
			"-e ansible_shell_executable=<dash> 를 붙입니다",
		"whether the AIPub and Harbor hosts resolve to the ingress VIP (getent hosts)": "AIPub·Harbor 호스트가 인그레스 VIP 로 풀리는지 (getent hosts)",
		"Whether the AIPub and Harbor hosts (<prefix>.<aipub_ingress_zone>) resolve to the ingress VIP %s, " +
			"by getent hosts on the control node. DNS is often registered after the install, so only a warning": "AIPub·Harbor 호스트 (<접두사>.<aipub_ingress_zone>) 가 인그레스 VIP %s 로 풀리는지 " +
			"control node 에서 getent hosts 로 확인. DNS 는 설치 뒤에 등록하는 경우가 많아 경고만",
		"whether StorageClass %s is in the existing cluster (kubectl, read only)": "기존 클러스터에 StorageClass %s 가 있는지 (kubectl, 읽기만)",
		"Existing site: kubectl get storageclass %s on the first control plane node, read only " +
			"(a warning when it is missing). Skipped on a new site, where the class is made after the cluster": "기존 사이트: 첫 컨트롤 플레인 노드에서 kubectl get storageclass %s" +
			" 로 있는지 읽기만 확인 (없으면 경고). 새 사이트는 클러스터를 만든 뒤에 만들어져 건너뜀",
		"Deploys the installer to every node":              "모든 노드에 설치본 배포",
		"ki cp services: DNS, NTP, registry, apiserver LB": "ki cp 서비스: DNS, NTP, 레지스트리, apiserver LB",
		"The cluster is there already: the installer and the ki cp services of the nodes are upgraded with upgrade-k8s-installer.yml, not by this plan": "클러스터가 이미 있음: 노드의 설치본·ki cp 서비스는 이 계획이 아니라 upgrade-k8s-installer.yml 로 올림",
		"Pushes the image and chart archive (%s) to the user registry":                                                                                  "이미지·차트 아카이브 (%s) 를 사용자 레지스트리에",
		"No archive path: the user registry of a new site is empty, so the steps from A1 on cannot pull their images":                                   "아카이브 경로 없음: 새 사이트의 사용자 레지스트리는 비어 있어 A1 부터 이미지를 받지 못합니다",
		"No archive path (fine when it was pushed already)":                                                                                             "아카이브 경로 없음 (이미 넣었으면 건너뜀)",
		"Certificates, and the CA installed on every node (%s)":                                                                                         "인증서, 모든 노드에 CA 설치 (%s)",
		"Put the certificate files in /etc/k8s-installer/certs of the runner node before the run":                                                       "실행 전에 러너 노드의 /etc/k8s-installer/certs에 인증서 파일을 두세요",
		"OpenSearch, VictoriaMetrics. Before aipub, so that aipub need not run again":                                                                   "OpenSearch, VictoriaMetrics. aipub보다 먼저 두어 aipub 재실행이 필요 없게 함",
		"Monitoring not installed":                                                                       "모니터링 설치 안 함",
		"Cilium, L2 VIP %s. The nodes are Ready when it ends":                                            "Cilium, L2 VIP %s. 끝나면 노드가 Ready",
		"update-cluster (apply the apiserver authentication webhook)":                                    "update-cluster (apiserver 인증 웹훅 적용)",
		"Applies the authentication webhook entries left out of K3.":                                     "K3 에서 뺀 인증 웹훅 항목을 적용.",
		"JWKS entries left out (-e), applied in A5.2":                                                    "JWKS 항목은 빼고 (-e), A5.2 에서 적용",
		"AIPub services. The Python components are not in aipub-installer yet, so they are not included": "AIPub 서비스. Python 쪽 컴포넌트는 아직 aipub-installer 밖이라 포함되지 않음",
		updateClusterTime: "컨트롤 플레인 노드 한 대에 3~6분 (세 대면 15분 안팎): kubeadm 이 노드마다 etcd 를 백업·검사하고, " +
			"플래그와 파일이 함께 바뀌면 apiserver 를 두 번 재시작합니다 (하나만 바뀌면 한 번). 한 태스크가 5분 가까이 걸리기도 합니다. " +
			"기다림마다 시한이 있어 넘기면 그 노드를 LB 에 되돌리고 멈춥니다. 멈추려면 Ctrl+C 한 번 (이 단계가 끝난 뒤 멈춤): " +
			"두 번 (바로 멈춤) 이면 노드가 LB 에서 빠진 채 남아, ki cp 노드마다 lb-admin.sh ready <노드> 로 되돌려야 합니다",
		"Takes the control plane nodes out of the LB one at a time and restarts the apiserver.": "컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다.",
		"update-cluster (apply the control plane entries)":                                      "update-cluster (컨트롤 플레인 항목 적용)",
		"Existing site: applies every control plane entry at once (no -e). A kube-apiserver.yaml edited by hand gets the same " +
			"flags, managed by the installer (P3 checks first).": "기존 사이트: 컨트롤 플레인 항목을 한 번에 적용 (-e 없음). 손으로 고친 kube-apiserver.yaml 은 설치기가 관리하는 같은 " +
			"플래그로 바뀜 (P3 로 먼저 확인).",
		"Fill in the JWKS CA": "JWKS CA 채우기",
		"Reads the CA A2 setup-cert put there (%s), makes files/%s again with the issuer and the CA, " +
			"and writes it to both the saved site and the install directory (the same when run again). Stops here when there is no CA": "A2 setup-cert 가 둔 CA (%s) 를 읽어 files/%s 를 issuer·CA 로 다시 만들어 " +
			"저장한 사이트와 설치 디렉터리 양쪽에 씀 (다시 실행해도 같음). CA 가 없으면 여기서 멈춤",
		"update-cluster (apply the apiserver JWKS authentication)": "update-cluster (apiserver JWKS 인증 적용)",
		"Applies the JWKS entries left out of K3 (authentication-config), so that kubeadm keeps what " +
			"k8s-jwks of setup-aipub put in by hand.": "K3 에서 뺀 JWKS 항목 (authentication-config) 을 적용. setup-aipub 의 k8s-jwks 가 손으로 넣은 것을 " +
			"kubeadm 이 보관하게 함.",
		"kubeadm. When it ends the nodes are NotReady, as there is no CNI yet: that is expected": "kubeadm. 끝나면 CNI가 없어 노드가 NotReady인 것이 정상",
		"update-cluster (control plane entries of the cluster stage)":                            "update-cluster (클러스터 단계 컨트롤 플레인 항목)",
		"Existing cluster: update-cluster.yml, in place of setup-cluster, applies the control plane entries of the cluster stage " +
			"(the audit log, runtime-config and the like) before the AIPub steps.": "기존 클러스터: setup-cluster 대신 update-cluster.yml 로 클러스터 단계의 컨트롤 플레인 항목 (감사 로그, runtime-config 등) 을 " +
			"AIPub 단계 전에 적용.",
		"Existing cluster: K5 applies the control plane entries at once":  "기존 클러스터: K5 가 컨트롤 플레인 항목을 한 번에 적용",
		"Existing site: A2.5 applies the control plane entries at once":   "기존 사이트: A2.5 가 컨트롤 플레인 항목을 한 번에 적용",
		"Existing cluster: no control plane entry to apply at this stage": "기존 클러스터: 이 단계에서 적용할 컨트롤 플레인 항목 없음",
		"the authentication webhook in A2.5":                              "인증 웹훅은 A2.5",
		"JWKS in A5.2":                                                    "JWKS 는 A5.2",
		"AIPub authentication entries left out (-e), applied later: %s":   "AIPub 인증 항목은 빼고 (-e), %s 에서 적용합니다",
		"not HA, data on %s":                                              "HA 아님, 데이터는 %s",
		"# Install plan (k8s-installer-tui). It stops at the step that fails. The setup playbooks are safe\n" +
			"# to run again: fix the cause, then run it again from the failed step.\n": "# 설치 계획 (k8s-installer-tui). 실패하면 그 단계에서 멈춥니다. setup 플레이북은 다시 실행해도\n" +
			"# 안전하므로, 고친 뒤 실패한 단계부터 다시 실행하면 됩니다.\n",
		"# Copy the saved settings to the install directory (sudo: it is root's)\n": "# 저장한 설정 파일을 설치 디렉터리로 (root 소유라 sudo)\n",
		"skipped (%s)":                "건너뜀 (%s)",
		"(manual)":                    "(수동)",
		"Enter once it is installed:": "설치했으면 Enter:",

		// stages.go, site.go
		"Copy the settings": "설정 복사",
		"Copies the saved settings to %s (sudo; copied again on a resume too)":                                                                 "저장한 설정 파일을 %s 로 (sudo, 이어서 실행할 때도 다시 복사)",
		"Existing site: given with --existing":                                                                                                 "기존 사이트: --existing 으로 지정",
		"Existing site: %s is there (the record k8s-installer leaves once it built the cluster, which reset-cluster.yml removes)":              "기존 사이트: %s 있음 (k8s-installer 가 클러스터를 만든 기록, reset-cluster.yml 이 지움)",
		"New site: no %s (start with --existing when there is a cluster already)":                                                              "새 사이트: %s 없음 (클러스터가 이미 있으면 --existing 으로 시작)",
		"Planned as a new site: %s could not be looked at (%v). Start with --existing when there is a cluster already":                         "새 사이트로 계획: %s 를 확인하지 못함 (%v). 클러스터가 이미 있으면 --existing 으로 시작",
		"AIPub: given with --aipub-installed":                                                                                                  "AIPub: --aipub-installed 로 지정",
		"AIPub planned as a new install (--aipub-installed when it is installed already: there is no record of it, so only this flag decides)": "AIPub 은 새로 설치하는 것으로 계획 (이미 설치돼 있으면 --aipub-installed: 설치 기록이 없어 플래그로만 정함)",
		"New site":          "새 사이트",
		"Existing site":     "기존 사이트",
		"(AIPub installed)": "(AIPub 설치됨)",
		"%s: as planned by the last run (started %s). To change it, run from the first step (decided now: %s)": "%s: 지난 실행 (%s 에 시작) 의 계획대로. 바꾸려면 처음부터 실행 (지금 판단: %s)",

		// runner.go
		"another k8s-installer-tui (pid %s) is running this install (%s)": "다른 k8s-installer-tui (pid %s) 가 이 설치를 실행 중입니다 (%s)",
		"state directory %s": "상태 디렉터리 %s",
		"%s of the last run is still running (process group %d, pid %d). Wait for it to end, or stop it with kill -INT -- -%d, then run again": "지난 실행의 %s 가 아직 실행 중입니다 (프로세스 그룹 %d, pid %d). 끝나기를 기다리거나 kill -INT -- -%d 로 멈춘 뒤 다시 실행하세요",
		"Steps %s run with sudo on this control node, and sudo asks for a password (sudo -n true: %s). " +
			"Give this user sudo without a password (NOPASSWD), then run again": "%s 는 이 제어 노드에서 sudo 로 실행하는데, sudo 가 비밀번호를 묻습니다 (sudo -n true: %s). " +
			"이 사용자에게 비밀번호 없는 sudo (NOPASSWD) 를 준 뒤 다시 실행하세요",
		"Playbooks that delete or reset are never run: %s": "삭제·초기화 플레이북은 실행하지 않습니다: %s",
		"Log file: %v": "로그 파일: %v",
		"Overwritten unchecked, as they could not be read (not in the history): %s": "읽을 수 없어 확인하지 못하고 덮어씀 (이력에 없음): %s",
		"The check before the copy failed: %v":                                      "복사 전 확인 실패: %v",
		"Files on the installer side were edited by hand since the last copy: %s. " +
			"What they held is kept in %s. Look at it (and bring it into the site if needed), then run again to overwrite them": "설치기 쪽 파일을 마지막 복사 뒤 손으로 고쳤습니다: %s. " +
			"그 내용은 %s 에 남겼습니다. 확인한 뒤 (필요하면 사이트에 반영하고) 다시 실행하면 덮어씁니다",
		"The copy could not be recorded in the history: %v": "이력에 복사를 남기지 못함: %v",
		"end: exit code %d": "끝: 종료 코드 %d",
		"Only backend-informer failed, and the steps after this one do not depend on it: this step stays failed and the run goes on": "실패한 것은 backend-informer 하나이고 뒤 단계는 그것에 기대지 않아, 이 단계는 실패로 남기고 계속합니다",
		"Stopped (Ctrl+C twice)": "중단함 (Ctrl+C 두 번)",
		"Known issue: backend-informer waits for the Workspace CRD the Python services make. The services before it are installed. " +
			"Install the Python services, then resume (aipub-installer README)": "알려진 일: backend-informer 는 Python 서비스가 만드는 Workspace CRD 를 기다립니다. 그 앞의 서비스는 설치됐습니다. " +
			"Python 서비스를 설치한 뒤 이어서 실행하세요 (aipub-installer README)",
		"Diagnosis: pods did not start, and helm stopped waiting for them. The usual cause is images missing from the user registry (ImagePullBackOff). " +
			"Look with kubectl %s get pods on a control plane node": "진단: 파드가 시작되지 못해 helm 이 기다리다 멈췄습니다. 흔한 원인은 사용자 레지스트리에 이미지가 없는 것 (ImagePullBackOff) 입니다. " +
			"컨트롤 플레인 노드에서 kubectl %s get pods 로 확인하세요",
		"This run skipped K4 push-user-registry-artifacts: give the path of %s as the archive path and resume " +
			"(--install --resume --archive <file>). K4 runs first, then this step": "이번 실행은 K4 push-user-registry-artifacts 를 건너뛰었습니다: 아카이브 경로에 %s" +
			" 의 경로를 넣고 이어서 실행하세요 (--install --resume --archive <파일>). K4 를 먼저 실행한 뒤 이 단계부터 갑니다",
		"Diagnosis: a firewall blocks etcd between the control plane nodes (ports 2379/2380). Fix: open ports 2379 and 2380/tcp on every node, or check the state of the firewall.": "진단: 컨트롤 플레인 간 etcd 통신(포트 2379/2380)이 방화벽에 차단되었습니다. 복구: 각 노드에서 방화벽 포트(2379, 2380/tcp)를 개방하거나 방화벽 상태를 확인하세요.",
		"Diagnosis: an earlier install left containerd on the nodes. Fix: reset the nodes with reset-cluster.yml, then run again from the first step " +
			"(x on the install screen when it offers x, then type reset)": "진단: 노드에 이전 설치 잔재(containerd)가 남아있습니다. 복구: reset-cluster.yml 로 노드를 초기화한 뒤 처음부터 다시 실행하세요 " +
			"(설치 화면에 x 가 나오면 x, reset 입력)",
		"Diagnosis: the openssl-devel package conflicts with the dependencies of python3.12-libs. Fix: run 'sudo dnf remove -y openssl-devel' on the node.": "진단: openssl-devel 패키지로 인해 python3.12-libs 의존성 충돌이 발생했습니다. 복구: 노드에서 'sudo dnf remove -y openssl-devel' 이 필요합니다.",
		"Diagnosis: the kubelet service is enabled already: left by an earlier install, or a cluster already runs on this node. " +
			"A cluster in service is not to be reset: start again with --existing. When it is a leftover, reset the nodes with reset-cluster.yml, then " +
			"run again from the first step (x on the install screen when it offers x, then type reset)": "진단: kubelet 서비스가 이미 활성화되어 있습니다. 이전 설치의 잔재이거나, 이 노드에 이미 클러스터가 돌고 있는 것입니다. " +
			"운영 중인 클러스터라면 초기화하지 말고 --existing 으로 다시 시작하세요. 잔재가 맞으면 reset-cluster.yml 로 노드를 초기화한 뒤 " +
			"처음부터 다시 실행하세요 (설치 화면에 x 가 나오면 x, reset 입력)",
		"Diagnosis: a file that k8s_cp_extra_files names (the JWKS authentication config or another, made by A5.1) is not in the install directory. " +
			"At each step the wizard leaves out, with -e, the files not yet written, so this refusal comes from a playbook run by hand. " +
			"Fix: run it with the wizard (or --install), or, when running it by hand, add the -e from the ── line of this step's log as it is. " +
			"Do not make an empty file: an empty authentication config copied to the control plane nodes keeps the apiserver from starting": "진단: k8s_cp_extra_files 가 가리키는 파일 (JWKS 인증 설정 등, A5.1 이 만듦) 이 설치 디렉터리에 없습니다. " +
			"위저드는 아직 없는 파일을 단계마다 -e 로 빼고 실행하므로, 이 거부는 플레이북을 손으로 실행했을 때 납니다. " +
			"복구: 위저드 (또는 --install) 로 실행하거나, 손으로 실행할 때는 이 단계 로그의 ── 줄에 있는 -e 를 그대로 붙이세요. " +
			"빈 파일을 만들지 마세요: 빈 인증 설정이 컨트롤 플레인 노드로 복사되면 apiserver 가 뜨지 않습니다",
		"The Workspace CRD error of backend-informer is in the log (on a new cluster it always is, until the Python services are installed). " +
			"But it is not shown to be the only failure, so the services before it are not known to be installed. Look for other failures in the log first; " +
			"when the informer is the only one, install the Python services, then resume": "backend-informer 의 Workspace CRD 오류가 보입니다 (새 클러스터에서는 Python 서비스를 설치하기 전까지 늘 나옵니다). " +
			"다만 실패가 그것 하나인지 확인되지 않아 앞의 서비스가 설치됐다고 볼 수 없습니다. 로그에서 다른 실패를 먼저 확인하고, " +
			"informer 하나뿐이면 Python 서비스를 설치한 뒤 이어서 실행하세요",
		"Raw failure: %s":                                "실패 원문: %s",
		"Checking (kubectl get storageclass)":            "확인 중 (kubectl get storageclass)",
		"Stopped":                                        "중단함",
		"Missing: %s — %s":                               "없음: %s — %s",
		"Installed (by the operator), checking again":    "설치함 (운영자), 다시 확인",
		"Stopped while waiting":                          "멈춤: 기다리던 중 중단",
		"Stopped: another step of the same stage failed": "같은 단계의 다른 단계가 실패해 멈춤",

		// manual.go
		DefaultClass:                      "(클러스터 기본 StorageClass)",
		"Install the StorageClass driver": "StorageClass 드라이버 설치",
		"No step uses a StorageClass (not HA, monitoring and the shared volume static)": "StorageClass 를 쓰는 단계가 없음 (HA 아님, 모니터링·공유 볼륨이 static)",
		"Waits alongside setup-cert. Install the storage driver (CSI) of the site to make the classes: %s. " +
			"The next stage starts once kubectl get storageclass on the first k8s control plane node shows them all": "setup-cert 와 함께 멈춤. 사이트의 스토리지 드라이버 (CSI) 를 설치해 클래스를 만드세요: %s" +
			". 첫 k8s 컨트롤 플레인 노드에서 kubectl get storageclass 로 모두 있어야 다음 단계로",
		"Install the StorageClass driver — the classes the wizard uses: %s": "스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: %s",
		"No k8s control plane node to check with":                           "k8s 컨트롤 플레인 노드가 없어 확인하지 못했습니다",
		"StorageClass name %s is not valid":                                 "StorageClass 이름 %s 이(가) 올바르지 않습니다",
		"%s: StorageClass %s is there":                                      "%s: StorageClass %s 있음",
		"%s: StorageClass %s missing":                                       "%s: StorageClass %s 없음",
		"%s: could not check StorageClass %s (%s)":                          "%s: StorageClass %s 를 확인하지 못했습니다 (%s)",
		"Could not check StorageClass %s: %s":                               "StorageClass %s 를 확인하지 못했습니다: %s",
		"%s: default StorageClass %s is there":                              "%s: 기본 StorageClass %s 있음",
		"%s: no default StorageClass (no class with %s: \"true\")":          "%s: 기본 StorageClass 가 없습니다 (%s: \"true\" 인 클래스 없음)",
		"%s: could not check the default StorageClass (%s)":                 "%s: 기본 StorageClass 를 확인하지 못했습니다 (%s)",
		"Could not check the default StorageClass: %s":                      "기본 StorageClass 를 확인하지 못했습니다: %s",

		// storageclass.go, dns.go
		"AIPub not installed":                               "AIPub 을 설치하지 않음",
		"No StorageClass (aipub_ha_mode_storage_class) set": "StorageClass (aipub_ha_mode_storage_class) 가 정해지지 않음",
		"New site: no cluster to check yet (StorageClass %s has to be there once the cluster is built)": "새 사이트: 클러스터가 아직 없어 확인할 수 없습니다 (StorageClass %s 는 클러스터를 만든 뒤에 있어야 함)",
		"No k8s control plane node":                                "k8s 컨트롤 플레인 노드가 없음",
		"StorageClass name %s is not valid, so it was not checked": "StorageClass 이름 %s 이(가) 올바르지 않아 확인하지 않았습니다",
		"%s: StorageClass %s is missing. Make the class, or change aipub_ha_mode_storage_class, " +
			"before volumes are left Pending": "%s: StorageClass %s 가 없습니다. 볼륨이 Pending 으로 남기 전에 클래스를 만들거나 " +
			"aipub_ha_mode_storage_class 를 바꾸세요",
		"No ingress VIP set":                       "인그레스 VIP 가 정해지지 않음",
		"No AIPub domain (aipub_ingress_zone) set": "AIPub 도메인 (aipub_ingress_zone) 이 정해지지 않음",
		"%s: no answer (ingress VIP %s expected)":  "%s: 답 없음 (기대값 인그레스 VIP %s)",
		"%s → %s (the ingress VIP)":                "%s → %s (인그레스 VIP 와 같음)",
		"%s: %s (ingress VIP %s expected)":         "%s: %s (기대값 인그레스 VIP %s)",
		"DNS is often registered after the install, so this is only a warning. For access from outside, *.%s has to point at the ingress VIP": "DNS 는 설치 뒤에 등록하는 경우가 많아 경고로만 표시합니다. 외부에서 접속하려면 *.%s 가 인그레스 VIP 를 가리켜야 합니다",

		// check.go, adhoc.go, become.go
		"%s/release is not there. Run sudo ./setup.sh in the package directory first": "%s/release 가 없습니다. 패키지 디렉터리에서 sudo ./setup.sh 를 먼저 실행하세요",
		"Installed release %s":                             "설치된 릴리스 %s",
		"Unreachable (check the SSH key, address, port)":   "접속 안 됨 (SSH 키, 주소, 포트 확인)",
		"sudo needs a password or failed (check NOPASSWD)": "sudo 비밀번호 필요 또는 실패 (NOPASSWD 확인)",
		"ready over ssh":                                   "ssh 로는 준비됨",
		"Unreachable — %s: %s":                             "접속 안 됨 — %s: %s",
		"ansible could not be run":                         "ansible을 실행하지 못했습니다",
		"OK: %s":                                           "정상: %s",
		"Nodes with a firewall active (firewalld/ufw): %s (turning it off is advised, so that no port is blocked)": "방화벽(firewalld/ufw) 활성 노드: %s (포트 차단 방지를 위해 비활성화 권장)",
		"OK (sudo -n /bin/sh -c true): %s":                    "정상 (sudo -n /bin/sh -c true): %s",
		"Nodes where sudo itself fails, which P2 reports: %s": "sudo 자체가 안 되는 노드는 P2 에서 봅니다: %s",
		"Nodes where sudo refuses /bin/sh: %s (seemingly !/bin/sh in sudoers). If left as is, Ansible become fails with 'Shared connection closed'": "sudo 가 /bin/sh 를 거부하는 노드: %s (sudoers 의 !/bin/sh 로 보임). 그대로 두면 Ansible become 이 'Shared connection closed' 로 실패합니다",
		"%s: no dash": "%s: dash 가 없습니다",
		"%s: dash (%s) is there, but sudo refuses it":                                                                     "%s: dash (%s) 가 있으나 sudo 가 거부합니다",
		"%s: could not look for dash (check the connection and sudo)":                                                     "%s: dash 를 확인하지 못했습니다 (접속·sudo 확인)",
		"The path of dash differs between the nodes (%s): ansible_shell_executable takes one only, so make them the same": "노드마다 dash 경로가 다릅니다 (%s): ansible_shell_executable 은 하나만 쓸 수 있으니 경로를 맞추세요",
		"Let sudoers allow /bin/sh, or put a shell sudo may run (dash) on these nodes":                                    "sudoers 가 /bin/sh 를 허용하게 하거나, sudo 로 실행할 수 있는 셸 (dash) 을 이 노드들에 두세요",
		"All have dash (%s) and sudo runs it: the ansible-playbook lines of the plan get -e ansible_shell_executable=%s":  "모두 dash (%s) 가 있고 sudo 로 실행됩니다: 계획의 ansible-playbook 에 -e ansible_shell_executable=%s 를 붙입니다",

		// aipubbin.go, archive.go, catalog.go
		"No deploy tool is helmfile":                                                             "배포 도구가 helmfile 이 아님",
		"%s is not there, so whether aipub-bin was carried is not known":                         "%s 가 없어 aipub-bin 이 실렸는지 알 수 없습니다",
		"%s could not be read: %v":                                                               "%s 를 읽지 못했습니다: %v",
		"%s has an empty aipub_bin (the package was made with --no-aipub-bin)":                   "%s 의 aipub_bin 이 비어 있습니다 (패키지를 --no-aipub-bin 으로 만듦)",
		"aipub-bin of the package: %s":                                                           "패키지의 aipub-bin: %s",
		"The paths of helmfile and kustomize are set as site values":                             "helmfile 과 kustomize 경로는 사이트 값으로 정해 둠",
		"Deployed with helmfile, but the package has no helmfile or kustomize (aipub-bin): %s":   "helmfile 로 배포하는데 패키지에 helmfile·kustomize (aipub-bin) 가 없습니다: %s",
		"The site has to put both binaries on the nodes itself and set their paths: %s":          "사이트가 두 바이너리를 노드에 직접 두고 경로를 정해야 합니다: %s",
		"archive %s not found: %v":                                                               "아카이브 %s 를 찾을 수 없습니다: %v",
		"archive %s is not a regular file":                                                       "아카이브 %s 는 일반 파일이 아닙니다",
		"archive %s cannot be read: %v":                                                          "아카이브 %s 를 읽을 수 없습니다: %v",
		"no catalog file (%s)":                                                                   "카탈로그 파일이 없음 (%s)",
		"Catalog OK: %s":                                                                         "카탈로그 이상 없음: %s",
		"Catalog problems: %d, which the playbooks refuse (P5 of the install plan): %s and more": "카탈로그 문제 %d건, 설치 계획의 P5 에서 플레이북이 거부합니다: %s 외",
		"Catalog problems: %d, which the playbooks refuse (P5 of the install plan): %s":          "카탈로그 문제 %d건, 설치 계획의 P5 에서 플레이북이 거부합니다: %s",
		"the catalog is empty (aipub-packager fills it when it makes a package)":                 "카탈로그가 비어 있습니다 (aipub-packager 가 패키지를 만들 때 채웁니다)",
		"module[aipub]: images.aipub and images.java differ (keep one only)":                     "module[aipub]: images.aipub 와 images.java 가 다름 (하나만 두세요)",
		"%s/<name>:<tag>":                      "%s/<이름>:<태그>",
		"/ or ":                                "/ 또는 ",
		"module[%s]: %s is not of the form %s": "module[%s]: %s 는 %s 형식이 아님",
		"module[%s]: %s missing":               "module[%s]: %s 없음",
		"(busybox of monitoring, or aipub/busybox of the aipub module)": "(monitoring 의 busybox, 또는 aipub 모듈의 aipub/busybox)",
		"module[%s]: the name %s appears more than once":                "module[%s]: %s 이름이 두 번 이상 나옴",

		// legacy.go
		"could not read %s":           "%s 를 읽지 못했습니다",
		"%s has no container":         "%s 에 컨테이너가 없습니다",
		"(not mounted)":               "(마운트 없음)",
		"--%s=%s (the preset has %s)": "--%s=%s (프리셋은 %s)",
		"The webhook kubeconfig holds what looks like a credential (a token or key), so the URL was not taken. Enter it yourself": "웹훅 kubeconfig 에 자격 증명(토큰·키)으로 보이는 것이 있어 URL 을 가져오지 않았습니다. 직접 입력하세요",
		"No server found in the webhook kubeconfig. Enter it yourself":                                                            "웹훅 kubeconfig 에서 server 를 찾지 못했습니다. 직접 입력하세요",
		"The webhook server is not an https URL (with no credential or query), so it was not taken. Enter it yourself":            "웹훅 server 가 https URL (자격 증명·쿼리 없이) 이 아니라 가져오지 않았습니다. 직접 입력하세요",
		"authentication-config.yaml holds what looks like a key, so the issuer was not taken. Check it yourself":                  "authentication-config.yaml 에 키로 보이는 것이 있어 issuer 를 가져오지 않았습니다. 직접 확인하세요",
		"No issuer found in authentication-config.yaml":                                                                           "authentication-config.yaml 에서 issuer 를 찾지 못했습니다",
		"The issuer is not an https URL, so it was not taken":                                                                     "issuer 가 https URL 이 아니라 가져오지 않았습니다",
		"there is no k8s control plane node":                                                                                      "k8s 컨트롤 플레인 노드가 없습니다",
		"Could not read the webhook kubeconfig: %v":                                                                               "웹훅 kubeconfig 를 읽지 못했습니다: %v",
		"Could not read authentication-config.yaml: %v":                                                                           "authentication-config.yaml 을 읽지 못했습니다: %v",
		"%s is an empty file": "%s 가 비어 있습니다",
		"it holds what looks like a credential (a token or key), so it was not taken": "자격 증명(토큰·키)으로 보이는 것이 있어 가져오지 않았습니다",
		"⚠ Could not write the audit policy of the node to %s: %v":                    "⚠ 노드의 감사 정책을 %s 에 쓰지 못했습니다: %v",
		"All on: but K3 update-cluster applies the entries of the cluster stage only, so the authentication flags put in by hand (%s) " +
			"are out of the apiserver from K3 until A2.5 or A5.2 applies them": "모두 켜져 있음: 다만 K3 update-cluster 는 클러스터 단계 항목만 적용해, 손으로 넣은 인증 플래그 (" +
			"%s) 는 K3 부터 A2.5·A5.2 가 적용할 때까지 apiserver 에서 빠집니다",
		"✗ AIPub authentication (%s) is on in the apiserver already: AIPub is installed on this site. " +
			"Start again with --aipub-installed (run as it is, AIPub login is gone from K3 until A5.2)": "✗ apiserver 에 AIPub 인증 (%s) 이 이미 켜져 있습니다: AIPub 이 설치된 사이트입니다. " +
			"--aipub-installed 로 다시 시작하세요 (이대로 실행하면 K3 부터 A5.2 까지 AIPub 로그인이 빠짐)",
		"%s: no AIPub apiserver settings added by hand": "%s: 손으로 넣은 AIPub apiserver 설정 없음",
		"Put in by hand in %s of %s:":                   "%[2]s 의 %[1]s 에 손으로 넣은 설정:",
		"✗ %s is off: after update-cluster this feature is gone from the cluster. Turn it on in 4b and save again": "✗ %s 이(가) 꺼져 있음: update-cluster 뒤 이 기능이 클러스터에서 사라집니다. 4b 에서 켜고 다시 저장하세요",
		"All on: update-cluster replaces what was edited by hand with the same flags, managed by the installer":    "모두 켜져 있음: update-cluster 가 손으로 고친 것을 설치기가 관리하는 같은 플래그로 바꿉니다",
		"Audit policy: %s": "감사 정책: %s",
		"⚠ Could not read the audit policy of the node (%s): the site's file in ansible/files is used":                                                        "⚠ 노드의 감사 정책을 읽지 못했습니다 (%s): ansible/files 의 사이트 파일을 씁니다",
		"⚠ Could not read the audit policy of the node, but the default policy was chosen to replace it (o in 4b): update-cluster replaces the node's policy": "⚠ 노드의 감사 정책을 읽지 못했지만 기본 정책으로 바꾸기로 했습니다 (4b 의 o): update-cluster 가 노드의 정책을 바꿉니다",
		"✗ Could not read the audit policy of the node (%s): the default policy would replace a policy not read. " +
			"Put the policy file in ansible/files/%s yourself, or confirm writing the default policy with o on the audit log row of 4b, and save again": "✗ 노드의 감사 정책을 읽지 못했습니다 (%s): 기본 정책이 읽지 못한 정책을 바꾸게 됩니다. " +
			"정책 파일을 ansible/files/%s 에 직접 두거나, 4b 감사 로그 줄에서 o 로 기본 정책 쓰기를 확인하고 다시 저장하세요",

		// reset.go
		"The record of the run is not what the screen showed (another run happened since). Go back to the plan screen and look again": "실행 기록이 화면에 보인 것과 다릅니다 (그 사이 다른 실행이 있었습니다). 계획 화면으로 나갔다가 다시 확인하세요",
		"Cannot tell whether this control node has a cluster record":                                                                  "이 제어 노드에 클러스터 기록이 있는지 확인할 수 없습니다",
		"Not a new site — %s":           "새 사이트가 아닙니다 — %s",
		"Could not check the nodes: %s": "노드를 확인하지 못했습니다: %s",
		"Nodes of a cluster k8s-installer built (%s is there): %s. " +
			"It looks like a cluster built from another control node. Not resetting": "k8s-installer 가 만든 클러스터의 노드입니다 (%s 있음): %s" +
			". 다른 제어 노드에서 만든 클러스터로 보입니다. 초기화하지 않습니다",
		"Nodes not checked: %s. The reset needs every node shown to have no cluster record " +
			"(fix the connection, or put dead nodes in broken_node of the inventory)": "확인하지 못한 노드가 있습니다: %s" +
			". 모든 노드에 클러스터 기록이 없는 것을 확인해야 초기화합니다 (접속을 고치거나, 죽은 노드는 inventory 의 broken_node 로)",

		// sshkey.go
		"ansible-inventory failed: %v — %s":                   "ansible-inventory 실패: %v — %s",
		"could not read the output of ansible-inventory — %s": "ansible-inventory 출력을 읽지 못했습니다 — %s",
		"could not read the output of ansible-inventory: %v":  "ansible-inventory 출력을 읽지 못했습니다: %v",
		"could not read _meta of ansible-inventory: %v":       "ansible-inventory 의 _meta 를 읽지 못했습니다: %v",
		"Node %s sets %s. This check and sending the key do not use them, " +
			"so what they find may differ from what a playbook sees": "노드 %s 가 %s" +
			" 를 설정했습니다. 이 확인과 키 보내기는 그 값을 쓰지 않으므로 플레이북이 겪는 것과 다를 수 있습니다",
		"node %s has a name, address, port or user that is empty or has a space in it":                                  "노드 %s 의 이름·주소·포트·사용자 중 비었거나 공백이 든 값이 있습니다",
		"the address (%s) or the user (%s) of node %s starts with '-', which ssh reads as an option: fix the inventory": "노드 %[3]s 의 주소 (%[1]s) 나 사용자 (%[2]s) 가 '-' 로 시작합니다. ssh 가 옵션으로 읽으므로 inventory 를 고치세요",
		"key refused":                 "키 거부",
		"cannot connect":              "접속 불가",
		"host key changed":            "호스트 키 변경",
		"password login off":          "비밀번호 로그인 꺼짐",
		"could not check":             "확인 못 함",
		"control node key unreadable": "제어 노드 키 읽기 실패",
		"sudo not checked":            "sudo 미확인",
		"sudo needs a password":       "sudo 비밀번호 필요",
		"Does not take the key of the control node (~/.ssh/id_rsa). Send the key (k or p on the install screen, or --send-ssh-key)":                            "제어 노드의 키 (~/.ssh/id_rsa) 를 받지 않음. 키를 보내세요 (설치 화면 k·p, 또는 --send-ssh-key)",
		"When sshd refuses password login for root, the key has to be put on the node by hand":                                                                 "root 로 비밀번호 로그인을 sshd 가 막으면 키를 노드에 직접 넣어야 함",
		"Could not connect. Check the address, port, firewall and sshd":                                                                                        "접속하지 못함. 주소·포트·방화벽·sshd 를 확인하세요",
		"The host key differs from known_hosts. If the node really was reinstalled, run ssh-keygen -R %s, then check again":                                    "known_hosts 의 호스트 키와 다름. 노드를 다시 설치한 것이 맞으면 ssh-keygen -R %s 뒤 다시 확인하세요",
		"Does not take the key of the control node, and password login is off — the key has to be put there by hand (as configure-control-node-ssh.sh prints)": "제어 노드의 키를 받지 않고 비밀번호 로그인이 꺼져 있음 — 키를 직접 넣어야 함 (configure-control-node-ssh.sh 가 출력하는 안내)",
		"Not checked in time. Check again with --check-ssh":                                                                                                    "시간 안에 확인하지 못했습니다. --check-ssh 로 다시 확인하세요",
		"ssh cannot read the key of this control node (~/.ssh/id_rsa). Check the permissions and owner of the file (this user's, chmod 600)":                   "이 제어 노드의 키 (~/.ssh/id_rsa) 를 ssh 가 읽지 못함. 파일 권한·소유자를 확인하세요 (이 사용자 소유, chmod 600)",
		"The key of the control node gets in. sudo was not checked in time. Check again (c on the install screen, or --check-ssh)":                             "제어 노드 키로 접속은 됨. sudo 는 시간 안에 확인하지 못했습니다. 다시 확인하세요 (설치 화면 c, 또는 --check-ssh)",
		"User %s needs sudo without a password (NOPASSWD)":                                                                                                     "사용자 %s 에 비밀번호 없는 sudo (NOPASSWD) 가 필요합니다",
		"The key of the control node gets in and runs sudo. When only ansible cannot connect, check the ansible_ssh_* settings of the inventory":               "제어 노드 키로 접속·sudo 가 됨. ansible 만 접속하지 못하면 inventory 의 ansible_ssh_* 설정을 확인하세요",
		"Sending the key failed (%s).": "키 보내기 실패 (%s).",
		"%s is there but %s is not. Restore the public key from the private key, then run again: ssh-keygen -y -f %s > %s": "%s 는 있는데 %s 가 없습니다. 개인 키에서 공개 키를 되살린 뒤 다시 실행하세요: ssh-keygen -y -f %s > %s",
		"%s failed: %v — %s":             "%s 실패: %v — %s",
		"%s is still not there after %s": "%[2]s 뒤에도 %[1]s 가 없습니다",
		"── %s (%s@%s, port %s): sending the key. ssh asks for the password": "── %s (%s@%s, 포트 %s): 키를 보냅니다. 비밀번호는 ssh 가 묻습니다",
		"✗ %s: the key was not sent (see what ssh said above)":               "✗ %s: 키를 보내지 못했습니다 (위의 ssh 메시지 참고)",
		"Enter to go back to the screen":                                     "Enter 를 누르면 화면으로 돌아갑니다",
		"%s cannot be run: %v %s":                                            "%s 를 실행할 수 없음: %v %s",
		"no directory where the SSH_ASKPASS helper can run safely, so the key was not sent (check noexec mounts, owners and permissions): %s": "SSH_ASKPASS 도우미를 안전하게 실행할 수 있는 디렉터리가 없어 키를 보내지 않았습니다 (noexec 마운트, 소유자·권한 확인): %s",
		"%s is a symbolic link":                              "%s 가 심볼릭 링크입니다",
		"%s is not a directory":                              "%s 가 디렉터리가 아닙니다",
		"%s can be written by its group or other users (%v)": "%s 를 그룹이나 다른 사용자가 쓸 수 있습니다 (%v)",
		"the owner of %s (uid %d) is not this user (uid %d)": "%s 의 소유자 (uid %d) 가 이 사용자 (uid %d) 가 아닙니다",
		"ssh could not run the SSH_ASKPASS helper: %s":       "ssh 가 SSH_ASKPASS 도우미를 실행하지 못했습니다: %s",
		// the table of --check-ssh: one word a cell
		"NODE":              "노드",
		"ADDRESS":           "주소",
		"KEY":               "키",
		"NOTE":              "비고",
		"refused":           "거부",
		"unreachable":       "접속불가",
		"host-key-changed":  "호스트키변경",
		"no-password-login": "비밀번호로그인꺼짐",
		"unchecked":         "미확인",
		"key-unreadable":    "키읽기실패",
		"needs-password":    "비밀번호필요",
	})
}

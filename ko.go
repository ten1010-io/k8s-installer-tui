package main

import "github.com/ten1010-io/k8s-installer-tui/internal/i18n"

// The Korean of what this package says (i18n): keys are the English in the code.
func init() {
	i18n.Register(i18n.KO, map[string]string{
		"language of the screens and messages: en (default) or ko; %s=ko does the same": "화면과 메시지의 언어: en (기본) 또는 ko. %s=ko 도 같음",
		"skipped: %s": "건너뜀: %s",

		// flags
		"path of inventory.yml":                           "inventory.yml 경로",
		"path of vars.yml (k8s-installer)":                "vars.yml 경로 (k8s-installer)",
		"path of the AIPub site values (aipub-installer)": "AIPub 사이트 값 경로 (aipub-installer)",
		"headless: render the three files and the overview site-values.yml into this directory and print the validation result (existing files are left as they are)": "화면 없이 세 파일과 한눈에 보는 site-values.yml 을 이 디렉터리에 렌더링하고 검증 결과를 출력 (기존 파일은 그대로)",
		"directory k8s-installer is installed in (setup.sh)": "k8s-installer가 설치된 디렉터리 (setup.sh)",
		"headless: rebuild files/aipub/authentication-config.yaml of the AIPub authentication (JWKS) from the issuer and the CA and write it " +
			"to both the saved site and <installer-root>/ansible/files (running it again changes nothing). Fails when there is no CA": "화면 없이 AIPub 인증 (JWKS) 의 files/aipub/authentication-config.yaml 을 issuer 와 CA 로 다시 만들어 " +
			"저장한 사이트와 <installer-root>/ansible/files 양쪽에 씀 (다시 실행해도 같음). CA 가 없으면 실패",
		"JWKS CA file (default %s, put there by aipub-installer setup-cert)": "JWKS CA 파일 (기본 %s, aipub-installer setup-cert 가 둠)",
		"headless: print a table of whether every node of --inventory accepts the key (~/.ssh/id_rsa) of this control node and runs sudo without a password " +
			"(read only, known_hosts untouched). 0 all ready, 1 a node is not ready, 2 could not run": "화면 없이 --inventory 의 모든 노드가 이 제어 노드의 키 (~/.ssh/id_rsa) 를 받고 비밀번호 없이 sudo 하는지 " +
			"표로 출력 (읽기만, known_hosts 도 그대로). 0 모두 준비됨, 1 준비 안 된 노드 있음, 2 실행 못 함",
		"headless: send the key with ssh-copy-id to the nodes that do not accept it (ssh asks for each node's password) " +
			"and print the table of --check-ssh. If there is no key, it is made with <installer-root>/ansible/configure-control-node-ssh.sh. Ctrl-C 130, SIGTERM 143, SIGHUP 129": "화면 없이 키를 받지 않는 노드에 ssh-copy-id 로 키를 보내고 (비밀번호는 노드마다 ssh 가 물음) " +
			"--check-ssh 의 표를 출력. 키가 없으면 <installer-root>/ansible/configure-control-node-ssh.sh 로 만듦. Ctrl-C 130, SIGTERM 143, SIGHUP 129",
		"with --send-ssh-key, ask for the password once on the terminal (not shown) and use it for every node": "--send-ssh-key 에서 비밀번호를 터미널에서 한 번만 물어 (화면에 보이지 않음) 모든 노드에 씀",
		"headless: run the install. After the checks (P1..P8, it does not start on a ✗) run the steps of the plan in turn and " +
			"wait for Enter at the StorageClass step (M1). State and logs go to <installer-root>/tui-install. " +
			"Ctrl-C once stops after the running step, twice stops at once. Exit code 0 done, 1 failed, 2 could not start, 130 interrupted": "화면 없이 설치를 실행: 사전 확인 (P1..P8, ✗ 이면 시작 안 함) 뒤 설치 계획의 단계를 차례로 실행하고 " +
			"StorageClass 단계 (M1) 에서 Enter 를 기다림. 상태·로그는 <installer-root>/tui-install. " +
			"Ctrl-C 한 번 실행 중인 단계 뒤 멈춤, 두 번 바로 멈춤. 종료 코드 0 완료, 1 실패, 2 시작 못 함, 130 중단",
		"with --install, skip the steps that succeeded in the last run (state.json) and resume " +
			"(the plan uses --existing, --aipub-installed and --no-monitoring of the last run. --archive is the last run's if its K4 succeeded; " +
			"otherwise this run's --archive is used, starting again from K4)": "--install 에서 지난 실행 (state.json) 의 성공한 단계는 건너뛰고 이어서 실행 " +
			"(계획은 지난 실행의 --existing·--aipub-installed·--no-monitoring 으로. --archive 는 지난 실행의 K4 가 성공했으면 그때 것, " +
			"아니면 이번 것으로 K4 부터)",
		"--install: run every step from the start even if a last run did not finish (without it, --install does not start). " +
			"The site is judged by its state now (%s)": "--install: 끝나지 않은 지난 실행이 있어도 처음부터 모든 단계를 실행 (없으면 --install 은 시작하지 않음). " +
			"사이트는 지금 상태로 판단 (%s)",
		"plan for a site that already has a cluster (wizard and --install). Without it, the site is an existing one only when %s " +
			"(the record k8s-installer leaves after it creates the cluster) is there. --resume uses the last run's": "클러스터가 이미 있는 사이트로 계획 (마법사와 --install). 없으면 %s" +
			" (k8s-installer 가 클러스터를 만든 뒤 남기는 기록) 이 있을 때만 기존 사이트. --resume 은 지난 실행의 것을 씀",
		"plan for an existing site that has AIPub installed too (wizard and --install. " +
			"The CA of setup-cert is on the nodes and the JWKS issuer answers: the control plane entries are applied in one update-cluster). " +
			"aipub-installer leaves no install record, so only this flag decides. Default is no AIPub": "기존 사이트에 AIPub 도 이미 설치된 것으로 계획 (마법사와 --install. " +
			"setup-cert 의 CA 가 노드에 있고 JWKS issuer 가 응답함: 컨트롤 플레인 항목을 update-cluster 한 번에 적용). " +
			"aipub-installer 는 설치 기록을 남기지 않아 이 플래그로만 정함. 기본은 AIPub 없음",
		"--install: path of the user registry archive (%s) for K4. " +
			"Needed for a new site that installs AIPub (it does not start without it, --no-archive). On an existing site, leaving it empty skips K4": "--install: K4 의 사용자 레지스트리 아카이브 (%s) 경로. " +
			"AIPub 을 설치하는 새 사이트에는 필요 (없으면 시작하지 않음, --no-archive). 기존 사이트는 비우면 K4 건너뜀",
		"--install: start a new site that installs AIPub without --archive (skips K4: the registry is empty and the steps from A1 on may fail)": "--install: AIPub 을 설치하는 새 사이트를 --archive 없이 시작 (K4 건너뜀: 레지스트리가 비어 A1 부터 실패할 수 있음)",
		"--install: do not install monitoring (A4)": "--install: 모니터링 (A4) 을 설치하지 않음",
		"print the version":                         "버전 출력",

		// main.go
		"load error: %v": "로드 오류: %v",
		"--aipub-installed is for an existing site only (AIPub cannot be there without a cluster): %s": "--aipub-installed 는 기존 사이트에만 씁니다 (클러스터 없이 AIPub 이 있을 수 없음): %s",
		"--resume and --fresh cannot be used together":                                                 "--resume 과 --fresh 는 함께 쓸 수 없습니다",
		"error: %v":        "오류: %v",
		"render error: %v": "렌더링 오류: %v",
		"JWKS: waiting for the CA (%s is not there) — %s is not written, --fill-auth-config of the install plan makes it after setup-cert": "JWKS: CA 대기 (%s 없음) — %s 는 쓰지 않음, 설치 계획의 --fill-auth-config 가 setup-cert 뒤에 만듦",
		"JWKS: CA file error: %v":      "JWKS: CA 파일 오류: %v",
		"✓ validation passed":          "✓ 검증 통과",
		"preparing the key failed: %v": "키 준비 실패: %v",
		"! %s is not there: no node is ready. --send-ssh-key makes it and sends it": "! %s 가 없습니다: 어느 노드도 준비되지 않았습니다. --send-ssh-key 가 만들고 보냅니다",
		"listing the nodes failed: %v":                                              "노드 목록 실패: %v",
		"no nodes in %s":                                                            "%s 에 노드가 없습니다",
		"the check failed: %v":                                                      "확인 실패: %v",
		"── sending the key (%d nodes)":                                             "── 키 보내는 중 (%d대)",
		"key accepted":                                                              "키 받음",
		"sending the key failed: %v":                                                "키 보내기 실패: %v",
		"✗ some nodes are not ready. Fix them as the notes say and run again":                "✗ 준비되지 않은 노드가 있습니다. 비고대로 고친 뒤 다시 실행하세요",
		"✓ every node accepts the key of this control node and runs sudo without a password": "✓ 모든 노드가 이 제어 노드의 키를 받고 비밀번호 없이 sudo 합니다",
		"stopped (exit code %d)":                       "중단했습니다 (종료 코드 %d)",
		"a terminal is needed to ask for the password": "비밀번호를 물을 터미널이 필요합니다",
		"SSH password of the nodes (not shown): ":      "노드의 SSH 비밀번호 (화면에 보이지 않음): ",
		"the password is empty":                        "비밀번호가 비어 있습니다",
		"path error: %v":                               "경로 오류: %v",

		// install.go
		"state.json: %v — to start over, use --fresh": "state.json: %v — 처음부터 다시 하려면 --fresh",
		"the last run (started at %s) did not finish. Steps left: %s\n" +
			"To resume, add --resume; to run every step again from the start, add --fresh": "지난 실행 (%s 에 시작) 이 끝나지 않았습니다. 남은 단계: %s\n" +
			"이어서 하려면 --resume, 끝난 단계까지 처음부터 다시 하려면 --fresh 를 붙이세요",
		"%s/state.json is not there, so there is no run to resume. Run without --resume":                                                      "%s/state.json 이 없어 이어서 할 실행이 없습니다. --resume 없이 실행하세요",
		"! a resumed run plans with the settings of the last run (different from the flags now): %s. To change them, start over with --fresh": "! 이어서 실행은 지난 실행의 설정으로 계획합니다 (이번 플래그와 다름): %s. 바꾸려면 --fresh 로 처음부터",
		"── K4 did not finish in the last run: running from K4 with this run's --archive: %s":                                                 "── K4 가 끝나지 않은 실행: 이번 --archive 로 K4 부터 실행합니다: %s",
		"── resuming (the run started at %s). Steps already done: %s":                                                                         "── 이어서 실행 (%s 에 시작한 실행). 이미 끝난 단계: %s",
		"! the site files changed since the last run. Steps already done are not run again. Continue? [y/N] ":                                 "! 지난 실행 뒤에 사이트 파일이 바뀌었습니다. 끝난 단계는 다시 실행하지 않습니다. 계속할까요? [y/N] ",
		"not started":                       "시작하지 않았습니다",
		"✗ %v. Fix --archive and run again": "✗ %v. --archive 를 고친 뒤 다시 실행하세요",
		"✗ this is a new site but there is no --archive. The user registry K2 makes is empty, so without K4 (push-user-registry-artifacts) " +
			"the images cannot be pulled and the steps from A1 setup-network on fail.\n" +
			"  Add --archive <path of %s>. To run without K4 anyway, use --no-archive": "✗ 새 사이트인데 --archive 가 없습니다. K2 가 만드는 사용자 레지스트리는 비어 있어, K4 (push-user-registry-artifacts) 없이는 " +
			"A1 setup-network 부터 이미지를 받지 못해 실패합니다.\n" +
			"  --archive <%s 의 경로> 를 붙이세요. 알고도 K4 없이 하려면 --no-archive",
		"  found: %s": "  찾은 파일: %s",
		"── stopping when the running step ends (Ctrl-C once more stops at once)": "── 실행 중인 단계가 끝나면 멈춥니다 (한 번 더 Ctrl-C 면 바로 멈춤)",
		"── stopping the running step":                                            "── 실행 중인 단계를 멈춥니다",
		"── site: %s":                                                             "── 사이트: %s",
		"── checks":                                                               "── 사전 확인",
		"✗ checks failed: %s. Fix them and run again":                             "✗ 사전 확인 실패: %s. 고친 뒤 다시 실행하세요",
		"── steps (| next step, + run together): %s":                              "── 단계 (| 다음 단계, + 함께 실행): %s",
		"── no input, stopping (the StorageClass step is answered with Enter)":    "── 입력이 없어 멈춥니다 (StorageClass 단계는 Enter 로 답합니다)",
		"✗ could not start: %v":                                                   "✗ 시작하지 못했습니다: %v",
		"! could not write %s/state.json (%v). The record may be behind the run":  "! %s/state.json 을 쓰지 못했습니다 (%v). 기록이 실행보다 뒤처졌을 수 있습니다",
		"--archive %s (K4 done)":                                                  "--archive %s (K4 끝남)",
		"(log %s)":                                                                "(로그 %s)",
		"When installed, press Enter (installed, continue). Ctrl-C stops":         "설치했으면 Enter (설치함, 계속). Ctrl-C 멈춤",
		" (exit code %d)":                                                         " (종료 코드 %d)",
		"failed":                                                                  "실패",
		"✓ install finished (%s)":                                                 "✓ 설치를 마쳤습니다 (%s)",
		"■ stopped. --install --resume resumes from here":                         "■ 멈췄습니다. --install --resume 로 이어서 실행합니다",
		"■ ran to the end, but the steps below stay failed. After fixing the cause, --install --resume runs only those steps again": "■ 끝까지 실행했지만 아래 단계는 실패로 남았습니다. 원인을 푼 뒤 --install --resume 로 그 단계만 다시 실행합니다",
		"✗ failed. After fixing it, --install --resume resumes from the failed step":                                                "✗ 실패했습니다. 고친 뒤 --install --resume 로 실패한 단계부터 이어서 실행합니다",
		"✗ %s %s  log: %s": "✗ %s %s  로그: %s",
		"Steps left: %s":   "남은 단계: %s",
	})
}

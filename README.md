# k8s-installer-tui

[k8s-installer](https://github.com/ten1010-io/k8s-installer)와 aipub-installer의 설치 전 설정 파일 3개를 터미널 UI(TUI)로 작성하고, 저장한 설정으로 설치 순서를 보여 주는 도구입니다.

| 파일 | 읽는 쪽 |
|------|---------|
| `inventory.yml` | k8s-installer |
| `group_vars/all/vars.yml` | k8s-installer |
| `files/aipub/vars.yml` | aipub-installer (AIPub을 설치할 때만) |

## 사용 흐름

```
1. 이 레포에서 Go 소스 코드 유지 및 배포
        ↓
2. 서버(또는 CI)에서 빌드 → 바이너리 패키지로 저장
        ↓
3. k8s-installer의 download-bin.sh로 바이너리 다운로드
        ↓
4. 설치 담당자가 TUI 실행 → 설정 파일 3개 생성
        ↓
5. 9단계 설치 계획 화면의 순서대로 k8s-installer·aipub-installer 플레이북 실행
```

## 빌드와 테스트

### 요구 사항

- Go 1.21 이상 (`go.mod` 기준)
- `make`
- 인터넷 연결 (첫 빌드 시 의존성 다운로드)

### 빠른 시작 — `set-env.sh` (Ubuntu/Debian 서버)

Go와 `make`가 없는 서버에서 환경 세팅 + 빌드를 한 번에 수행합니다.

```bash
bash set-env.sh
```

`apt install -y make` → Go 1.24.3(linux-amd64) 설치(`/usr/local/go`) → `PATH` 등록(`/etc/profile`) → `make build` 순서로 동작하며 Ubuntu/Debian 계열에서만 동작합니다.

### 명령어

```bash
make build              # go mod tidy 후 현재 OS용 바이너리 -> dist/k8s-installer-tui
make build-linux        # Linux amd64 -> dist/k8s-installer-tui-linux-amd64
make build-linux-arm64  # Linux arm64 -> dist/k8s-installer-tui-linux-arm64
make clean              # dist/ 삭제
```

`make build`는 먼저 `go mod tidy`를 실행합니다. `go.sum`은 의도적으로 `.gitignore`에 있어 커밋하지 않고, 이 과정에서 만들어집니다.

### 테스트

```bash
go test ./...
```

`internal/fileio`(읽기·쓰기·구버전 파일 이전), `internal/plan`(설치 순서), `internal/ui`(화면 흐름) 테스트가 있습니다.

설치 도구와 함께 렌더링을 확인하는 시뮬레이션은 Windows에서 다음과 같이 실행합니다.

```powershell
powershell -File test\simulate\run.ps1          # -Keep <dir> 로 렌더링 결과를 남김
```

Docker와 aipub-installer의 `tools/chart-images`가 만드는 `aipub-chart-images` 이미지가 필요하고, `k8s-installer`, `aipub-installer`가 이 레포와 같은 상위 디렉터리에 있어야 합니다. `test/simulate/fixtures/`의 각 사이트(`legacy-0.1`, `new-site-non-ha`)를 `--render`로 렌더링한 뒤 두 설치 도구의 검증과 차트 렌더링에 통과하는지 확인합니다.

SSH 키 배포(`--check-ssh`, `--send-ssh-key`)는 진짜 sshd가 도는 버리는 컨테이너(ubuntu:24.04의 OpenSSH 9.6, rockylinux:8의 OpenSSH 8.0)에서 확인합니다. 이 PC나 다른 호스트에는 접속하지 않습니다.

```powershell
powershell -File test\sshkeys\run.ps1                 # -K8sInstaller <checkout> -Images ubuntu:24.04
```

Docker와 네트워크(패키지, PyPI의 ansible-core)가 필요합니다. 이미지마다 ansible-core 2.17.x와 2.19.x로 한 번씩, `/tmp`를 noexec tmpfs로 둔 채 돌립니다. `configure-control-node-ssh.sh`는 이 레포 옆의 `k8s-installer`에서 읽기 전용으로 마운트합니다.

## 실행

```bash
# 기본 실행 (현재 디렉터리 기준)
./k8s-installer-tui

# 경로 직접 지정
./k8s-installer-tui \
  --inventory /opt/k8s-installer/ansible/inventory.yml \
  --vars /opt/k8s-installer/ansible/group_vars/all/vars.yml \
  --aipub-vars /opt/k8s-installer/ansible/files/aipub/vars.yml
```

| 플래그 | 기본값 | 설명 |
|--------|--------|------|
| `--inventory` | `inventory.yml` | inventory.yml 경로 |
| `--vars` | `group_vars/all/vars.yml` | k8s-installer의 vars.yml 경로 |
| `--aipub-vars` | `files/aipub/vars.yml` | aipub-installer의 사이트 값 경로 |
| `--installer-root` | `/opt/k8s-installer` | `setup.sh`로 k8s-installer를 설치한 디렉터리 (9단계에서 사용) |
| `--render <dir>` | (없음) | 화면 없이 저장될 세 파일을 `<dir>`에 렌더링하고 검증 결과를 출력. 기존 파일은 그대로 둠 |
| `--check-ssh` | | 화면 없이 `--inventory`의 모든 노드가 이 제어 노드의 키를 받고 비밀번호 없이 sudo 하는지 표로 출력 (읽기만). 아래 "SSH 키 배포" 참고 |
| `--send-ssh-key` | | 화면 없이 키를 받지 않는 노드에 키를 보내고 같은 표를 출력. 비밀번호는 노드마다 ssh가 물음 |
| `--ask-pass-once` | | `--send-ssh-key`와 함께: 비밀번호를 터미널에서 한 번만(화면에 보이지 않게) 물어 모든 노드에 씀 |
| `--install` | | 화면 없이 설치 실행: 사전 확인 뒤 9단계의 단계를 차례로 실행. 아래 "설치 실행" 참고 |
| `--resume` | | `--install`과 함께: 지난 실행에서 성공한 단계는 건너뛰고 이어서 실행. 계획은 지난 실행의 `--existing`·`--aipub-installed`·`--no-monitoring`으로 하고, 이번 플래그와 다르면 알려 줌. `--archive`는 지난 실행의 K4가 성공했으면 그때 것, 아니면(건너뜀·실패) 이번 `--archive`로 K4부터 실행 |
| `--fresh` | | `--install`과 함께: 끝나지 않은 지난 실행이 있어도 처음부터 모든 단계를 실행. 사이트는 지금 상태로 판단(지난 실행이 아니라). 없이 `--install`만 쓰면 그런 실행이 있을 때 시작하지 않음(종료 코드 2) |
| `--existing` | | 마법사와 `--install`: 클러스터가 이미 있는 사이트로 계획. 없으면 이 제어 노드에 `/etc/k8s-installer/applied-release`가 있을 때만 기존 사이트(아래 "기존 사이트 판정") |
| `--aipub-installed` | | 마법사와 `--install`, 기존 사이트에서: AIPub도 이미 설치된 사이트로 계획(setup-cert의 CA가 노드에 있고 JWKS issuer가 응답: 컨트롤 플레인 항목을 update-cluster 한 번에 적용). 기본은 AIPub 없음. aipub-installer는 설치를 마쳤다는 기록을 남기지 않으므로 이 플래그로만 정함(`files/aipub/vars.yml`은 위저드가 AIPub 사이트마다 씀). 새 사이트에 주면 시작하지 않음 |
| `--archive <path>` | (없음) | `--install`과 함께: K4의 사용자 레지스트리 아카이브(`ki-cp-user-registry-artifacts.tgz`). AIPub을 설치하는 새 사이트에는 필요: 없으면 시작하지 않음(종료 코드 2). 읽을 수 있는 일반 파일이 아니면 시작하지 않음. 기존 사이트·AIPub 없음은 비우면 K4 건너뜀 |
| `--no-archive` | | `--install`과 함께: AIPub을 설치하는 새 사이트를 `--archive` 없이 시작(K4 건너뜀. 레지스트리가 비어 A1부터 이미지를 받지 못할 수 있음을 알고 쓸 때) |
| `--no-monitoring` | | `--install`과 함께: 모니터링(A4)을 설치하지 않음 |
| `--version` | | 버전 출력 |

`--render`의 종료 코드: `0` 검증 통과, `2` 검증 오류 있음(파일은 렌더링됨), `1` 로드·렌더링·쓰기 오류. 렌더링 결과는 `<dir>` 아래에 k8s-installer의 `ansible` 디렉터리 배치(`inventory.yml`, `group_vars/all/vars.yml`, `files/aipub/vars.yml`)로 쓰며, AIPub을 설치하지 않으면 `files/aipub/vars.yml`은 만들지 않습니다.

### 기존 사이트 판정

계획(K1~K3을 실행할지, P3, 감사 정책 기본값을 쓸지)은 "클러스터가 이미 있는가"로 갈립니다. 이것은 사이트 파일이 있는지로 정하지 않습니다. `setup.sh`가 `/opt/k8s-installer/ansible`에 템플릿 vars.yml을 두고, 한 번 저장한 뒤 설치 전에 다시 연 사이트도 파일이 모두 있기 때문입니다. 대신:

- `--existing`을 주면 기존 사이트.
- 아니면 이 제어 노드의 `/etc/k8s-installer/applied-release`가 있을 때만 기존 사이트. k8s-installer는 setup-cluster·update-cluster·add-node·upgrade-cluster가 성공한 마지막에 이 파일을 모든 노드(제어 노드 포함)에 쓰고(`tasks/record-applied-state.yml`), reset-cluster·remove-node가 지웁니다(`tasks/reset-cluster-record.yml`). 같은 때 쓰는 `applied-vars.yml`은 reset-cluster 뒤에도 일부러 남으므로 쓰지 않습니다.
- 그 파일을 확인할 수 없으면(권한) 기존이라고 짐작하지 않고 새 사이트로 계획하며, 그 이유를 보입니다.
- AIPub 설치 여부는 `--aipub-installed`로만 정합니다.

9단계 화면과 `--install`은 판정과 이유를 한 줄로 보입니다. 예: `새 사이트: /etc/k8s-installer/applied-release 없음 (클러스터가 이미 있으면 --existing 으로 시작)`, `기존 사이트: /etc/k8s-installer/applied-release 있음 (...)`, `기존 사이트: --existing 으로 지정`. `--render`도 같은 판정으로 감사 정책 기본값을 쓸지 정합니다.

### 기존 파일이 있는 경우

기존 파일이 있으면 읽어서 편집 모드로 시작합니다. 저장은 새로 쓰지 않고 기존 YAML을 그 자리에서 고칩니다(`internal/fileio/yamldoc.go`). 이 도구가 다루지 않는 변수와 주석은 그대로 남고, 템플릿에는 있고 사이트 파일에는 없는 변수는 주석과 함께 추가됩니다. 파일은 통째로 새로 써서 이름을 바꿔 넣으므로(rename) 중간에 끊겨도 반쯤 쓴 파일이 남지 않습니다.

이력은 저장한 파일 옆 `history.jsonl` 하나에 한 줄씩 덧붙입니다(덧붙이기만 함, 0600). `.bak` 대신입니다.

| kind | 언제 | 내용 |
|---|---|---|
| `save` | 저장할 때마다, 바뀐 파일만 | 쓴 내용 |
| `edit` | 저장 전 파일이 이력의 마지막과 다를 때 (손으로 고쳤거나 이력 전부터 있던 파일) | 덮어쓰기 전 내용 |
| `copy` | 설치 실행의 C1 이 설치기(`<installer-root>/ansible`)로 복사한 뒤 | 복사한 내용 |
| `installer` | C1 이 복사하기 전 설치기 쪽 파일이 사이트와도, 마지막 복사와도 다를 때 | 덮어쓰기 전 설치기 쪽 내용 |

C1 은 설치기 쪽 파일이 이력에 없는 내용이면(마지막 복사 뒤 손으로 고침) 그 내용을 `installer` 로 남기고 한 번 멈춥니다. 확인한 뒤(사이트에 반영하거나 버려도 되면) 다시 실행하면 덮어씁니다. 이력에 있는 내용(예전 저장, 스크립트로 한 복사, 되돌린 수정)이면 멈추지 않습니다. 첫 복사 때 설치기에 있던 파일(설치기 템플릿 등)은 멈추지 않고 이력에만 남깁니다. 이 사용자가 읽을 수 없는 설치기 쪽 파일은 확인하지 못하고 덮어쓰며, C1 로그에 그렇게 적습니다. `--fill-auth-config` 가 설치기 쪽 인증 설정을 쓸 때도 이전 내용과 복사를 이력에 남깁니다. 설치기 디렉터리에 바로 저장하면 C1 이 없고, `upgrade.sh` 가 ansible 디렉터리를 바꿀 때 `history.jsonl` 도 지워집니다.

UTF-8 이 아닌 파일은 `"encoding":"base64"` 로 바이트 그대로 남깁니다. 이력에는 바꾸기 전의 토큰·비밀번호도 남으므로(0600), 필요 없어진 이력은 지워도 됩니다(없으면 다음 저장부터 새로 시작).

```bash
# 마지막으로 저장한 vars.yml (끊긴 줄은 건너뜀)
jq -nrR '[inputs | fromjson? | select(.kind=="save" and .path=="group_vars/all/vars.yml")] | last | .content' history.jsonl
# 무엇이 언제 바뀌었는지
jq -rR 'fromjson? | [.time, .kind, .path, .sha256[0:12]] | @tsv' history.jsonl
```

새 사이트는 `internal/fileio/templates/`에 내장한 설치 도구의 예제 파일에서 시작합니다. 이전 버전이 쓴 파일(`k8s_load_balancers`, `ki_cert_mode`, vars.yml에 있던 `aipub_*`, `ansible_ssh_user`, `nvidia_gpu` 등)은 읽어서 새 위치로 옮겨 저장합니다. 구버전 변수 `aipub_java_*`를 쓴 사이트 파일도 그대로 읽히고 값이 보존됩니다.

## 키 조작

| 키 | 동작 |
|----|------|
| `Ctrl+N` / `Ctrl+P` | 다음 / 이전 단계 (다음으로 갈 때 현재 단계 검증) |
| `PgDn` / `PgUp` | 다음 / 이전 단계 |
| `Ctrl+C` | 종료 |
| `Tab` / `Shift+Tab`, `↑` / `↓` | 필드·항목 이동 |
| `←` / `→` | 열·탭 전환, 하단 버튼 선택 |
| `Space` / `Enter` | 토글·선택·편집 |
| `s` | 8단계 미리보기에서 저장 |
| `c` / `d` / `w` | 9단계: 사전 확인 / 명령 보기 / 스크립트 저장 |
| `k` / `p` | 9단계: P2에서 키를 받지 않는 노드가 있을 때만, 키 보내기 (ssh가 노드마다 비밀번호를 물음 / 비밀번호 한 번 입력) |

## 9단계 Wizard

```
[1.노드] → [2.그룹] → [3.네트워크] → [4.Kubernetes] → [5.AIPub] → [6.AIPub LB] → [7.인증서] → [8.미리보기·저장] → [9.설치 계획]
```

AIPub을 설치하지 않으면 AIPub 전용 화면(6·7단계)은 건너뜁니다.

| 단계 | 화면 | 파일 | 내용 |
|------|------|------|------|
| 1 | 노드 (`s1_nodes.go`) | `inventory.yml` | 노드 이름, `ansible_host`, `ansible_port`, SSH 사용자 |
| 2 | 그룹 (`s2_groups.go`) | `inventory.yml` | 노드별 `ki_cp_node` / Master(`k8s_node` 호스트의 `k8s_cp: true`) 지정. 워커는 Master가 아닌 노드. GPU 열은 없음(k8s-installer가 `nvidia_gpu`를 없애고 노드에서 찾은 GPU를 CDI로 전달) |
| 3 | 네트워크 (`s3_network.go`) | `vars.yml` | 내부 서브넷, ki cp HA 모드와 VIP, DNS·NTP upstream, DNSSEC, 내부 DNS 서버(AIPub 도메인을 인그레스 VIP로 해석) |
| 4 | Kubernetes (`s4_kubernetes.go`) | `vars.yml` | 마이너 버전, 파드·서비스 서브넷, 인증서 유효기간 |
| 5 | AIPub (`s5_aipub.go`) | `files/aipub/vars.yml`, `inventory.yml` | 설치 여부, 인그레스 도메인, HA 모드와 StorageClass(HA 아니면 Harbor 데이터 노드), AIPub 서비스가 실행될 노드(노드 레이블), Harbor 서브도메인·레플리카·스토리지 크기 |
| 6 | AIPub 로드밸런서 (`s6_aipub_lb.go`) | `files/aipub/vars.yml` | 인그레스 모드(별도 IP / 공유 IP), Ingress·Gateway LB 이름과 VIP, 호스트 네트워크 포트 |
| 7 | 인증서 모드 (`s6_cert_mode.go`) | `files/aipub/vars.yml` | `aipub_cert_mode`: `self_signed`(기본, CA 자동 생성), `ca_provided`(CA 제공), `tls_provided`(TLS 인증서 제공) |
| 8 | 미리보기 (`s7_preview.go`) | 3개 모두 | 저장될 파일을 탭으로 보고 전체 검증 결과 확인. 오류가 있으면 저장 불가, `s`로 저장 |
| 9 | 설치 계획 (`s9_install.go`) | | 아래 참고 |

(파일 이름 `s6_cert_mode.go`, `s7_preview.go`는 화면이 하나 늘기 전의 번호가 남은 것으로, 화면 순서는 위 표를 따릅니다.)

### 검증 규칙

저장 전 `internal/validator/rules.go`의 규칙을 통과해야 합니다. 주요 규칙은 다음과 같습니다.

| # | 규칙 |
|---|------|
| R1 | ki cp 노드의 VIP는 `internal_network_subnets` 안에 있어야 함 |
| R2 | `ki_cp_ha_mode=true`이면 `ki_cp_ha_mode_vip` 필수 |
| R5 | `aipub_ha_mode=true`이면 StorageClass 필수, 아니면 Harbor 데이터 노드 필수 |
| R6 | AIPub 서비스 노드는 k8s control plane 노드의 부분집합이어야 함 |
| R7 | AIPub LB VIP(인그레스, 별도 IP 모드의 게이트웨이)는 `ki_cp_ha_mode_vip`(HA일 때)·노드의 `ansible_host`(IP일 때)와 다르고, 파드·서비스 서브넷 밖이어야 함. `internal_network_subnets` 밖이면 오류가 아니라 8단계 미리보기의 경고(두 번째 NIC의 대역일 수 있음) |

그 밖에 형식(CIDR, IP, 크기 등)과 필수 항목(`k8s_node` 최소 1개 등)도 같은 곳에서 검사합니다.

### 9단계 — 설치 계획 (`internal/plan`)

저장한 설정으로 실행해야 할 순서를 보여 줍니다. 순서는 `internal/plan/plan.go` 한 곳에 있어, 설치 도구의 절차가 바뀌면 그 표만 고칩니다.

| ID | 단계 | 종류 |
|----|------|------|
| P1 | `setup.sh` 완료 확인 | 확인 |
| P2 | 모든 노드에 SSH·sudo 접속 | 확인 |
| K1 | `setup-k8s-installer` (기존 사이트: 건너뜀) | 플레이북 |
| K2 | `setup-control-node` (기존 사이트: 건너뜀) | 플레이북 |
| K3 | `setup-cluster` (기존 사이트: `update-cluster`, 또는 건너뜀) | 플레이북 |
| K4 | `push-user-registry-artifacts` (아카이브 경로를 넣었을 때. AIPub을 설치하는 새 사이트에는 필요) | 플레이북 |
| A1 | `setup-network` | 플레이북 (AIPub) |
| A2 | `setup-cert` | 플레이북 (AIPub) |
| M1 | StorageClass 드라이버 설치 (A2와 함께, 쓰는 클래스가 없으면 건너뜀) | 수동 (AIPub) |
| A3 | `setup-base` | 플레이북 (AIPub) |
| A4 | `setup-monitoring` (선택) | 플레이북 (AIPub) |
| A5 | `setup-aipub` | 플레이북 (AIPub) |

A1~A5는 AIPub을 설치할 때만 나옵니다. 화면에서 K4의 아카이브 경로와 모니터링 설치 여부를 정합니다.

K4의 아카이브(`ki-cp-user-registry-artifacts.tgz`, 패키지에 들어 있음)는 새 사이트에서 빼면 안 됩니다. 사용자 레지스트리는 K2가 만들므로 비어 있고, 아카이브를 넣지 않으면 A1 `setup-network`부터 이미지를 받지 못합니다(Cilium 파드가 `ImagePullBackOff`, helm이 15분 기다리다 실패). 그래서 AIPub을 설치하는 새 사이트에서 아카이브 경로가 비어 있으면 `실행`·`f`는 바로 시작하지 않고 묻습니다(기본은 아니오, `Enter`도 아니오). 넣은 경로가 읽을 수 있는 일반 파일이 아니면 시작하지 않습니다. 아카이브 경로가 비어 있으면 화면이 이 프로그램의 디렉터리, 작업 디렉터리, 홈 디렉터리 한 단계 아래(`~/<패키지>/`)에서 그 파일을 찾아, 하나면 채우고 어디서 찾았는지 말하고, 여럿이면 목록만 보입니다. 기존 사이트와 AIPub을 설치하지 않는 사이트는 지금처럼 비워 두면 K4를 건너뜁니다. 이어서 실행하는데 지난 실행의 K4가 이미 성공했으면 아무것도 묻지 않습니다.

기존 사이트(위 "기존 사이트 판정")에서는 K1·K2를 계획에 남기되 건너뜁니다: 노드의 설치본과 ki cp 서비스는 이미 있고, 올리는 것은 이 계획이 아니라 `upgrade-k8s-installer.yml`입니다. K3은 setup-cluster가 아닙니다(살아 있는 클러스터에서는 `init-k8s-cluster.sh`가 "Kubelet already enabled"로 거부). 뒤의 한 단계가 컨트롤 플레인 항목을 한 번에 적용하면(AIPub 없는 사이트의 K5, AIPub이 설치된 사이트의 A2.5) K3은 건너뛰고, 그렇지 않으면(기존 클러스터에 AIPub을 지금 설치) K3은 클러스터 단계 항목(감사 로그, runtime-config)을 AIPub 단계 전에 적용하는 `update-cluster.yml`이며, 그 단계에 적용할 항목이 없으면 건너뜁니다. ID는 그대로 K3이라 `state.json`과 로그의 이름이 바뀌지 않습니다.

컨트롤 플레인 항목(`k8s_cp_extra_files` 등) 중 뒤 단계가 만드는 것을 읽는 항목(인증 웹훅: setup-cert의 CA, JWKS: setup-aipub의 issuer와 A5.1이 채우는 `files/aipub/authentication-config.yaml`)이 있으면, 모든 플레이북이 그때까지 적용된 목록을 `-e '{...}'`로 받습니다: K3 전부터 A2.5까지는 클러스터 단계, A2.5가 JWKS를 빼고 적용했으면 그다음부터 그 단계, 모두 적용한 단계(A5.2, A2.5, K5) 뒤로는 `-e` 없이 vars.yml 그대로. k8s-installer가 `prepare-vars.yml`을 읽는 모든 플레이북(setup-control-node, push-user-registry-artifacts, aipub 플레이북)에서 `k8s_cp_extra_files`의 파일이 `ansible/files`에 있는지 확인하기 때문입니다(`validate-hostvars.py`). K1은 그 확인을 하지 않지만 같은 규칙으로 같은 `-e`를 받습니다. 뒤로 미룰 항목이 없으면(AIPub 없음, 이미 AIPub이 설치되어 한 번에 적용) `-e`는 없습니다. `install-plan.sh`와 러너는 같은 인자를 씁니다.

| 키 | 동작 |
|----|------|
| `c` | 사전 확인 실행: P1(설치 디렉터리), P2(모든 노드에 `sudo -n true`) |
| `d` | 각 단계의 명령 보기 |
| `w` | 명령을 `install-plan.sh`로 저장 (설정을 저장한 디렉터리에) |
| `f` | 지난 실행이 있을 때: 처음부터 다시 실행. 사이트를 지금 상태로 다시 판단합니다(지난 실행이 끝나지 않았어도: K3가 클러스터를 만들었으면 기존 사이트 계획이라 setup-cluster를 다시 돌리지 않음). `실행`과 같이 사전 확인을 거친 뒤(✗면 안 함) 확인을 묻습니다 |
| `x` | 클러스터 초기화(`reset-cluster.yml`). **새 사이트에서 K1~K3 중 하나가 실패해 끝난 실행 뒤에만** 나옵니다(이전 설치 잔재, 이미 켜진 kubelet처럼 클러스터를 만드는 중에 막힌 경우). K3 뒤 단계가 실패했을 때, 기존 사이트, 실행 중에는 없고, 이 제어 노드의 클러스터 기록을 확인해 없을 때만 나옵니다. `reset`을 입력하고 `Enter`해야 실행하고, 그 밖의 입력은 취소입니다 |
| `k` | 키를 받지 않는 노드에 키 보내기: 화면이 터미널을 넘기고, 노드마다 `ssh-copy-id`가 비밀번호를 물음. 끝나면 Enter로 돌아옴 |
| `p` | 키를 받지 않는 노드에 키 보내기: 화면의 가림 입력칸에 비밀번호를 한 번 넣음. 노드별 진행이 P2 아래에 나옴. `Ctrl+C`로 중단. 중단 중에 한 번 더 누르면 보내기가 멈출 때까지 "정리 중…"을 보인 뒤 종료(그때 한 번 더 누르면 바로 종료하고, 정리는 프로그램이 끝나기 전에 마저 함) |

`k`·`p`는 P2가 키를 받지 않는 노드(키 거부)를 찾았을 때만 P2 아래 한 줄 안내와 함께 나옵니다. 둘 다 끝나면 P2를 자동으로 다시 확인합니다. P2는 ansible이 접속하지 못한 노드를 ssh로 직접 다시 보고 이유를 붙입니다: `키 거부`(키 보내기), `접속 불가`(주소·포트·방화벽·sshd 확인), `호스트 키 변경`(노드를 다시 설치했다면 `ssh-keygen -R <주소>`), `sudo 비밀번호 필요`(NOPASSWD 설정). 키 거부가 아닌 노드에는 키를 보내지 않고 고칠 방법만 보여 줍니다. 비밀번호를 입력하거나 키를 보내는 동안에는 다른 단계로 이동하지 않습니다(`Esc`·`Ctrl+C`로 끝낸 뒤 이동). P2는 접속하지 못한 노드를 8대씩 동시에, 노드마다 시간을 따로 두고 봅니다. 시간 안에 못 본 노드는 `확인 못 함`으로 나옵니다. sshd가 비밀번호 로그인을 받지 않는 노드는 `비밀번호 로그인 꺼짐`으로, 키를 보내 줄 수 없으니 직접 넣어야 합니다.

### SSH 키 배포 (`k`·`p`, `--check-ssh`, `--send-ssh-key`)

노드가 "준비됨"이란 이 제어 노드 사용자의 키 `~/.ssh/id_rsa`로 (에이전트나 다른 키 없이) 접속되고 `sudo -n true`가 통과한다는 뜻입니다. 플레이북이 접속하는 방법과 같습니다. 노드 목록은 설치 디렉터리 venv의 `ansible-inventory -i <inventory> --list`로 읽습니다(group_vars 포함, `ansible_connection: local` 노드는 제외). 노드가 `ansible_ssh_private_key_file`, `ansible_ssh_common_args`, `ansible_password`, `ansible_become_method` 같은 값을 설정했으면 경고합니다. 이 경로는 그 값을 쓰지 않으므로 결과가 플레이북과 다를 수 있습니다.

```bash
cd /opt/k8s-installer/ansible
k8s-installer-tui --check-ssh                       # 읽기만: 표 출력
k8s-installer-tui --send-ssh-key                    # 노드마다 ssh가 비밀번호를 물음
k8s-installer-tui --send-ssh-key --ask-pass-once    # 비밀번호를 한 번만 물음 (노드가 같은 비밀번호일 때)
```

표의 열은 노드, 사용자@주소, 키(`ok`/`거부`/`접속불가`/`호스트키변경`/`비밀번호로그인꺼짐`/`키읽기실패`/`미확인`), sudo(`ok`/`비밀번호필요`/`미확인`/`-`), 비고(고칠 방법과 ssh의 마지막 줄)입니다. 종료 코드: `0` 모든 노드 준비됨, `1` 준비 안 된 노드 있음, `2` 실행하지 못함(노드 목록·키 준비·터미널 없음 등), 중단되면 128+신호 번호: `130` `Ctrl+C`, `143` SIGTERM, `129` SIGHUP(운영자의 ssh 세션이 끊김). `키읽기실패`는 이 제어 노드의 `~/.ssh/id_rsa`를 ssh가 읽지 못한 것(권한·소유자 확인)이라 키를 보내 줄 수 없고, sudo `미확인`은 키로는 접속됐지만 sudo를 시간 안에 확인하지 못한 것(다시 확인)입니다. 시리얼 콘솔처럼 전체 화면을 쓸 수 없는 곳에서도 씁니다. 다시 실행하면 이미 키를 받는 노드는 건드리지 않습니다.

하는 일:

- 키 준비: `~/.ssh/id_rsa.pub`가 있으면 아무것도 하지 않습니다(이 제어 노드의 `authorized_keys`도 그대로). `~/.ssh/id_rsa`만 있고 `.pub`가 없으면 멈추고, `ssh-keygen -y -f ~/.ssh/id_rsa > ~/.ssh/id_rsa.pub`로 공개 키를 되살리라고 안내합니다(스크립트를 실행하지 않고 아무것도 만들지 않음). 둘 다 없을 때만 k8s-installer의 `<installer-root>/ansible/configure-control-node-ssh.sh`를 실행해 키 쌍을 만듭니다(출력하는 수동 안내는 버림). 키를 받지 않는 노드에만 `ssh-copy-id`로 공개 키를 보냅니다.
- `--check-ssh`와 P2는 읽기만 합니다. `~/.ssh/known_hosts`도 바꾸지 않습니다(복사본에 기록하고 버림).
- 키를 보낼 때는 `StrictHostKeyChecking=accept-new`: 처음 보는 노드의 호스트 키는 기록하고, 바뀐 호스트 키는 거부합니다(`ssh-keygen -R`은 사람이 판단해서 실행).
- 비밀번호 한 번 입력(`p`, `--ask-pass-once`): 비밀번호는 `ssh-copy-id` 자식 프로세스의 환경 변수로만 넘어가고, ssh는 `SSH_ASKPASS` 도우미(비밀번호가 아니라 변수 이름만 담김)로 읽습니다. 도우미는 실행할 수 있는 사용자 전용 디렉터리(`$XDG_RUNTIME_DIR`, 안 되면 `~/.cache/k8s-installer-tui/`) 안에 매번 새로 만든 0700 디렉터리에 둡니다. 도우미를 바꿔치기하면 비밀번호를 받을 수 있으므로, 가는 길의 디렉터리가 심볼릭 링크이거나 이 사용자 소유가 아니거나 그룹·다른 사용자가 쓸 수 있으면 그 위치는 쓰지 않습니다. 이 디렉터리는 보내기가 끝날 때, `Ctrl+C`로 중단할 때, 화면 모드와 `--send-ssh-key`가 SIGTERM·SIGHUP을 받았을 때 지웁니다. SIGKILL처럼 프로그램이 정리할 틈이 없이 끝나면 남을 수 있습니다(비밀번호는 담겨 있지 않음). `/tmp`가 noexec여도 동작하고, 둘 다 실행할 수 없으면 "키 거부"가 아니라 그 이유를 말하고 보내지 않습니다. 터미널은 `setsid`로 떼어 OpenSSH 8.4 이전에서도 동작합니다. 노드마다 한 번만 시도하므로 틀린 비밀번호는 노드당 로그인 실패 한 번입니다. 비밀번호는 디스크, 명령 인자, 로그, 저장한 설정, 화면·메시지에 쓰지 않으며, 보내기가 끝날 때까지 이 프로세스의 메모리에만 있습니다(직접 가진 바이트 버퍼는 끝나면 0으로 지우지만, Go 문자열 사본까지 지운다고 보장하지는 않음). 중단하거나 SIGTERM·SIGHUP을 받으면 비밀번호를 환경에 가진 `ssh-copy-id`와 그 아래 ssh까지 세션째 끝냅니다. (`k`로 터미널을 넘긴 동안에는 그 명령이 신호를 직접 받고, 화면은 그 명령이 끝난 뒤 종료합니다.)

하지 않는 일:

- 노드에 사용자를 만들거나 sudo 규칙(NOPASSWD)을 넣지 않습니다. `sudo 비밀번호 필요`는 사이트가 고칩니다.
- `sshd_config`를 고치지 않습니다. 비밀번호 로그인이 막힌 노드, 특히 비밀번호 로그인이 막힌 `root`에는 키를 보낼 수 없어 직접 넣어야 합니다(`configure-control-node-ssh.sh`가 출력하는 안내).
- 바뀐 호스트 키를 지우지 않습니다.

`d`로 명령을 보거나 `w`로 저장한 스크립트를 직접 실행할 수도 있습니다. 설정을 `--installer-root`의 `ansible` 디렉터리가 아닌 곳에 저장했다면 스크립트에 복사 명령이 함께 들어갑니다. 스크립트도 M1에서 `read`로 한 번 멈춥니다(터미널이 없으면 그냥 지나감).

### 설치 실행 (`실행`, `--install`)

9단계의 `실행` 버튼(또는 `--install`)은 계획을 제어 노드에서 차례로 실행합니다. 플레이북은 설치 디렉터리에서 venv를 켜고 `ansible-playbook -i inventory.yml ...`로, `install-plan.sh`와 같은 명령으로 실행합니다(P6이 찾은 `-e ansible_shell_executable` 포함).

1. **사전 확인**: P1~P8을 아직 실행하지 않았으면 먼저 실행합니다. ✗가 하나라도 있으면 시작하지 않습니다(경고 `!`는 계속). `f`(처음부터)도 같습니다. 사전 확인이 도는 중이면 끝난 뒤 이어 갑니다. 확인한 뒤에 사이트 파일이나 옵션(기존 클러스터 여부, 설치 디렉터리)이 바뀌었으면 그 결과는 버리고 다시 확인합니다(P3가 손으로 넣은 apiserver 설정을 지키고, P6이 become 셸을 정하므로).
2. **단계(stage)**: 한 단계가 모두 성공해야 다음 단계로 갑니다. 순서는 `C1`(설정을 다른 곳에 저장했을 때 설치 디렉터리로 복사, sudo. 이어서 실행할 때도 매번 다시 복사해 다시 저장한 설정으로 플레이북이 돎) → K1 → K2 → K3 → K4 → A1 → **A2 + M1** → (A2.5) → A3 → A4 → A5 → (A5.1 → A5.2).
   - 플레이북은 언제나 하나씩 실행합니다. 두 설치 도구의 모든 플레이북이 `begin-playbook.yml`로 시작해 `sync-ansible.yml`(ansible 디렉터리를 tar로 묶어 다른 ki cp 노드의 것을 지우고 바꿈)과 `/tmp/k8s-installer`의 고정 파일(vars.yml, 아카이브)을 쓰기 때문에, 같은 제어 노드에서 둘을 동시에 돌리면 서로의 파일을 덮어씁니다. k8s-installer에는 자체 잠금도 없습니다.
   - 단계마다 명령은 자기 세션(setsid)에서 터미널 없이 돕니다. 러너가 이 제어 노드에서 실행하는 sudo(C1의 복사, A5.1의 `--fill-auth-config`)는 `sudo -n`이라 비밀번호를 물으면 기다리지 않고 실패합니다. 그런 단계가 남아 있으면 시작 전에 `sudo -n true`로 확인하고, 실패하면 시작하지 않습니다(NOPASSWD sudo가 필요). `install-plan.sh`는 사람이 돌리므로 그냥 `sudo`입니다.
   - 함께 실행하는 것은 **M1(StorageClass 드라이버 설치, 수동)과 A2 `setup-cert`** 뿐입니다. M1이 하는 일은 읽기만 하는 `ansible <첫 k8s 컨트롤 플레인 노드> -m command -a 'kubectl get storageclass ...'`이고 ansible 디렉터리나 `/tmp/k8s-installer`를 건드리지 않습니다. 드라이버 파드는 노드가 Ready여야 하므로(A1 Cilium 뒤) 그 자리이고, 운영자가 드라이버를 까는 동안 setup-cert가 돕니다.
3. **수동 멈춤 (M1)**: 사이트가 볼륨을 StorageClass에 청구하는 첫 단계(HA면 A3 Harbor, 그다음 A4 모니터링 `aipub_monitoring_storage_type: dynamic`, A5 백엔드 공유 볼륨 `aipub_app_file_storage_type: dynamic`) 앞입니다. 쓰는 클래스는 `files/aipub/vars.yml`에서 읽습니다(`aipub_ha_mode_storage_class`, 그리고 있으면 `aipub_monitoring_storage_class_name`·`aipub_app_file_storage_storage_class_name`·`aipub_app_storage_class_name`). 먼저 한 번 확인해 모두 있으면 멈추지 않고, 없으면 "스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: …"를 보이고 기다립니다. `Enter`(설치함, 계속)를 누르면 다시 확인하고, 모든 클래스가 있어야 다음 단계로 갑니다. 모니터링 볼륨이 dynamic인데 클래스 이름이 비어 있으면 클러스터의 기본 StorageClass를 쓰므로, 기본으로 표시된 클래스(`storageclass.kubernetes.io/is-default-class: "true"`)가 있는지 확인합니다. 먼저 누른 `Enter`는 답으로 치지 않습니다. 쓰는 클래스가 없으면(HA 아님, 모니터링·공유 볼륨이 static) M1은 건너뛰고 이유를 보입니다.
4. **진행 화면**: 단계마다 `·` 대기 `▶` 실행 중 `✓` 성공 `✗` 실패 `⏸` 기다림 `-` 건너뜀과 소요 시간, 실행 중인 단계 로그의 마지막 10줄. `↑/↓`로 단계를 고르고 `l`로 전체 로그. 실행 중에는 다른 단계로 이동하지 않습니다.
5. **멈추기**: `Ctrl+C` 한 번은 실행 중인 단계가 끝나면 멈춤(M1 기다림은 바로 끝냄), 두 번은 실행 중인 단계의 프로세스 그룹에 SIGINT, 10초 뒤에도 남으면 SIGKILL(상태 `cancelled`). setup 플레이북은 다시 실행해도 안전하므로 그 단계부터 다시 실행하면 됩니다. 프로그램을 끝내면 실행도 멈춥니다(아직 분리 실행은 없음).
6. **실패**: 단계가 실패하면 같은 stage의 다른 단계는 끝까지 돌게 두고(플레이북을 중간에 끊지 않음, M1 기다림은 끝냄) 거기서 멈춥니다. 끝에 실패한 단계, 종료 코드, 로그 경로, 로그 마지막 줄을 보여 줍니다.
   - `setup-aipub`이 backend-informer에서 Workspace CRD가 없다고 실패하면, aipub-installer README대로 Python 서비스를 설치한 뒤 이어서 실행하라고 안내합니다. 다시 돌려도 같은 이유로 실패하므로 자동 재시도는 하지 않습니다. 실패한 컴포넌트가 backend-informer 하나뿐일 때는(설치 스크립트의 결과 줄로 확인) 이 단계를 실패로 남긴 채 뒤 단계(A5.1, A5.2)를 계속 실행합니다. 뒤 단계는 백엔드와 CA 만 필요하고 informer 에 기대지 않기 때문입니다. 실행 결과는 실패(종료 코드 1)로 남고, Python 서비스를 설치한 뒤 이어서 실행하면 이 단계만 다시 돕니다. 다른 컴포넌트가 함께 실패했거나 닿지 않는 노드가 있으면 여느 실패처럼 그 자리에서 멈춥니다.
   - helm으로 설치하는 단계(A1 `setup-network`, A3, A4, A5)가 파드를 기다리다 실패하면(`context deadline exceeded`, `ImagePullBackOff`, `ErrImagePull`, not ready), 흔한 원인이 사용자 레지스트리에 이미지가 없는 것이라고 말하고 확인 방법(컨트롤 플레인 노드에서 `kubectl -n <네임스페이스> get pods`)을 보입니다. 이번 실행이 K4를 건너뛰었거나 실행하지 않았으면 아카이브 경로를 넣고 이어서 실행하라고 덧붙입니다.
   - `k8s_cp_extra_files`가 가리키는 파일(JWKS 인증 설정 등)이 없다는 거부는 플레이북을 손으로 실행했을 때 납니다(이 도구의 단계는 아직 없는 파일을 `-e`로 뺌). 위저드로 실행하거나 그 단계 로그의 `-e`를 붙이라고 안내합니다. 빈 파일을 만들면 안 됩니다: 빈 인증 설정이 컨트롤 플레인 노드로 복사되면 apiserver가 뜨지 않습니다.
   - 실패한 실행 화면에서 `r`은 실패한 단계부터 이어서 실행(`실행`과 같음)입니다. `x`는 아래 "클러스터 초기화"의 조건에서만 나옵니다.
7. **이어서 실행**: 9단계에 들어오면 지난 실행(`state.json`)의 끝난 단계와 남은 단계를 보여 주고, 버튼이 `이어서 실행`이 됩니다. 성공한 단계는 다시 실행하지 않고 첫 미완료 단계부터 실행합니다. 지난 실행에서 건너뛴 단계라도 지금 계획이 건너뛰지 않으면 실행합니다(K4를 아카이브 없이 건너뛴 뒤 경로를 넣고 이어서 실행하면 K4를 먼저 실행하고 실패한 단계로 감). 지난 실행 뒤에 사이트 파일(inventory, vars, AIPub vars, 웹훅 kubeconfig)이 바뀌었으면 먼저 묻습니다. `f`는 처음부터(사전 확인과 확인 후)이며 사이트를 지금 상태로 다시 판단합니다. 끝나지 않은 실행은 시작할 때의 사이트 사실(기존 클러스터·AIPub 여부)과 그때 고른 모니터링으로 계획하고, 아카이브 경로는 K4가 성공했으면 그때 것을, 아니면 지금 넣은 것을 씁니다(`state.json`에 기록). 화면에서 지난 실행의 값을 바꾼 뒤 이어서 실행하면 지난 실행의 값으로 한다고 알려 줍니다(바꾸려면 `f`).
8. **강제 종료**: 프로그램이 SIGKILL·OOM으로 죽으면 잠금은 풀리지만 실행 중이던 단계(자기 세션)는 남습니다. `state.json`에 실행 중인 단계의 프로세스 그룹을 적어 두므로, 다음 시작은 그 그룹이 살아 있는 동안 시작하지 않고 pid를 알려 줍니다(같은 플레이북이 두 번 돌지 않게).

상태와 로그는 `<installer-root>/tui-install/`에 있습니다: `state.json`(단계별 상태·시작·끝·종료 코드·로그 경로, 실행 중인 단계의 프로세스 그룹, 사이트 파일 해시, 실행의 기존 클러스터·AIPub·모니터링·아카이브 경로; 원자적으로 쓰고 디렉터리까지 fsync. 실행 중에 쓰지 못하면 끝에 알려 줌), `logs/<단계>.log`(시도마다 이어 씀), `lock`(flock: 두 TUI가 같은 설치를 동시에 실행하지 못함). 디렉터리는 0700, 로그는 0600(플레이북 출력이 들어 있음). ansible 디렉터리 안이 아닌 이유: 매 플레이북의 `sync-ansible.yml`이 그 디렉터리를 tar로 묶기 때문이고, `upgrade.sh`는 `ansible/`·`scripts/`·`bundle/`·`venv/`만 바꿉니다. 설치 디렉터리가 root 소유이면 처음 한 번 `sudo -n install -d -m 0700`으로 만들어 이 사용자에게 줍니다.

하지 않는 일: 계획의 단계로는 reset·remove 플레이북(셸 단계의 명령 안에 있어도)과 `delete_data`·`delete_namespace`를 실행하지 않습니다(러너가 거부). 비밀번호를 묻지 않습니다(NOPASSWD sudo는 P2가 확인하는 전제). 예외는 운영자가 화면에서 고르는 클러스터 초기화 하나입니다.

**클러스터 초기화 (`x`)**: 새 사이트(실행의 기존 사이트 판정이 아님)에서 K1·K2·K3 중 하나가 실패해 끝난 실행 뒤에만 나옵니다. 클러스터를 만드는 중이라 초기화가 도움이 되는 경우(이전 설치 잔재, 이미 켜진 kubelet)입니다. K3 뒤 단계가 실패했을 때(클러스터가 이미 섬), 기존 사이트, 실행 중에는 나오지 않고 눌러도 아무것도 하지 않습니다. 이 제어 노드의 `applied-release`를 확인할 수 있고 없어야 나옵니다(있거나 확인하지 못하면 나오지 않음). `reset`을 입력한 뒤에는 실행 전에 다시 확인합니다: 다른 프로그램이 이 설치를 실행 중이 아닌지(실행 잠금을 잡고 초기화가 끝날 때까지 쥠), 실행 기록이 화면에 보인 그대로인지, 인벤토리의 모든 노드(broken_node 제외)에 `applied-release`가 없는지(`sudo -n`으로 확인. 다른 제어 노드에서 만든 클러스터의 노드이거나, 답하지 않거나 sudo가 거부되는 노드가 있으면 초기화하지 않음). k8s-installer가 만들지 않은 클러스터(손으로 한 kubeadm)는 기록이 없어 알아볼 수 없습니다. 확인은 `y`가 아니라 `reset`을 입력하고 `Enter`입니다(그 밖의 입력·`Esc`·`Ctrl+C`는 취소). 단계처럼 이 사용자로(sudo 아님) venv를 켜고 `ansible-playbook -i inventory.yml playbooks/reset-cluster.yml -e sdfc= -e '{"k8s_cp_extra_files":[]}'`(P6의 셸이 있으면 그것도)를 터미널에서 실행하고, 출력은 `logs/reset-cluster.log`에도 남깁니다. `sdfc=`는 k8s-installer의 "skip deleting from cluster"로, apiserver에 노드 삭제를 묻는 단계를 건너뜁니다(만들다 만 클러스터에는 물을 apiserver가 없을 수 있음). `k8s_cp_extra_files=[]`는 아직 쓰지 않은 파일(JWKS 설정) 때문에 `validate-hostvars.py`가 거부하지 않게 하며, 노드에서 지울 파일은 각 노드의 기록(`applied-vars.yml`)에서 읽으므로 덜 지우지 않습니다. 성공하면 지난 실행 기록을 `state.json.reset`으로 옮깁니다(초기화가 K2의 ki cp 서비스까지 지우므로 이어서 실행하면 안 됨). 그다음 `실행`은 K1부터이고, 클러스터 기록(`applied-release`)이 지워졌으므로 새 사이트 계획입니다.

```bash
cd /opt/k8s-installer/ansible
k8s-installer-tui --install --archive ~/pkg/ki-cp-user-registry-artifacts.tgz   # 새 사이트: 사전 확인, 그다음 모든 단계. M1에서 Enter를 기다림
k8s-installer-tui --install --resume        # 실패·중단한 단계부터
k8s-installer-tui --install --resume --archive ~/pkg/ki-cp-user-registry-artifacts.tgz   # K4를 건너뛴 실행: K4부터
k8s-installer-tui --install --fresh         # 끝나지 않은 실행이 있어도 처음부터 (사이트는 지금 상태로 판단)
k8s-installer-tui --install --existing --aipub-installed   # 클러스터와 AIPub이 이미 있는 사이트 (기록이 있으면 --existing 은 없어도 됨)
```

`--install`은 먼저 `── 사이트: ...`로 기존 사이트 판정과 그 이유를 찍고(이어서 실행이면 지난 실행의 판정), 단계가 바뀔 때마다 한 줄씩(`▶`·`✓`·`✗`·`⏸`) 찍고, 끝에 요약을 찍습니다. 종료 코드: `0` 모두 성공, `1` 실패(또는 M1에서 입력이 끝나 멈춤), `2` 시작하지 못함(사전 확인 ✗, 다른 실행이 잠금을 가짐, 지난 실행의 단계가 아직 실행 중, `--resume`·`--fresh` 없이 끝나지 않은 실행이 있음, `--resume`인데 지난 실행 없음, 사이트 파일이 바뀌어 묻는 말에 아니오, 이 제어 노드의 sudo가 비밀번호를 물음, AIPub을 설치하는 새 사이트에 `--archive`도 `--no-archive`도 없음(찾은 아카이브가 있으면 그 경로를 보임), `--archive`가 읽을 수 있는 일반 파일이 아님), `130` `Ctrl+C`(SIGTERM `143`, SIGHUP `129`). 시험: `test/install/run.sh`가 가짜 `ansible`·`ansible-playbook`으로 컨테이너 안에서 처음부터 끝까지 돌려 봅니다(서버에 접속하지 않음). 가짜 `ansible-playbook`은 k8s-installer처럼 `k8s_cp_extra_files`가 가리키는 파일이 `files/`에 없으면 거부하고, 8번 시나리오는 컨테이너의 `/etc/k8s-installer/applied-release`를 만들어 기존 클러스터 계획을, 9번은 아카이브 없는 새 사이트(거부, `--no-archive`로 A1까지 가서 이미지 진단, `--resume --archive`로 K4부터)를 돌려 봅니다. 새 사이트 시나리오는 작은 가짜 아카이브를 씁니다.

```bash
docker run --rm -v tui-gomod:/go/pkg/mod -v "$PWD:/src" -w /src golang:1.23 bash test/install/run.sh
```

## 프로젝트 구조

```
k8s-installer-tui/
├── main.go                  # 플래그 파싱, 파일 로드, TUI 실행 / --render
├── Makefile, go.mod, set-env.sh
├── internal/
│   ├── state/               # AppState, NodeConfig
│   ├── fileio/
│   │   ├── loader.go        # 파일 3개 -> AppState (구버전 파일 이전 포함)
│   │   ├── writer.go        # AppState -> 파일 3개 (history.go: history.jsonl)
│   │   ├── yamldoc.go       # 주석을 보존하는 YAML 편집
│   │   └── templates/       # 설치 도구 예제 파일의 복사본 (README.md 참고)
│   ├── validator/rules.go   # 검증 규칙
│   ├── plan/                # 설치 순서, 명령, 사전 확인
│   └── ui/
│       ├── app.go           # Bubbletea App, 화면 라우팅
│       ├── styles/
│       └── screens/         # s1~s9 화면
├── test/simulate/           # 설치 도구와 맞춰 보는 시뮬레이션
└── test/sshkeys/            # SSH 키 배포를 진짜 sshd 컨테이너에서 확인
```

## 의존성

| 라이브러리 | 용도 |
|-----------|------|
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI 프레임워크 (Elm Architecture) |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | 텍스트 입력, 뷰포트 등 UI 컴포넌트 |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | 터미널 스타일링 |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | YAML 파싱 및 직렬화 |

## 라이선스

Apache License 2.0

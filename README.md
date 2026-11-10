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
| `--version` | | 버전 출력 |

`--render`의 종료 코드: `0` 검증 통과, `2` 검증 오류 있음(파일은 렌더링됨), `1` 로드·렌더링·쓰기 오류. 렌더링 결과는 `<dir>` 아래에 k8s-installer의 `ansible` 디렉터리 배치(`inventory.yml`, `group_vars/all/vars.yml`, `files/aipub/vars.yml`)로 쓰며, AIPub을 설치하지 않으면 `files/aipub/vars.yml`은 만들지 않습니다.

### 기존 파일이 있는 경우

기존 파일이 있으면 읽어서 편집 모드로 시작합니다. 저장은 새로 쓰지 않고 기존 YAML을 그 자리에서 고칩니다(`internal/fileio/yamldoc.go`). 이 도구가 다루지 않는 변수와 주석은 그대로 남고, 템플릿에는 있고 사이트 파일에는 없는 변수는 주석과 함께 추가됩니다. 덮어쓰기 전의 파일은 `.bak`으로 보관합니다.

```
vars.yml      →  vars.yml.bak   (백업)
vars.yml      ←  수정한 내용으로 저장
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
| R7 | AIPub LB VIP(인그레스, 별도 IP 모드의 게이트웨이)는 `internal_network_subnets` 안에 있고, `ki_cp_ha_mode_vip`(HA일 때)·노드의 `ansible_host`(IP일 때)와 다르며, 파드·서비스 서브넷 밖이어야 함 |

그 밖에 형식(CIDR, IP, 크기 등)과 필수 항목(`k8s_node` 최소 1개 등)도 같은 곳에서 검사합니다.

### 9단계 — 설치 계획 (`internal/plan`)

저장한 설정으로 실행해야 할 순서를 보여 줍니다. 순서는 `internal/plan/plan.go` 한 곳에 있어, 설치 도구의 절차가 바뀌면 그 표만 고칩니다.

| ID | 단계 | 종류 |
|----|------|------|
| P1 | `setup.sh` 완료 확인 | 확인 |
| P2 | 모든 노드에 SSH·sudo 접속 | 확인 |
| K1 | `setup-k8s-installer` | 플레이북 |
| K2 | `setup-control-node` | 플레이북 |
| K3 | `setup-cluster` | 플레이북 |
| K4 | `push-user-registry-artifacts` (선택, 아카이브 경로를 넣었을 때) | 플레이북 |
| A1 | `setup-network` | 플레이북 (AIPub) |
| A2 | `setup-cert` | 플레이북 (AIPub) |
| A3 | `setup-base` | 플레이북 (AIPub) |
| A4 | `setup-monitoring` (선택) | 플레이북 (AIPub) |
| A5 | `setup-aipub` | 플레이북 (AIPub) |

A1~A5는 AIPub을 설치할 때만 나옵니다. 화면에서 K4의 아카이브 경로와 모니터링 설치 여부를 정합니다.

| 키 | 동작 |
|----|------|
| `c` | 사전 확인 실행: P1(설치 디렉터리), P2(모든 노드에 `sudo -n true`) |
| `d` | 각 단계의 명령 보기 |
| `w` | 명령을 `install-plan.sh`로 저장 (설정을 저장한 디렉터리에) |

"실행"은 다음 버전에서 지원합니다. 이 버전은 사전 확인 외에는 아무것도 실행하지 않으므로, `d`로 명령을 보거나 `w`로 저장한 스크립트를 직접 실행하세요. 설정을 `--installer-root`의 `ansible` 디렉터리가 아닌 곳에 저장했다면 스크립트에 복사 명령이 함께 들어갑니다.

## 프로젝트 구조

```
k8s-installer-tui/
├── main.go                  # 플래그 파싱, 파일 로드, TUI 실행 / --render
├── Makefile, go.mod, set-env.sh
├── internal/
│   ├── state/               # AppState, NodeConfig
│   ├── fileio/
│   │   ├── loader.go        # 파일 3개 -> AppState (구버전 파일 이전 포함)
│   │   ├── writer.go        # AppState -> 파일 3개, .bak 백업
│   │   ├── yamldoc.go       # 주석을 보존하는 YAML 편집
│   │   └── templates/       # 설치 도구 예제 파일의 복사본 (README.md 참고)
│   ├── validator/rules.go   # 검증 규칙
│   ├── plan/                # 설치 순서, 명령, 사전 확인
│   └── ui/
│       ├── app.go           # Bubbletea App, 화면 라우팅
│       ├── styles/
│       └── screens/         # s1~s9 화면
└── test/simulate/           # 설치 도구와 맞춰 보는 시뮬레이션
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

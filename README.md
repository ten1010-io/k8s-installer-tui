# k8s-installer-tui

`inventory.yml`과 `group_vars/all/vars.yml`을 터미널 UI(TUI)로 작성하는 도구입니다.  
[k8s-installer](https://github.com/ten1010-io/k8s-installer)의 설치 전 설정 단계에서 사용합니다.

## 사용 흐름

```
1. 이 레포에서 Go 소스 코드 유지 및 배포
        ↓
2. 서버(또는 CI)에서 빌드 → 바이너리 패키지로 저장
        ↓
3. k8s-installer의 download-bin.sh로 바이너리 다운로드
        ↓
4. 설치 담당자가 TUI 실행 → inventory.yml / vars.yml 생성
        ↓
5. k8s-installer 플레이북 실행
```

## 빌드

### 요구 사항

- Go 1.21 이상
- 인터넷 연결 (첫 빌드 시 의존성 다운로드)

### 명령어

```bash
# 의존성 설치 + 현재 OS용 바이너리 빌드
make build

# Linux amd64 바이너리 빌드 (배포용)
make build-linux

# Linux arm64 바이너리 빌드
make build-linux-arm64

# 빌드 결과물 삭제
make clean
```

빌드된 바이너리는 `dist/` 디렉터리에 생성됩니다.

```
dist/
└── k8s-installer-tui-linux-amd64
```

## 실행

```bash
# 기본 실행 (현재 디렉터리의 inventory.yml, group_vars/all/vars.yml 사용)
./k8s-installer-tui

# 경로 직접 지정
./k8s-installer-tui \
  --inventory /path/to/inventory.yml \
  --vars /path/to/group_vars/all/vars.yml

# 버전 확인
./k8s-installer-tui --version
```

### 기존 파일이 있는 경우

기존 `inventory.yml`과 `vars.yml`이 있으면 자동으로 읽어 편집 모드로 시작합니다.  
저장 시 기존 파일은 `.bak`으로 백업됩니다.

```
inventory.yml      →  inventory.yml.bak  (백업)
inventory.yml      ←  새 내용으로 저장
```

## 키 조작

| 키 | 동작 |
|----|------|
| `Ctrl+N` | 다음 단계 (현재 단계 검증 후 이동) |
| `Ctrl+P` | 이전 단계 |
| `Ctrl+C` | 종료 |
| `Tab` / `Shift+Tab` | 필드 또는 섹션 이동 |
| `↑` / `↓` | 목록 이동 |
| `Space` | 체크박스/라디오 토글 |
| `a` | 항목 추가 |
| `e` / `Enter` | 항목 편집 |
| `d` | 항목 삭제 |
| `s` | 저장 (7단계 미리보기 화면에서) |

## 7단계 Wizard

```
[1.노드] → [2.그룹] → [3.네트워크] → [4.K8s] → [5.AIPub] → [6.인증서] → [7.저장]
```

### 1단계 — 노드 정의 (`inventory.yml: all.hosts`)

클러스터를 구성하는 노드를 추가합니다.

| 필드 | 설명 | 기본값 |
|------|------|--------|
| 이름 | 노드 식별자 (예: `node1`) | — |
| ansible_host | 접속할 IP 또는 호스트명 | — |
| ansible_port | SSH 포트 | `22` |
| ansible_ssh_user | SSH 사용자 | `root` |

### 2단계 — 그룹 할당 (`inventory.yml: ki_cp_node, k8s_node`)

노드별로 역할을 체크박스로 지정합니다.

| 열 | 설명 |
|----|------|
| `ki_cp_node` | KI Control Plane 노드 (DNS, NTP, 레지스트리 등) |
| `k8s_node` | Kubernetes 클러스터 노드 |
| `k8s_cp` | Kubernetes Control Plane 노드 (`k8s_node` 필수) |
| `nvidia_gpu` | GPU 노드 (`k8s_node` 필수) |

### 3단계 — 네트워크 (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 내부 서브넷 | `internal_network_subnets` (CIDR 목록) |
| CP HA 모드 | `ki_cp_ha_mode` + `ki_cp_ha_mode_vip` |
| DNS upstream | `ki_cp_dns_server_upstream_servers` |
| NTP upstream | `ki_cp_ntp_server_upstream_servers` |
| DNSSEC 검증 | `ki_cp_dns_dnssec_validation` |
| 추가 DNS 존 | `internal_network_extra_zone` (선택) |

### 4단계 — Kubernetes (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 인증서 유효기간 | `k8s_certificate_validity_period` (예: `26280h`) |
| 로드밸런서 | `k8s_load_balancers` (이름, VIP, 노드 목록) |
| 기본 인그레스 | `k8s_default_ingress_class` (LB 선택, 호스트 포트) |

### 5단계 — AIPub (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 인그레스 도메인 | `aipub_ingress_zone` |
| AIPub HA 모드 | `aipub_ha_mode` + `aipub_ha_mode_storage_class` |
| AIPub CP 노드 | `aipub_cp_nodes` |
| Harbor 설정 | 서브도메인, 레플리카 수, 스토리지 크기 4종 |

### 6단계 — 인증서 모드 (`vars.yml: ki_cert_mode`)

| 값 | 설명 |
|----|------|
| `self_signed` | CA 자동 생성 + TLS 인증서 발급 (기본값) |
| `ca_provided` | 고객이 `ca.crt` + `ca.key` 제공, TLS만 발급 |
| `tls_provided` | 고객이 `tls.crt` + `tls.key` 모두 제공 |

### 7단계 — 미리보기 & 저장

- `Tab`으로 `inventory.yml` / `vars.yml` 탭 전환
- 전체 검증 결과 표시 (오류가 있으면 저장 불가)
- `s`로 저장

## 검증 규칙

저장 전 다음 규칙을 모두 통과해야 합니다.

| # | 규칙 |
|---|------|
| R1 | `ki_cp_ha_mode=true`이면 `ki_cp_ha_mode_vip` 필수이며 VIP는 CP 노드 서브넷 내에 있어야 함 |
| R2 | 모든 `ki_cp_node`는 `internal_network_subnets` 에 속해야 함 |
| R3 | `k8s_load_balancers[].nodes`는 `k8s_node` 그룹의 부분집합이어야 함 |
| R4 | `k8s_default_ingress_class.load_balancer`는 정의된 LB 이름 중 하나여야 함 |
| R5 | `aipub_ha_mode=true`이면 `aipub_ha_mode_storage_class` 필수 |
| R6 | `aipub_cp_nodes`는 `k8s_node` 그룹의 부분집합이어야 함 |

## 프로젝트 구조

```
k8s-installer-tui/
├── main.go                          # 진입점 (플래그 파싱, 파일 로드, TUI 실행)
├── go.mod
├── Makefile
└── internal/
    ├── state/
    │   ├── types.go                 # NodeConfig, LBConfig, IngressConfig, ARecord
    │   └── state.go                 # AppState, DefaultState()
    ├── fileio/
    │   ├── loader.go                # YAML 파일 → AppState
    │   └── writer.go                # AppState → YAML + .bak 백업
    ├── validator/
    │   └── rules.go                 # R1~R6 검증
    └── ui/
        ├── app.go                   # Bubbletea App, 화면 라우팅
        ├── styles.go                # Lipgloss 스타일
        ├── keys.go                  # 키 바인딩
        └── screens/
            ├── common.go            # Screen 인터페이스
            ├── s1_nodes.go
            ├── s2_groups.go
            ├── s3_network.go
            ├── s4_kubernetes.go
            ├── s5_aipub.go
            ├── s6_cert_mode.go
            └── s7_preview.go
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

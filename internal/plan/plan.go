// Package plan turns the saved settings into the steps of an installation: what
// to check first and which playbooks to run, in the order the installers want
// them. It runs nothing; the commands are shown, written to a script, and in a
// later version run by the tool.
//
// The order is that of the README of k8s-installer (클러스터 설치) and of
// aipub-installer (network → cert → base → monitoring → aipub). It is kept in this
// one table so that a change of the installers - GitOps taking over the modules
// after base, the python module joining - is a change here and nowhere else.
package plan

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Kind tells a check (looked at, nothing changes) from a playbook (run) and from
// commands of the control node (run).
type Kind int

const (
	Check Kind = iota
	Playbook
	Shell
)

type Step struct {
	ID       string // P1, K1, A1, ...
	Title    string
	Kind     Kind
	Playbook string   // relative to the ansible directory, for Kind Playbook
	Args     []string // more arguments of ansible-playbook
	Shell    []string // the commands, for Kind Shell: a line each, run from the ansible directory
	Note     string   // one line shown under the step
	Optional bool     // the operator may leave it out
	Skip     bool     // left out: optional and turned off, or missing what it needs
	SkipWhy  string
}

// Options are what the screen asks besides the settings.
type Options struct {
	// The directory k8s-installer is installed in (setup.sh), /opt/k8s-installer
	Root string
	// The archive of the user registry (ki-cp-user-registry-artifacts.tgz) on the
	// control node; empty when it was pushed already
	ArchivePath string
	// Whether the monitoring module is installed with AIPub
	Monitoring bool
}

func DefaultOptions() Options {
	return Options{Root: "/opt/k8s-installer", Monitoring: true}
}

func (o Options) AnsibleDir() string { return o.Root + "/ansible" }

// Build is the plan for a new installation with these settings.
func Build(s *state.AppState, o Options) []Step {
	steps := []Step{
		{ID: "P1", Title: "setup.sh 완료", Kind: Check,
			Note: "번들을 풀고 " + o.Root + " 에 설치하는 단계. sudo가 필요해 직접 실행합니다"},
		{ID: "P2", Title: "SSH·sudo 접속", Kind: Check,
			Note: "configure-control-node-ssh.sh 안내대로 키를 등록한 뒤, 모든 노드에 비밀번호 없이 접속·sudo"},
		{ID: "K1", Title: "setup-k8s-installer", Kind: Playbook, Playbook: "playbooks/setup-k8s-installer.yml",
			Note: "모든 노드에 설치본 배포"},
		{ID: "K2", Title: "setup-control-node", Kind: Playbook, Playbook: "playbooks/setup-control-node.yml",
			Note: "ki cp 서비스: DNS, NTP, 레지스트리, apiserver LB"},
		setupCluster(s),
	}

	push := Step{ID: "K4", Title: "push-user-registry-artifacts", Kind: Playbook,
		Playbook: "playbooks/push-user-registry-artifacts.yml", Optional: true,
		Note: "이미지·차트 아카이브를 사용자 레지스트리에"}
	if o.ArchivePath == "" {
		push.Skip, push.SkipWhy = true, "아카이브 경로 없음 (이미 넣었으면 건너뜀)"
	} else {
		push.Args = []string{"-e", "archive=" + o.ArchivePath}
	}
	steps = append(steps, push)

	if !s.AipubEnabled {
		return steps
	}

	cert := Step{ID: "A2", Title: "setup-cert", Kind: Playbook, Playbook: "aipub-playbooks/setup-cert.yml",
		Note: "인증서, 모든 노드에 CA 설치 (" + s.AipubCertMode + ")"}
	if s.AipubCertMode == "ca_provided" || s.AipubCertMode == "tls_provided" {
		cert.Note += ". 실행 전에 러너 노드의 /etc/k8s-installer/certs에 인증서 파일을 두세요"
	}
	monitoring := Step{ID: "A4", Title: "setup-monitoring", Kind: Playbook,
		Playbook: "aipub-playbooks/setup-monitoring.yml", Optional: true,
		Note: "OpenSearch, VictoriaMetrics. aipub보다 먼저 두어 aipub 재실행이 필요 없게 함"}
	if !o.Monitoring {
		monitoring.Skip, monitoring.SkipWhy = true, "모니터링 설치 안 함"
	}

	steps = append(steps,
		Step{ID: "A1", Title: "setup-network", Kind: Playbook, Playbook: "aipub-playbooks/setup-network.yml",
			Note: "Cilium, L2 VIP " + s.K8sIngressVIP + ". 끝나면 노드가 Ready"},
		cert,
	)
	cp := s.AppliedControlPlane()
	jwks := cp.HasStage(state.StageAfterAipub)
	if cp.HasStage(state.StageAfterCert) {
		// The webhook preset reads the CA setup-cert has just put on the nodes;
		// update-cluster.yml finds the entries setup-cluster left out and applies
		// them (cp_extra_files, then control_plane), one control plane node at a time
		a25 := Step{ID: "A2.5", Title: "update-cluster (apiserver 인증 웹훅 적용)", Kind: Playbook,
			Playbook: "playbooks/update-cluster.yml",
			Note:     "K3 에서 뺀 인증 웹훅 항목을 적용. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"}
		if jwks {
			// the JWKS entries wait for the issuer, after setup-aipub
			a25.Args = stageArgs(cp, state.StageAfterCert)
			a25.Note += ". JWKS 항목은 빼고 (-e), A5.2 에서 적용"
		}
		steps = append(steps, a25)
	}
	steps = append(steps,
		Step{ID: "A3", Title: "setup-base", Kind: Playbook, Playbook: "aipub-playbooks/setup-base.yml",
			Note: "Harbor (" + haNote(s) + ")"},
		monitoring,
		Step{ID: "A5", Title: "setup-aipub", Kind: Playbook, Playbook: "aipub-playbooks/setup-aipub.yml",
			Note: "AIPub 서비스. Python 쪽 컴포넌트는 아직 aipub-installer 밖이라 포함되지 않음"},
	)
	if jwks {
		steps = append(steps, jwksSteps(s)...)
	}
	return steps
}

// jwksSteps apply the JWKS preset where aipub-installer applies JWKS: its k8s-jwks
// component runs inside setup-aipub, after the backend (the issuer) is up and
// before aipub-nav (tasks/aipub/install-aipub.yml, charts/aipub/services.yaml), on
// one control plane node at a time, with the CA setup-cert put on the nodes.
// k8s-jwks edits the static pod manifest by hand, which kubeadm drops on the next
// upgrade or join; update-cluster.yml makes the same flag, volume and file part
// of the cluster configuration, which the next ones keep.
//
// The config takes the CA inline, so A5.1 copies it in from a control plane node
// first, as k8s-jwks does (yq ... certificateAuthority = the PEM).
func jwksSteps(s *state.AppState) []Step {
	cfg := "files/" + state.JWKSConfigFile
	fill := Step{ID: "A5.1", Title: "JWKS CA 복사", Kind: Shell,
		Note: "A2 setup-cert 가 노드에 둔 CA (" + state.JWKSCAPath + ") 를 " + cfg + " 에 넣음 (k8s-jwks 처럼 인라인 PEM)"}
	if len(s.K8sCpNodes) == 0 {
		fill.Skip, fill.SkipWhy = true, "k8s 컨트롤 플레인 노드 없음"
	} else {
		const tmp = "/tmp/aipub-jwks-ca.crt"
		fill.Shell = []string{
			"ansible -i inventory.yml " + shellQuote(s.K8sCpNodes[0]) + " -b -m fetch -a " +
				shellQuote("src="+state.JWKSCAPath+" dest="+tmp+" flat=yes"),
			"sudo python3 -c " + shellQuote(caSplice) + " " + shellQuote(cfg) + " " + tmp,
			"rm -f " + tmp,
		}
	}
	apply := Step{ID: "A5.2", Title: "update-cluster (apiserver JWKS 인증 적용)", Kind: Playbook,
		Playbook: "playbooks/update-cluster.yml",
		Note: "K3 에서 뺀 JWKS 항목 (authentication-config) 을 적용. setup-aipub 의 k8s-jwks 가 손으로 넣은 것을 " +
			"kubeadm 이 보관하게 함. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"}
	if fill.Skip {
		apply.Skip, apply.SkipWhy = true, "CA 를 넣지 못함 (A5.1)"
	}
	return []Step{fill, apply}
}

// caSplice puts the PEM of argv[2] under certificateAuthority of the config at
// argv[1], in place of what is there (the placeholder, or a CA replaced since),
// and fails when the block is not there.
const caSplice = `import re,sys;f,c=sys.argv[1:3];t=open(f).read();` +
	`ca="".join("        "+l+"\n" for l in open(c).read().strip().splitlines());` +
	`t,n=re.subn(r"(?m)^(      certificateAuthority: \|\n)(?:        .*\n)+",lambda m:m.group(1)+ca,t);` +
	`assert n==1,"no certificateAuthority block in "+f;open(f,"w").write(t)`

// stageArgs are the extra vars that leave out of a run the presets applied after
// step stage: the apiserver lists as they are by then. Extra vars win over
// vars.yml for the whole run, so they are what the run applies and what
// record-applied-state.yml records; the update-cluster.yml of a later step then
// sees vars.yml differ from the record and applies the rest.
func stageArgs(cp *state.ControlPlane, stage int) []string {
	data, err := json.Marshal(map[string]any{
		state.ArgsVar("apiserver"):    cp.ExtraArgs("apiserver", stage),
		state.VolumesVar("apiserver"): cp.ExtraVolumes("apiserver", stage),
		state.CpExtraFilesVar:         cp.ExtraFiles(stage),
	})
	if err != nil {
		return nil
	}
	return []string{"-e", string(data)}
}

// setupCluster is K3. The entries of the control plane that need what a later
// step makes (the webhook preset needs the CA of setup-cert, the JWKS preset the
// issuer setup-aipub brings up) are left out of it (stageArgs) and applied by
// update-cluster.yml after that step (A2.5, A5.2).
func setupCluster(s *state.AppState) Step {
	st := Step{ID: "K3", Title: "setup-cluster", Kind: Playbook, Playbook: "playbooks/setup-cluster.yml",
		Note: "kubeadm. 끝나면 CNI가 없어 노드가 NotReady인 것이 정상"}
	cp := s.AppliedControlPlane()
	if !s.AipubEnabled || !cp.HasLater() {
		return st
	}
	st.Args = stageArgs(cp, state.StageCluster)
	var later []string
	if cp.HasStage(state.StageAfterCert) {
		later = append(later, "인증 웹훅은 A2.5")
	}
	if cp.HasStage(state.StageAfterAipub) {
		later = append(later, "JWKS 는 A5.2")
	}
	st.Note += ". AIPub 인증 항목은 빼고 만들고 (-e), " + strings.Join(later, ", ") + " 에서 적용합니다"
	return st
}

func haNote(s *state.AppState) string {
	if s.AipubHaMode {
		return "HA, StorageClass " + s.AipubHaModeStorageClass
	}
	return "HA 아님, 데이터는 " + s.AipubHarborLocalPVNode
}

// Command is the line a playbook step runs, from the ansible directory with the
// venv of k8s-installer active.
func Command(st Step) string {
	if st.Kind == Shell {
		return strings.Join(st.Shell, "\n")
	}
	if st.Kind != Playbook {
		return ""
	}
	parts := append([]string{"ansible-playbook", "-i", "inventory.yml", st.Playbook}, st.Args...)
	for i, p := range parts {
		parts[i] = shellQuote(p)
	}
	return strings.Join(parts, " ")
}

// Script is the plan as a bash script an operator can read and run by hand: the
// copy of the saved files when they were saved elsewhere, then every step that is
// not skipped, stopping at the first that fails. Every setup playbook is safe to
// run again, so running it again from the failed step is how it is resumed.
func Script(steps []Step, o Options, savedIn string) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# 설치 계획 (k8s-installer-tui). 실패하면 그 단계에서 멈춥니다. setup 플레이북은 다시 실행해도\n")
	b.WriteString("# 안전하므로, 고친 뒤 실패한 단계부터 다시 실행하면 됩니다.\n")
	b.WriteString("set -euo pipefail\n\n")

	if copies := CopyCommands(o, savedIn); len(copies) > 0 {
		b.WriteString("# 저장한 설정 파일을 설치 디렉터리로 (root 소유라 sudo)\n")
		for _, c := range copies {
			b.WriteString(c + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("source " + shellQuote(o.Root+"/venv/bin/activate") + "\n")
	b.WriteString("cd " + shellQuote(o.AnsibleDir()) + "\n")
	b.WriteString("export ANSIBLE_DISPLAY_SKIPPED_HOSTS=false\n\n")

	for _, st := range steps {
		switch {
		case st.Kind == Check:
			fmt.Fprintf(&b, "# %s %s: %s\n", st.ID, st.Title, st.Note)
		case st.Skip:
			fmt.Fprintf(&b, "# %s %s: 건너뜀 (%s)\n#   %s\n", st.ID, st.Title, st.SkipWhy, strings.ReplaceAll(Command(st), "\n", "\n#   "))
		default:
			fmt.Fprintf(&b, "echo '── %s %s'\n%s\n", st.ID, st.Title, Command(st))
		}
	}
	return b.String()
}

// CopyCommands are the commands that put the saved files where k8s-installer
// reads them, when the tool saved them somewhere else (a copy on a laptop, a
// directory of the package). None when they are already there.
func CopyCommands(o Options, savedIn string) []string {
	if savedIn == "" || filepath.ToSlash(filepath.Clean(savedIn)) == o.AnsibleDir() {
		return nil
	}
	src := strings.TrimRight(filepath.ToSlash(savedIn), "/")
	dst := o.AnsibleDir()
	return []string{
		"sudo cp " + shellQuote(src+"/inventory.yml") + " " + shellQuote(dst+"/inventory.yml"),
		"sudo cp " + shellQuote(src+"/group_vars/all/vars.yml") + " " + shellQuote(dst+"/group_vars/all/vars.yml"),
		"[ -f " + shellQuote(src+"/files/aipub/vars.yml") + " ] && sudo install -D -m 0644 " +
			shellQuote(src+"/files/aipub/vars.yml") + " " + shellQuote(dst+"/files/aipub/vars.yml") + " || true",
		// the kubeconfig of the webhook preset of the control plane, when it is on
		"[ -f " + shellQuote(src+"/files/"+state.WebhookConfigFile) + " ] && sudo install -D -m 0644 " +
			shellQuote(src+"/files/"+state.WebhookConfigFile) + " " + shellQuote(dst+"/files/"+state.WebhookConfigFile) + " || true",
		// the authentication config of the JWKS preset, when it is on
		"[ -f " + shellQuote(src+"/files/"+state.JWKSConfigFile) + " ] && sudo install -D -m 0644 " +
			shellQuote(src+"/files/"+state.JWKSConfigFile) + " " + shellQuote(dst+"/files/"+state.JWKSConfigFile) + " || true",
	}
}

// shellQuote quotes a word for bash when it needs it.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@%+,", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

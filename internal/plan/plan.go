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

// Kind tells a check (looked at, nothing changes) from a playbook (run).
type Kind int

const (
	Check Kind = iota
	Playbook
)

type Step struct {
	ID       string // P1, K1, A1, ...
	Title    string
	Kind     Kind
	Playbook string   // relative to the ansible directory, for Kind Playbook
	Args     []string // more arguments of ansible-playbook
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
	if s.ControlPlane.HasLater() {
		// The webhook preset reads the CA setup-cert has just put on the nodes;
		// update-cluster.yml finds the entries setup-cluster left out and applies
		// them (cp_extra_files, then control_plane), one control plane node at a time
		steps = append(steps, Step{ID: "A2.5", Title: "update-cluster (apiserver 인증 웹훅 적용)", Kind: Playbook,
			Playbook: "playbooks/update-cluster.yml",
			Note:     "K3 에서 뺀 인증 웹훅 항목을 적용. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"})
	}
	return append(steps,
		Step{ID: "A3", Title: "setup-base", Kind: Playbook, Playbook: "aipub-playbooks/setup-base.yml",
			Note: "Harbor (" + haNote(s) + ")"},
		monitoring,
		Step{ID: "A5", Title: "setup-aipub", Kind: Playbook, Playbook: "aipub-playbooks/setup-aipub.yml",
			Note: "AIPub 서비스. Python 쪽 컴포넌트는 아직 aipub-installer 밖이라 포함되지 않음"},
	)
}

// setupCluster is K3. The entries of the control plane that need what a later
// step makes (the webhook preset needs the CA of setup-cert) are left out of it:
// the lists without them are given as extra vars, which win over vars.yml for
// the whole run and so are what the cluster is built with and what
// record-applied-state.yml records. update-cluster.yml (A2.5) then sees vars.yml
// differ from the record and applies the rest.
func setupCluster(s *state.AppState) Step {
	st := Step{ID: "K3", Title: "setup-cluster", Kind: Playbook, Playbook: "playbooks/setup-cluster.yml",
		Note: "kubeadm. 끝나면 CNI가 없어 노드가 NotReady인 것이 정상"}
	cp := &s.ControlPlane
	if !s.AipubEnabled || !cp.HasLater() {
		return st
	}
	initial := map[string]any{
		state.ArgsVar("apiserver"):    cp.ExtraArgs("apiserver", true),
		state.VolumesVar("apiserver"): cp.ExtraVolumes("apiserver", true),
		state.CpExtraFilesVar:         cp.ExtraFiles(true),
	}
	data, err := json.Marshal(initial)
	if err != nil {
		return st
	}
	st.Args = []string{"-e", string(data)}
	st.Note += ". AIPub 인증 웹훅 항목은 빼고 만들고 (-e), A2.5 에서 적용합니다"
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
			fmt.Fprintf(&b, "# %s %s: 건너뜀 (%s)\n#   %s\n", st.ID, st.Title, st.SkipWhy, Command(st))
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

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
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// sameDir reports whether a and b are one directory: the same path once cleaned
// (a / at the end, a // inside), or the same directory on disk (a symlink on the
// way). cp would refuse to copy a file onto itself.
func sameDir(a, b string) bool {
	if path.Clean(filepath.ToSlash(a)) == path.Clean(filepath.ToSlash(b)) {
		return true
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fa, fb)
}

// Kind tells a check (looked at, nothing changes) from a playbook (run), from
// commands of the control node (run) and from what a person does (Manual: the run
// stops there until the operator says it is done and a check agrees).
type Kind int

const (
	Check Kind = iota
	Playbook
	Shell
	Manual
)

type Step struct {
	ID       string // P1, K1, A1, ...
	Title    string
	Kind     Kind
	Playbook string   // relative to the ansible directory, for Kind Playbook
	Args     []string // more arguments of ansible-playbook
	Shell    []string // the commands, for Kind Shell: a line each, run from the ansible directory
	Note     string   // one line shown under the step
	// Sees is what a check looks at, one line, shown on the plan screen in
	// place of the longer Note (which stays in the script). Var is the schema
	// variable the check stands behind, whose Preflight hint is shown with it
	Sees     string
	Var      string
	Optional bool // the operator may leave it out
	Skip     bool // left out: optional and turned off, or missing what it needs
	SkipWhy  string
	// WithPrev: runs in the stage of the step before it, at the same time (Stages)
	WithPrev bool
	// AlwaysRun: a resume runs it again even when the last run finished it (the
	// copy C1: the site may have been saved again since, and copying is safe to repeat)
	AlwaysRun bool
	// Classes are the StorageClasses a Manual step waits for (StorageNeeds)
	Classes []string

	// applies: the step applies the control plane (setup-cluster,
	// update-cluster) up to stage (state.StageCluster..StageAll); the playbooks
	// from it on run with the entries of that stage (withStages)
	applies bool
	stage   int
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

	// Self is this program on the control node, which the plan runs for
	// --fill-auth-config
	Self string
	// SiteDir is the directory the settings were saved in; empty: AnsibleDir
	SiteDir string
	// Existing: the cluster is there already (DetectSite: --existing, or the
	// record k8s-installer leaves on this control node)
	Existing bool
	// AipubInstalled: and AIPub is installed (--aipub-installed): the CA of
	// setup-cert is on the nodes and the issuer of JWKS answers
	AipubInstalled bool
	// SiteWhy is the line that says why Existing and AipubInstalled are what
	// they are (Site.Why), shown with the plan
	SiteWhy string
	// Detect decides the site again as it is now (DetectSite with the flags the
	// program was started with): a run from the first step must not take the
	// site as an unfinished run, or the start of the program, saw it, since a
	// cluster built (or reset) since then changes what K1..K3 do. Nil: the
	// screen keeps Existing and AipubInstalled as given
	Detect func() Site
	// JWKSCAReady: the authentication config of the JWKS preset was written with
	// its CA when the settings were saved (fileio.LookupCA)
	JWKSCAReady bool
	// ShellExecutable is the shell ansible runs become through, when P6 found
	// sudo refusing /bin/sh and dash usable: every playbook of the plan gets
	// -e ansible_shell_executable=<it>. Empty: ansible's default
	ShellExecutable string
	// AuditPolicy is where the wizard keeps the audit policy of the audit preset
	// (fileio.Paths.AuditPolicy): the check of an existing site puts the policy of
	// the node there when nothing is. Empty: the check writes nothing
	AuditPolicy string
	// StorageVars are the storage keys of files/aipub/vars.yml the wizard does
	// not manage (ReadStorageVars), which say what StorageClasses AIPub claims
	StorageVars map[string]string
}

func DefaultOptions() Options {
	self, err := os.Executable()
	if err != nil {
		self = "k8s-installer-tui"
	}
	return Options{Root: "/opt/k8s-installer", Monitoring: true, Self: filepath.ToSlash(self)}
}

func (o Options) AnsibleDir() string { return o.Root + "/ansible" }

// siteDir is where the settings were saved.
func (o Options) siteDir() string {
	if o.SiteDir == "" {
		return o.AnsibleDir()
	}
	return strings.TrimRight(filepath.ToSlash(o.SiteDir), "/")
}

// direct reports whether the control plane entries are applied all at once: on
// a site whose cluster and AIPub are there already, with the CA of the JWKS
// preset in its config (or the preset off), nothing waits for a later step.
func (o Options) direct(cp *state.ControlPlane) bool {
	return o.Existing && o.AipubInstalled && (!cp.JWKS || o.JWKSCAReady)
}

// Build is the plan for a new installation with these settings.
func Build(s *state.AppState, o Options) []Step {
	steps := withStages(build(s, o), s, o)
	if o.ShellExecutable != "" {
		// next to the -e of a step (the JWKS lists): ansible takes any number of them
		for i := range steps {
			if steps[i].Kind == Playbook {
				steps[i].Args = append(append([]string{}, steps[i].Args...), "-e", "ansible_shell_executable="+o.ShellExecutable)
			}
		}
	}
	return steps
}

func build(s *state.AppState, o Options) []Step {
	steps := []Step{
		{ID: "P1", Title: "setup.sh 완료", Kind: Check, Sees: "setup.sh 가 설치 디렉터리에 남긴 release 파일이 있는지",
			Note: "번들을 풀고 " + o.Root + " 에 설치하는 단계. sudo가 필요해 직접 실행합니다"},
		{ID: "P2", Title: "SSH·sudo 접속", Kind: Check, Sees: "모든 노드에 ansible_user 로 접속해 sudo -n true 가 통과하는지",
			Note: "configure-control-node-ssh.sh 가 만든 키를 모든 노드에 등록한 뒤 (설치 화면 k·p, 또는 --send-ssh-key), 모든 노드에 비밀번호 없이 접속·sudo. " +
				"키 상태만 보려면 --check-ssh"},
	}
	if o.Existing {
		// read only, and only offered: a new site has no apiserver to look at
		steps = append(steps, Step{ID: "P3", Title: "apiserver 수동 설정 확인", Kind: Check,
			Sees: "첫 컨트롤 플레인 노드의 kube-apiserver.yaml 에 손으로 넣은 설정이 켜진 프리셋과 맞는지 (읽기만)",
			Note: "첫 k8s 컨트롤 플레인 노드의 " + APIServerManifest + " 를 읽기만 함 (slurp). 손으로 넣은 감사 로그·runtime-config·" +
				"인증 플래그가 켜진 프리셋과 맞는지. 감지된 것을 끄고 update-cluster 하면 그 기능이 사라짐"})
	}
	if HelmfileChosen(s) {
		// only offered when it matters, like P3
		steps = append(steps, Step{ID: "P4", Title: "aipub-bin", Kind: Check,
			Sees: "패키지가 helmfile·kustomize 를 실었는지 (aipub-package.json 의 aipub_bin)",
			Note: "배포 도구가 helmfile: 패키지가 helmfile·kustomize 를 실었는지 (" + PackageFile + " 의 aipub_bin). " +
				"없으면 사이트가 노드에 직접 두고 경로를 변수로 지정해야 함"})
	}
	if s.AipubEnabled {
		steps = append(steps, Step{ID: "P5", Title: "카탈로그", Kind: Check,
			Sees: "카탈로그에 aipub 모듈이 쓰는 이미지가 이름으로 모두 있는지",
			Note: "aipub 플레이북은 모듈이 쓰는 이미지가 카탈로그 (ansible/aipub_vars/catalog.yml) 에 이름으로 없으면 실행 전에 거부합니다. " +
				"같은 규칙으로 미리 확인 (카탈로그 파일이 없으면 건너뜀)"})
	}
	steps = append(steps, Step{ID: "P6", Title: "become 셸", Kind: Check, Var: "ansible_user",
		Sees: "모든 노드에서 sudo -n /bin/sh -c true 가 통과하는지 (막히면 dash 로 우회할 수 있는지)",
		Note: "sudoers 가 /bin/sh 를 막으면 Ansible become 이 'Shared connection closed' 로 실패합니다. 모든 노드에서 " +
			"sudo -n /bin/sh -c true 를 확인 (읽기만). 막혔고 dash 가 있으면 계획의 ansible-playbook 에 " +
			"-e ansible_shell_executable=<dash> 를 붙입니다"})
	if s.AipubEnabled {
		steps = append(steps, Step{ID: "P7", Title: "DNS", Kind: Check, Var: "aipub_ingress_zone",
			Sees: "AIPub·Harbor 호스트가 인그레스 VIP 로 풀리는지 (getent hosts)",
			Note: "AIPub·Harbor 호스트 (<접두사>.<aipub_ingress_zone>) 가 인그레스 VIP " + s.K8sIngressVIP + " 로 풀리는지 " +
				"control node 에서 getent hosts 로 확인. DNS 는 설치 뒤에 등록하는 경우가 많아 경고만"})
		steps = append(steps, Step{ID: "P8", Title: "StorageClass", Kind: Check, Var: "aipub_ha_mode_storage_class",
			Sees: "기존 클러스터에 StorageClass " + s.AipubHaModeStorageClass + " 가 있는지 (kubectl, 읽기만)",
			Note: "기존 사이트: 첫 컨트롤 플레인 노드에서 kubectl get storageclass " + s.AipubHaModeStorageClass +
				" 로 있는지 읽기만 확인 (없으면 경고). 새 사이트는 클러스터를 만든 뒤에 만들어져 건너뜀"})
	}
	k1 := Step{ID: "K1", Title: "setup-k8s-installer", Kind: Playbook, Playbook: "playbooks/setup-k8s-installer.yml",
		Note: "모든 노드에 설치본 배포"}
	k2 := Step{ID: "K2", Title: "setup-control-node", Kind: Playbook, Playbook: "playbooks/setup-control-node.yml",
		Note: "ki cp 서비스: DNS, NTP, 레지스트리, apiserver LB"}
	if o.Existing {
		// kept in the plan, so that state.json and the screen name every step of
		// an installation, but not run: the nodes have the installer and the ki cp
		// services already, and raising them is a release upgrade, not a setup
		why := "클러스터가 이미 있음: 노드의 설치본·ki cp 서비스는 이 계획이 아니라 upgrade-k8s-installer.yml 로 올림"
		k1.Skip, k1.SkipWhy = true, why
		k2.Skip, k2.SkipWhy = true, why
	}
	steps = append(steps, k1, k2, clusterStep(s, o))
	cp := s.AppliedControlPlane()

	push := Step{ID: pushStepID, Title: "push-user-registry-artifacts", Kind: Playbook,
		Playbook: "playbooks/push-user-registry-artifacts.yml", Optional: true,
		Note: "이미지·차트 아카이브 (" + ArchiveName + ") 를 사용자 레지스트리에"}
	switch {
	case o.ArchivePath == "" && NeedsArchive(s.AipubEnabled, o):
		// the registry K2 makes is empty: nothing "was pushed already"
		push.Skip, push.SkipWhy = true, "아카이브 경로 없음: 새 사이트의 사용자 레지스트리는 비어 있어 A1 부터 이미지를 받지 못합니다"
	case o.ArchivePath == "":
		push.Skip, push.SkipWhy = true, "아카이브 경로 없음 (이미 넣었으면 건너뜀)"
	default:
		push.Args = []string{"-e", "archive=" + o.ArchivePath}
	}
	steps = append(steps, push)

	if !s.AipubEnabled {
		if o.Existing && !cp.Empty() {
			steps = append(steps, applyAll("K5"))
		}
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
		// beside setup-cert, which touches no storage: the driver needs the nodes
		// Ready (A1), and base, monitoring and aipub its classes (manual.go)
		storageStep(s, o),
	)
	direct := o.direct(cp)
	jwks := cp.HasStage(state.StageAfterAipub) && !direct
	switch {
	case direct:
		// an existing site: the CA and the issuer are there, the config is
		// written; one update-cluster.yml applies every entry, where the webhook
		// preset is applied on a new site
		if !cp.Empty() {
			steps = append(steps, applyAll("A2.5"))
		}
	case cp.HasStage(state.StageAfterCert):
		// The webhook preset reads the CA setup-cert has just put on the nodes;
		// update-cluster.yml finds the entries setup-cluster left out and applies
		// them (cp_extra_files, then control_plane), one control plane node at a time
		a25 := Step{ID: "A2.5", Title: "update-cluster (apiserver 인증 웹훅 적용)", Kind: Playbook,
			Playbook: "playbooks/update-cluster.yml",
			Note:     "K3 에서 뺀 인증 웹훅 항목을 적용. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다",
			applies:  true, stage: state.StageAll}
		if jwks {
			// the JWKS entries wait for the issuer, after setup-aipub
			a25.stage = state.StageAfterCert
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
		steps = append(steps, jwksSteps(s, o)...)
	}
	return steps
}

// applyAll is the update-cluster.yml of an existing site: every entry of the
// control plane as vars.yml holds it, no -e. Where the apiserver was edited by
// hand (P3 says so), kubeadm writes the manifest again from the cluster
// configuration: the same flags, kept from then on.
func applyAll(id string) Step {
	return Step{ID: id, Title: "update-cluster (컨트롤 플레인 항목 적용)", Kind: Playbook,
		Playbook: "playbooks/update-cluster.yml", applies: true, stage: state.StageAll,
		Note: "기존 사이트: 컨트롤 플레인 항목을 한 번에 적용 (-e 없음). 손으로 고친 kube-apiserver.yaml 은 설치기가 관리하는 같은 " +
			"플래그로 바뀜 (P3 로 먼저 확인). 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"}
}

// jwksSteps apply the JWKS preset where aipub-installer applies JWKS: its k8s-jwks
// component runs inside setup-aipub, after the backend (the issuer) is up and
// before aipub-nav (tasks/aipub/install-aipub.yml, charts/aipub/services.yaml), on
// one control plane node at a time, with the CA setup-cert put on the nodes.
// k8s-jwks edits the static pod manifest by hand, which kubeadm drops on the next
// upgrade or join; update-cluster.yml makes the same flag, volume and file part
// of the cluster configuration, which the next ones keep.
//
// The config takes the CA inline and the wizard writes it only once there is a
// CA, so A5.1 runs this program again (--fill-auth-config) on the control node,
// where setup-cert has put the CA by then: it writes the config whole to the
// saved site and to the installer, and stops the script when there is no CA.
func jwksSteps(s *state.AppState, o Options) []Step {
	site := o.siteDir()
	cmd := []string{"sudo", o.Self, "--fill-auth-config",
		"--inventory", site + "/inventory.yml",
		"--vars", site + "/group_vars/all/vars.yml",
		"--aipub-vars", site + "/files/aipub/vars.yml",
		"--installer-root", o.Root}
	if f := strings.TrimSpace(s.ControlPlane.JWKSCAFile); f != "" {
		cmd = append(cmd, "--jwks-ca", f)
	}
	for i, c := range cmd {
		cmd[i] = shellQuote(c)
	}
	ca := state.JWKSCAPath
	if f := strings.TrimSpace(s.ControlPlane.JWKSCAFile); f != "" {
		ca = f
	}
	fill := Step{ID: "A5.1", Title: "JWKS CA 채우기", Kind: Shell, Shell: []string{strings.Join(cmd, " ")},
		Note: "A2 setup-cert 가 둔 CA (" + ca + ") 를 읽어 files/" + state.JWKSConfigFile + " 를 issuer·CA 로 다시 만들어 " +
			"저장한 사이트와 설치 디렉터리 양쪽에 씀 (다시 실행해도 같음). CA 가 없으면 여기서 멈춤"}
	apply := Step{ID: "A5.2", Title: "update-cluster (apiserver JWKS 인증 적용)", Kind: Playbook,
		Playbook: "playbooks/update-cluster.yml", applies: true, stage: state.StageAll,
		Note: "K3 에서 뺀 JWKS 항목 (authentication-config) 을 적용. setup-aipub 의 k8s-jwks 가 손으로 넣은 것을 " +
			"kubeadm 이 보관하게 함. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"}
	return []Step{fill, apply}
}

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

// staged reports whether some entries of the control plane wait for a later
// step of this plan (the webhook preset for the CA of setup-cert, the JWKS
// preset for the issuer setup-aipub brings up): only then do the playbooks get
// the lists as they are applied by then (withStages). Without AIPub nothing
// waits, and on a site whose AIPub is there already (Options.direct) neither.
func staged(s *state.AppState, o Options) bool {
	cp := s.AppliedControlPlane()
	return s.AipubEnabled && cp.HasLater() && !o.direct(cp)
}

// withStages gives every playbook the control plane lists as they are applied
// by the time it runs. k8s-installer checks the files of k8s_cp_extra_files in
// every playbook that imports tasks/prepare-vars.yml, not only in the ones that
// apply them (scripts/preflight/validate-hostvars.py), and the authentication
// config of JWKS is written only once there is a CA (A5.1): a run that saw
// vars.yml whole before then would stop on it. So the stage starts at what
// setup-cluster builds, a step that applies the control plane (applies) moves
// it on for itself and the steps after it, and the lists of the stage go with
// the run as extra vars (stageArgs) until a step applies everything, after which
// vars.yml is what is applied and no -e is given. K1 (setup-k8s-installer) does
// not read them; it gets them too, so that one rule holds for every playbook.
// The script and the runner both take the arguments from here (Command).
func withStages(steps []Step, s *state.AppState, o Options) []Step {
	if !staged(s, o) {
		return steps
	}
	cp := s.AppliedControlPlane()
	at := state.StageCluster
	for i := range steps {
		st := &steps[i]
		if st.Kind != Playbook {
			continue
		}
		if st.applies && !st.Skip {
			at = st.stage
		}
		if at < state.StageAll {
			// after the step's own -e: ansible takes any number of them
			st.Args = append(append([]string{}, st.Args...), stageArgs(cp, at)...)
		}
	}
	return steps
}

// emptyAt reports whether nothing of the control plane is applied by step stage.
func emptyAt(cp *state.ControlPlane, stage int) bool {
	if len(cp.ExtraFiles(stage)) > 0 {
		return false
	}
	for _, comp := range state.CpComponents {
		if len(cp.ExtraArgs(comp, stage)) > 0 || len(cp.ExtraVolumes(comp, stage)) > 0 {
			return false
		}
	}
	return true
}

// clusterStep is K3. On a new site setup-cluster builds the cluster. The entries
// of the control plane that need what a later step makes are left out of it
// (withStages) and applied by update-cluster.yml after that step (A2.5, A5.2).
//
// On an existing cluster setup-cluster would fail (init-k8s-cluster.sh refuses a
// node whose kubelet is enabled), after it had put the extra files on the
// nodes. There K3 is update-cluster.yml with the entries of the cluster stage
// (the audit log, runtime-config), so that they are there before the AIPub
// steps, the same as on a new site; it is skipped when a later step applies
// everything at once (K5 without AIPub, the A2.5 of Options.direct) or when
// nothing is applied at that stage. The ID stays K3, so that state.json and the
// logs keep one name for it.
func clusterStep(s *state.AppState, o Options) Step {
	cp := s.AppliedControlPlane()
	st := Step{ID: "K3", Title: "setup-cluster", Kind: Playbook, Playbook: "playbooks/setup-cluster.yml",
		Note: "kubeadm. 끝나면 CNI가 없어 노드가 NotReady인 것이 정상", applies: true, stage: state.StageCluster}
	if o.Existing {
		st.Title, st.Playbook = "update-cluster (클러스터 단계 컨트롤 플레인 항목)", "playbooks/update-cluster.yml"
		st.Note = "기존 클러스터: setup-cluster 대신 update-cluster.yml 로 클러스터 단계의 컨트롤 플레인 항목 (감사 로그, runtime-config 등) 을 " +
			"AIPub 단계 전에 적용. 컨트롤 플레인 노드를 한 대씩 LB 에서 빼고 apiserver 를 재시작합니다"
		switch {
		case !s.AipubEnabled && !cp.Empty():
			st.Skip, st.SkipWhy = true, "기존 클러스터: K5 가 컨트롤 플레인 항목을 한 번에 적용"
		case s.AipubEnabled && o.direct(cp) && !cp.Empty():
			st.Skip, st.SkipWhy = true, "기존 사이트: A2.5 가 컨트롤 플레인 항목을 한 번에 적용"
		case emptyAt(cp, state.StageCluster):
			st.Skip, st.SkipWhy = true, "기존 클러스터: 이 단계에서 적용할 컨트롤 플레인 항목 없음"
		}
	}
	if !staged(s, o) {
		return st
	}
	var later []string
	if cp.HasStage(state.StageAfterCert) {
		later = append(later, "인증 웹훅은 A2.5")
	}
	if cp.HasStage(state.StageAfterAipub) {
		later = append(later, "JWKS 는 A5.2")
	}
	st.Note += ". AIPub 인증 항목은 빼고 (-e), " + strings.Join(later, ", ") + " 에서 적용합니다"
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
		case st.Kind == Manual && st.Skip:
			fmt.Fprintf(&b, "# %s %s: 건너뜀 (%s)\n", st.ID, st.Title, st.SkipWhy)
		case st.Kind == Manual:
			// a person running the script stops here too; without a terminal it goes on
			fmt.Fprintf(&b, "# %s %s (수동): %s\nread -r -p %s _ || true\n", st.ID, st.Title, st.Note,
				shellQuote("── "+st.ID+" "+ManualPrompt(st)+". 설치했으면 Enter: "))
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
	if savedIn == "" || sameDir(savedIn, o.AnsibleDir()) {
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
		// the authentication config of the JWKS preset, once it has its CA (A5.1
		// writes both copies otherwise)
		"[ -f " + shellQuote(src+"/files/"+state.JWKSConfigFile) + " ] && sudo install -D -m 0644 " +
			shellQuote(src+"/files/"+state.JWKSConfigFile) + " " + shellQuote(dst+"/files/"+state.JWKSConfigFile) + " || true",
		// the audit policy of the audit preset, when it is on: the default the
		// wizard wrote, the site's or the one taken from a node
		"[ -f " + shellQuote(src+"/files/"+state.AuditPolicyFile) + " ] && sudo install -D -m 0644 " +
			shellQuote(src+"/files/"+state.AuditPolicyFile) + " " + shellQuote(dst+"/files/"+state.AuditPolicyFile) + " || true",
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

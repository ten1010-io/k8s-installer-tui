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

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
		{ID: "P1", Title: i18n.T("setup.sh done"), Kind: Check, Sees: i18n.T("whether the release file setup.sh leaves in the install directory is there"),
			Note: i18n.T("Unpacks the bundle and installs it in %s. It needs sudo, so you run it yourself", o.Root)},
		{ID: "P2", Title: i18n.T("SSH and sudo access"), Kind: Check, Sees: i18n.T("whether every node lets ansible_user in and passes sudo -n true"),
			Note: i18n.T("Once the key configure-control-node-ssh.sh made is on every node (k or p on the install screen, or --send-ssh-key), " +
				"every node lets you in and runs sudo without a password. For the state of the key alone, --check-ssh")},
	}
	if o.Existing {
		// read only, and only offered: a new site has no apiserver to look at
		steps = append(steps, Step{ID: "P3", Title: i18n.T("apiserver settings made by hand"), Kind: Check,
			Sees: i18n.T("whether what was put by hand in kube-apiserver.yaml of the first control plane node matches the presets turned on (read only)"),
			Note: i18n.T("Reads %s of the first k8s control plane node, read only (slurp): whether the audit log, runtime-config and "+
				"authentication flags put in by hand match the presets turned on. A feature found and turned off is gone after update-cluster", APIServerManifest)})
	}
	if HelmfileChosen(s) {
		// only offered when it matters, like P3
		steps = append(steps, Step{ID: "P4", Title: "aipub-bin", Kind: Check,
			Sees: i18n.T("whether the package carries helmfile and kustomize (aipub_bin of aipub-package.json)"),
			Note: i18n.T("Deploy tool helmfile: whether the package carries helmfile and kustomize (aipub_bin of %s). "+
				"Without them, the site has to put them on the nodes itself and set their paths as variables", PackageFile)})
	}
	if s.AipubEnabled {
		steps = append(steps, Step{ID: "P5", Title: i18n.T("Catalog"), Kind: Check,
			Sees: i18n.T("whether the catalog has every image the aipub modules use, by name"),
			Note: i18n.T("The aipub playbooks refuse to run when an image their module uses is not in the catalog (ansible/aipub_vars/catalog.yml) by name. " +
				"Checked first by the same rules (skipped when there is no catalog file)")})
	}
	steps = append(steps, Step{ID: "P6", Title: i18n.T("become shell"), Kind: Check, Var: "ansible_user",
		Sees: i18n.T("whether sudo -n /bin/sh -c true passes on every node (and when refused, whether dash gets around it)"),
		Note: i18n.T("When sudoers refuses /bin/sh, Ansible become fails with 'Shared connection closed'. Checks " +
			"sudo -n /bin/sh -c true on every node (read only). When it is refused and dash is there, the ansible-playbook lines of the plan get " +
			"-e ansible_shell_executable=<dash>")})
	if s.AipubEnabled {
		steps = append(steps, Step{ID: "P7", Title: "DNS", Kind: Check, Var: "aipub_ingress_zone",
			Sees: i18n.T("whether the AIPub and Harbor hosts resolve to the ingress VIP (getent hosts)"),
			Note: i18n.T("Whether the AIPub and Harbor hosts (<prefix>.<aipub_ingress_zone>) resolve to the ingress VIP %s, "+
				"by getent hosts on the control node. DNS is often registered after the install, so only a warning", s.K8sIngressVIP)})
		steps = append(steps, Step{ID: "P8", Title: "StorageClass", Kind: Check, Var: "aipub_ha_mode_storage_class",
			Sees: i18n.T("whether StorageClass %s is in the existing cluster (kubectl, read only)", s.AipubHaModeStorageClass),
			Note: i18n.T("Existing site: kubectl get storageclass %s on the first control plane node, read only "+
				"(a warning when it is missing). Skipped on a new site, where the class is made after the cluster", s.AipubHaModeStorageClass)})
	}
	k1 := Step{ID: "K1", Title: "setup-k8s-installer", Kind: Playbook, Playbook: "playbooks/setup-k8s-installer.yml",
		Note: i18n.T("Deploys the installer to every node")}
	k2 := Step{ID: "K2", Title: "setup-control-node", Kind: Playbook, Playbook: "playbooks/setup-control-node.yml",
		Note: i18n.T("ki cp services: DNS, NTP, registry, apiserver LB")}
	if o.Existing {
		// kept in the plan, so that state.json and the screen name every step of
		// an installation, but not run: the nodes have the installer and the ki cp
		// services already, and raising them is a release upgrade, not a setup
		why := i18n.T("The cluster is there already: the installer and the ki cp services of the nodes are upgraded with upgrade-k8s-installer.yml, not by this plan")
		k1.Skip, k1.SkipWhy = true, why
		k2.Skip, k2.SkipWhy = true, why
	}
	steps = append(steps, k1, k2, clusterStep(s, o))
	cp := s.AppliedControlPlane()

	push := Step{ID: pushStepID, Title: "push-user-registry-artifacts", Kind: Playbook,
		Playbook: "playbooks/push-user-registry-artifacts.yml", Optional: true,
		Note: i18n.T("Pushes the image and chart archive (%s) to the user registry", ArchiveName)}
	switch {
	case o.ArchivePath == "" && NeedsArchive(s.AipubEnabled, o):
		// the registry K2 makes is empty: nothing "was pushed already"
		push.Skip, push.SkipWhy = true, i18n.T("No archive path: the user registry of a new site is empty, so the steps from A1 on cannot pull their images")
	case o.ArchivePath == "":
		push.Skip, push.SkipWhy = true, i18n.T("No archive path (fine when it was pushed already)")
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
		Note: i18n.T("Certificates, and the CA installed on every node (%s)", s.AipubCertMode)}
	if s.AipubCertMode == "ca_provided" || s.AipubCertMode == "tls_provided" {
		cert.Note += ". " + i18n.T("Put the certificate files in /etc/k8s-installer/certs of the runner node before the run")
	}
	monitoring := Step{ID: "A4", Title: "setup-monitoring", Kind: Playbook,
		Playbook: "aipub-playbooks/setup-monitoring.yml", Optional: true,
		Note: i18n.T("OpenSearch, VictoriaMetrics. Before aipub, so that aipub need not run again")}
	if !o.Monitoring {
		monitoring.Skip, monitoring.SkipWhy = true, i18n.T("Monitoring not installed")
	}

	steps = append(steps,
		Step{ID: "A1", Title: "setup-network", Kind: Playbook, Playbook: "aipub-playbooks/setup-network.yml",
			Note: i18n.T("Cilium, L2 VIP %s. The nodes are Ready when it ends", s.K8sIngressVIP)},
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
		a25 := Step{ID: "A2.5", Title: i18n.T("update-cluster (apply the apiserver authentication webhook)"), Kind: Playbook,
			Playbook: "playbooks/update-cluster.yml",
			Note:     i18n.T("Applies the authentication webhook entries left out of K3.") + " " + updateNote(),
			applies:  true, stage: state.StageAll}
		if jwks {
			// the JWKS entries wait for the issuer, after setup-aipub
			a25.stage = state.StageAfterCert
			a25.Note += ". " + i18n.T("JWKS entries left out (-e), applied in A5.2")
		}
		steps = append(steps, a25)
	}
	steps = append(steps,
		Step{ID: "A3", Title: "setup-base", Kind: Playbook, Playbook: "aipub-playbooks/setup-base.yml",
			Note: "Harbor (" + haNote(s) + ")"},
		monitoring,
		Step{ID: "A5", Title: "setup-aipub", Kind: Playbook, Playbook: "aipub-playbooks/setup-aipub.yml",
			Note: i18n.T("AIPub services. The Python components are not in aipub-installer yet, so they are not included")},
	)
	if jwks {
		steps = append(steps, jwksSteps(s, o)...)
	}
	return steps
}

// updateClusterTime is what an update-cluster.yml takes, for the notes of the
// steps that run it, so that a step minutes long is not taken for one that
// hangs. Per control plane node, one after another: `kubeadm upgrade node phase
// control-plane` (update-k8s-cp-config.sh) backs up and checks etcd on every node,
// --etcd-upgrade being on by default, even when only apiserver flags change; a
// flag and a file changed together restart the apiserver twice (the manifest
// kubeadm writes again, then restart-k8s-cp-components.sh), each followed by 30
// seconds of /readyz. Measured on three control plane nodes: 3 to 6 minutes a
// node, about 15 in all, one task of a healthy node close to 5 minutes. Every
// wait has its limit - kubeadm's own for each component (UpgradeManifests, 5
// minutes by default), then 300 seconds each for the apiserver and for the LB
// (wait-ready) - and a failure has the rescue put the node back in the LB and
// stop. A kill (Ctrl+C twice) runs no rescue: the node is left drained in the LB
// until `lb-admin.sh ready <node>` on every ki cp node. English, the key of its
// translation: shown through i18n.T (updateNote).
const updateClusterTime = "3 to 6 minutes per control plane node (about 15 for three): kubeadm backs up and checks etcd on every node, " +
	"and restarts the apiserver twice when a flag and a file change together (once when only one does). A single task may take close to 5 minutes. " +
	"Every wait has a limit, past which the node is put back in the LB and the run stops. To stop, Ctrl+C once (stops after this step); " +
	"twice (stops at once) leaves the node out of the LB, to be put back with lb-admin.sh ready <node> on every ki cp node"

// updateNote ends the note of every step that runs update-cluster.yml: what it
// does to the control plane nodes, and how long it takes.
func updateNote() string {
	return i18n.T("Takes the control plane nodes out of the LB one at a time and restarts the apiserver.") + " " + i18n.T(updateClusterTime)
}

// applyAll is the update-cluster.yml of an existing site: every entry of the
// control plane as vars.yml holds it, no -e. Where the apiserver was edited by
// hand (P3 says so), kubeadm writes the manifest again from the cluster
// configuration: the same flags, kept from then on.
func applyAll(id string) Step {
	return Step{ID: id, Title: i18n.T("update-cluster (apply the control plane entries)"), Kind: Playbook,
		Playbook: "playbooks/update-cluster.yml", applies: true, stage: state.StageAll,
		Note: i18n.T("Existing site: applies every control plane entry at once (no -e). A kube-apiserver.yaml edited by hand gets the same "+
			"flags, managed by the installer (P3 checks first).") + " " + updateNote()}
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
	// in the language of this run, said always: sudo may or may not pass
	// K8S_INSTALLER_TUI_LANG on, and the flag wins over it
	cmd = append(cmd, "--lang", string(i18n.Current()))
	for i, c := range cmd {
		cmd[i] = shellQuote(c)
	}
	ca := state.JWKSCAPath
	if f := strings.TrimSpace(s.ControlPlane.JWKSCAFile); f != "" {
		ca = f
	}
	fill := Step{ID: "A5.1", Title: i18n.T("Fill in the JWKS CA"), Kind: Shell, Shell: []string{strings.Join(cmd, " ")},
		Note: i18n.T("Reads the CA A2 setup-cert put there (%s), makes files/%s again with the issuer and the CA, "+
			"and writes it to both the saved site and the install directory (the same when run again). Stops here when there is no CA", ca, state.JWKSConfigFile)}
	apply := Step{ID: "A5.2", Title: i18n.T("update-cluster (apply the apiserver JWKS authentication)"), Kind: Playbook,
		Playbook: "playbooks/update-cluster.yml", applies: true, stage: state.StageAll,
		Note: i18n.T("Applies the JWKS entries left out of K3 (authentication-config), so that kubeadm keeps what "+
			"k8s-jwks of setup-aipub put in by hand.") + " " + updateNote()}
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
		Note: i18n.T("kubeadm. When it ends the nodes are NotReady, as there is no CNI yet: that is expected"), applies: true, stage: state.StageCluster}
	if o.Existing {
		st.Title, st.Playbook = i18n.T("update-cluster (control plane entries of the cluster stage)"), "playbooks/update-cluster.yml"
		st.Note = i18n.T("Existing cluster: update-cluster.yml, in place of setup-cluster, applies the control plane entries of the cluster stage "+
			"(the audit log, runtime-config and the like) before the AIPub steps.") + " " + updateNote()
		switch {
		case !s.AipubEnabled && !cp.Empty():
			st.Skip, st.SkipWhy = true, i18n.T("Existing cluster: K5 applies the control plane entries at once")
		case s.AipubEnabled && o.direct(cp) && !cp.Empty():
			st.Skip, st.SkipWhy = true, i18n.T("Existing site: A2.5 applies the control plane entries at once")
		case emptyAt(cp, state.StageCluster):
			st.Skip, st.SkipWhy = true, i18n.T("Existing cluster: no control plane entry to apply at this stage")
		}
	}
	if !staged(s, o) {
		return st
	}
	var later []string
	if cp.HasStage(state.StageAfterCert) {
		later = append(later, i18n.T("the authentication webhook in A2.5"))
	}
	if cp.HasStage(state.StageAfterAipub) {
		later = append(later, i18n.T("JWKS in A5.2"))
	}
	st.Note += ". " + i18n.T("AIPub authentication entries left out (-e), applied later: %s", strings.Join(later, ", "))
	return st
}

func haNote(s *state.AppState) string {
	if s.AipubHaMode {
		return "HA, StorageClass " + s.AipubHaModeStorageClass
	}
	return i18n.T("not HA, data on %s", s.AipubHarborLocalPVNode)
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
	b.WriteString(i18n.T("# Install plan (k8s-installer-tui). It stops at the step that fails. The setup playbooks are safe\n" +
		"# to run again: fix the cause, then run it again from the failed step.\n"))
	b.WriteString("set -euo pipefail\n\n")

	if copies := CopyCommands(o, savedIn); len(copies) > 0 {
		b.WriteString(i18n.T("# Copy the saved settings to the install directory (sudo: it is root's)\n"))
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
			fmt.Fprintf(&b, "# %s %s: %s\n", st.ID, st.Title, i18n.T("skipped (%s)", st.SkipWhy))
		case st.Kind == Manual:
			// a person running the script stops here too; without a terminal it goes on
			fmt.Fprintf(&b, "# %s %s %s: %s\nread -r -p %s _ || true\n", st.ID, st.Title, i18n.T("(manual)"), st.Note,
				shellQuote("── "+st.ID+" "+ManualPrompt(st)+". "+i18n.T("Enter once it is installed:")+" "))
		case st.Skip:
			fmt.Fprintf(&b, "# %s %s: %s\n#   %s\n", st.ID, st.Title, i18n.T("skipped (%s)", st.SkipWhy), strings.ReplaceAll(Command(st), "\n", "\n#   "))
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

package plan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCheckArchive(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ArchiveName)
	os.WriteFile(file, []byte("x"), 0644)
	if err := CheckArchive(file); err != nil {
		t.Errorf("a regular file: %v", err)
	}
	if err := CheckArchive(filepath.Join(dir, "missing.tgz")); err == nil || !strings.Contains(err.Error(), "찾을 수 없습니다") {
		t.Errorf("missing: %v", err)
	}
	if err := CheckArchive(dir); err == nil || !strings.Contains(err.Error(), "일반 파일이 아닙니다") {
		t.Errorf("a directory: %v", err)
	}
	if os.Getuid() != 0 {
		locked := filepath.Join(dir, "locked.tgz")
		os.WriteFile(locked, []byte("x"), 0)
		if err := CheckArchive(locked); err == nil || !strings.Contains(err.Error(), "읽을 수 없습니다") {
			t.Errorf("unreadable: %v", err)
		}
	}
}

// FindArchive: one, several (each once), none; a directory of the name is no archive.
func TestFindArchive(t *testing.T) {
	exe, wd, pkg, empty := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(pkg, ArchiveName), []byte("x"), 0644)
	os.Mkdir(filepath.Join(empty, ArchiveName), 0755)
	if got := FindArchive([]string{exe, wd, empty}); len(got) != 0 {
		t.Errorf("none: %v", got)
	}
	if got := FindArchive([]string{exe, pkg, wd, pkg}); len(got) != 1 || got[0] != filepath.Join(pkg, ArchiveName) {
		t.Errorf("one: %v", got)
	}
	os.WriteFile(filepath.Join(wd, ArchiveName), []byte("x"), 0644)
	if got := FindArchive([]string{exe, wd, pkg}); len(got) != 2 || got[0] != filepath.Join(wd, ArchiveName) {
		t.Errorf("several: %v", got)
	}
}

// ArchiveDirs looks one level under the home directory: ~/<package>/.
func TestArchiveDirsLookUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Mkdir(filepath.Join(home, "aipub-package"), 0755)
	os.WriteFile(filepath.Join(home, "aipub-package", ArchiveName), []byte("x"), 0644)
	if got := FindArchive(ArchiveDirs()); len(got) != 1 || got[0] != filepath.Join(home, "aipub-package", ArchiveName) {
		t.Errorf("found %v", got)
	}
}

// K4 on a new AIPub site without an archive says why A1 will fail; an
// existing site keeps the old word.
func TestK4SkipSaysWhy(t *testing.T) {
	k4 := func(o Options) Step {
		for _, st := range Build(aipubState(), o) {
			if st.ID == "K4" {
				return st
			}
		}
		t.Fatal("no K4")
		return Step{}
	}
	o := testOptions()
	if st := k4(o); !st.Skip || !strings.Contains(st.SkipWhy, "A1 부터 이미지를 받지 못합니다") {
		t.Errorf("new site: %+v", st)
	}
	o.Existing = true
	if st := k4(o); !st.Skip || st.SkipWhy != "아카이브 경로 없음 (이미 넣었으면 건너뜀)" {
		t.Errorf("existing site: %+v", st)
	}
	if NeedsArchive(true, o) || NeedsArchive(false, testOptions()) || !NeedsArchive(true, testOptions()) {
		t.Error("NeedsArchive: only a new site with AIPub")
	}
}

// A resume runs a K4 the last run skipped once the plan has an archive: before
// the step it stopped at, with the path recorded.
func TestRunnerResumeRunsASkippedK4(t *testing.T) {
	o := testOptions()
	noArchive := Stages(Build(aipubState(), o), o, o.AnsibleDir())
	r, f, inputs := newTestRunner(t, noArchive)
	f.fail["A1"] = 1
	if outcome, _ := r.Run(context.Background(), false); outcome != OutcomeFailed {
		t.Fatalf("first run: %s", outcome)
	}
	if st := r.Snapshot(); st.Step(pushStepID).Status != StatusSkipped {
		t.Fatalf("K4: %s", statuses(st))
	}

	withArchive := r.Opts
	withArchive.ArchivePath = "/media/usb/" + ArchiveName
	stages := Stages(Build(aipubState(), withArchive), withArchive, withArchive.AnsibleDir())
	r2 := NewRunner(withArchive, r.State, stages, inputs)
	r2.Dir = r.Dir
	f2 := newFakeSite(stages)
	f2.classes["nfs-client"] = true
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	if outcome, err := r2.Run(context.Background(), true); outcome != OutcomeOK || err != nil {
		t.Fatalf("resume: %s %v", outcome, err)
	}
	if got := f2.ranIDs(); !strings.HasPrefix(got, "K4 A1 A2") {
		t.Errorf("resume ran %s", got)
	}
	st, _ := LoadRunState(r.Dir)
	if st.ArchivePath != withArchive.ArchivePath || st.Step("K4").Status != StatusOK {
		t.Errorf("state.json: %q %s", st.ArchivePath, statuses(*st))
	}
}

// The failure of a helm step whose pods did not start points at the images,
// and at K4 when this run skipped it.
func TestDiagnosisOfPodsThatDidNotStart(t *testing.T) {
	log := filepath.Join(t.TempDir(), "A1.log")
	os.WriteFile(log, []byte("TASK [Install cilium]\nfatal: [master1]: FAILED! => {\"msg\": \"Error: INSTALLATION FAILED: context deadline exceeded\"}\n"), 0644)
	network := Step{ID: "A1", Kind: Playbook, Playbook: "aipub-playbooks/setup-network.yml"}
	note := knownFailure(network, log, failureContext{k4: StatusSkipped})
	for _, want := range []string{"파드가 시작되지 못해", "사용자 레지스트리에 이미지가 없는 것", "kubectl -n kube-system get pods",
		"K4 push-user-registry-artifacts 를 건너뛰었습니다", ArchiveName, "--install --resume --archive"} {
		if !strings.Contains(note, want) {
			t.Errorf("no %q in %q", want, note)
		}
	}
	if note := knownFailure(network, log, failureContext{k4: StatusPending}); !strings.Contains(note, "K4 push-user-registry-artifacts 를 건너뛰었습니다") {
		t.Errorf("K4 never ran: %q", note)
	}
	note = knownFailure(network, log, failureContext{k4: StatusOK})
	if !strings.Contains(note, "kubectl -n kube-system get pods") || strings.Contains(note, "K4") {
		t.Errorf("K4 ran: %q", note)
	}
	os.WriteFile(log, []byte("cilium-abcde   0/1   ImagePullBackOff\n"), 0644)
	base := Step{ID: "A3", Kind: Playbook, Playbook: "aipub-playbooks/setup-base.yml"}
	if note := knownFailure(base, log, failureContext{k4: StatusOK}); !strings.Contains(note, "kubectl -n aipub get pods") {
		t.Errorf("A3: %q", note)
	}
	// not a helm step: no word of pods
	cluster := Step{ID: "K3", Kind: Playbook, Playbook: "playbooks/setup-cluster.yml"}
	if note := knownFailure(cluster, log, failureContext{k4: StatusSkipped}); strings.Contains(note, "파드") {
		t.Errorf("K3: %q", note)
	}
}

// Through the runner: the K4 of the run is what the note says.
func TestRunnerDiagnosesImagesWhenK4WasSkipped(t *testing.T) {
	o := testOptions()
	r, f, _ := newTestRunner(t, Stages(Build(aipubState(), o), o, o.AnsibleDir()))
	f.fail["A1"] = 1
	f.out["A1"] = "Error: INSTALLATION FAILED: resource not ready, name: cilium, kind: DaemonSet\n"
	r.Run(context.Background(), false)
	snap := r.Snapshot()
	if a1 := snap.Step("A1"); !strings.Contains(a1.Note, "K4 push-user-registry-artifacts 를 건너뛰었습니다") {
		t.Errorf("A1: %+v", a1)
	}
}

// A refusal over a file of k8s_cp_extra_files never says to make it empty.
func TestDiagnosisOfAMissingExtraFile(t *testing.T) {
	log := filepath.Join(t.TempDir(), "x.log")
	os.WriteFile(log, []byte(`fatal: [localhost]: FAILED! => {"msg": "File[\"/opt/k8s-installer/ansible/files/aipub/authentication-config.yaml\"] of the control node is not there. k8s_cp_extra_files"}`+"\n"), 0644)
	note := knownFailure(Step{ID: "K1", Playbook: "playbooks/setup-control-node.yml"}, log, failureContext{})
	if strings.Contains(note, "touch") || strings.Contains(note, "빈 파일을 생성") || !strings.Contains(note, "손으로 실행") ||
		!strings.Contains(note, "빈 파일을 만들지 마세요") {
		t.Errorf("%q", note)
	}
}

// The leftovers of an earlier install point at a reset only where it is
// offered: a new site, K1..K3.
func TestDiagnosisOffersAResetOnlyWhereItHelps(t *testing.T) {
	log := filepath.Join(t.TempDir(), "K3.log")
	os.WriteFile(log, []byte("fatal: [master1]: FAILED! => {\"msg\": \"Kubelet already enabled\"}\n"), 0644)
	k3 := Step{ID: "K3", Playbook: "playbooks/setup-cluster.yml"}
	if note := knownFailure(k3, log, failureContext{resettable: true}); !strings.Contains(note, "reset-cluster.yml") {
		t.Errorf("new site K3: %q", note)
	}
	if note := knownFailure(k3, log, failureContext{}); strings.Contains(note, "reset") || strings.Contains(note, "설치 화면") {
		t.Errorf("not resettable: %q", note)
	}
	for _, c := range []struct {
		existing bool
		id       string
		want     bool
	}{{false, "K1", true}, {false, "K3", true}, {false, "K4", false}, {false, "A1", false}, {true, "K3", false}, {false, "C1", false}} {
		if ResetHelps(c.existing, c.id) != c.want {
			t.Errorf("ResetHelps(%t, %s) != %t", c.existing, c.id, c.want)
		}
	}
}

// reset-cluster.yml runs as the steps do: this user, the venv, its log.
func TestResetScript(t *testing.T) {
	o := testOptions()
	o.ShellExecutable = "/usr/bin/dash"
	script := ResetScript(o)
	for _, want := range []string{"source /opt/k8s-installer/venv/bin/activate", "ansible-playbook -i inventory.yml playbooks/reset-cluster.yml -e sdfc= -e '{\"k8s_cp_extra_files\":[]}' -e ansible_shell_executable=/usr/bin/dash",
		"tee -a /opt/k8s-installer/tui-install/logs/reset-cluster.log", "set -o pipefail"} {
		if !strings.Contains(script, want) {
			t.Errorf("no %q in\n%s", want, script)
		}
	}
	if strings.Contains(script, "sudo") {
		t.Errorf("under sudo:\n%s", script)
	}
}

func TestCleanErrorLineCutsOnRunes(t *testing.T) {
	got := cleanErrorLine(strings.Repeat("가", 200))
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 140 || !strings.HasSuffix(got, "...") {
		t.Errorf("%q (%d runes)", got, utf8.RuneCountInString(got))
	}
	if got := cleanErrorLine("짧은 오류"); got != "짧은 오류" {
		t.Errorf("%q", got)
	}
}

// The raw module runs under a terminal: -o prints its \r\n as text.
func TestParseAdhocRawOutput(t *testing.T) {
	out := `n1 | CHANGED | rc=0 | (stdout) /usr/bin/dash\r\n (stderr) Shared connection to n1 closed.\r\n` + "\n" +
		`n2 | CHANGED | rc=0 | (stdout) inactive\r\nactive\r\n` + "\n"
	res := parseAdhoc([]byte(out))
	if a := res["n1"]; a.Stdout != "/usr/bin/dash" {
		t.Errorf("n1: %q", a.Stdout)
	}
	if a := res["n2"]; a.Stdout != "inactive\nactive" {
		t.Errorf("n2: %q", a.Stdout)
	}
	// P6 takes the dash path from it whole
	plain := "n1 | CHANGED | rc=0 | (stdout) \n"
	viaSh := "n1 | FAILED | rc=1 | (stdout) Sorry, user tester is not allowed to execute '/bin/sh -c true' as root on n1.\\r\\n (stderr) Shared connection to n1 closed.\\r\\n\n"
	probe := `n1 | CHANGED | rc=0 | (stdout) /usr/bin/dash\r\n (stderr) Shared connection to n1 closed.\r\n` + "\n"
	if _, shell := CheckBecomeShell(context.Background(), testOptions(), becomeNodes(plain, viaSh, probe)); shell != "/usr/bin/dash" {
		t.Errorf("shell %q", shell)
	}
}

// P2 warns of a firewall that is active on a node, ufw after an absent firewalld too.
func TestCheckSSHWarnsOfAFirewall(t *testing.T) {
	run := func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if strings.Contains(args[len(args)-1], "systemctl is-active") {
			return []byte(`n1 | CHANGED | rc=0 | (stdout) inactive\r\nactive\r\n` + "\n" +
				`n2 | CHANGED | rc=0 | (stdout) inactive\r\ninactive\r\ninactive\r\n` + "\n"), nil
		}
		return []byte("n1 | CHANGED | rc=0 | (stdout) \nn2 | CHANGED | rc=0 | (stdout) \n"), nil
	}
	r := CheckSSH(context.Background(), testOptions(), run)
	got := strings.Join(r.Lines, "\n")
	if !r.OK || !r.Warn || !strings.Contains(got, "방화벽(firewalld/ufw) 활성 노드: n1 ") {
		t.Errorf("%+v", r)
	}
}

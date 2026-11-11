package plan

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

func hangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// inEnglish runs the rest of the test in English, back to Korean (TestMain) at its end.
func inEnglish(t *testing.T) {
	i18n.Set(i18n.EN)
	t.Cleanup(func() { i18n.Set(i18n.KO) })
}

// The plan says nothing in Korean by default: the titles, notes and skip
// reasons of every step, the script, the stages and the prompt of the stop.
func TestPlanInEnglish(t *testing.T) {
	inEnglish(t)
	allOn := func(s *state.AppState) *state.AppState {
		for _, p := range state.Presets {
			s.ControlPlane.Set(p.ID, true)
		}
		return s
	}
	type site struct {
		name string
		s    *state.AppState
		o    Options
	}
	var sites []site
	{
		s := allOn(aipubState())
		s.AipubCertMode, s.AipubAppDeployTool = "ca_provided", "helmfile"
		sites = append(sites, site{"new site", s, testOptions()})
	}
	{
		s := allOn(aipubState())
		s.AipubHaMode, s.AipubHarborLocalPVNode = false, "node1"
		o := testOptions()
		o.Monitoring = false
		o.StorageVars = map[string]string{"aipub_app_file_storage_type": "static"}
		sites = append(sites, site{"no StorageClass, no monitoring", s, o})
	}
	{
		s := aipubState()
		s.AipubHaMode = false
		o := testOptions()
		o.StorageVars = map[string]string{"aipub_monitoring_storage_type": "dynamic"}
		sites = append(sites, site{"default class", s, o})
	}
	for _, installed := range []bool{false, true} {
		o := testOptions()
		o.Existing, o.AipubInstalled, o.JWKSCAReady = true, installed, true
		sites = append(sites, site{"existing site", allOn(aipubState()), o})
	}
	{
		s := allOn(state.DefaultState())
		o := testOptions()
		o.Existing = true
		sites = append(sites, site{"existing cluster without AIPub", s, o})
	}

	seen := map[string]bool{}
	for _, c := range sites {
		steps := Build(c.s, c.o)
		for _, st := range steps {
			seen[st.ID] = true
			for what, text := range map[string]string{"title": st.Title, "note": st.Note, "sees": st.Sees, "skip": st.SkipWhy} {
				if hangul(text) {
					t.Errorf("%s: %s %s in Korean: %q", c.name, st.ID, what, text)
				}
			}
			if st.Kind == Manual && !st.Skip {
				if p := ManualPrompt(st); hangul(p) {
					t.Errorf("%s: the prompt of %s in Korean: %q", c.name, st.ID, p)
				}
			}
			if st.Playbook == "playbooks/update-cluster.yml" && !st.Skip && !strings.Contains(st.Note, updateClusterTime) {
				t.Errorf("%s: %s says nothing of the time in English: %q", c.name, st.ID, st.Note)
			}
		}
		for _, stage := range Stages(steps, c.o, "/home/ops/site") {
			for _, st := range stage {
				if hangul(st.Title) || hangul(st.Note) {
					t.Errorf("%s: stage step %s in Korean: %q %q", c.name, st.ID, st.Title, st.Note)
				}
			}
		}
		if script := Script(steps, c.o, "/home/ops/site"); hangul(script) {
			t.Errorf("%s: the script in Korean:\n%s", c.name, script)
		}
	}
	for _, id := range []string{"P3", "P4", "P5", "P8", "K3", "K4", "M1", "A2.5", "A5.1", "A5.2", "K5"} {
		if !seen[id] {
			t.Errorf("no plan had %s", id)
		}
	}

	// a few phrases, as the operator reads them
	s := allOn(aipubState())
	steps := Build(s, testOptions())
	script := Script(steps, testOptions(), "/home/ops/site")
	for _, want := range []string{
		"# Install plan (k8s-installer-tui). It stops at the step that fails.",
		"# Copy the saved settings to the install directory (sudo: it is root's)",
		"# K4 push-user-registry-artifacts: skipped (No archive path: the user registry of a new site is empty",
		"# M1 Install the StorageClass driver (manual): Waits alongside setup-cert.",
		"read -r -p '── M1 Install the StorageClass driver — the classes the wizard uses: nfs-client. Enter once it is installed: ' _ || true",
		"echo '── A2.5 update-cluster (apply the apiserver authentication webhook)'",
		"echo '── A5.1 Fill in the JWKS CA'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the script lacks %q:\n%s", want, script)
		}
	}
	for _, st := range steps {
		if st.ID == "K3" && !strings.HasPrefix(st.Note, "kubeadm. When it ends the nodes are NotReady") {
			t.Errorf("K3: %q", st.Note)
		}
	}
}

// What the checks, the site and the runner say is English by default too.
func TestChecksInEnglish(t *testing.T) {
	inEnglish(t)
	o := testOptions()
	o.Root = t.TempDir()
	old := ClusterRecord
	ClusterRecord = filepath.Join(t.TempDir(), "applied-release")
	t.Cleanup(func() { ClusterRecord = old })

	site := DetectSite(false, false)
	prev := &RunState{Existing: true, AipubInstalled: true, Started: time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)}
	legacy := &Legacy{Node: "node1", Presets: map[string]bool{state.PresetRuntimeConfig: true},
		Found: []string{"--runtime-config=" + runtimeConfigValue}}
	cpOn := aipubState()
	cpOn.ControlPlane.Set(state.PresetRuntimeConfig, true)
	nodes := []NodeSSH{
		{Node: SSHNode{"n1", "10.0.0.1", "22", "root"}, Key: KeyRefused, SendErr: "Permission denied"},
		{Node: SSHNode{"n2", "10.0.0.2", "2222", "ops"}, Key: KeyOK, Sudo: SudoNeedsPassword},
		{Node: SSHNode{"n3", "10.0.0.3", "22", "ops"}, Key: KeyOK},
	}
	_, inventoryWarnings, _ := parseInventory([]byte(inventoryList), "me")
	lines := map[string][]string{
		"P1":        CheckInstalled(o).Lines,
		"P5":        CheckCatalog(o).Lines,
		"P4":        CheckAipubBin(o, func() *state.AppState { s := aipubState(); s.AipubAppDeployTool = "helmfile"; return s }()).Lines,
		"P7":        CheckDNS(context.Background(), o, state.DefaultState(), nil).Lines,
		"P8":        CheckStorageClass(context.Background(), o, aipubState(), nil).Lines,
		"P3":        LegacyResult(legacy, cpOn).Lines,
		"site":      {site.Why, ResumedSite(prev, site.Why)},
		"lock":      {(&LockedError{Path: "/x/lock", PID: "42"}).Error(), (&StillRunningError{Step: "K3", PGID: 7}).Error()},
		"archive":   {CheckArchive(filepath.Join(o.Root, ArchiveName)).Error()},
		"ssh":       {SSHTable(nodes), nodes[0].Label(), nodes[0].Note(), nodes[1].Label(), nodes[1].Note()},
		"inventory": inventoryWarnings,
		"send":      {AskSendScript(DefaultSSHKey(), []SSHNode{nodes[0].Node}, true)},
	}
	for what, ls := range lines {
		if len(ls) == 0 {
			t.Errorf("%s: nothing said", what)
		}
		for _, l := range ls {
			if hangul(l) {
				t.Errorf("%s in Korean: %q", what, l)
			}
		}
	}
	for what, want := range map[string]string{
		"P1":      "/release is not there. Run sudo ./setup.sh in the package directory first",
		"P5":      "no catalog file",
		"P3":      "All on: update-cluster replaces what was edited by hand",
		"site":    "New site: no " + ClusterRecord,
		"archive": "archive " + filepath.Join(o.Root, ArchiveName) + " not found",
		"ssh":     "NODE",
	} {
		if !strings.Contains(strings.Join(lines[what], "\n"), want) {
			t.Errorf("%s lacks %q: %q", what, want, lines[what])
		}
	}
}

// The constants given to i18n.T, which the check of the root (it reads only
// literal keys) does not see, have their Korean.
func TestConstantKeysHaveTheirKorean(t *testing.T) {
	ko := i18n.Catalog(i18n.KO)
	for _, k := range []string{updateClusterTime, DefaultClass} {
		if ko[k] == "" || hangul(k) {
			t.Errorf("no Korean for %q", k)
		}
	}
}

// The header of an attempt is found by the step's ID and its command, not its
// title: a run started in Korean and resumed in English reads only its last
// attempt, with the title of either language. A step the last run finished
// takes the title of this one.
func TestAttemptLinesAcrossLanguages(t *testing.T) {
	steps := func() [][]Step {
		return [][]Step{
			{{ID: "X1", Title: i18n.T("Copy the settings"), Kind: Shell, Shell: []string{"true"}}},
			{{ID: "K3", Title: i18n.T("update-cluster (control plane entries of the cluster stage)"), Kind: Playbook, Playbook: "playbooks/update-cluster.yml"}},
		}
	}
	ko := steps() // in Korean (TestMain)
	r, f, inputs := newTestRunner(t, ko)
	f.fail["K3"] = 2
	f.out["K3"] = "first attempt\n"
	if outcome, err := r.Run(context.Background(), false); outcome != OutcomeFailed || err != nil {
		t.Fatalf("first run: %s %v", outcome, err)
	}

	inEnglish(t)
	en := steps()
	if en[1][0].Title == ko[1][0].Title {
		t.Fatalf("the title is the same in both languages: %q", en[1][0].Title)
	}
	r2 := NewRunner(r.Opts, r.State, en, inputs)
	f2 := newFakeSite(en)
	f2.fail["K3"] = 2
	f2.out["K3"] = "second attempt\n"
	r2.Exec, r2.Cmd = f2.exec, f2.cmd
	if outcome, err := r2.Run(context.Background(), true); outcome != OutcomeFailed || err != nil || f2.ranIDs() != "K3" {
		t.Fatalf("resume: %s %v, ran %s", outcome, err, f2.ranIDs())
	}
	snap := r2.Snapshot()
	if x1 := snap.Step("X1"); x1.Status != StatusOK || x1.Title != en[0][0].Title {
		t.Errorf("X1 kept from the last run: %+v", x1)
	}
	log := filepath.Join(r.Dir, "logs", "K3.log")
	for _, st := range []Step{en[1][0], ko[1][0]} {
		got := strings.Join(attemptLines(log, st), "\n")
		if strings.Contains(got, "first attempt") || !strings.Contains(got, "second attempt") || !strings.HasPrefix(got, "── ") {
			t.Errorf("read with the title %q:\n%s", st.Title, got)
		}
	}
}

package plan

import (
	"regexp"
	"strings"
)

// A stage is the steps that run at the same time; the next starts when every
// one of them has succeeded.
//
// Every playbook of both installers opens with begin-playbook.yml (k8s-installer
// playbooks/tasks): sync-ansible.yml tars the whole ansible directory and
// replaces it on the other ki cp nodes (rm -rf, tar xf), and prepare-vars.yml /
// reset-vars-file.yml write and remove the fixed files of /tmp/k8s-installer
// (vars.yml, localhost-vars.yml, the ansible archive). Two playbooks at once from
// the one control node would overwrite each other's archive and vars file, and
// k8s-installer has no lock of its own, so every playbook is a stage alone. The
// shell steps are too: the copy goes first, and A5.1 writes files/ that the next
// playbook's sync-ansible.yml tars.
//
// What does run beside another step is the StorageClass stop (M1, WithPrev):
// the operator installs the driver while setup-cert runs. Its only command is a
// read-only ansible ad-hoc kubectl, which touches neither the ansible directory
// nor /tmp/k8s-installer.

// Stages are the steps that run, in stages: the checks are left out (the screen
// and --install run them first), a skipped step stays to be recorded. When the
// settings were saved elsewhere, a first step copies them where k8s-installer
// reads them (CopyCommands, as the script does), on every run: a resume after
// the site was saved again must not run the playbooks on the old copy.
func Stages(steps []Step, o Options, savedIn string) [][]Step {
	var stages [][]Step
	if copies := CopyCommands(o, savedIn); len(copies) > 0 {
		stages = append(stages, []Step{noPrompt(Step{ID: copyStepID, Title: "설정 복사", Kind: Shell, Shell: copies, AlwaysRun: true,
			Note: "저장한 설정 파일을 " + o.AnsibleDir() + " 로 (sudo, 이어서 실행할 때도 다시 복사)"})})
	}
	for _, st := range steps {
		if st.Kind == Check {
			continue
		}
		st = noPrompt(st)
		if st.WithPrev && len(stages) > 0 {
			stages[len(stages)-1] = append(stages[len(stages)-1], st)
			continue
		}
		stages = append(stages, []Step{st})
	}
	return stages
}

// copyStepID is the step that copies the settings to the installer; the runner
// checks and records it in the history of the site (fileio.GuardCopy).
const copyStepID = "C1"

// sudoWord is a sudo that starts a command of a shell line, with its -n if it has one.
var sudoWord = regexp.MustCompile(`(^|[\s;&|(])sudo (-n )?`)

// noPrompt is a shell step as the runner runs it: in a session of its own with
// no terminal, so every sudo of it is sudo -n, which fails at once where a
// password would be asked. The script (install-plan.sh) keeps plain sudo for
// the person who runs it.
func noPrompt(st Step) Step {
	if st.Kind != Shell {
		return st
	}
	shell := make([]string, len(st.Shell))
	for i, l := range st.Shell {
		shell[i] = sudoWord.ReplaceAllString(l, "${1}sudo -n ")
	}
	st.Shell = shell
	return st
}

// usesSudo reports whether a shell step runs sudo on the control node.
func usesSudo(st Step) bool {
	if st.Kind != Shell {
		return false
	}
	for _, l := range st.Shell {
		if sudoWord.MatchString(l) {
			return true
		}
	}
	return false
}

// StageIDs is the stages as text: "K1 | A2+M1 | ...", skipped steps in brackets.
func StageIDs(stages [][]Step) string {
	var out []string
	for _, stage := range stages {
		var ids []string
		for _, st := range stage {
			id := st.ID
			if st.Skip {
				id = "(" + id + ")"
			}
			ids = append(ids, id)
		}
		out = append(out, strings.Join(ids, "+"))
	}
	return strings.Join(out, " | ")
}

package plan

import (
	"errors"
	"io/fs"
	"os"
)

// ClusterRecord is the file that says a cluster was built from this control
// node: k8s-installer writes ki_etc_applied_release_path (ki_etc_root_path is
// the constant /etc/k8s-installer, group_vars/all/constant-vars.yml) on every
// node of the inventory, the control node's own entry included, as the last
// task of setup-cluster, update-cluster, add-node and upgrade-cluster
// (playbooks/tasks/record-applied-state.yml), so only once one of them has
// succeeded; reset-cluster and remove-node take it away again
// (tasks/reset-cluster-record.yml). Its neighbour applied-vars.yml is written
// at the same time but outlives reset-cluster on purpose, so it would call a
// reset site existing. The files of the wizard say nothing: setup.sh lays a
// template vars.yml, and a site saved and opened again before it was installed
// has all of them. A variable so that tests can point it elsewhere.
var ClusterRecord = "/etc/k8s-installer/applied-release"

// Site is what the plan takes the site to be, and why: whether the cluster is
// there (Options.Existing) and AIPub with it (Options.AipubInstalled).
type Site struct {
	Existing       bool
	AipubInstalled bool
	// Why is the one line the plan screen and --install show: the decision,
	// what it rests on and how to say otherwise
	Why string
}

// DetectSite decides it from the flags (--existing, --aipub-installed) and the
// record of this control node. Existing: the flag, or the record is there. A
// record that cannot be looked at (permission) is said, not guessed: the site
// is planned new and the line says to give --existing. AIPub is taken from the
// flag only: aipub-installer keeps nothing that is written only once
// setup-aipub has succeeded (aipub_state_path holds values rendered before the
// install and the logs of it), so there is nothing to read it from.
func DetectSite(existing, aipubInstalled bool) Site {
	s := Site{Existing: existing, AipubInstalled: aipubInstalled}
	switch _, err := os.Stat(ClusterRecord); {
	case existing:
		s.Why = "기존 사이트: --existing 으로 지정"
	case err == nil:
		s.Existing = true
		s.Why = "기존 사이트: " + ClusterRecord + " 있음 (k8s-installer 가 클러스터를 만든 기록, reset-cluster.yml 이 지움)"
	case errors.Is(err, fs.ErrNotExist):
		s.Why = "새 사이트: " + ClusterRecord + " 없음 (클러스터가 이미 있으면 --existing 으로 시작)"
	default:
		s.Why = "새 사이트로 계획: " + ClusterRecord + " 를 확인하지 못함 (" + err.Error() + "). 클러스터가 이미 있으면 --existing 으로 시작"
	}
	if s.Existing {
		if aipubInstalled {
			s.Why += ". AIPub: --aipub-installed 로 지정"
		} else {
			s.Why += ". AIPub 은 새로 설치하는 것으로 계획 (이미 설치돼 있으면 --aipub-installed: 설치 기록이 없어 플래그로만 정함)"
		}
	}
	return s
}

// ResumedSite is the line of a run that goes on with the plan of the last one
// (state.json): what that run took the site to be, whatever is decided now.
func ResumedSite(prev *RunState, now string) string {
	kind := "새 사이트"
	if prev.Existing {
		kind = "기존 사이트"
		if prev.AipubInstalled {
			kind += " (AIPub 설치됨)"
		}
	}
	return kind + ": 지난 실행 (" + prev.Started.Format("2006-01-02 15:04") + " 에 시작) 의 계획대로. 바꾸려면 처음부터 실행 (지금 판단: " + now + ")"
}

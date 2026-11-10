package plan

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// P8: the volumes of AIPub are claims on the StorageClass the site names
// (aipub_ha_mode_storage_class; the monitoring volumes take it by default). The
// class is the site's own, so an existing cluster either has it or the claims
// stay Pending. The check asks the first control plane node (kubectl as root with
// the admin kubeconfig, read only). On a new site the cluster does not exist yet:
// skipped. A miss is a warning: the site may create the class before AIPub runs.

// kubernetes names a StorageClass as a DNS subdomain
var storageClassName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

// adminKubeconfig is where kubeadm leaves the admin credentials on a control plane node.
const adminKubeconfig = "/etc/kubernetes/admin.conf"

// CheckStorageClass is P8.
func CheckStorageClass(ctx context.Context, o Options, s *state.AppState, run RunFunc) CheckResult {
	skip := func(why string) CheckResult { return CheckResult{OK: true, Skipped: true, Lines: []string{why}} }
	class := strings.TrimSpace(s.AipubHaModeStorageClass)
	switch {
	case !s.AipubEnabled:
		return skip("AIPub 을 설치하지 않음")
	case class == "":
		return skip("StorageClass (aipub_ha_mode_storage_class) 가 정해지지 않음")
	case !o.Existing:
		return skip("새 사이트: 클러스터가 아직 없어 확인할 수 없습니다 (StorageClass " + class + " 는 클러스터를 만든 뒤에 있어야 함)")
	case len(s.K8sCpNodes) == 0:
		return skip("k8s 컨트롤 플레인 노드가 없음")
	case !storageClassName.MatchString(class):
		return CheckResult{OK: true, Warn: true, Lines: []string{"StorageClass 이름 " + class + " 이(가) 올바르지 않아 확인하지 않았습니다"}}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	node := s.K8sCpNodes[0]
	res, out, err := adhoc(ctx, o, run, node, true, "command",
		"kubectl --kubeconfig "+adminKubeconfig+" get storageclass "+class+" -o name")
	got, found := res[node]
	switch {
	case !found:
		return CheckResult{OK: true, Warn: true, Lines: []string{"StorageClass " + class + " 를 확인하지 못했습니다: " + couldNotRun(out, err)}}
	case got.ok():
		return CheckResult{OK: true, Lines: []string{node + ": StorageClass " + class + " 있음"}}
	case strings.Contains(got.Raw, "NotFound"):
		return CheckResult{OK: true, Warn: true, Lines: []string{
			node + ": StorageClass " + class + " 가 없습니다. 볼륨이 Pending 으로 남기 전에 클래스를 만들거나 " +
				"aipub_ha_mode_storage_class 를 바꾸세요",
		}}
	}
	return CheckResult{OK: true, Warn: true, Lines: []string{
		node + ": StorageClass " + class + " 를 확인하지 못했습니다 (" + firstLine(strings.TrimSpace(got.Raw)) + ")",
	}}
}

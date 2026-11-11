package plan

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
		return skip(i18n.T("AIPub not installed"))
	case class == "":
		return skip(i18n.T("No StorageClass (aipub_ha_mode_storage_class) set"))
	case !o.Existing:
		return skip(i18n.T("New site: no cluster to check yet (StorageClass %s has to be there once the cluster is built)", class))
	case len(s.K8sCpNodes) == 0:
		return skip(i18n.T("No k8s control plane node"))
	case !storageClassName.MatchString(class):
		return CheckResult{OK: true, Warn: true, Lines: []string{i18n.T("StorageClass name %s is not valid, so it was not checked", class)}}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	node := s.K8sCpNodes[0]
	res, out, err := adhoc(ctx, o, run, node, true, "command",
		"kubectl --kubeconfig "+adminKubeconfig+" get storageclass "+class+" -o name")
	got, found := res[node]
	switch {
	case !found:
		return CheckResult{OK: true, Warn: true, Lines: []string{i18n.T("Could not check StorageClass %s: %s", class, couldNotRun(out, err))}}
	case got.ok():
		return CheckResult{OK: true, Lines: []string{i18n.T("%s: StorageClass %s is there", node, class)}}
	case strings.Contains(got.Raw, "NotFound"):
		return CheckResult{OK: true, Warn: true, Lines: []string{
			i18n.T("%s: StorageClass %s is missing. Make the class, or change aipub_ha_mode_storage_class, "+
				"before volumes are left Pending", node, class),
		}}
	}
	return CheckResult{OK: true, Warn: true, Lines: []string{
		i18n.T("%s: could not check StorageClass %s (%s)", node, class, firstLine(strings.TrimSpace(got.Raw))),
	}}
}

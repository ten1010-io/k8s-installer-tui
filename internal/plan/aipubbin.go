package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// PackageFile is what aipub-packager records about the package, under the
// installer root. aipub_bin lists the binaries it carried (helmfile, kustomize,
// k9s); it is empty for a package made with --no-aipub-bin.
const PackageFile = "aipub-package.json"

// PackagePath is the package record of the installer.
func (o Options) PackagePath() string { return o.Root + "/" + PackageFile }

// HelmfileChosen reports whether a deploy tool of AIPub is helmfile, which needs
// the helmfile and kustomize binaries on the control plane nodes.
func HelmfileChosen(s *state.AppState) bool {
	return s.AipubEnabled && (s.AipubAppDeployTool == "helmfile" || s.AipubMonitoringDeployTool == "helmfile")
}

// unsetBinVars are the variables that name where helmfile and kustomize are,
// for the deploy tools chosen, that the site did not set. Monitoring takes its
// helmfile from aipub_app_helmfile_bin and its kustomize from beside it unless
// its own variables say otherwise, so a site that set the first has set them.
func unsetBinVars(s *state.AppState) []string {
	var out []string
	if s.AipubAppDeployTool == "helmfile" && s.AipubAppHelmfileBin == "" {
		out = append(out, "aipub_app_helmfile_bin")
	}
	if s.AipubMonitoringDeployTool == "helmfile" && s.AipubAppHelmfileBin == "" {
		if s.AipubMonitoringHelmfileBin == "" {
			out = append(out, "aipub_monitoring_helmfile_bin")
		}
		if s.AipubMonitoringKustomizeBin == "" {
			out = append(out, "aipub_monitoring_kustomize_bin")
		}
	}
	return out
}

// CheckAipubBin is P4: with helmfile as a deploy tool, the package has to have
// carried the binaries (aipub-bin), which setup-aipub-bin.yml lays on the
// control plane nodes. It reads <root>/aipub-package.json when there is one. A
// package without them is a warning, not a failure: the site may lay them by
// hand, and the paths are then variables of the site.
func CheckAipubBin(o Options, s *state.AppState) CheckResult {
	if !HelmfileChosen(s) {
		return CheckResult{OK: true, Skipped: true, Lines: []string{"배포 도구가 helmfile 이 아님"}}
	}
	data, err := os.ReadFile(o.PackagePath())
	var why string
	var bins []string
	switch {
	case err != nil:
		why = o.PackagePath() + " 가 없어 aipub-bin 이 실렸는지 알 수 없습니다"
	default:
		var pkg struct {
			AipubBin []any `json:"aipub_bin"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			why = o.PackagePath() + " 를 읽지 못했습니다: " + err.Error()
			break
		}
		for _, b := range pkg.AipubBin {
			bins = append(bins, binName(b))
		}
		if len(bins) == 0 {
			why = o.PackagePath() + " 의 aipub_bin 이 비어 있습니다 (패키지를 --no-aipub-bin 으로 만듦)"
		}
	}
	if why == "" {
		return CheckResult{OK: true, Lines: []string{"패키지의 aipub-bin: " + strings.Join(bins, ", ")}}
	}
	missing := unsetBinVars(s)
	if len(missing) == 0 {
		return CheckResult{OK: true, Lines: []string{why + ". helmfile 과 kustomize 경로는 사이트 값으로 정해 둠"}}
	}
	return CheckResult{OK: true, Warn: true, Lines: []string{
		"helmfile 로 배포하는데 패키지에 helmfile·kustomize (aipub-bin) 가 없습니다: " + why,
		"사이트가 두 바이너리를 노드에 직접 두고 경로를 정해야 합니다: " + strings.Join(missing, ", "),
	}}
}

// binName is the name of an entry of aipub_bin, a plain name or a mapping that
// has one.
func binName(v any) string {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"name", "binary", "path"} {
			if n, ok := m[k].(string); ok {
				return n
			}
		}
	}
	return fmt.Sprint(v)
}

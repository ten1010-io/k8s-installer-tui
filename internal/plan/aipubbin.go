package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
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
		return CheckResult{OK: true, Skipped: true, Lines: []string{i18n.T("No deploy tool is helmfile")}}
	}
	data, err := os.ReadFile(o.PackagePath())
	var why string
	var bins []string
	switch {
	case err != nil:
		why = i18n.T("%s is not there, so whether aipub-bin was carried is not known", o.PackagePath())
	default:
		var pkg struct {
			AipubBin []any `json:"aipub_bin"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			why = i18n.T("%s could not be read: %v", o.PackagePath(), err)
			break
		}
		for _, b := range pkg.AipubBin {
			bins = append(bins, binName(b))
		}
		if len(bins) == 0 {
			why = i18n.T("%s has an empty aipub_bin (the package was made with --no-aipub-bin)", o.PackagePath())
		}
	}
	if why == "" {
		return CheckResult{OK: true, Lines: []string{i18n.T("aipub-bin of the package: %s", strings.Join(bins, ", "))}}
	}
	missing := unsetBinVars(s)
	if len(missing) == 0 {
		return CheckResult{OK: true, Lines: []string{why + ". " + i18n.T("The paths of helmfile and kustomize are set as site values")}}
	}
	return CheckResult{OK: true, Warn: true, Lines: []string{
		i18n.T("Deployed with helmfile, but the package has no helmfile or kustomize (aipub-bin): %s", why),
		i18n.T("The site has to put both binaries on the nodes itself and set their paths: %s", strings.Join(missing, ", ")),
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

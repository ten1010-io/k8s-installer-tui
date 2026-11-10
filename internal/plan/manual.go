package plan

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"gopkg.in/yaml.v3"
)

// M1: the StorageClass driver is the site's own (NFS, Ceph, a vendor CSI), which
// neither installer brings, so the run stops for a person to install it. Its pods
// need the nodes Ready (setup-network, A1); the first step that claims a volume
// of a class is base with HA (harbor), then monitoring (aipub_monitoring_storage_type
// dynamic) and aipub (aipub_app_file_storage_type dynamic, the shared data volume
// of the backend) - aipub-installer README "Before setup-base.yml / setup-monitoring.yml
// / setup-aipub.yml" and ansible/aipub_vars/defaults. The stop runs beside
// setup-cert (A2), which touches no storage, so the person works while it runs; the
// next stage starts only when every class is there.

// StorageKeys are the keys of files/aipub/vars.yml that choose the classes, which
// the wizard does not manage: the site file keeps them as written.
var StorageKeys = []string{
	"aipub_app_storage_class_name",
	"aipub_app_file_storage_type",
	"aipub_app_file_storage_storage_class_name",
	"aipub_monitoring_storage_type",
	"aipub_monitoring_storage_class_name",
}

// ReadStorageVars reads StorageKeys from the AIPub site file; nothing when it is
// not there. A value that is a template ({{ ... }}) is the default, as unset.
func ReadStorageVars(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var all map[string]any
	if yaml.Unmarshal(data, &all) != nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range StorageKeys {
		v, ok := all[k].(string)
		if ok && !strings.Contains(v, "{{") {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// DefaultClass stands among the classes of StorageNeeds for the default class
// of the cluster, which a claim with no class name gets: what is checked is
// that some class is marked default.
const DefaultClass = "(클러스터 기본 StorageClass)"

// isDefaultClass is the annotation that marks the default StorageClass.
const isDefaultClass = "storageclass.kubernetes.io/is-default-class"

// StorageNeeds are the StorageClasses the site claims volumes on, with the steps
// that claim them, by class name (DefaultClass for the default one).
func StorageNeeds(s *state.AppState, o Options) map[string][]string {
	needs := map[string][]string{}
	if !s.AipubEnabled {
		return needs
	}
	v := func(k, def string) string {
		if x, ok := o.StorageVars[k]; ok {
			return x
		}
		return def
	}
	ha := ""
	if s.AipubHaMode {
		ha = strings.TrimSpace(s.AipubHaModeStorageClass)
		if ha != "" {
			needs[ha] = append(needs[ha], "A3 setup-base")
		}
	}
	if o.Monitoring {
		def := "static"
		if s.AipubHaMode {
			def = "dynamic"
		}
		if c := v("aipub_monitoring_storage_class_name", ha); v("aipub_monitoring_storage_type", def) == "dynamic" {
			if c == "" {
				// an empty name takes the default class of the cluster
				c = DefaultClass
			}
			needs[c] = append(needs[c], "A4 setup-monitoring")
		}
	}
	if v("aipub_app_file_storage_type", "dynamic") == "dynamic" {
		// empty: check-aipub.yml stops and says so
		if c := v("aipub_app_file_storage_storage_class_name", v("aipub_app_storage_class_name", ha)); c != "" {
			needs[c] = append(needs[c], "A5 setup-aipub")
		}
	}
	return needs
}

func storageStep(s *state.AppState, o Options) Step {
	st := Step{ID: "M1", Title: "StorageClass 드라이버 설치", Kind: Manual, WithPrev: true}
	needs := StorageNeeds(s, o)
	if len(needs) == 0 {
		st.Skip, st.SkipWhy = true, "StorageClass 를 쓰는 단계가 없음 (HA 아님, 모니터링·공유 볼륨이 static)"
		return st
	}
	var uses []string
	for _, c := range sortedKeys(needs) {
		st.Classes = append(st.Classes, c)
		uses = append(uses, c+" ← "+strings.Join(needs[c], ", "))
	}
	st.Note = "setup-cert 와 함께 멈춤. 사이트의 스토리지 드라이버 (CSI) 를 설치해 클래스를 만드세요: " + strings.Join(uses, "; ") +
		". 첫 k8s 컨트롤 플레인 노드에서 kubectl get storageclass 로 모두 있어야 다음 단계로"
	return st
}

// ManualPrompt is what the operator is asked to do at a Manual step.
func ManualPrompt(st Step) string {
	return "스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: " + strings.Join(st.Classes, ", ")
}

// CheckClasses asks the first k8s control plane node which of classes exist
// (kubectl as root with the admin kubeconfig, read only, as P8), and returns the
// missing ones with a line for each class.
func CheckClasses(ctx context.Context, o Options, s *state.AppState, run RunFunc, classes []string) (missing, lines []string) {
	if len(s.K8sCpNodes) == 0 {
		return classes, []string{"k8s 컨트롤 플레인 노드가 없어 확인하지 못했습니다"}
	}
	node := s.K8sCpNodes[0]
	for _, c := range classes {
		if c == DefaultClass {
			line, ok := checkDefaultClass(ctx, o, run, node)
			lines = append(lines, line)
			if !ok {
				missing = append(missing, c)
			}
			continue
		}
		if !storageClassName.MatchString(c) {
			missing = append(missing, c)
			lines = append(lines, "StorageClass 이름 "+c+" 이(가) 올바르지 않습니다")
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		res, out, err := adhoc(cctx, o, run, node, true, "command",
			"kubectl --kubeconfig "+adminKubeconfig+" get storageclass "+c+" -o name")
		cancel()
		got, found := res[node]
		switch {
		case found && got.ok():
			lines = append(lines, node+": StorageClass "+c+" 있음")
			continue
		case found && strings.Contains(got.Raw, "NotFound"):
			lines = append(lines, node+": StorageClass "+c+" 없음")
		case found:
			lines = append(lines, node+": StorageClass "+c+" 를 확인하지 못했습니다 ("+firstLine(strings.TrimSpace(got.Raw))+")")
		default:
			lines = append(lines, "StorageClass "+c+" 를 확인하지 못했습니다: "+couldNotRun(out, err))
		}
		missing = append(missing, c)
	}
	sort.Strings(missing)
	return missing, lines
}

// checkDefaultClass asks node for the classes marked default (is-default-class),
// read only like the rest of CheckClasses; ok when there is one.
func checkDefaultClass(ctx context.Context, o Options, run RunFunc, node string) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// the jsonpath in single quotes for the split of the command module
	res, out, err := adhoc(cctx, o, run, node, true, "command",
		"kubectl --kubeconfig "+adminKubeconfig+" get storageclass -o "+
			`'jsonpath={.items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")].metadata.name}'`)
	got, found := res[node]
	switch {
	case found && got.ok() && strings.TrimSpace(got.Stdout) != "":
		return node + ": 기본 StorageClass " + strings.TrimSpace(got.Stdout) + " 있음", true
	case found && got.ok():
		return node + ": 기본 StorageClass 가 없습니다 (" + isDefaultClass + ": \"true\" 인 클래스 없음)", false
	case found:
		return node + ": 기본 StorageClass 를 확인하지 못했습니다 (" + firstLine(strings.TrimSpace(got.Raw)) + ")", false
	}
	return "기본 StorageClass 를 확인하지 못했습니다: " + couldNotRun(out, err), false
}

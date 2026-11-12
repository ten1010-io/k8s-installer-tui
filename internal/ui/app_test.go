package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui/screens"
	"gopkg.in/yaml.v3"
)

// The wizard driven by key presses, as an operator would at the terminal, without
// one: each key goes through App.Update, and the navigation a screen asks for is
// fed back the way bubbletea would.

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type driver struct {
	t   *testing.T
	app *App
}

// A site whose nodes and groups were loaded from an inventory: the screens after
// S1 are what is driven.
func newDriver(t *testing.T) (*driver, fileio.Paths) {
	s := state.DefaultState()
	s.Nodes = []state.NodeConfig{
		{Name: "node1", AnsibleHost: "192.168.0.11"},
		{Name: "node2", AnsibleHost: "192.168.0.12"},
		{Name: "node3", AnsibleHost: "192.168.0.21"},
	}
	s.KiCpNodes = []string{"node1"}
	s.K8sNodes = []string{"node1", "node2", "node3"}
	s.K8sCpNodes = []string{"node1", "node2"}
	s.KiCpHaMode = false
	withCA(t, "") // before setup-cert, whatever the machine running the test has
	paths := fileio.DefaultPaths(t.TempDir())
	d := &driver{t: t, app: NewApp(s, paths, plan.DefaultOptions(), "test")}
	d.send(tea.WindowSizeMsg{Width: 110, Height: 60})
	return d, paths
}

func (d *driver) send(msg tea.Msg) {
	_, cmd := d.app.Update(msg)
	d.run(cmd)
}

// run executes what a screen returned when it is navigation; the blink timers of
// the text inputs are left alone (they would sleep and loop).
func (d *driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		switch msg.(type) {
		case screens.NavigateNextMsg, screens.NavigatePrevMsg:
			d.send(msg)
		default:
			// the result of the checks of 4b (c) and 9.설치
			if strings.HasSuffix(fmt.Sprintf("%T", msg), "DoneMsg") {
				d.send(msg)
			}
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func (d *driver) keys(names ...string) {
	for _, n := range names {
		switch n {
		case "enter":
			d.send(tea.KeyMsg{Type: tea.KeyEnter})
		case "down":
			d.send(tea.KeyMsg{Type: tea.KeyDown})
		case "up":
			d.send(tea.KeyMsg{Type: tea.KeyUp})
		case "left":
			d.send(tea.KeyMsg{Type: tea.KeyLeft})
		case "right":
			d.send(tea.KeyMsg{Type: tea.KeyRight})
		case "tab":
			d.send(tea.KeyMsg{Type: tea.KeyTab})
		case "ctrl+n":
			d.send(tea.KeyMsg{Type: tea.KeyCtrlN})
		case "ctrl+p":
			d.send(tea.KeyMsg{Type: tea.KeyCtrlP})
		case "ctrl+s":
			d.send(tea.KeyMsg{Type: tea.KeyCtrlS})
		case "ctrl+u": // clears a text input
			d.send(tea.KeyMsg{Type: tea.KeyCtrlU})
		case "esc":
			d.send(tea.KeyMsg{Type: tea.KeyEsc})
		case "pgdown":
			d.send(tea.KeyMsg{Type: tea.KeyPgDown})
		default:
			// typed: one key per character, as at a terminal
			for _, r := range n {
				if r == ' ' {
					d.send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
				} else {
					d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
			}
		}
	}
}

func (d *driver) at(want string) {
	d.t.Helper()
	got := i18n.T(stepTitles[d.app.current]) // as the header says it
	if got != want {
		d.t.Fatalf("on step %s, want %s; errors: %v\n%s", got, want, d.app.screenErrors, d.screen())
	}
}

func (d *driver) screen() string {
	return ansi.ReplaceAllString(d.app.screenList[d.app.current].View(), "")
}

func readYAML(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s not written: %v", path, err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWizardAipubWithoutHA(t *testing.T) {
	d, paths := newDriver(t)

	d.at("1.노드")
	d.keys("ctrl+n")
	d.at("2.그룹")
	t.Logf("S2:\n%s", d.screen())
	d.keys("ctrl+n")
	d.at("3.네트워크")
	d.keys("ctrl+n")
	d.at("4.K8s")
	t.Logf("S4:\n%s", d.screen())
	d.keys("ctrl+n")
	d.at("4b.컨트롤플레인")
	d.keys("ctrl+n")

	// S5: AIPub on, its domain, HA off and harbor on node1
	d.at("5.AIPub")
	d.keys("enter")                              // 설치
	d.keys("down", "ctrl+u", "aipub.lkjh.local") // 인그레스 도메인, h/j/k/l in it
	d.keys("down", "enter")                      // HA 끄기
	d.keys("down", "node3")                      // a node that is not an aipub cp node
	d.keys("ctrl+n")
	d.at("5.AIPub") // refused: node3 is a worker
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "node3") {
		t.Errorf("no error naming node3: %v", d.app.screenErrors)
	}
	d.keys("ctrl+u", "node1")
	t.Logf("S5:\n%s", d.screen())
	d.keys("ctrl+n")

	// S5b: a size that is not one is refused, a right one is kept; the rest stays at the defaults
	d.at("5b.모니터링")
	d.keys("ctrl+u", "5T")
	d.keys("ctrl+n")
	d.at("5b.모니터링")
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "aipub_monitoring_opensearch_storage_size") {
		t.Errorf("no error naming the size: %v", d.app.screenErrors)
	}
	d.keys("ctrl+u", "5Ti")
	d.keys("ctrl+n")

	// S5c: the cluster name of the coaster license keys is required; a name with h/j/k/l is typed as text
	d.at("5c.Python")
	d.keys("ctrl+u", "ctrl+n")
	d.at("5c.Python")
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "aipub_python_coaster_cluster_name") {
		t.Errorf("no error naming the cluster name: %v", d.app.screenErrors)
	}
	d.keys("seoul-hjkl", "ctrl+n")

	// S6: a load balancer name with h/j/k/l in it, and the ingress VIP
	d.at("6.AIPub LB")
	d.keys("ctrl+n")
	d.at("6.AIPub LB") // refused: no VIP yet
	d.keys("down", "ctrl+u", "lb-hjkl")
	d.keys("down", "192.168.0.100")
	t.Logf("S6:\n%s", d.screen())
	d.keys("ctrl+n")

	d.at("7.인증서")
	d.keys("ctrl+n")
	d.at("8.저장")
	if strings.Contains(d.screen(), "검증 오류") {
		t.Fatalf("validation errors on the preview:\n%s", d.screen())
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}

	a := readYAML(t, paths.AipubVars)
	if a["aipub_ha_mode"] != false || a["aipub_harbor_local_pv_node"] != "node1" {
		t.Errorf("ha=%v local pv node=%v", a["aipub_ha_mode"], a["aipub_harbor_local_pv_node"])
	}
	if _, ok := a["aipub_ha_mode_storage_class"]; ok {
		t.Error("storage class written without HA")
	}
	if a["aipub_monitoring_opensearch_storage_size"] != "5Ti" {
		t.Errorf("monitoring size = %v", a["aipub_monitoring_opensearch_storage_size"])
	}
	for _, k := range []string{"aipub_monitoring_api_replicas", "aipub_monitoring_metrics_storage_size", "aipub_monitoring_cp_nodes"} {
		if _, ok := a[k]; ok {
			t.Errorf("%s written at its default", k)
		}
	}
	if a["aipub_ingress_zone"] != "aipub.lkjh.local" {
		t.Errorf("zone = %v", a["aipub_ingress_zone"])
	}
	lb := a["aipub_load_balancers"].([]interface{})[0].(map[string]interface{})
	if lb["vip"] != "192.168.0.100" || lb["name"] != "lb-hjkl" {
		t.Errorf("load balancer = %v (h/j/k/l typed into an input must stay text)", lb)
	}
	v := readYAML(t, paths.Vars)
	if v["k8s_kube_proxy_enabled"] != false {
		t.Error("kube-proxy left on with AIPub")
	}
	// the JWKS preset, on for a new site, with the issuer derived from the zone
	if got := fmt.Sprint(v["k8s_apiserver_extra_args"]); got != "[map[name:authentication-config value:/etc/kubernetes/authentication-config.yaml]]" {
		t.Errorf("apiserver args %s", got)
	}
	// its config waits for the CA of setup-cert: not written, and said so
	if _, err := os.Stat(paths.AuthConfig()); err == nil {
		t.Error("authentication config written before there is a CA")
	}
	if !strings.Contains(d.screen(), "AIPub 인증 (JWKS): CA 대기") || !strings.Contains(d.screen(), "'소스 파일 없음'으로 거부") {
		t.Errorf("the preview does not say the config waits for the CA:\n%s", d.screen())
	}
	inv := readYAML(t, paths.Inventory)
	hosts := inv["k8s_node"].(map[string]interface{})["hosts"].(map[string]interface{})
	for _, n := range []string{"node1", "node2"} {
		labels, _ := hosts[n].(map[string]interface{})["k8s_node_labels"].(map[string]interface{})
		if _, ok := labels[fileio.AipubCpNodeLabel]; !ok {
			t.Errorf("%s not labelled as an aipub cp node", n)
		}
	}

	// 9.설치: the plan of what was saved
	d.keys("enter")
	d.at("9.설치")
	d.send(tea.WindowSizeMsg{Width: 110, Height: 200}) // the whole plan in sight, not scrolled
	s9 := d.app.screenList[11].(*screens.S9Install)
	s9.SetRunner(func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte("node1 | CHANGED | rc=0 | (stdout) \nnode2 | CHANGED | rc=0 | (stdout) \n" +
			"node3 | UNREACHABLE! => {\"msg\": \"No route to host\"}\n"), fmt.Errorf("exit status 4")
	})
	if !strings.Contains(d.screen(), "건너뜀: 아카이브 경로 없음") {
		t.Errorf("K4 not skipped without an archive:\n%s", d.screen())
	}
	d.keys("/media/usb/artifacts.tgz") // the archive path, focused first
	d.keys("down", " ")                // monitoring off
	d.keys("down")                     // off the input
	d.keys("c")                        // checks, with the fake ansible
	t.Logf("S9:\n%s", d.screen())
	for _, want := range []string{"K4  push-user-registry-artifacts", "건너뜀: 모니터링 설치 안 함",
		"node3: 접속 안 됨", "/opt/k8s-installer/release 가 없습니다"} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("9.설치 does not show %q", want)
		}
	}
	if strings.Contains(d.screen(), "건너뜀: 아카이브 경로 없음") {
		t.Error("K4 still skipped with an archive path")
	}

	d.keys("d")
	for i := 0; i < 12; i++ {
		d.keys("down") // past the copies (wrapped by the length of the temp dir) and K3, whose -e leaves JWKS out
	}
	if !strings.Contains(d.screen(), "ansible-playbook -i inventory.yml aipub-playbooks/setup-network.yml") {
		t.Errorf("the commands are not shown:\n%s", d.screen())
	}
	d.keys("d")
	d.keys("w")
	script, err := os.ReadFile(filepath.Join(filepath.Dir(paths.Inventory), "install-plan.sh"))
	if err != nil {
		t.Fatalf("install-plan.sh not written: %v", err)
	}
	for _, want := range []string{
		"sudo cp " + filepath.ToSlash(filepath.Dir(paths.Inventory)) + "/inventory.yml /opt/k8s-installer/ansible/inventory.yml",
		"-e archive=/media/usb/artifacts.tgz",
		"# A4 setup-monitoring: 건너뜀",
		// JWKS: built without, the CA copied in and applied after setup-aipub
		`"k8s_apiserver_extra_args":[]`,
		"echo '── A5.1 JWKS CA 채우기'",
		" --fill-auth-config --inventory " + filepath.ToSlash(filepath.Dir(paths.Inventory)) + "/inventory.yml",
		"echo '── A5.2 update-cluster (apiserver JWKS 인증 적용)'",
		"/files/aipub/authentication-config.yaml /opt/k8s-installer/ansible/files/aipub/authentication-config.yaml",
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("install-plan.sh lacks %q:\n%s", want, script)
		}
	}
}

func TestPreviewNeedsSave(t *testing.T) {
	d, _ := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	d.keys("ctrl+n")
	d.at("8.저장") // not saved yet
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "저장") {
		t.Errorf("no reason given: %v", d.app.screenErrors)
	}
}

func TestWizardWithoutAipub(t *testing.T) {
	d, paths := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("5.AIPub")
	d.keys("ctrl+n") // left off
	d.at("8.저장")     // 6 and 7 skipped
	d.keys("s")
	if _, err := os.Stat(paths.AipubVars); err == nil {
		t.Error("files/aipub/vars.yml written without AIPub")
	}
	if v := readYAML(t, paths.Vars); v["k8s_kube_proxy_enabled"] != true {
		t.Error("kube-proxy turned off without AIPub")
	}
}

// The preview has one more tab: every variable in one annotated view. It is a
// view only; saving writes the three files and nothing else.
func TestPreviewSiteValuesTab(t *testing.T) {
	d, paths := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	if !strings.Contains(d.screen(), "site-values.yml") {
		t.Fatalf("no site-values tab:\n%s", d.screen())
	}
	d.keys("l", "l") // inventory.yml → vars.yml → site-values.yml
	if !strings.Contains(d.screen(), "[INVENTORY]") {
		t.Errorf("site-values tab not shown:\n%s", d.screen())
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(paths.Inventory), "site-values.yml")); err == nil {
		t.Error("site-values.yml saved with the files")
	}
}

// What the preview reports is in a tab of its own, the first and the one shown;
// ←/→ go to the files and back, and once saved it lists the files written. A
// preview with nothing to report has no such tab.
func TestPreviewNotesTab(t *testing.T) {
	d, paths := newDriver(t)
	d.app.appState.ControlPlane.Files = []state.CpExtraFile{{Src: "site/custom.yaml", Dest: "/etc/custom.yaml", Mode: "0644"}}
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("4b.컨트롤플레인")
	d.keys("ctrl+n")
	d.at("5.AIPub")
	d.keys("ctrl+n")
	d.at("8.저장")
	for _, want := range []string{"[ 확인할 점 ]", "inventory.yml", "⚠ 확인할 점 1건 — '확인할 점' 탭", "custom.yaml"} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("the preview lacks %q:\n%s", want, d.screen())
		}
	}
	d.keys("right")
	if !strings.Contains(d.screen(), "[ inventory.yml ]") || !strings.Contains(d.screen(), "k8s_node:") {
		t.Errorf("→ is not on inventory.yml:\n%s", d.screen())
	}
	d.keys("left")
	if !strings.Contains(d.screen(), "[ 확인할 점 ]") {
		t.Errorf("← is not back on 확인할 점:\n%s", d.screen())
	}
	d.keys("s")
	for _, want := range []string{"✓ 저장 완료 — 이전 내용은 history.jsonl 에 남김", "저장한 파일 (이전 내용은 history.jsonl 에 남김)", paths.Inventory, paths.Vars} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("after saving the preview lacks %q:\n%s", want, d.screen())
		}
	}

	// nothing to report: the files first
	n, npaths := newDriver(t)
	n.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	n.at("8.저장")
	if strings.Contains(n.screen(), "확인할 점") || !strings.Contains(n.screen(), "[ inventory.yml ]") {
		t.Errorf("a preview with nothing to report:\n%s", n.screen())
	}
	n.keys("s") // saved: the files written are listed in a 확인할 점 tab, the file shown stays
	if !strings.Contains(n.screen(), "  확인할 점  ") || !strings.Contains(n.screen(), "[ inventory.yml ]") {
		t.Errorf("after saving with nothing to report:\n%s", n.screen())
	}
	n.keys("tab", "left") // from the buttons to the files, then the tab before
	if !strings.Contains(n.screen(), "[ 확인할 점 ]") || !strings.Contains(n.screen(), npaths.Inventory) {
		t.Errorf("the files written are not listed:\n%s", n.screen())
	}
}

// Opened on an existing cluster, the preview warns about a value the cluster can
// not take any more, and still saves.
func TestPreviewWarnsFixedChange(t *testing.T) {
	d0, paths := newDriver(t)
	s := d0.app.appState
	s.KiCpHaMode = false
	if err := fileio.WriteAll(paths, s); err != nil {
		t.Fatal(err)
	}
	s.InternalNetworkSubnets = []string{"192.168.0.0/23"}

	// saved once, not installed yet (no --existing, no record): nothing is fixed
	saved := &driver{t: t, app: NewApp(s, paths, plan.DefaultOptions(), "test")}
	saved.send(tea.WindowSizeMsg{Width: 110, Height: 60})
	saved.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	saved.at("8.저장")
	if strings.Contains(saved.screen(), "바꿀 수 없습니다") {
		t.Errorf("a saved site that is not installed warns:\n%s", saved.screen())
	}

	existing := plan.DefaultOptions()
	existing.Existing = true
	d := &driver{t: t, app: NewApp(s, paths, existing, "test")}
	d.send(tea.WindowSizeMsg{Width: 110, Height: 60})
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	if !strings.Contains(d.screen(), "internal_network_subnets 는 설치 뒤 바꿀 수 없습니다") {
		t.Fatalf("no warning:\n%s", d.screen())
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Errorf("the warning blocked saving:\n%s", d.screen())
	}

	// a new site warns about nothing
	n, _ := newDriver(t)
	n.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	n.at("8.저장")
	if strings.Contains(n.screen(), "바꿀 수 없습니다") {
		t.Errorf("a new site warns:\n%s", n.screen())
	}
}

// The control plane screen: presets switched on, entries typed in and one taken
// out again, and what reaches vars.yml.
func TestWizardControlPlane(t *testing.T) {
	d, paths := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("4b.컨트롤플레인")

	d.keys(" ")             // AIPub 감사 로그
	d.keys("down", "enter") // runtime-config
	d.keys("down")          // AIPub 인증 (JWKS): on for a new site, the issuer prefilled, its help shown
	for _, want := range []string{"[x] AIPub 인증 (JWKS)", "https://aipub.example.com", "A5.2 update-cluster"} {
		if !strings.Contains(d.screen(), want) {
			t.Errorf("S4b does not show %q:\n%s", want, d.screen())
		}
	}
	// the legacy webhook is not offered up front on a new site
	if strings.Contains(d.screen(), "AIPub 인증 웹훅") {
		t.Errorf("S4b shows the legacy webhook up front on a new site:\n%s", d.screen())
	}
	d.keys("down", "ctrl+u", "http://idp.example.com") // issuer, not https
	d.keys("ctrl+n")
	d.at("4b.컨트롤플레인") // refused: the issuer is not https
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "JWKS issuer") {
		t.Errorf("no error on the issuer: %v", d.app.screenErrors)
	}
	d.keys("ctrl+u", "https://aipub.example.com") // back to the derived one
	d.keys("up", " ")                             // JWKS off
	if !strings.Contains(d.screen(), "[ ] AIPub 인증 (JWKS)") || strings.Contains(d.screen(), "issuer URL") {
		t.Errorf("JWKS not switched off, or its fields still shown:\n%s", d.screen())
	}
	d.keys(" ")            // and on again
	d.keys("down", "down") // CA 파일, prefilled; nothing there yet
	if !strings.Contains(d.screen(), "CA 대기") {
		t.Errorf("S4b does not say the CA is waited for:\n%s", d.screen())
	}
	d.keys("down", "enter") // ▸ 고급: expanded
	d.keys("down", " ")     // AIPub 인증 웹훅 (레거시), with JWKS, inside the advanced section
	if !strings.Contains(d.screen(), "[x] AIPub 인증 웹훅 (레거시)") || !strings.Contains(d.screen(), "웹훅 URL") {
		t.Errorf("the webhook is not switched on in the advanced section:\n%s", d.screen())
	}
	d.keys("ctrl+n")
	d.at("4b.컨트롤플레인") // refused: no URL
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "URL") {
		t.Errorf("no error asking for the URL: %v", d.app.screenErrors)
	}
	d.keys(" ") // webhook off again

	// an apiserver argument, a bad volume line, then a file
	d.keys("down") // 대상: apiserver 인자
	d.keys("down", "oidc-issuer-url=https://idp.example.com", "enter")
	d.keys("up", "right", "down")     // 대상: apiserver 볼륨
	d.keys("oidc /etc/oidc", "enter") // too few fields
	if !strings.Contains(d.screen(), "이름 hostPath mountPath") {
		t.Errorf("a bad volume line is not refused:\n%s", d.screen())
	}
	d.keys("ctrl+u", "oidc /etc/oidc /etc/oidc ro Directory", "enter")
	d.keys("up")
	for i := 0; i < 6; i++ {
		d.keys("right") // 대상: 컨트롤플레인 파일
	}
	d.keys("down", "oidc/ca.crt /etc/oidc/ca.crt", "enter")
	d.keys("down", "down", "down") // the third entry: the file
	t.Logf("S4b:\n%s", d.screen())
	if !strings.Contains(d.screen(), "oidc/ca.crt → /etc/oidc/ca.crt (0644)") {
		t.Fatalf("file entry not listed:\n%s", d.screen())
	}
	d.keys("d")
	if strings.Contains(d.screen(), "oidc/ca.crt →") {
		t.Errorf("file entry not removed:\n%s", d.screen())
	}
	d.keys("ctrl+n")
	d.at("5.AIPub")
	d.keys("ctrl+n")
	d.at("8.저장")
	// the default audit policy is written on save: a tab of its own, and no
	// warning that the file is missing
	if !strings.Contains(d.screen(), "aipub/audit-policy.yaml") {
		t.Errorf("no tab of the audit policy:\n%s", d.screen())
	}
	if strings.Contains(d.screen(), "확인할 점") || strings.Contains(d.screen(), "ansible/files 에 없습니다") {
		t.Errorf("a warning although the wizard writes the policy:\n%s", d.screen())
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}

	v := readYAML(t, paths.Vars)
	args := v["k8s_apiserver_extra_args"].([]interface{})
	var names []string
	for _, a := range args {
		names = append(names, a.(map[string]interface{})["name"].(string))
	}
	if got, want := strings.Join(names, " "), "audit-policy-file audit-log-path audit-log-maxage audit-log-maxbackup "+
		"audit-log-maxsize runtime-config oidc-issuer-url"; got != want {
		t.Errorf("apiserver args %s, want %s", got, want)
	}
	vols := v["k8s_apiserver_extra_volumes"].([]interface{})
	if len(vols) != 3 || vols[2].(map[string]interface{})["pathType"] != "Directory" {
		t.Errorf("apiserver volumes %v", vols)
	}
	files := v["k8s_cp_extra_files"].([]interface{})
	if len(files) != 1 || files[0].(map[string]interface{})["src"] != "aipub/audit-policy.yaml" {
		t.Errorf("cp extra files %v", files)
	}
	if _, err := os.Stat(paths.AuditPolicy()); err != nil {
		t.Errorf("audit policy not written: %v", err)
	}
	if _, err := os.Stat(paths.WebhookConfig()); err == nil {
		t.Error("webhook kubeconfig written with the preset off")
	}
	// JWKS stays on, but its issuer is AIPub, which this site does not install
	if _, err := os.Stat(paths.AuthConfig()); err == nil {
		t.Error("authentication config written without AIPub")
	}
	if strings.Contains(fmt.Sprint(args), "authentication-config") {
		t.Errorf("JWKS written without AIPub: %v", args)
	}
}

// helmfile as a deploy tool, on a package without the binaries: the preview says
// the site has to lay them, and does not block the save.
func TestPreviewWarnsHelmfileWithoutAipubBin(t *testing.T) {
	d, paths := newDriver(t)
	s := d.app.appState
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.AipubCpNodes = []string{"node1"}
	s.K8sIngressVIP = "192.168.0.100"
	s.AipubAppDeployTool = "helmfile"

	opts := plan.DefaultOptions()
	opts.Root = t.TempDir() // no aipub-package.json
	p := screens.NewS7Preview(s, paths, opts.AnsibleDir()+"/files")
	p.SetInstaller(opts)
	p.SetSize(100, 60)
	p.SyncFromState(s)
	view := ansi.ReplaceAllString(p.View(), "")
	for _, want := range []string{"helmfile 로 배포하는데", "aipub_app_helmfile_bin", "검증 통과"} {
		if !strings.Contains(view, want) {
			t.Errorf("the preview lacks %q:\n%s", want, view)
		}
	}

	if err := os.WriteFile(filepath.Join(opts.Root, plan.PackageFile), []byte(`{"aipub_bin": ["helmfile", "kustomize"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	p.SyncFromState(s)
	if view := ansi.ReplaceAllString(p.View(), ""); strings.Contains(view, "helmfile 로 배포하는데") {
		t.Errorf("warned although the package has aipub-bin:\n%s", view)
	}
}

// The catalog of the package: a summary line on the preview, and P5 on the plan
// screen, which names what the playbooks would refuse.
func TestCatalogOnPreviewAndPlan(t *testing.T) {
	fixture := "../../test/simulate/fixtures/new-site-helmfile/installer-root"
	d, paths := newDriver(t)
	s := d.app.appState
	s.AipubEnabled = true
	s.AipubHaModeStorageClass = "nfs-client"
	s.AipubCpNodes = []string{"node1"}
	s.K8sIngressVIP = "192.168.0.100"

	opts := plan.DefaultOptions()
	opts.Root = fixture
	p := screens.NewS7Preview(s, paths, opts.AnsibleDir()+"/files")
	p.SetInstaller(opts)
	p.SetSize(100, 60)
	p.SyncFromState(s)
	if view := ansi.ReplaceAllString(p.View(), ""); !strings.Contains(view, "✓ 카탈로그 이상 없음: network 3 · base 8 · aipub 12 · monitoring 29") {
		t.Errorf("no catalog summary:\n%s", view)
	}

	// the same package without aipub-web in its catalog
	root := t.TempDir()
	data, err := os.ReadFile(filepath.Join(fixture, "ansible/aipub_vars/catalog.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.Contains(l, "/aipub-web:") {
			kept = append(kept, l)
		}
	}
	broken := plan.Options{Root: root, Monitoring: true}
	if err := os.MkdirAll(filepath.Dir(broken.CatalogPath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken.CatalogPath(), []byte(strings.Join(kept, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	p.SetInstaller(broken)
	p.SyncFromState(s)
	if view := ansi.ReplaceAllString(p.View(), ""); !strings.Contains(view, "⚠ 카탈로그 문제 1건") || !strings.Contains(view, "module[aipub]: aipub-web 없음") {
		t.Errorf("the preview does not say what is missing:\n%s", view)
	}

	// 9.설치: c runs the checks; P5 is read at once
	s9 := screens.NewS9Install(s, paths, broken)
	s9.SetSize(100, 60)
	s9.SyncFromState(s)
	s9.SetRunner(func(ctx context.Context, dir, name string, args ...string) ([]byte, error) { return nil, nil })
	s9.Update(tea.KeyMsg{Type: tea.KeyDown}) // off the archive path
	s9.Update(tea.KeyMsg{Type: tea.KeyDown})
	s9.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	view := ansi.ReplaceAllString(s9.View(), "")
	if !strings.Contains(view, "✗ P5  카탈로그") || !strings.Contains(view, "module[aipub]: aipub-web 없음") {
		t.Errorf("P5 not failed on the plan screen:\n%s", view)
	}

	// without the file: skipped, with the reason
	s9 = screens.NewS9Install(s, paths, plan.Options{Root: t.TempDir(), Monitoring: true})
	s9.SetSize(100, 60)
	s9.SyncFromState(s)
	s9.SetRunner(func(ctx context.Context, dir, name string, args ...string) ([]byte, error) { return nil, nil })
	s9.Update(tea.KeyMsg{Type: tea.KeyDown})
	s9.Update(tea.KeyMsg{Type: tea.KeyDown})
	s9.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	view = ansi.ReplaceAllString(s9.View(), "")
	if !strings.Contains(view, "- P5  카탈로그") || !strings.Contains(view, "건너뜀: 카탈로그 파일이 없음") {
		t.Errorf("P5 not skipped without the file:\n%s", view)
	}
}

// Ctrl+N and Ctrl+P are the app's, on every screen: Ctrl+N checks the screen
// first, as the 다음 button does.
func TestGlobalNavKeys(t *testing.T) {
	d, _ := newDriver(t)
	d.keys("ctrl+n")
	d.at("2.그룹")
	d.keys("ctrl+p")
	d.at("1.노드")

	// no nodes: refused, and said why
	s := state.DefaultState()
	s.Nodes = nil
	e := &driver{t: t, app: NewApp(s, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions(), "test")}
	e.send(tea.WindowSizeMsg{Width: 110, Height: 60})
	e.keys("ctrl+n")
	e.at("1.노드")
	if !strings.Contains(strings.Join(e.app.screenErrors, ""), "노드를 최소 1개") {
		t.Errorf("no reason given: %v", e.app.screenErrors)
	}
	e.keys("ctrl+s") // not the preview: nothing
	e.at("1.노드")

	// in a text input the keys still navigate, and letters stay text
	d.keys("ctrl+n", "ctrl+n")
	d.at("3.네트워크")
	d.keys("down", "down", "10.0.0.1 hjkld") // the VIP, a key per character
	d.keys("ctrl+n")
	d.at("4.K8s")
	if d.app.appState.KiCpHaModeVIP != "10.0.0.1 hjkld" {
		t.Errorf("VIP = %q (h/j/k/l/d and space typed into it must stay text)", d.app.appState.KiCpHaModeVIP)
	}
	d.keys("ctrl+p")
	d.at("3.네트워크")
}

func TestFooterKeyHints(t *testing.T) {
	d, _ := newDriver(t)
	view := ansi.ReplaceAllString(d.app.View(), "")
	if !strings.Contains(view, "Ctrl+N 다음 · Ctrl+P 이전 · ↑↓ 이동 · Enter 선택") {
		t.Errorf("no key hints on 1.노드:\n%s", view)
	}
	if strings.Contains(view, "Ctrl+S 저장") {
		t.Error("the save key shown off the preview")
	}
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	view = ansi.ReplaceAllString(d.app.View(), "")
	if !strings.Contains(view, "Ctrl+S 저장 · Ctrl+N 다음") {
		t.Errorf("no save hint on the preview:\n%s", view)
	}
	if n := strings.Count(view, "Ctrl+N 다음"); n != 1 {
		t.Errorf("key hints shown %d times", n)
	}
}

// On the narrowest box the footer keeps one line and its first hints: save on
// the preview, next and previous.
func TestFooterKeyHintsNarrow(t *testing.T) {
	d, _ := newDriver(t)
	d.send(tea.WindowSizeMsg{Width: 40, Height: 60}) // the box is at its minimum
	footer := func() string {
		for _, l := range strings.Split(ansi.ReplaceAllString(d.app.View(), ""), "\n") {
			if strings.Contains(l, "Ctrl+") {
				return l
			}
		}
		return ""
	}
	if f := footer(); !strings.Contains(f, "Ctrl+N 다음 · Ctrl+P 이전") || strings.Contains(f, "종료") {
		t.Errorf("footer on a narrow box: %q", f)
	}
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	if f := footer(); !strings.Contains(f, "Ctrl+S 저장 · Ctrl+N 다음 · Ctrl+P 이전") {
		t.Errorf("footer of the preview on a narrow box: %q", f)
	}
}

// While a node is typed into the form of 1.노드, the wizard stays and says why.
func TestNavHeldBySubEdit(t *testing.T) {
	d, _ := newDriver(t)
	d.keys("a", "node4")
	for _, k := range []string{"ctrl+n", "pgdown"} {
		d.keys(k)
		d.at("1.노드")
		if !strings.Contains(strings.Join(d.app.screenErrors, ""), "저장하거나 취소한 뒤") {
			t.Fatalf("%s: no reason given: %v", k, d.app.screenErrors)
		}
	}
	if !strings.Contains(d.screen(), "node4") {
		t.Fatalf("the form was left:\n%s", d.screen())
	}
	d.keys("esc", "ctrl+n")
	d.at("2.그룹")
	if len(d.app.screenErrors) != 0 {
		t.Errorf("errors left: %v", d.app.screenErrors)
	}
	d.keys("ctrl+n")
	d.at("3.네트워크")
	d.keys("enter", "10.1.0.0/16") // a subnet typed in
	d.keys("ctrl+p")
	d.at("3.네트워크")
	d.keys("enter", "ctrl+p")
	d.at("2.그룹")
}

// The message of a sub-edit that held the move goes when the edit closes, though
// the screen would still refuse to be left: that is said on the next move.
func TestBusyMessageClearsWhenEditCloses(t *testing.T) {
	s := state.DefaultState()
	s.Nodes = nil
	d := &driver{t: t, app: NewApp(s, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions(), "test")}
	d.send(tea.WindowSizeMsg{Width: 110, Height: 60})
	d.keys("a", "node4", "ctrl+n")
	d.at("1.노드")
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "저장하거나 취소한 뒤") {
		t.Fatalf("not held by the form: %v", d.app.screenErrors)
	}
	d.keys("esc")
	if len(d.app.screenErrors) != 0 {
		t.Errorf("the message stays after the form closed: %v", d.app.screenErrors)
	}
	d.keys("ctrl+n")
	d.at("1.노드")
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "노드를 최소 1개") {
		t.Errorf("the next move is not refused with its reason: %v", d.app.screenErrors)
	}
}

var (
	nextFocused = regexp.MustCompile(`▶\s*< 다음 >`)
	prevFocused = regexp.MustCompile(`▶\s*< 이전 >`)
)

// ↓ past the last item lands on 다음, ↑ goes back, ←/→ switch, Enter goes on.
func TestArrowsReachNextButton(t *testing.T) {
	d, _ := newDriver(t)
	for _, step := range []string{"1.노드", "2.그룹", "3.네트워크", "4.K8s", "4b.컨트롤플레인", "5.AIPub"} {
		d.at(step)
		for i := 0; i < 40; i++ {
			d.keys("down")
		}
		if !nextFocused.MatchString(d.screen()) {
			t.Fatalf("%s: ↓ to the end is not on 다음:\n%s", step, d.screen())
		}
		d.keys("up")
		if nextFocused.MatchString(d.screen()) || prevFocused.MatchString(d.screen()) {
			t.Fatalf("%s: ↑ stays on the buttons:\n%s", step, d.screen())
		}
		d.keys("down", "left")
		if !prevFocused.MatchString(d.screen()) {
			t.Fatalf("%s: ← is not on 이전:\n%s", step, d.screen())
		}
		d.keys("right", "enter")
	}
	d.at("8.저장") // AIPub off: its screens skipped
}

// The preview: Tab reaches the button that saves, which then reads 다음; Ctrl+S
// saves from the files as well.
func TestPreviewSaveKeys(t *testing.T) {
	d, paths := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	d.keys("tab")
	if !regexp.MustCompile(`▶\s*< 저장 \(s\) >`).MatchString(d.screen()) {
		t.Fatalf("Tab is not on 저장:\n%s", d.screen())
	}
	d.keys("enter")
	if !strings.Contains(d.screen(), "저장 완료") || !nextFocused.MatchString(d.screen()) {
		t.Fatalf("not saved, or not on 다음:\n%s", d.screen())
	}
	if _, err := os.Stat(paths.Vars); err != nil {
		t.Fatalf("vars.yml not written: %v", err)
	}
	d.keys("enter")
	d.at("9.설치")

	// Ctrl+S with the files focused, then Ctrl+N
	e, epaths := newDriver(t)
	e.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	e.at("8.저장")
	e.keys("ctrl+s")
	if !strings.Contains(e.screen(), "저장 완료") {
		t.Fatalf("Ctrl+S did not save:\n%s", e.screen())
	}
	if _, err := os.Stat(epaths.Inventory); err != nil {
		t.Fatalf("inventory.yml not written: %v", err)
	}
	e.keys("ctrl+n")
	e.at("9.설치")
}

// Ctrl+N before saving is refused with a message, which goes once the save
// makes the move possible.
func TestPreviewRefusalClearsOnSave(t *testing.T) {
	d, _ := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	d.keys("ctrl+n")
	d.at("8.저장")
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "저장(s)한 뒤에") {
		t.Fatalf("Ctrl+N before saving was not refused with a message: %v", d.app.screenErrors)
	}
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}
	if len(d.app.screenErrors) != 0 {
		t.Fatalf("the refusal is still shown after saving: %v", d.app.screenErrors)
	}
}

// Saving twice keeps, in the history, the file that was there before the first
// save, and leaves no .bak.
func TestPreviewSaveTwiceKeepsHistory(t *testing.T) {
	d, paths := newDriver(t)
	if err := os.MkdirAll(filepath.Dir(paths.Inventory), 0755); err != nil {
		t.Fatal(err)
	}
	const original = "# the inventory before the wizard\n"
	if err := os.WriteFile(paths.Inventory, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	d.keys("s")
	if !strings.Contains(d.screen(), "저장 완료") {
		t.Fatalf("not saved:\n%s", d.screen())
	}
	d.keys("ctrl+s", "s")
	h, err := fileio.ReadHistory(filepath.Dir(paths.Inventory))
	if err != nil {
		t.Fatal(err)
	}
	var edits []string
	for _, e := range h {
		if e.Kind == fileio.HistoryEdit && e.Path == "inventory.yml" {
			edits = append(edits, e.Content)
		}
	}
	if len(edits) != 1 || edits[0] != original {
		t.Errorf("history edits of inventory.yml = %q; want the file from before the save, once", edits)
	}
	if _, err := os.Stat(paths.Inventory + ".bak"); err == nil {
		t.Error("inventory.yml.bak written; the history takes its place")
	}
}

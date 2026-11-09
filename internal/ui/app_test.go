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
			// the result of the checks of 9.설치
			if strings.HasSuffix(fmt.Sprintf("%T", msg), "checkDoneMsg") {
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
		case "ctrl+n":
			d.send(tea.KeyMsg{Type: tea.KeyCtrlN})
		case "ctrl+u": // clears a text input
			d.send(tea.KeyMsg{Type: tea.KeyCtrlU})
		default:
			d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(n)})
		}
	}
}

func (d *driver) at(want string) {
	d.t.Helper()
	got := stepTitles[d.app.current]
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
	s9 := d.app.screenList[8].(*screens.S9Install)
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
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("install-plan.sh lacks %q:\n%s", want, script)
		}
	}
}

func TestPreviewNeedsSave(t *testing.T) {
	d, _ := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
	d.at("8.저장")
	d.keys("ctrl+n")
	d.at("8.저장") // not saved yet
	if !strings.Contains(strings.Join(d.app.screenErrors, ""), "저장") {
		t.Errorf("no reason given: %v", d.app.screenErrors)
	}
}

func TestWizardWithoutAipub(t *testing.T) {
	d, paths := newDriver(t)
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
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
	d.keys("ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n", "ctrl+n")
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

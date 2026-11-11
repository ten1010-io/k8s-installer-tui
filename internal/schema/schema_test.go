package schema

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var templates = map[File]string{
	FileK8s:   "../fileio/templates/k8s-vars.yml",
	FileAipub: "../fileio/templates/aipub-vars.yml",
}

var commentedKey = regexp.MustCompile(`(?m)^#([a-z][a-z0-9_]*):`)

// Every key of the templates, set or commented out, is in the schema; a key the
// template sets has the template's value as its default; and Long is the comment
// of the template as it is there.
func TestTemplates(t *testing.T) {
	for f, path := range templates {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var live map[string]any
		if err := yaml.Unmarshal(data, &live); err != nil {
			t.Fatal(err)
		}
		for k, val := range live {
			v, ok := GetIn(f, k)
			if !ok {
				t.Errorf("%s: %s is not in the schema", path, k)
				continue
			}
			if !reflect.DeepEqual(Normalize(v, v.DefaultValue()), Normalize(v, val)) {
				t.Errorf("%s: default %q, template %#v", v, v.Default, val)
			}
		}
		for _, m := range commentedKey.FindAllStringSubmatch(string(data), -1) {
			if _, ok := GetIn(f, m[1]); !ok {
				t.Errorf("%s: %s (commented out) is not in the schema", path, m[1])
			}
		}
		text := string(data)
		for _, v := range ByFile(f) {
			if v.Long == "" {
				continue
			}
			var lines []string
			for _, l := range strings.Split(v.Long, "\n") {
				lines = append(lines, strings.TrimRight("# "+l, " "))
			}
			if block := strings.Join(lines, "\n"); !strings.Contains(text, block) && !strings.Contains(text, "# "+v.Long) {
				t.Errorf("%s: Long is not the comment of the template:\n%s", v, v.Long)
			}
		}
	}
}

var classes = map[string]bool{
	"service": true, "kubelet": true, "kubelet_policy": true, "cluster_upgrade": true, "cluster_config": true,
	"control_plane": true, "cp_extra_files": true, "node_metadata": true, "node_kernel": true,
	ClassNodeRebuild: true, "no_action": true, ClassImmutable: true,
}

func TestRegistry(t *testing.T) {
	groups := map[string]bool{}
	for _, g := range Groups {
		groups[g.Name] = true
	}
	seen := map[string]bool{}
	for _, v := range All() {
		id := fmt.Sprint(v)
		if seen[id] {
			t.Errorf("%s twice", v)
		}
		seen[id] = true
		if !groups[v.Group] {
			t.Errorf("%s: group %q is not in Groups", v, v.Group)
		}
		if v.Short == "" {
			t.Errorf("%s has no Short", v)
		}
		if (v.Type == TypeEnum) != (len(v.Enum) > 0) {
			t.Errorf("%s: Enum goes with TypeEnum", v)
		}
		if v.Type == TypeEnum && v.Default != "" && !contains(v.Enum, v.Default) {
			t.Errorf("%s: default %q is not one of %v", v, v.Default, v.Enum)
		}
		if v.Elem != "" && v.Type != TypeList {
			t.Errorf("%s: Elem on a %s", v, v.Type)
		}
		if v.Host && v.File != FileInventory {
			t.Errorf("%s: only inventory.yml has variables of hosts", v)
		}
		if v.VarClass != "" && !classes[v.VarClass] {
			t.Errorf("%s: unknown var class %q", v, v.VarClass)
		}
		if v.File == FileAipub && v.VarClass != "" {
			t.Errorf("%s: k8s-installer does not classify aipub variables", v)
		}
		if key, val, ok := v.Condition(); v.Required != Never && v.Required != Always {
			cv, found := GetIn(v.File, key)
			if !ok || !found {
				t.Errorf("%s: condition %q names no variable of its file", v, v.Required)
			} else if cv.Type == TypeBool && val != "true" && val != "false" {
				t.Errorf("%s: condition %q: %s is a bool", v, v.Required, key)
			}
		}
	}
	if _, ok := Get("k8s_pod_subnet"); !ok {
		t.Error("Get")
	}
	if v, _ := GetIn(FileK8s, "ansible_user"); v.File != FileK8s {
		t.Error("GetIn")
	}
	if len(ByFile(FileInventory))+len(ByFile(FileK8s))+len(ByFile(FileAipub)) != len(All()) {
		t.Error("ByFile")
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

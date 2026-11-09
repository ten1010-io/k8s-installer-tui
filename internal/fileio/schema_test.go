package fileio

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// The states the writer is driven through below: between them every key the
// writer can write is written.
func schemaStates() map[string]*state.AppState {
	full := sampleState()
	full.InternalDNSServerEnabled = true
	full.K8sIngressMode = 1
	full.K8sGatewayVIP = "192.168.0.101"
	full.Nodes[1].AnsiblePort = "2222"
	full.Nodes[2].AnsibleUser = "ubuntu"

	noHA := sampleState()
	noHA.AipubHaMode = false
	noHA.AipubHarborLocalPVNode = "node1"

	noAipub := sampleState()
	noAipub.AipubEnabled = false
	noAipub.KiCpHaMode = false
	noAipub.KiCpHaModeVIP = ""

	return map[string]*state.AppState{"full": full, "no HA": noHA, "no AIPub": noAipub}
}

// written lists, per file, the values the rendered files hold under the names
// the schema uses: the top-level keys of the two vars files, and for
// inventory.yml the groups and the variables of the hosts (host -> value).
func written(t *testing.T, r *Rendered) map[schema.File]map[string]any {
	out := map[schema.File]map[string]any{schema.FileK8s: decode(t, r.Vars), schema.FileInventory: {}}
	if r.AipubVars != nil {
		out[schema.FileAipub] = decode(t, r.AipubVars)
	}
	inv := decode(t, r.Inventory)
	for group := range inv {
		hosts, _ := dig(inv, group, "hosts").(map[string]any)
		if group != "all" && group != "control_node" {
			names := []any{}
			for _, h := range sortedKeys(hosts) {
				names = append(names, h)
			}
			out[schema.FileInventory][group] = names
		}
		for host, vars := range hosts {
			vm, _ := vars.(map[string]any)
			for k, val := range vm {
				m, _ := out[schema.FileInventory][k].(map[string]any)
				if m == nil {
					m = map[string]any{}
					out[schema.FileInventory][k] = m
				}
				m[host] = val
			}
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Every key the writer writes is described by the schema, so that the schema
// can not fall behind the writer.
func TestWriterKeysInSchema(t *testing.T) {
	for name, s := range schemaStates() {
		r, err := Render(DefaultPaths(t.TempDir()), s)
		must(t, err)
		for f, keys := range written(t, r) {
			for k := range keys {
				if _, ok := schema.GetIn(f, k); !ok {
					t.Errorf("%s: %s of %s is written but not in the schema", name, k, f.Path())
				}
			}
		}
	}
}

// What schema.Value says the state gives a variable is what the writer wrote for
// it. A host that does not set a variable of hosts has its default.
func TestSchemaValueMatchesWriter(t *testing.T) {
	for name, s := range schemaStates() {
		r, err := Render(DefaultPaths(t.TempDir()), s)
		must(t, err)
		files := written(t, r)
		for _, v := range schema.All() {
			if !schema.Written(s, v.File) {
				continue
			}
			val, ok := schema.Value(s, v)
			if !ok {
				continue
			}
			want := schema.Normalize(v, val)
			got, present := files[v.File][v.Key]
			if v.Host {
				gotHosts, _ := got.(map[string]any)
				wantHosts := want.(map[string]any)
				for h, w := range wantHosts {
					g, set := gotHosts[h]
					if !set {
						g = v.DefaultValue()
					}
					if w == nil {
						w = v.DefaultValue()
					}
					if fmt.Sprint(g) != fmt.Sprint(w) {
						t.Errorf("%s: %s of %s: file has %v, schema.Value %v", name, v.Key, h, g, w)
					}
				}
				continue
			}
			if v.File == schema.FileInventory && (v.Key == "ki_cp_node" || v.Key == "k8s_node") {
				// group members: compared as sets, the file decodes into a map
				got = sortSlice(got)
				want = sortSlice(want)
			}
			if !present || !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s: file has %#v (present %v), schema.Value %#v", name, v, got, present, want)
			}
		}
	}
}

func sortSlice(v any) any {
	l, _ := v.([]any)
	m := map[string]any{}
	for _, x := range l {
		m[fmt.Sprint(x)] = nil
	}
	out := []any{}
	for _, k := range sortedKeys(m) {
		out = append(out, k)
	}
	return out
}

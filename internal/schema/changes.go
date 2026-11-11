package schema

import (
	"reflect"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Loaded is what the files of an existing site held when the wizard started.
type Loaded struct {
	// Values are, per file that existed, the value of each top-level key as yaml
	// decodes it. For inventory.yml they are the variables of the hosts instead,
	// each a map of host name to value
	Values map[File]map[string]any
	Hosts  []string // the nodes of inventory.yml (all.hosts)
}

// Change is a variable of class immutable or node_rebuild that the state gives a
// value other than the one the site's file holds.
type Change struct {
	Var      Var
	Host     string // the node, for a variable of hosts
	Old, New string // as written in the file
}

func (c Change) Message() string {
	key := c.Var.Key
	if c.Host != "" {
		key += " (" + c.Host + ")"
	}
	format := "%s cannot be changed after the install (current file: %s → new value: %s)"
	if c.Var.VarClass == ClassNodeRebuild {
		format = "%s rebuilds the node when changed (current file: %s → new value: %s)"
	}
	return i18n.T(format, key, c.Old, c.New)
}

// FixedChanges lists the variables that can not be changed on an existing
// cluster, or rebuild a node when they are, and that the state changes from what
// the site's files held. Nothing for a new site (old is nil), a file that did not
// exist, a key the file did not have, or a node the inventory did not have.
func FixedChanges(old *Loaded, s *state.AppState) []Change {
	if old == nil {
		return nil
	}
	known := map[string]bool{}
	for _, h := range old.Hosts {
		known[h] = true
	}
	var out []Change
	for _, v := range All() {
		if !v.Fixed() || !Written(s, v.File) {
			continue
		}
		file, existed := old.Values[v.File]
		if !existed {
			continue
		}
		val, managed := Value(s, v)
		if !managed {
			continue
		}
		if hosts, ok := val.([]HostValue); ok {
			before, _ := file[v.Key].(map[string]any)
			for _, h := range hosts {
				if !known[h.Host] {
					continue
				}
				was, set := before[h.Host]
				if !set {
					was = v.DefaultValue()
				}
				now := h.Value
				if now == nil {
					now = v.DefaultValue()
				}
				if c, changed := compare(v, was, now); changed {
					c.Host = h.Host
					out = append(out, c)
				}
			}
			continue
		}
		was, set := file[v.Key]
		if !set {
			continue
		}
		if c, changed := compare(v, was, val); changed {
			out = append(out, c)
		}
	}
	return out
}

func compare(v Var, was, now any) (Change, bool) {
	a, b := Normalize(v, was), Normalize(v, now)
	if reflect.DeepEqual(a, b) {
		return Change{}, false
	}
	return Change{Var: v, Old: flowYAML(a), New: flowYAML(b)}, true
}

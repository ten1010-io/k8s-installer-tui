// Package schema describes every variable the wizard writes, and the ones the
// templates of the installers let a site set: which file it lives in, what it
// holds, whether it is required, its default, and what it means. The screens,
// the validator and the site-values view read it from here, so a variable is
// described once.
package schema

import (
	"fmt"
	"strings"
)

// File is which of the three output files a variable lives in.
type File string

const (
	FileInventory File = "inventory" // inventory.yml (k8s-installer)
	FileK8s       File = "k8s"       // group_vars/all/vars.yml (k8s-installer)
	FileAipub     File = "aipub"     // files/aipub/vars.yml (aipub-installer)
)

// Files in the order they are described.
var Files = []File{FileInventory, FileK8s, FileAipub}

// Path is where the file sits under the ansible directory of k8s-installer.
func (f File) Path() string {
	switch f {
	case FileInventory:
		return "inventory.yml"
	case FileK8s:
		return "group_vars/all/vars.yml"
	case FileAipub:
		return "files/aipub/vars.yml"
	}
	return string(f)
}

// Type is what a variable holds, and so the format it is checked against.
type Type string

const (
	TypeString    Type = "string"
	TypeInt       Type = "int"       // a positive integer
	TypeBool      Type = "bool"      //
	TypeCIDR      Type = "cidr"      // x.x.x.x/xx
	TypeIP        Type = "ip"        // an address
	TypeFQDN      Type = "fqdn"      // a domain: example.com
	TypeSubdomain Type = "subdomain" // a dns label or labels under a domain: aipub-harbor
	TypeMinor     Type = "minor"     // a kubernetes minor: 1.36
	TypeDuration  Type = "duration"  // hours, as kubeadm takes them: 26280h
	TypeSize      Type = "size"      // a quantity in binary units: 512Gi
	TypeDays      Type = "days"      // a number of days, as the monitoring installer takes them: 180d
	TypePort      Type = "port"      // 1-65535
	TypeEnum      Type = "enum"      // one of Enum
	TypeList      Type = "list"      // a sequence; its items are Elem when that is set
	TypeMap       Type = "map"       // a mapping

	// The items of the lists of the control plane, each a mapping of one shape
	// (validate-hostvars.py of k8s-installer: ExtraArgModel, ExtraVolumeModel,
	// CpExtraFileModel)
	TypeExtraArg    Type = "extra_arg"     // {name, value}: a flag without its dashes
	TypeExtraVolume Type = "extra_volume"  // {name, hostPath, mountPath, readOnly?, pathType?}
	TypeCpExtraFile Type = "cp_extra_file" // {src, dest, mode}
)

// Required values.
const (
	Never  = ""       // optional
	Always = "always" // required whenever the file is written
)

// Var describes one variable.
type Var struct {
	Key   string // the yaml key
	File  File   // the file it is written to
	Group string // the section it is shown under (Groups)
	Type  Type
	Elem  Type // the type of the items of a list, when they have a format
	// Host marks a variable of a host in inventory.yml, set per node rather than
	// once for the file
	Host bool
	// Default is the value as text: what the installer uses when the key is not
	// set, or for a key of the templates, the value a new site file starts with.
	// Empty is null
	Default string
	// Required is Always, Never, or a condition "<key>=<value>": required while
	// the variable named holds that value (ki_cp_ha_mode=true)
	Required string
	Enum     []string // the values allowed, for TypeEnum
	Short    string   // one line, for the screens and the site-values view
	Long     string   // the comment of the installer's template, as it is there
	// Preflight is a check worth running before the installer does, one line
	Preflight string
	// Decided are the flags the installer decides for the component itself and
	// refuses in a list of TypeExtraArg (ki_decided_args of ki_k8s_cp_components)
	Decided []string
	// VarClass is what changing the variable on an existing cluster takes, from
	// ki_var_classes of k8s-installer (immutable, node_rebuild, service, ...).
	// Empty for the variables of aipub, which k8s-installer does not classify
	VarClass string
	Screen   string // the wizard step it is edited on, when one edits it
	// Optional marks a variable of aipub the wizard writes only while the state
	// differs from Default (an empty text counts as the default for a TypeString),
	// and takes out of the file once it is back at it, so that a file keeps what
	// the installer defaults to. Value says "not managed" for it at the default
	Optional bool
}

// Groups are the sections of the site-values view, in the order shown. Title is
// English; show it through i18n.T (it is a table, built before --lang is read).
var Groups = []struct{ Name, Title string }{
	{"nodes", "Nodes / access"},
	{"host", "Node disk / kernel"},
	{"network", "Internal network / ki-cp"},
	{"kubernetes", "Kubernetes"},
	{"control_plane", "Extra control plane arguments"},
	{"preflight", "Checks before the run / etcd backup"},
	{"kubelet", "kubelet"},
	{"ingress", "Domain / ingress / load balancer"},
	{"cert", "Certificates"},
	{"harbor", "Harbor / HA storage"},
	{"app", "AIPub app"},
	{"deploy", "Deploy tools"},
	{"monitoring", "Monitoring"},
	{"python", "Python services"},
}

// Classes of k8s-installer a change to an existing cluster is refused for or
// rebuilds a node for.
const (
	ClassImmutable   = "immutable"
	ClassNodeRebuild = "node_rebuild"
)

// All returns every variable, in the order of the files and of the templates.
func All() []Var { return vars }

// Get returns the variable named key. ansible_user is the one key two files
// carry (a host of inventory.yml and vars.yml); Get returns the inventory one,
// GetIn picks.
func Get(key string) (Var, bool) {
	for _, v := range vars {
		if v.Key == key {
			return v, true
		}
	}
	return Var{}, false
}

// GetIn returns the variable named key of file f.
func GetIn(f File, key string) (Var, bool) {
	for _, v := range vars {
		if v.File == f && v.Key == key {
			return v, true
		}
	}
	return Var{}, false
}

// ByFile returns the variables of file f, in order.
func ByFile(f File) []Var {
	var out []Var
	for _, v := range vars {
		if v.File == f {
			out = append(out, v)
		}
	}
	return out
}

// Condition splits a Required condition into the key and the value it names. ok
// is false for Always and Never.
func (v Var) Condition() (key, value string, ok bool) {
	if v.Required == Never || v.Required == Always {
		return "", "", false
	}
	key, value, ok = strings.Cut(v.Required, "=")
	return key, value, ok
}

// Fixed reports whether changing the variable on an existing cluster is refused
// or rebuilds a node.
func (v Var) Fixed() bool {
	return v.VarClass == ClassImmutable || v.VarClass == ClassNodeRebuild
}

func (v Var) String() string { return fmt.Sprintf("%s (%s)", v.Key, v.File.Path()) }

package main

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// A key given to i18n.T by identifier (a constant, a variable set to
// literals, a field of a table) has its Korean too, which
// TestEveryKeyHasItsKorean cannot see. The keys are read from the program's
// own source: the constant, the literals the variable is set to, the rows of
// the table a field is read from. A key this cannot trace is an error, so that
// a new way of giving T a key is looked at.
func TestKeysByIdentifierHaveTheirKorean(t *testing.T) {
	ko := i18n.Catalog(i18n.KO)
	fset, files := sourceFiles(t)
	r := newTableReader(files)
	var problems []string
	read := map[string]bool{}
	for path, f := range files {
		dir := filepath.ToSlash(filepath.Dir(path))
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 || !isI18nT(call) {
					return true
				}
				if _, literal := constString(call.Args[0]); literal {
					return true // TestEveryKeyHasItsKorean
				}
				at := fset.Position(call.Pos()).String() + ": i18n.T(" + types.ExprString(call.Args[0]) + ")"
				keys, ok := r.keys(call.Args[0], scope{dir, fn, call})
				if !ok || len(keys) == 0 {
					problems = append(problems, at+": cannot tell its keys (give it a literal, a constant or a table this test reads)")
					return true
				}
				for _, k := range keys {
					read[k] = true
					switch {
					case hasHangul(k):
						problems = append(problems, at+": a Korean key "+strconv.Quote(k))
					case ko[k] == "":
						problems = append(problems, at+": no Korean for "+strconv.Quote(k))
					}
				}
				return true
			})
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}

	// a guard against reading nothing and passing: a key of each kind, from
	// recheckMessage, notesTab, the cpTargets (a label, a hint), certModes,
	// the s4 fields, stepTitles, updateClusterTime, DefaultClass,
	// siteValuesHeader, the formats of changes.go, the schema's Groups and
	// Short, and state.Presets
	for _, want := range []string{
		"Checking the StorageClass again...",
		"Notes",
		"apiserver args",
		"name=value",
		"The customer provides both tls.crt and tls.key.\n  The step that issues certificates is skipped.",
		"Pod subnet",
		"7.Certs",
		updateClusterTimeKey(ko),
		"(cluster default StorageClass)",
		siteValuesHeaderKey(ko),
		"%s rebuilds the node when changed (current file: %s → new value: %s)",
		"Nodes / access",
		"Address to reach the node (IP)",
		"AIPub audit log",
	} {
		if !read[want] {
			t.Errorf("not read from the source: %q", want)
		}
	}
}

// updateClusterTimeKey and siteValuesHeaderKey are the long keys of
// plan.updateClusterTime and schema.siteValuesHeader, found in the catalog by
// how they start.
func updateClusterTimeKey(ko map[string]string) string { return keyStarting(ko, "3 to 6 minutes per ") }

func siteValuesHeaderKey(ko map[string]string) string {
	return keyStarting(ko, "# ============================================================\n#  Site settings")
}

func keyStarting(ko map[string]string, prefix string) string {
	for k := range ko {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			return k
		}
	}
	return prefix
}

func isI18nT(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "i18n"
}

// scope is where an expression is read: its package (by directory), the
// function it is in and the call it is part of.
type scope struct {
	dir  string
	fn   *ast.FuncDecl // nil at package level
	call ast.Node
}

// pkgIndex is what a package declares at its top level.
type pkgIndex struct {
	values  map[string]ast.Expr // constants and variables, by name
	structs map[string][]string // struct types, by name: their fields in order
}

// tableReader reads the keys an expression can hold from the program's source.
type tableReader struct {
	byDir  map[string]*pkgIndex
	byName map[string]string   // package name → directory
	keyed  map[string][]string // exported field → the constant strings given to it by name
}

func newTableReader(files map[string]*ast.File) *tableReader {
	r := &tableReader{byDir: map[string]*pkgIndex{}, byName: map[string]string{}, keyed: map[string][]string{}}
	for path, f := range files {
		dir := filepath.ToSlash(filepath.Dir(path))
		p := r.byDir[dir]
		if p == nil {
			p = &pkgIndex{values: map[string]ast.Expr{}, structs: map[string][]string{}}
			r.byDir[dir] = p
			r.byName[f.Name.Name] = dir
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for i, n := range s.Names {
						if i < len(s.Values) {
							p.values[n.Name] = s.Values[i]
						}
					}
				case *ast.TypeSpec:
					if st, ok := s.Type.(*ast.StructType); ok {
						p.structs[s.Name.Name] = fieldNames(st)
					}
				}
			}
		}
		// for a field read off a value the reader cannot follow (what a
		// function returns or is given), every constant string given to that
		// field by name: the schema's Short and Preflight
		ast.Inspect(f, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if k, ok := kv.Key.(*ast.Ident); ok && ast.IsExported(k.Name) {
					if s, ok := constString(kv.Value); ok && s != "" {
						r.keyed[k.Name] = append(r.keyed[k.Name], s)
					}
				}
			}
			return true
		})
	}
	return r
}

func fieldNames(st *ast.StructType) []string {
	var names []string
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// binding is a value an identifier is given in a function: an expression, or
// each element of the table it ranges over. One with neither is a value the
// reader cannot follow (a parameter, one of several results).
type binding struct {
	value, rangeOver ast.Expr
}

// bindings are the values name is given in the function of sc, but by the
// assignment sc's call is part of; false when the function does not bind it.
func (r *tableReader) bindings(name string, sc scope) ([]binding, bool) {
	if sc.fn == nil {
		return nil, false
	}
	var out []binding
	for _, fl := range sc.fn.Type.Params.List {
		for _, n := range fl.Names {
			if n.Name == name {
				out = append(out, binding{})
			}
		}
	}
	within := func(n ast.Node) bool { return sc.call.Pos() >= n.Pos() && sc.call.End() <= n.End() }
	ast.Inspect(sc.fn.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, l := range s.Lhs {
				if id, ok := l.(*ast.Ident); ok && id.Name == name {
					switch {
					case len(s.Rhs) != len(s.Lhs):
						out = append(out, binding{})
					case !within(s.Rhs[i]):
						out = append(out, binding{value: s.Rhs[i]})
					}
				}
			}
		case *ast.RangeStmt:
			if id, ok := s.Value.(*ast.Ident); ok && id.Name == name {
				out = append(out, binding{rangeOver: s.X})
			}
		case *ast.ValueSpec:
			for i, id := range s.Names {
				if id.Name == name {
					if i < len(s.Values) {
						out = append(out, binding{value: s.Values[i]})
					} else {
						out = append(out, binding{})
					}
				}
			}
		}
		return true
	})
	return out, len(out) > 0
}

// keys are the strings e holds where sc reads it.
func (r *tableReader) keys(e ast.Expr, sc scope) ([]string, bool) {
	if s, ok := constString(e); ok {
		return []string{s}, true
	}
	switch e := e.(type) {
	case *ast.Ident:
		if bs, bound := r.bindings(e.Name, sc); bound {
			var out []string
			for _, b := range bs {
				ks, ok := r.bindingKeys(b, sc)
				if !ok {
					return nil, false
				}
				out = append(out, ks...)
			}
			return out, true
		}
		if v, ok := r.byDir[sc.dir].values[e.Name]; ok {
			return r.keys(v, scope{dir: sc.dir})
		}
	case *ast.SelectorExpr:
		if rows, fields, in, ok := r.rows(e.X, sc); ok {
			var out []string
			for _, row := range rows {
				v, set := field(row, fields, e.Sel.Name)
				if !set {
					continue
				}
				ks, ok := r.keys(v, in)
				if !ok {
					return nil, false
				}
				out = append(out, ks...)
			}
			return out, true
		}
		if ks := r.keyed[e.Sel.Name]; len(ks) > 0 {
			return ks, true
		}
	}
	return nil, false
}

// bindingKeys are the strings an identifier holds by binding b.
func (r *tableReader) bindingKeys(b binding, sc scope) ([]string, bool) {
	switch {
	case b.value != nil:
		return r.keys(b.value, sc)
	case b.rangeOver != nil:
		els, _, in, ok := r.elems(b.rangeOver, sc)
		if !ok {
			return nil, false
		}
		var out []string
		for _, el := range els {
			ks, ok := r.keys(el, in)
			if !ok {
				return nil, false
			}
			out = append(out, ks...)
		}
		return out, true
	}
	return nil, false
}

// rows are the rows of the tables x is one of, the fields of their type and
// where they are read.
func (r *tableReader) rows(x ast.Expr, sc scope) ([]*ast.CompositeLit, []string, scope, bool) {
	var els []ast.Expr
	var fields []string
	in := sc
	switch x := x.(type) {
	case *ast.IndexExpr:
		var ok bool
		if els, fields, in, ok = r.elems(x.X, sc); !ok {
			return nil, nil, sc, false
		}
	case *ast.Ident:
		bs, bound := r.bindings(x.Name, sc)
		if !bound {
			return nil, nil, sc, false
		}
		var all []*ast.CompositeLit
		for _, b := range bs {
			var rows []*ast.CompositeLit
			var f []string
			var ok bool
			switch {
			case b.rangeOver != nil:
				var e []ast.Expr
				if e, f, in, ok = r.elems(b.rangeOver, sc); ok {
					rows, ok = compositeLits(e)
				}
			case b.value != nil:
				rows, f, in, ok = r.rows(b.value, sc)
			}
			if !ok || f == nil || (fields != nil && !sameFields(f, fields)) {
				return nil, nil, sc, false
			}
			fields = f
			all = append(all, rows...)
		}
		return all, fields, in, true
	default:
		return nil, nil, sc, false
	}
	rows, ok := compositeLits(els)
	if !ok || fields == nil {
		return nil, nil, sc, false
	}
	return rows, fields, in, true
}

func compositeLits(els []ast.Expr) ([]*ast.CompositeLit, bool) {
	var out []*ast.CompositeLit
	for _, el := range els {
		lit, ok := el.(*ast.CompositeLit)
		if !ok {
			return nil, false
		}
		out = append(out, lit)
	}
	return out, true
}

func sameFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// elems are the elements of the table e names (a composite literal, or a
// variable set to one, of this package or another), the fields of their type
// when it is a struct, and where they are read.
func (r *tableReader) elems(e ast.Expr, sc scope) ([]ast.Expr, []string, scope, bool) {
	switch e := e.(type) {
	case *ast.CompositeLit:
		if arr, ok := e.Type.(*ast.ArrayType); ok {
			return e.Elts, r.structOf(arr.Elt, sc.dir), sc, true
		}
	case *ast.Ident:
		if bs, bound := r.bindings(e.Name, sc); bound {
			if len(bs) == 1 && bs[0].value != nil {
				return r.elems(bs[0].value, sc)
			}
			return nil, nil, sc, false
		}
		if v, ok := r.byDir[sc.dir].values[e.Name]; ok {
			return r.elems(v, scope{dir: sc.dir})
		}
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			if dir, ok := r.byName[pkg.Name]; ok {
				if v, ok := r.byDir[dir].values[e.Sel.Name]; ok {
					return r.elems(v, scope{dir: dir})
				}
			}
		}
	}
	return nil, nil, sc, false
}

// structOf are the fields of the struct type t, in order; nil for another type.
func (r *tableReader) structOf(t ast.Expr, dir string) []string {
	switch t := t.(type) {
	case *ast.StructType:
		return fieldNames(t)
	case *ast.Ident:
		return r.byDir[dir].structs[t.Name]
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			if d, ok := r.byName[pkg.Name]; ok {
				return r.byDir[d].structs[t.Sel.Name]
			}
		}
	}
	return nil
}

// field is the value row gives the field name, by name or by its place.
func field(row *ast.CompositeLit, fields []string, name string) (ast.Expr, bool) {
	for i, el := range row.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if k, ok := kv.Key.(*ast.Ident); ok && k.Name == name {
				return kv.Value, true
			}
			continue
		}
		if i < len(fields) && fields[i] == name {
			return el, true
		}
	}
	return nil, false
}

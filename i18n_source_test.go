package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// hasHangul reports whether s has a Hangul letter (syllables or jamo).
func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// catalogFile reports whether a Go file holds translations (ko.go, ko_*.go),
// the one place Korean may be.
func catalogFile(name string) bool {
	return name == "ko.go" || strings.HasPrefix(name, "ko_")
}

// sourceFiles parses every Go file of the program: not the tests, not testdata.
func sourceFiles(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "testdata" || n == ".git" || n == "test" || (strings.HasPrefix(n, ".") && n != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(path)] = f
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fset, files
}

// The English default says no Korean: no string in the program has Hangul but
// the Korean catalogs, and nothing embedded to be written (the templates, the
// audit policy that goes to the nodes) has any.
func TestNoKoreanOutsideTheCatalogs(t *testing.T) {
	fset, files := sourceFiles(t)
	perDir := map[string]int{}
	var found []string
	for path, f := range files {
		if catalogFile(filepath.Base(path)) {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !hasHangul(s) {
				return true
			}
			perDir[filepath.ToSlash(filepath.Dir(path))]++
			found = append(found, fset.Position(lit.Pos()).String()+": "+firstLine(s))
			return true
		})
	}
	templates, _ := filepath.Glob(filepath.Join("internal", "fileio", "templates", "*"))
	for _, p := range templates {
		if filepath.Base(p) == "README.md" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for i, l := range strings.Split(string(data), "\n") {
			if hasHangul(l) {
				perDir[filepath.ToSlash(filepath.Dir(p))]++
				found = append(found, p+":"+strconv.Itoa(i+1)+": "+l)
			}
		}
	}
	if len(found) == 0 {
		return
	}
	sort.Strings(found)
	var dirs []string
	for d, n := range perDir {
		dirs = append(dirs, d+" "+strconv.Itoa(n))
	}
	sort.Strings(dirs)
	t.Errorf("Korean outside the catalogs, %d in all (%s):\n%s", len(found), strings.Join(dirs, ", "), strings.Join(found, "\n"))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

// constString is the value of an expression made of string literals joined
// with +, as an i18n.T key is written; false for anything else.
func constString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok := constString(e.X)
		if !ok {
			return "", false
		}
		b, ok := constString(e.Y)
		return a + b, ok
	case *ast.ParenExpr:
		return constString(e.X)
	}
	return "", false
}

// Every key given to i18n.T as text has its Korean, and every Korean takes
// the same arguments as its English. Keys of the catalogs are English.
func TestEveryKeyHasItsKorean(t *testing.T) {
	ko := i18n.Catalog(i18n.KO)
	fset, files := sourceFiles(t)
	var missing []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "T" {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "i18n" {
				return true
			}
			key, ok := constString(call.Args[0])
			if !ok {
				return true // a key from a table: its own test checks it
			}
			if _, ok := ko[key]; !ok {
				missing = append(missing, fset.Position(call.Pos()).String()+": "+strconv.Quote(key))
			}
			return true
		})
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no Korean for %d keys:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	for _, k := range i18n.Keys(i18n.KO) {
		if hasHangul(k) {
			t.Errorf("a Korean key (keys are the English): %q", k)
		}
		if a, b := verbArgs(k), verbArgs(ko[k]); a != b {
			t.Errorf("%q takes %d arguments, its Korean %d: %q", k, a, b, ko[k])
		}
	}
}

// verbArgs is the number of arguments a format reads: the highest argument a
// verb takes, %[n] included; %% reads none.
func verbArgs(format string) int {
	next, most := 1, 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			continue
		}
		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
		if i < len(format) && format[i] == '[' {
			j := strings.IndexByte(format[i:], ']')
			if j > 0 {
				if n, err := strconv.Atoi(format[i+1 : i+j]); err == nil {
					next = n
				}
				i += j + 1
			}
		}
		for i < len(format) && (format[i] >= '0' && format[i] <= '9' || format[i] == '.' || format[i] == '*') {
			i++
		}
		if i < len(format) {
			if next > most {
				most = next
			}
			next++
		}
	}
	return most
}

func TestVerbArgs(t *testing.T) {
	for f, want := range map[string]int{
		"plain": 0, "100%% done": 0, "%s and %d": 2, "%[2]s %[1]s": 2, "%-8s|%5.2f": 2, "%[1]s then %s": 2, "%v": 1,
	} {
		if got := verbArgs(f); got != want {
			t.Errorf("%q: %d, want %d", f, got, want)
		}
	}
}

// Package i18n picks the language the program speaks: English by default,
// Korean with --lang ko (or K8S_INSTALLER_TUI_LANG=ko). Closed networks run
// Linux without Korean fonts or locales, where Korean comes out as boxes, so
// nothing Korean is said unless asked for.
//
// The text in the code is English, and it is the key:
//
//	i18n.T("archive %s not found: %v", path, err)
//
// Each package that says something registers its Korean in a ko*.go file,
// keyed by that English (Register(KO, map[string]string{...})); a translation
// may reorder the arguments with %[n]s. A key with no Korean is said in English.
// What goes to the nodes (the audit policy, the comments of the configs copied
// there) is English whatever the language, and is not passed through T.
//
// T is called where the text is shown, never in a package-level var: those are
// set before --lang is read. Text kept in a table (the schema's descriptions)
// is English there and passed through T where it is shown.
package i18n

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Lang is a language the program speaks.
type Lang string

const (
	EN Lang = "en"
	KO Lang = "ko"
)

// EnvVar names the language when --lang does not.
const EnvVar = "K8S_INSTALLER_TUI_LANG"

var (
	mu       sync.RWMutex
	current  = EN
	catalogs = map[Lang]map[string]string{}
)

// Register adds translations of English keys to l. A key registered twice
// with two translations is a mistake in the catalogs, and panics at start.
func Register(l Lang, m map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	c := catalogs[l]
	if c == nil {
		c = map[string]string{}
		catalogs[l] = c
	}
	for k, v := range m {
		if old, ok := c[k]; ok && old != v {
			panic(fmt.Sprintf("i18n: %q has two %s translations: %q and %q", k, l, old, v))
		}
		c[k] = v
	}
}

// Parse names the language of s: en or ko (any case, a locale like ko_KR.UTF-8
// too); empty is English.
func Parse(s string) (Lang, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "" || s == "en" || strings.HasPrefix(s, "en_") || strings.HasPrefix(s, "en-") || s == "c" || s == "posix":
		return EN, nil
	case s == "ko" || strings.HasPrefix(s, "ko_") || strings.HasPrefix(s, "ko-"):
		return KO, nil
	}
	return EN, fmt.Errorf("unknown language %q: en or ko", s)
}

// Set makes l the language from now on.
func Set(l Lang) {
	mu.Lock()
	current = l
	mu.Unlock()
}

// Current is the language spoken now.
func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// FromArgs finds the language before the flags are defined, as their help is
// in it: --lang/-lang in args (as "--lang ko" or "--lang=ko"; the last one, as
// the flag package takes it), else the environment variable, else English.
func FromArgs(args []string, env string) (Lang, error) {
	val, found := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		name, v, eq := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "lang" {
			continue
		}
		if !eq {
			if i+1 >= len(args) {
				return EN, fmt.Errorf("--lang needs a value: en or ko")
			}
			i++
			v = args[i]
		}
		val, found = v, true
	}
	if found {
		return Parse(val)
	}
	return Parse(env)
}

// Init sets the language of this run from the command line and the
// environment (FromArgs). An unknown one is an error, and English is kept.
func Init() error {
	l, err := FromArgs(os.Args[1:], os.Getenv(EnvVar))
	Set(l)
	return err
}

// T is format said in the current language: its translation when there is
// one, then formatted with args (fmt.Sprintf). With no args the text is
// returned as it is, so a lone % needs no escaping there.
func T(format string, args ...any) string {
	if l := Current(); l != EN {
		mu.RLock()
		if tr, ok := catalogs[l][format]; ok {
			format = tr
		}
		mu.RUnlock()
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Catalog is a copy of the translations to l, for tests.
func Catalog(l Lang) map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]string, len(catalogs[l]))
	for k, v := range catalogs[l] {
		out[k] = v
	}
	return out
}

// Keys are the keys of the translations to l, sorted, for tests.
func Keys(l Lang) []string {
	c := Catalog(l)
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

package schema

import (
	"testing"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

func hasHangul(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// The text of the tables is English in the table (built before --lang is read)
// and shown through i18n.T: each human-readable field has its Korean. The root
// check of keys cannot see it, as the keys are not passed to T as literals.
func TestTableTextHasItsKorean(t *testing.T) {
	ko := i18n.Catalog(i18n.KO)
	check := func(what, text string) {
		if text == "" {
			return
		}
		if hasHangul(text) {
			t.Errorf("%s: Korean in the table: %q", what, text)
		}
		if ko[text] == "" {
			t.Errorf("%s: no Korean for %q", what, text)
		}
	}
	for _, v := range All() {
		check(v.String()+" Short", v.Short)
		check(v.String()+" Preflight", v.Preflight)
		if hasHangul(v.Long) || hasHangul(v.Screen) || hasHangul(v.Group) {
			t.Errorf("%s: Korean in Long, Screen or Group", v)
		}
	}
	for _, g := range Groups {
		check("group "+g.Name, g.Title)
	}
}

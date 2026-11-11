package state

import (
	"testing"
	"unicode"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// A preset title is English in the table (built before --lang is read) and
// shown through i18n.T: each has its Korean.
func TestPresetTitlesHaveTheirKorean(t *testing.T) {
	ko := i18n.Catalog(i18n.KO)
	for _, p := range Presets {
		if p.Title == "" {
			t.Errorf("preset %s has no Title", p.ID)
		}
		for _, r := range p.Title {
			if unicode.Is(unicode.Hangul, r) {
				t.Errorf("preset %s: Title is Korean in the table: %q", p.ID, p.Title)
				break
			}
		}
		if ko[p.Title] == "" {
			t.Errorf("preset %s: no Korean for Title %q", p.ID, p.Title)
		}
	}
}

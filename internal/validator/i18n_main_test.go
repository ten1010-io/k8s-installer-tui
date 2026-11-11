package validator

import (
	"os"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
)

// The tests were written against the Korean text, which the Korean catalog
// keeps word for word: they run in Korean, so that they also check it is
// unchanged. The English default is tested where it is said in English
// (TestNoKoreanOutsideTheCatalogs at the root, and tests that set i18n.EN).
func TestMain(m *testing.M) {
	i18n.Set(i18n.KO)
	os.Exit(m.Run())
}

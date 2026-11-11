package plan

import (
	"sort"
	"strings"
	"testing"

	"github.com/ten1010-io/k8s-installer-tui/internal/i18n"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// Every step that runs update-cluster.yml says how long it takes, so that a
// quarter of an hour on three control plane nodes is not taken for a hang.
func TestUpdateClusterStepsSayHowLongTheyTake(t *testing.T) {
	seen := map[string]bool{}
	for _, aipub := range []bool{true, false} {
		for _, existing := range []bool{false, true} {
			for _, installed := range []bool{false, true} {
				if installed && (!existing || !aipub) {
					continue
				}
				s := aipubState()
				s.AipubEnabled = aipub
				for _, p := range state.Presets {
					s.ControlPlane.Set(p.ID, true)
				}
				o := testOptions()
				o.Existing, o.AipubInstalled = existing, installed
				for _, st := range Build(s, o) {
					if st.Playbook != "playbooks/update-cluster.yml" || st.Skip {
						continue
					}
					seen[st.ID] = true
					if !strings.Contains(st.Note, i18n.T(updateClusterTime)) {
						t.Errorf("aipub %v existing %v installed %v: %s says nothing of the time: %q", aipub, existing, installed, st.ID, st.Note)
					}
				}
			}
		}
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	t.Logf("update-cluster steps seen: %v", ids)
	if !seen["A5.2"] || !seen["K3"] {
		t.Errorf("A5.2 and the K3 of an existing site not among %v", ids)
	}
}

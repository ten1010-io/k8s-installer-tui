package screens

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

var s4bANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func s4bState() *state.AppState {
	st := state.DefaultState()
	st.AipubEnabled = true
	st.AipubIngressZone = "example.com"
	return st
}

func newS4b(st *state.AppState, existing bool) *S4bControlPlane {
	s := NewS4bControlPlane()
	s.SetSite(plan.Options{Existing: existing})
	s.SetSize(98, 48)
	s.SyncFromState(st)
	return s
}

func (s *S4bControlPlane) plain() string { return s4bANSI.ReplaceAllString(s.View(), "") }

// s4bKeys sends keys as a terminal would: a name, or text typed a key per character.
func s4bKeys(s *S4bControlPlane, keys ...string) {
	for _, k := range keys {
		switch k {
		case "down":
			s.Update(tea.KeyMsg{Type: tea.KeyDown})
		case "up":
			s.Update(tea.KeyMsg{Type: tea.KeyUp})
		case "left":
			s.Update(tea.KeyMsg{Type: tea.KeyLeft})
		case "right":
			s.Update(tea.KeyMsg{Type: tea.KeyRight})
		case "enter":
			s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		case "ctrl+u":
			s.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		default:
			for _, r := range k {
				if r == ' ' {
					s.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
				} else {
					s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				}
			}
		}
	}
}

// A new site: the presets and the JWKS fields, the advanced section collapsed and
// the legacy webhook in it, out of sight.
func TestS4bNewSiteCollapsed(t *testing.T) {
	s := newS4b(s4bState(), false)
	view := s.plain()
	for _, want := range []string{"[x] AIPub 인증 (JWKS)", "issuer URL", "CA 파일", "▸ 고급: 직접 추가 (항목 0개)",
		"그대로 다음(Ctrl+N)"} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen lacks %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"AIPub 인증 웹훅", "대상", "항목 (Enter: 추가)", "평문으로 복사"} {
		if strings.Contains(view, hidden) {
			t.Errorf("the screen shows %q while collapsed:\n%s", hidden, view)
		}
	}
	// the help is of the focused row only: the audit log's now, not JWKS's
	if !strings.Contains(view, "audit-policy.yaml 은 사이트가 둡니다") || strings.Contains(view, "A5.2 update-cluster") {
		t.Errorf("help not of the focused row:\n%s", view)
	}
	want := []s4bStop{{kind: s4bAudit}, {kind: s4bRuntime}, {kind: s4bJWKS}, {kind: s4bIssuer}, {kind: s4bCA},
		{kind: s4bAdvanced}, {kind: s4bNav}}
	if got := s.stops(); !reflect.DeepEqual(got, want) {
		t.Errorf("stops %v, want %v", got, want)
	}
}

// Entries the site has expand the section, so nothing it has is hidden.
func TestS4bExpandedWithEntries(t *testing.T) {
	st := s4bState()
	st.ControlPlane.Args = map[string][]state.ExtraArg{"apiserver": {{Name: "oidc-issuer-url", Value: "https://idp.example.com"}}}
	s := newS4b(st, false)
	view := s.plain()
	for _, want := range []string{"▾ 고급: 직접 추가 (항목 1개)", "oidc-issuer-url=https://idp.example.com",
		"[ ] AIPub 인증 웹훅 (레거시)", "평문으로 복사됩니다. 비밀(키·토큰)은 넣지 마세요"} {
		if !strings.Contains(view, want) {
			t.Errorf("the screen lacks %q:\n%s", want, view)
		}
	}
	// collapsed again with ←, its count stays
	s.focus = s4bStop{kind: s4bAdvanced}
	s.settle()
	s4bKeys(s, "left")
	if view := s.plain(); !strings.Contains(view, "▸ 고급: 직접 추가 (항목 1개)") || strings.Contains(view, "oidc-issuer-url") {
		t.Errorf("not collapsed:\n%s", view)
	}
	s4bKeys(s, "right")
	if !s.advanced {
		t.Error("→ does not expand")
	}
	// what reaches the state is what was loaded
	back := s4bState()
	s.SyncToState(back)
	if len(back.ControlPlane.Args["apiserver"]) != 1 {
		t.Errorf("entries lost: %+v", back.ControlPlane)
	}
}

// JWKS off hides the issuer and the CA, and the focus goes past them.
func TestS4bJWKSOffSkipsFields(t *testing.T) {
	s := newS4b(s4bState(), false)
	s4bKeys(s, "down", "down", " ") // JWKS off
	view := s.plain()
	if !strings.Contains(view, "[ ] AIPub 인증 (JWKS)") || strings.Contains(view, "issuer URL") || strings.Contains(view, "CA 파일") {
		t.Fatalf("JWKS fields shown while off:\n%s", view)
	}
	s4bKeys(s, "down")
	if s.focus.kind != s4bAdvanced {
		t.Errorf("↓ from JWKS off lands on %v, want the advanced row", s.focus)
	}
	s4bKeys(s, "up", " ", "down") // on again: the issuer is the next stop
	if s.focus.kind != s4bIssuer || !s.issuer.Focused() {
		t.Errorf("↓ from JWKS on lands on %v", s.focus)
	}
	// switched off with the focus on it, the focus stays on the row
	s4bKeys(s, "up", " ")
	if s.focus.kind != s4bJWKS {
		t.Errorf("focus %v after JWKS off", s.focus)
	}
	back := s4bState()
	s.SyncToState(back)
	if back.ControlPlane.JWKS {
		t.Error("JWKS still on in the state")
	}
}

// The legacy webhook: in the advanced section on a new site, up front on an
// existing one or when it is on.
func TestS4bWebhookPlace(t *testing.T) {
	s := newS4b(s4bState(), false)
	s4bKeys(s, "down", "down", "down", "down", "down") // ▸ 고급
	if s.focus.kind != s4bAdvanced {
		t.Fatalf("focus %v, want the advanced row", s.focus)
	}
	s4bKeys(s, "enter", "down", " ") // expanded, the webhook on
	if s.focus.kind != s4bWebhook || !strings.Contains(s.plain(), "[x] AIPub 인증 웹훅 (레거시)") {
		t.Fatalf("webhook not reached in the advanced section (focus %v):\n%s", s.focus, s.plain())
	}
	s4bKeys(s, "down", "https://aipub.example.com/token-review") // its URL, shown while on
	if !strings.Contains(s.plain(), "평문 복사 — 비밀 금지") {
		t.Errorf("no secrets warning on the URL:\n%s", s.plain())
	}
	back := s4bState()
	s.SyncToState(back)
	if !back.ControlPlane.Webhook || back.ControlPlane.WebhookURL != "https://aipub.example.com/token-review" {
		t.Errorf("state %+v", back.ControlPlane)
	}

	// entered again with it on, it is up front
	s.SyncFromState(back)
	if s.advanced || !strings.Contains(s.plain(), "[x] AIPub 인증 웹훅 (레거시)") {
		t.Errorf("the webhook on is not shown with the presets:\n%s", s.plain())
	}

	// an existing site lists it with the presets, off
	e := newS4b(s4bState(), true)
	if view := e.plain(); !strings.Contains(view, "[ ] AIPub 인증 웹훅 (레거시)") || !strings.Contains(view, "c 로 첫 컨트롤 플레인 노드") {
		t.Errorf("existing site:\n%s", view)
	}
}

var s4bNext = regexp.MustCompile(`▶\s*< 다음 >`)

// ↓ from the last stop lands on 다음, collapsed or expanded with entries.
func TestS4bDownReachesNext(t *testing.T) {
	st := s4bState()
	for _, entries := range []bool{false, true} {
		if entries {
			st.ControlPlane.Files = []state.CpExtraFile{{Src: "a", Dest: "/etc/a", Mode: "0644"}, {Src: "b", Dest: "/etc/b", Mode: "0644"}}
		}
		s := newS4b(st, false)
		for i := 0; i < 30; i++ {
			s4bKeys(s, "down")
		}
		if s.focus.kind != s4bNav || s.navIdx != 1 || !s4bNext.MatchString(s.plain()) {
			t.Errorf("entries %v: ↓ to the end is on %v, nav %d:\n%s", entries, s.focus, s.navIdx, s.plain())
		}
		s4bKeys(s, "up")
		want := s4bStop{kind: s4bAdvanced}
		if entries {
			want = s4bStop{kind: s4bList, i: 1}
		}
		if s.focus != want {
			t.Errorf("entries %v: ↑ from 다음 is on %v, want %v", entries, s.focus, want)
		}
	}
}

// Letters typed into the issuer stay text, c and d among them.
func TestS4bTypeIntoIssuer(t *testing.T) {
	s := newS4b(s4bState(), true) // c would read the node off the inputs
	s4bKeys(s, "down", "down", "down", "ctrl+u", "https://idp.cd hjkl")
	if got := s.issuer.Value(); got != "https://idp.cd hjkl" {
		t.Errorf("issuer = %q", got)
	}
	if s.checking {
		t.Error("c typed into the issuer ran the check")
	}
	s4bKeys(s, "ctrl+u", "https://login.example.com")
	back := s4bState()
	s.SyncToState(back)
	if back.ControlPlane.JWKSIssuerURL != "https://login.example.com" {
		t.Errorf("issuer in the state: %q", back.ControlPlane.JWKSIssuerURL)
	}
}

// d takes the focused entry out; the focus stays in the list, then on the input.
func TestS4bDeleteKeepsFocus(t *testing.T) {
	st := s4bState()
	st.ControlPlane.Files = []state.CpExtraFile{{Src: "a", Dest: "/etc/a", Mode: "0644"}, {Src: "b", Dest: "/etc/b", Mode: "0644"}}
	s := newS4b(st, false)
	s.focus = s4bStop{kind: s4bList, i: 1}
	s.settle()
	s4bKeys(s, "d")
	if s.focus != (s4bStop{kind: s4bList, i: 0}) || strings.Contains(s.plain(), "b → /etc/b") {
		t.Fatalf("after deleting the last entry: focus %v\n%s", s.focus, s.plain())
	}
	s4bKeys(s, "d")
	if s.focus.kind != s4bEntry || !s.entry.Focused() || !strings.Contains(s.plain(), "(직접 넣은 항목 없음)") {
		t.Errorf("after deleting every entry: focus %v\n%s", s.focus, s.plain())
	}
	s4bKeys(s, "d") // typed into the input now
	if s.entry.Value() != "d" {
		t.Errorf("d on the input: %q", s.entry.Value())
	}
}

// A screen lower than the view keeps the focused row in sight.
func TestS4bFitsHeight(t *testing.T) {
	st := s4bState()
	st.ControlPlane.Files = []state.CpExtraFile{{Src: "a", Dest: "/etc/a", Mode: "0644"}}
	s := newS4b(st, false)
	s.SetSize(98, 12)
	if n := strings.Count(s.plain(), "\n") + 1; n > 12 {
		t.Errorf("%d lines on a screen of 12", n)
	}
	for i := 0; i < 30; i++ {
		s4bKeys(s, "down")
	}
	view := s.plain()
	if n := strings.Count(view, "\n") + 1; n > 12 || !s4bNext.MatchString(view) || !strings.Contains(view, "위에 더 있음") {
		t.Errorf("다음 not in sight on a screen of 12:\n%s", view)
	}
	s4bKeys(s, "up", "up", "up", "up", "up", "up") // up to the CA field
	if view := s.plain(); !strings.Contains(view, "▶   CA 파일") {
		t.Errorf("the CA field not in sight:\n%s", view)
	}
}

// s4bLow is a new site with several entries, 고급 expanded, on a low screen.
func s4bLow(h int) *S4bControlPlane {
	st := s4bState()
	for _, f := range []string{"a", "b", "c", "d"} {
		st.ControlPlane.Files = append(st.ControlPlane.Files, state.CpExtraFile{Src: f, Dest: "/etc/" + f, Mode: "0644"})
	}
	s := newS4b(st, false)
	s.SetSize(98, h)
	return s
}

func (s *S4bControlPlane) focusOn(x s4bStop) string {
	s.focus = x
	s.settle()
	return s.plain()
}

func lineCount(view string) int { return strings.Count(view, "\n") + 1 }

// On a low screen the whole help of the focused row is in sight, its last
// warnings with it; help taller than the screen keeps the row at the top.
func TestS4bFitKeepsHelp(t *testing.T) {
	s := s4bLow(12)
	for _, c := range []struct {
		focus     s4bStop
		row, last string
	}{
		{s4bStop{kind: s4bWebhook}, "AIPub 인증 웹훅 (레거시)", "A2 setup-cert 다음 A2.5 update-cluster 로 적용합니다"},
		{s4bStop{kind: s4bCA}, "▶   CA 파일", "'소스 파일 없음'으로 거부합니다"},
	} {
		view := s.focusOn(c.focus)
		if lineCount(view) > 12 || !strings.Contains(view, c.row) || !strings.Contains(view, c.last) {
			t.Errorf("%v: the row or the end of its help not in sight (%d lines):\n%s", c.focus, lineCount(view), view)
		}
	}

	// 5 lines: the row, then as much of its help as fits
	s = s4bLow(5)
	view := s.focusOn(s4bStop{kind: s4bWebhook})
	lines := strings.Split(view, "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], "위에 더 있음") || !strings.Contains(lines[1], "AIPub 인증 웹훅 (레거시)") ||
		!strings.Contains(lines[4], "아래에 더 있음") {
		t.Errorf("help taller than the screen:\n%s", view)
	}
}

// A screen of one or two lines has no room for the marks: the focused row stays.
func TestS4bFitTinyHeight(t *testing.T) {
	for _, h := range []int{1, 2} {
		s := s4bLow(h)
		for _, c := range []struct {
			focus s4bStop
			row   string
		}{
			{s4bStop{kind: s4bAudit}, "AIPub 감사 로그"},
			{s4bStop{kind: s4bWebhook}, "AIPub 인증 웹훅 (레거시)"},
			{s4bStop{kind: s4bList, i: 2}, "c → /etc/c"},
			{s4bStop{kind: s4bNav}, "< 다음 >"},
		} {
			view := s.focusOn(c.focus)
			if lineCount(view) > h || !strings.Contains(view, c.row) || strings.Contains(view, "더 있음") {
				t.Errorf("h=%d %v:\n%s", h, c.focus, view)
			}
		}
	}
}

// Text left in the 항목 input with 고급 collapsed: the check that names it
// expands the section, so the input is in sight.
func TestS4bValidateShowsEntryInput(t *testing.T) {
	s := newS4b(s4bState(), false)
	s.focusOn(s4bStop{kind: s4bAdvanced})
	s4bKeys(s, "enter", "down", "down", "down") // expanded; webhook, 대상, 항목
	if s.focus.kind != s4bEntry {
		t.Fatalf("focus %v, want the input", s.focus)
	}
	s4bKeys(s, "zzq", "up", "up", "up", "enter") // typed, then 고급 collapsed
	if s.advanced || strings.Contains(s.plain(), "zzq") {
		t.Fatalf("not collapsed:\n%s", s.plain())
	}
	msgs := s.Validate()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "추가할 항목에 입력한 것이 있습니다") {
		t.Errorf("Validate: %v", msgs)
	}
	if view := s.plain(); !s.advanced || !strings.Contains(view, "항목 (Enter: 추가)") || !strings.Contains(view, "zzq") {
		t.Errorf("the input named is not in sight:\n%s", view)
	}
}

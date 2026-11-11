package screens

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/plan"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
)

// A run waiting for the checks holds the screen and says so; Esc drops the run
// and leaves the checks going, and the screen lets go once they end without
// starting anything.
func TestS9EscDropsTheRunWaitingForTheChecks(t *testing.T) {
	st := state.DefaultState()
	s := NewS9Install(st, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions())
	s.SyncFromState(st)
	s.checking, s.runAfterChecks = true, true
	if !s.Busy() || !strings.Contains(s.BusyReason(), "Esc") || !strings.Contains(strings.Join(s.KeyHints(), " "), "Esc 실행 취소") {
		t.Fatalf("busy %v, reason %q, hints %v", s.Busy(), s.BusyReason(), s.KeyHints())
	}
	s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if s.runAfterChecks || !s.checking || !s.Busy() || !strings.Contains(s.BusyReason(), "사전 확인 중") {
		t.Fatalf("after Esc: runAfterChecks %v checking %v busy %v reason %q", s.runAfterChecks, s.checking, s.Busy(), s.BusyReason())
	}
	_, cmd := s.Update(checkDoneMsg{results: map[string]plan.CheckResult{"P2": {OK: true}}})
	if cmd != nil || s.Busy() || s.running() {
		t.Errorf("the checks ended: cmd %v busy %v running %v", cmd != nil, s.Busy(), s.running())
	}
}

// The archive path typed on the screen goes to the plan absolute: K4 runs in
// <root>/ansible, not where the program was started.
func TestS9ArchivePathGoesToThePlanAbsolute(t *testing.T) {
	st := state.DefaultState()
	s := NewS9Install(st, fileio.DefaultPaths(t.TempDir()), plan.DefaultOptions())
	s.SyncFromState(st)
	s.archive.SetValue(" ./" + plan.ArchiveName + " ")
	s.rebuild()
	if !filepath.IsAbs(s.opts.ArchivePath) || filepath.Base(s.opts.ArchivePath) != plan.ArchiveName {
		t.Errorf("archive path %q", s.opts.ArchivePath)
	}
}

// 4b holds the wizard while c reads the apiserver of an existing site: what it
// read comes to this screen only.
func TestS4bHoldsTheWizardWhileChecking(t *testing.T) {
	s := newS4b(s4bState(), true)
	if s.Busy() {
		t.Fatal("busy before c")
	}
	s.checking = true
	if !s.Busy() || !strings.Contains(s.BusyReason(), "확인하는 중") {
		t.Fatalf("busy %v reason %q while checking", s.Busy(), s.BusyReason())
	}
	s.Update(legacyDoneMsg{err: errors.New("unreachable")})
	if s.Busy() {
		t.Error("busy after the check ended")
	}
}

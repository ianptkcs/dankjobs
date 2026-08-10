package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func newSelectionTestModel() appModel {
	m := appModel{
		recurringTable: newJobTable(),
		pendingTable:   newJobTable(),
		historyTable:   newJobTable(),
		selected:       map[panelFocus]map[string]bool{},
	}
	// The app's layout() does this before SetRows — mirror it so bubbles
	// doesn't panic indexing an empty column list while rendering.
	for _, t := range []*table.Model{&m.recurringTable, &m.pendingTable, &m.historyTable} {
		t.SetColumns([]table.Column{
			{Title: "job", Width: 10},
			{Title: "when", Width: 11},
			{Title: "status", Width: 8},
		})
		t.SetWidth(40)
	}
	return m
}

func TestToggleSelection(t *testing.T) {
	m := newSelectionTestModel()
	m.pendingJobs = []Job{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.pendingTable.SetRows([]table.Row{{"a", "", ""}, {"b", "", ""}, {"c", "", ""}})
	m.selectedSide = focusPending
	m.pendingTable.SetCursor(1)

	m.toggleSelection()
	if !m.selected[focusPending]["b"] {
		t.Fatal("expected the job under the cursor to be marked after toggle")
	}
	if m.selected[focusPending]["a"] {
		t.Fatal("a job not under the cursor must not be marked")
	}

	m.toggleSelection()
	if m.selected[focusPending]["b"] {
		t.Fatal("expected the job to be unmarked after a second toggle")
	}
}

func TestToggleSelectionEmptyPanel(t *testing.T) {
	m := newSelectionTestModel()
	m.selectedSide = focusPending
	m.toggleSelection() // must not panic, must not create a mark
	if len(m.selected[focusPending]) != 0 {
		t.Fatal("toggling on an empty panel must not mark anything")
	}
}

func TestSelectedJobsOrderAndScope(t *testing.T) {
	m := newSelectionTestModel()
	m.recurringJobs = []Job{{Name: "r1"}, {Name: "r2"}}
	m.pendingJobs = []Job{{Name: "p1"}, {Name: "p2"}, {Name: "p3"}}
	m.historyJobs = []Job{{Name: "h1"}, {Name: "h2"}}

	m.selected[focusRecurring] = map[string]bool{"r2": true}
	m.selected[focusPending] = map[string]bool{"p3": true, "p1": true}

	got := m.selectedJobs(focusPending)
	if len(got) != 2 || got[0].Name != "p1" || got[1].Name != "p3" {
		t.Fatalf("selectedJobs(pending) = %v, want [p1 p3] in display order", got)
	}
	// Marks made in another panel must not leak into this one's results.
	if got := m.selectedJobs(focusHistory); got != nil {
		t.Fatalf("selectedJobs(history) = %v, want nil (no marks there)", got)
	}
	if got := m.selectedJobs(focusRecurring); len(got) != 1 || got[0].Name != "r2" {
		t.Fatalf("selectedJobs(recurring) = %v, want [r2]", got)
	}
}

func TestPruneSelection(t *testing.T) {
	m := newSelectionTestModel()
	m.pendingJobs = []Job{{Name: "a"}, {Name: "b"}}
	m.selected[focusPending] = map[string]bool{"a": true, "b": true, "gone": true}

	m.pruneSelection()

	if !m.selected[focusPending]["a"] || !m.selected[focusPending]["b"] {
		t.Fatal("marks for jobs that still exist should survive pruning")
	}
	if m.selected[focusPending]["gone"] {
		t.Fatal("a mark for a job that no longer exists should be pruned")
	}
}

func TestNameCellMarker(t *testing.T) {
	m := newSelectionTestModel()
	m.selected[focusPending] = map[string]bool{"a": true}
	if got := m.nameCell(Job{Name: "a"}, focusPending); got != "*a" {
		t.Fatalf("marked nameCell = %q, want *a", got)
	}
	if got := m.nameCell(Job{Name: "b"}, focusPending); got != "b" {
		t.Fatalf("unmarked nameCell = %q, want b", got)
	}
}

func TestPanelTitleCount(t *testing.T) {
	m := newSelectionTestModel()
	if got := m.panelTitle("history", focusHistory); got != "history" {
		t.Fatalf("title without marks = %q, want history", got)
	}
	m.selected[focusHistory] = map[string]bool{"x": true, "y": true}
	if got := m.panelTitle("history", focusHistory); got != "history (2)" {
		t.Fatalf("title with 2 marks = %q, want history (2)", got)
	}
}

func TestDeleteSubject(t *testing.T) {
	if got := deleteSubject([]Job{{Name: "x"}}); got != "'x'" {
		t.Fatalf("single-job subject = %q, want 'x'", got)
	}
	if got := deleteSubject([]Job{{Name: "x"}, {Name: "y"}}); got != "2 jobs" {
		t.Fatalf("bulk subject = %q, want 2 jobs", got)
	}
}

func TestStartBulkDelete(t *testing.T) {
	m := newSelectionTestModel()
	m.pendingJobs = []Job{{Name: "a"}, {Name: "b"}}
	m.selectedSide = focusPending

	res, _ := m.startBulkDelete(deleteChoiceArchive)
	got, ok := res.(appModel)
	if !ok {
		t.Fatalf("expected an appModel back, got %T", res)
	}
	if got.mode != modeDelete {
		t.Fatalf("mode = %v, want modeDelete", got.mode)
	}
	if len(got.deleteJobs) != 2 {
		t.Fatalf("deleteJobs has %d entries, want 2 (every job in the panel)", len(got.deleteJobs))
	}
	if got.deleteForm == nil {
		t.Fatal("expected a confirmation form to be open")
	}
}

func TestStartBulkDeleteEmptyPanel(t *testing.T) {
	m := newSelectionTestModel()
	m.selectedSide = focusPending

	res, _ := m.startBulkDelete(deleteChoiceForever)
	got, _ := res.(appModel)
	if got.mode != modeList {
		t.Fatalf("mode = %v, want modeList (no-op on an empty panel)", got.mode)
	}
	if got.message == "" {
		t.Fatal("expected an explanatory message on an empty panel")
	}
}

func TestAllJobs(t *testing.T) {
	m := newSelectionTestModel()
	m.recurringJobs = []Job{{Name: "r1"}, {Name: "r2"}}
	m.pendingJobs = []Job{{Name: "p1"}}
	m.historyJobs = []Job{{Name: "h1"}}
	m.archivedJobs = []Job{{Name: "ar1"}}

	if got := m.allJobs(focusRecurring); len(got) != 2 {
		t.Fatalf("allJobs(recurring) has %d entries, want 2", len(got))
	}
	if got := m.allJobs(focusPending); len(got) != 1 || got[0].Name != "p1" {
		t.Fatalf("allJobs(pending) = %v, want [p1]", got)
	}
	m.historyMode = historyModeArchived
	if got := m.allJobs(focusHistory); len(got) != 1 || got[0].Name != "ar1" {
		t.Fatalf("allJobs(history, archived view) = %v, want [ar1]", got)
	}
}

func TestSGRHelpers(t *testing.T) {
	if got, want := sgrBg("#1e2e1e"), "\x1b[48;2;30;46;30m"; got != want {
		t.Fatalf("sgrBg = %q, want %q", got, want)
	}
	if got, want := sgrFg("#0000ff"), "\x1b[38;2;0;0;255m"; got != want {
		t.Fatalf("sgrFg = %q, want %q", got, want)
	}
	if got := sgrBg("red"); got != "" {
		t.Fatalf("sgrBg(named color) = %q, want empty fallback", got)
	}
	if r, g, b, ok := hexRGB("#a1b2c3"); !ok || r != 0xa1 || g != 0xb2 || b != 0xc3 {
		t.Fatalf("hexRGB(#a1b2c3) = %d,%d,%d,%v", r, g, b, ok)
	}
}

func TestRenderMarkedRow(t *testing.T) {
	markSGR := "\x1b[1m\x1b[48;2;1;2;3m"
	row := []rune("xABBByyyyyyyy")
	offset, width := 1, 4

	out := renderMarkedRow(row, offset, width, statusActive, true, markSGR)
	if !strings.HasPrefix(out, markSGR) {
		t.Fatalf("marked row should start with %q, got %q", markSGR, out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Fatal("marked row should end with a reset")
	}
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Fatal("colorStatus row should carry the status foreground SGR")
	}

	plain := renderMarkedRow(row, offset, width, statusActive, false, markSGR)
	if strings.Contains(plain, "\x1b[38;2;") {
		t.Fatal("colorStatus=false row must not carry a foreground SGR")
	}
	if !strings.HasPrefix(plain, markSGR) || !strings.HasSuffix(plain, "\x1b[0m") {
		t.Fatalf("plain marked row = %q, want markSGR prefix and reset suffix", plain)
	}
}

// TestDecorateTableHighlight renders a real bubbles table through the
// app's own layout() (same geometry production uses) and checks that
// decorateTable paints the marked row's whole background but leaves
// unmarked rows alone — the end-to-end shape the View() path relies on.
func TestDecorateTableHighlight(t *testing.T) {
	m := newSelectionTestModel()
	m.width, m.height = 120, 40
	m.pendingJobs = []Job{
		{Name: "alpha", TimerPath: "/tmp/a.timer", OnCalendar: "2099-01-01 09:00:00"},
		{Name: "beta", TimerPath: "/tmp/b.timer", OnCalendar: "2099-01-01 10:00:00"},
	}
	m.selected[focusPending] = map[string]bool{"beta": true}

	m.layout()
	rows := make([]table.Row, len(m.pendingJobs))
	for i, j := range m.pendingJobs {
		_, label := j.Status()
		rows[i] = table.Row{m.nameCell(j, focusPending), j.ScheduleHuman(), label}
	}
	m.pendingTable.SetRows(rows)
	m.pendingTable.SetCursor(0)

	out := decorateTable(m.pendingTable.View(), m.pendingJobs, m.pendingTable.Cursor(), m.pendingStatusOffset, m.statusCellWidth, scheduleColWidth, m.selected[focusPending], true)

	highlight := sgrBg(string(colSurface1))
	lines := strings.Split(out, "\n")
	// line 0 is the header; alpha (cursor) must be untouched; beta is the
	// one marked and must carry the highlight background.
	if strings.Contains(lines[1], highlight) {
		t.Fatal("cursor row must not be re-highlighted (bubbles already styles it)")
	}
	if !strings.Contains(lines[2], highlight) {
		t.Fatalf("marked row should carry the highlight background, got: %q", lines[2])
	}
}

// TestDecorateTableStatusColoring is a regression test for the name-cell
// matching: bubbles pads each cell with a leading space, and the pending
// panel's middle column is wider than the history one — both used to break
// the match so an unmarked row's status never got its color. The test runs
// with an explicit truecolor profile because lipgloss refuses to emit ANSI
// when it can't detect a color terminal.
func TestDecorateTableStatusColoring(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := newSelectionTestModel()
	m.width, m.height = 120, 40
	m.pendingJobs = []Job{
		{Name: "alpha", TimerPath: "/tmp/a.timer", OnCalendar: "2099-01-01 09:00:00"},
		{Name: "beta", TimerPath: "/tmp/b.timer", OnCalendar: "2099-01-01 10:00:00"},
		{Name: "gamma", TimerPath: "/tmp/g.timer", OnCalendar: "2099-01-01 11:00:00"},
	}
	m.layout()
	rows := make([]table.Row, len(m.pendingJobs))
	for i, j := range m.pendingJobs {
		_, label := j.Status()
		rows[i] = table.Row{j.Name, j.ScheduleHuman(), label}
	}
	m.pendingTable.SetRows(rows)
	m.pendingTable.SetCursor(2) // gamma is the cursor row

	out := decorateTable(m.pendingTable.View(), m.pendingJobs, m.pendingTable.Cursor(), m.pendingStatusOffset, m.statusCellWidth, scheduleColWidth, nil, true)

	lines := strings.Split(out, "\n")
	// Bold + foreground colors can prefix the 38;2 segment, so match on
	// just the truecolor foreground marker. (The cursor row carries its own
	// bubbles Selected color, so it's not asserted here.)
	if !strings.Contains(lines[1], "38;2;") {
		t.Fatalf("unmarked non-cursor row's status should carry its status color, got: %q", lines[1])
	}
}

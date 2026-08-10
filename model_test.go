package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestLayoutRowCap checks that layout() never hands a side panel more than
// maxVisibleRows of table height, even on a very tall terminal with far
// more jobs than that — the whole point of the cap (see the appModel row
// cap consts) is to keep panels compact regardless of terminal size.
func TestLayoutRowCap(t *testing.T) {
	m := appModel{
		recurringTable: newJobTable(),
		pendingTable:   newJobTable(),
		historyTable:   newJobTable(),
		width:          200,
		height:         500,
	}
	for i := 0; i < 50; i++ {
		m.pendingJobs = append(m.pendingJobs, Job{Name: fmt.Sprintf("job-%d", i), TimerPath: "x"})
	}
	m.layout()

	// setTableVisibleRows passes rows+1 to SetHeight to compensate for the
	// table's own header row, so Height() is expected to be maxVisibleRows+1.
	if got, want := m.pendingTable.Height(), maxVisibleRows+1; got > want {
		t.Fatalf("pendingTable.Height() = %d, want <= %d even with 50 jobs on a tall terminal", got, want)
	}
}

// TestOpenFocusPending checks that a fresh model starts focused on the
// pending panel — where a scheduled one-shot job the user most likely wants
// to act on lives.
func TestOpenFocusPending(t *testing.T) {
	m := newModel()
	if m.focus != focusPending || m.selectedSide != focusPending {
		t.Fatalf("expected focus/selectedSide = focusPending on open, got focus=%v selectedSide=%v", m.focus, m.selectedSide)
	}
}

// TestNavDispatch checks that the single "nav" action resolves direction by
// key position in the binding: [0]=left, [1]=right, [2]=down, [3]=up.
func TestNavDispatch(t *testing.T) {
	m := newModel()
	m.selectedSide = focusPending
	update := func(k tea.KeyType) {
		m2, _ := m.Update(tea.KeyMsg{Type: k})
		m = m2.(appModel)
	}

	// ctrl+h (position 0) from pending moves left to recurring.
	update(tea.KeyCtrlH)
	if m.focus != focusRecurring || m.selectedSide != focusRecurring {
		t.Fatalf("ctrl+h should move to recurring, got focus=%v selectedSide=%v", m.focus, m.selectedSide)
	}

	// ctrl+l (position 1) from recurring moves right to pending.
	update(tea.KeyCtrlL)
	if m.focus != focusPending || m.selectedSide != focusPending {
		t.Fatalf("ctrl+l should move to pending, got focus=%v selectedSide=%v", m.focus, m.selectedSide)
	}

	// ctrl+j (position 2) moves down into details.
	update(tea.KeyCtrlJ)
	if m.focus != focusDetail {
		t.Fatalf("ctrl+j should move into details, got focus=%v", m.focus)
	}

	// ctrl+k (position 3) moves back up to the selected side.
	update(tea.KeyCtrlK)
	if m.focus != focusPending || m.selectedSide != focusPending {
		t.Fatalf("ctrl+k should return to pending, got focus=%v selectedSide=%v", m.focus, m.selectedSide)
	}
}

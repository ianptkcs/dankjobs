package main

import (
	"fmt"
	"testing"
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

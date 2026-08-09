package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

type mode int

const (
	modeList mode = iota
	modeEdit
	modeDelete
	modeCreate
)

// reloadMsg triggers a re-scan of ~/jobs a moment after a "run now" was
// issued, so a one-shot job that finished (and self-removed its units) moves
// from pending to history without a manual 'r'.
type reloadMsg struct{}

// panelFocus selects which of the four panels (recurring, pending, history,
// or details) currently receives key input — neovim-style pane navigation:
// Ctrl+h/l move focus one panel left/right among the three side-by-side
// panels (no-op at either edge, same as vim-tmux-navigator), Ctrl+j/k move
// down into details and back up. focusDetail is a dead end for
// currentJob(): selectedSide (below) keeps tracking whichever side panel
// was last active, so the detail view keeps showing that job while focus is
// parked on details.
type panelFocus int

const (
	focusRecurring panelFocus = iota
	focusPending
	focusHistory
	focusDetail
)

// historyMode toggles what the history panel displays: resolved jobs
// (normal) or archived ones ('A' toggles between the two).
type historyMode int

const (
	historyModeNormal historyMode = iota
	historyModeArchived
)

// Column widths (content width, before bubbles/table's default Padding(0,1)
// adds 2 on each side). "name" flexes to fill whatever's left of the panel.
const (
	scheduleColWidth = 13 // "scheduled for" header, pending panel
	statusColWidth   = 8  // "removed" is the longest status label
	whenColWidth     = 11 // "27/07 14:32"
	minNameColWidth  = 12
)

// Vertical overhead around each panel's flexible content, in lines: 2
// border + 1 title + 1 blank line, plus the table's own header row. Layout
// math in (*appModel).layout keeps total rendered height within the
// terminal's actual height — content that doesn't fit gets clipped on
// purpose (missing a line beats the terminal silently scrolling content off
// the top).
const (
	headerLines       = 1
	footerLines       = 1
	tableBoxOverhead  = 2 + 1 + 1 + 1
	detailBoxOverhead = 2 + 1 + 1
	minVisibleRows    = 2
	minDetailLines    = 3
	// Share of body height given to the recurring/pending/history row (all
	// three panels sit side by side in that row, same height).
	jobsRowPercent = 45
	// Hard ceiling on visible rows per side panel, regardless of terminal
	// height — keeps the dashboard compact instead of growing to fill a
	// tall terminal. Whichever of this and the dynamic jobsRowPercent split
	// is smaller wins (see layout()).
	maxVisibleRows = 8
	// recurring:pending:history WIDTH ratio, side by side in that row.
	recurringWidthShare = 1
	pendingWidthShare   = 1
	historyWidthShare   = 1
	panelGap            = 1
	minPanelInnerWidth  = 20
)

type appModel struct {
	recurringJobs []Job
	pendingJobs   []Job
	historyJobs   []Job
	archivedJobs  []Job
	// historyMode selects whether the history panel currently shows
	// historyJobs or archivedJobs — toggled by 'A'.
	historyMode historyMode

	recurringTable table.Model
	pendingTable   table.Model
	historyTable   table.Model
	focus          panelFocus
	// selectedSide is which side panel currentJob() reads from — always
	// focusRecurring, focusPending, or focusHistory, never focusDetail. It
	// only changes on Ctrl+h/l, so it survives a Ctrl+j trip into details.
	selectedSide panelFocus
	// detailScroll is the first visible line of the current job's detail
	// text, adjusted by j/k (or the arrow keys) while focus is on details.
	detailScroll int

	mode       mode
	width      int
	height     int
	innerWidth int // content width for header/detail/footer (full width)
	// Content width of each side-by-side panel — equal 1:1:1 share of the
	// row across recurring:pending:history.
	recurringInnerWidth int
	pendingInnerWidth   int
	historyInnerWidth   int
	detailMaxLines      int
	message             string

	// Rune offset/width of the status column within a rendered table line,
	// used by colorizeStatusColumn — set by layout() alongside the column
	// widths they're derived from.
	recurringStatusOffset int
	pendingStatusOffset   int
	historyStatusOffset   int
	statusCellWidth       int

	editJob  *Job
	editForm *huh.Form

	deleteJob  *Job
	deleteForm *huh.Form
	// deleteFromArchive records which variant of newDeleteForm is showing,
	// so updateDelete knows an "Archive" choice isn't even on offer.
	deleteFromArchive bool

	createForm *huh.Form
}

func newJobTable() table.Model {
	t := table.New(table.WithFocused(true))

	styles := table.DefaultStyles()
	// Background is set explicitly (matching the panel's own background)
	// because Header's own reset code would otherwise blank it out to the
	// terminal default for every char it touches — same root cause as the
	// Cell/Selected note below.
	styles.Header = styles.Header.Foreground(colText).Background(colBase).Bold(true).BorderForeground(colSurface1)
	styles.Selected = styles.Selected.Foreground(colBase).Background(colPink).Bold(true)
	// Cell intentionally has no Foreground: bubbles/table renders each cell
	// independently (with its own reset code) before joining the row, then
	// wraps the whole joined row in Selected for the cursor line. A colored
	// Cell style embeds a reset partway through that row, which cuts the
	// Selected background off after that point. Leaving Cell plain (as
	// bubbles' own DefaultStyles does) avoids that — per-status color is
	// baked into just the status cell's own value instead (see statusCell).
	t.SetStyles(styles)
	return t
}

func newModel() appModel {
	m := appModel{
		recurringTable: newJobTable(),
		pendingTable:   newJobTable(),
		historyTable:   newJobTable(),
		width:          100,
		height:         30,
	}
	m.reloadJobs()
	return m
}

func (m appModel) Init() tea.Cmd {
	return nil
}

// activeHistoryJobs is whichever list currently backs the history panel —
// resolved jobs normally, or archived ones while historyMode is toggled.
func (m *appModel) activeHistoryJobs() []Job {
	if m.historyMode == historyModeArchived {
		return m.archivedJobs
	}
	return m.historyJobs
}

// focusedTable/focusedJobs dispatch on selectedSide (not focus — focus can
// also be focusDetail, which has no table of its own) so most code can work
// generically instead of switching on the side everywhere.
func (m *appModel) focusedTable() *table.Model {
	switch m.selectedSide {
	case focusRecurring:
		return &m.recurringTable
	case focusHistory:
		return &m.historyTable
	default:
		return &m.pendingTable
	}
}

func (m *appModel) focusedJobs() []Job {
	switch m.selectedSide {
	case focusRecurring:
		return m.recurringJobs
	case focusHistory:
		return m.activeHistoryJobs()
	default:
		return m.pendingJobs
	}
}

func (m *appModel) currentJob() *Job {
	jobs := m.focusedJobs()
	if len(jobs) == 0 {
		return nil
	}
	idx := m.focusedTable().Cursor()
	if idx < 0 || idx >= len(jobs) {
		return nil
	}
	return &jobs[idx]
}

// reloadJobs re-scans ~/jobs, splits jobs into "recurring" (has a timer and
// repeats), "pending" (one-shot, still on an active or paused timer), and
// "history" (resolved — completed, failed, or removed before ever running),
// plus a separate archived list from ~/jobs/.archive, and rebuilds all
// three tables, keeping each one's cursor on its previously selected job by
// name instead of resetting to row 0.
func (m *appModel) reloadJobs() {
	m.detailScroll = 0
	prevRecurring := jobName(m.recurringJobs, m.recurringTable.Cursor())
	prevPending := jobName(m.pendingJobs, m.pendingTable.Cursor())
	prevHistory := jobName(m.activeHistoryJobs(), m.historyTable.Cursor())

	all := discoverJobs()
	m.recurringJobs = nil
	m.pendingJobs = nil
	m.historyJobs = nil
	for _, j := range all {
		switch {
		// A recurring job's timer keeps firing regardless of the last run's
		// outcome, so it stays in its own panel — including while
		// "failed" — for as long as the timer unit itself exists.
		case j.IsRecurring():
			m.recurringJobs = append(m.recurringJobs, j)
		case j.IsPending():
			m.pendingJobs = append(m.pendingJobs, j)
		default:
			m.historyJobs = append(m.historyJobs, j)
		}
	}
	// Most recently touched first — that's what makes a history useful.
	sort.SliceStable(m.historyJobs, func(i, k int) bool {
		return m.historyJobs[i].historyModTime().After(m.historyJobs[k].historyModTime())
	})

	m.archivedJobs = discoverArchivedJobs()
	sort.SliceStable(m.archivedJobs, func(i, k int) bool {
		return m.archivedJobs[i].historyModTime().After(m.archivedJobs[k].historyModTime())
	})

	// Columns must exist before SetRows below — bubbles/table indexes
	// m.cols per cell while rendering, and panics if that's still empty.
	m.layout()

	recurringSelected := indexOfName(m.recurringJobs, prevRecurring)
	recurringRows := make([]table.Row, len(m.recurringJobs))
	for i, j := range m.recurringJobs {
		_, label := j.Status()
		recurringRows[i] = table.Row{j.Name, j.NextRunHuman(), label}
	}
	m.recurringTable.SetRows(recurringRows)
	m.recurringTable.SetCursor(recurringSelected)

	pendingSelected := indexOfName(m.pendingJobs, prevPending)
	pendingRows := make([]table.Row, len(m.pendingJobs))
	for i, j := range m.pendingJobs {
		_, label := j.Status()
		pendingRows[i] = table.Row{j.Name, j.ScheduleHuman(), label}
	}
	m.pendingTable.SetRows(pendingRows)
	m.pendingTable.SetCursor(pendingSelected)

	historyJobs := m.activeHistoryJobs()
	historySelected := indexOfName(historyJobs, prevHistory)
	historyRows := make([]table.Row, len(historyJobs))
	for i, j := range historyJobs {
		label := "archived"
		if m.historyMode == historyModeNormal {
			_, label = j.Status()
		}
		historyRows[i] = table.Row{j.Name, j.HistoryWhen(), label}
	}
	m.historyTable.SetRows(historyRows)
	m.historyTable.SetCursor(historySelected)
}

func jobName(jobs []Job, idx int) string {
	if idx < 0 || idx >= len(jobs) {
		return ""
	}
	return jobs[idx].Name
}

func indexOfName(jobs []Job, name string) int {
	if name == "" {
		return 0
	}
	for i, j := range jobs {
		if j.Name == name {
			return i
		}
	}
	return 0
}

// layout recomputes column widths for all three tables (so each one's
// rendered width exactly matches its panel's inner width — a mismatch there
// makes lipgloss hard-wrap rows mid-line) and the vertical space budget
// across the jobs row + details panel, so header + jobs row + details +
// footer never exceeds m.height. Recurring, pending, and history sit side
// by side in the same row, splitting its width equally.
func (m *appModel) layout() {
	m.innerWidth = m.width - 4
	if m.innerWidth < 40 {
		m.innerWidth = 40
	}

	totalShare := recurringWidthShare + pendingWidthShare + historyWidthShare
	totalRowWidth := m.width - 2*panelGap
	if minRowWidth := 3 * (minPanelInnerWidth + 4); totalRowWidth < minRowWidth {
		totalRowWidth = minRowWidth
	}
	recurringBoxWidth := totalRowWidth * recurringWidthShare / totalShare
	pendingBoxWidth := totalRowWidth * pendingWidthShare / totalShare
	historyBoxWidth := totalRowWidth - recurringBoxWidth - pendingBoxWidth

	m.recurringInnerWidth = recurringBoxWidth - 4
	if m.recurringInnerWidth < minPanelInnerWidth {
		m.recurringInnerWidth = minPanelInnerWidth
	}
	m.pendingInnerWidth = pendingBoxWidth - 4
	if m.pendingInnerWidth < minPanelInnerWidth {
		m.pendingInnerWidth = minPanelInnerWidth
	}
	m.historyInnerWidth = historyBoxWidth - 4
	if m.historyInnerWidth < minPanelInnerWidth {
		m.historyInnerWidth = minPanelInnerWidth
	}

	recurringNameWidth := m.recurringInnerWidth - 3*2 - whenColWidth - statusColWidth
	if recurringNameWidth < minNameColWidth {
		recurringNameWidth = minNameColWidth
	}
	m.recurringTable.SetColumns([]table.Column{
		{Title: "job", Width: recurringNameWidth},
		{Title: "next run", Width: whenColWidth},
		{Title: "status", Width: statusColWidth},
	})
	m.recurringTable.SetWidth(m.recurringInnerWidth)
	m.recurringStatusOffset = (recurringNameWidth + 2) + (whenColWidth + 2)

	pendingNameWidth := m.pendingInnerWidth - 3*2 - scheduleColWidth - statusColWidth
	if pendingNameWidth < minNameColWidth {
		pendingNameWidth = minNameColWidth
	}
	m.pendingTable.SetColumns([]table.Column{
		{Title: "job", Width: pendingNameWidth},
		{Title: "scheduled for", Width: scheduleColWidth},
		{Title: "status", Width: statusColWidth},
	})
	m.pendingTable.SetWidth(m.pendingInnerWidth)
	m.pendingStatusOffset = (pendingNameWidth + 2) + (scheduleColWidth + 2)

	historyNameWidth := m.historyInnerWidth - 3*2 - whenColWidth - statusColWidth
	if historyNameWidth < minNameColWidth {
		historyNameWidth = minNameColWidth
	}
	m.historyTable.SetColumns([]table.Column{
		{Title: "name", Width: historyNameWidth},
		{Title: "date", Width: whenColWidth},
		{Title: "status", Width: statusColWidth},
	})
	m.historyTable.SetWidth(m.historyInnerWidth)
	m.historyStatusOffset = (historyNameWidth + 2) + (whenColWidth + 2)
	m.statusCellWidth = statusColWidth + 2

	minBody := tableBoxOverhead + minVisibleRows + detailBoxOverhead + minDetailLines
	bodyHeight := m.height - headerLines - footerLines
	if bodyHeight < minBody {
		bodyHeight = minBody
	}

	jobsRowHeight := bodyHeight * jobsRowPercent / 100
	minJobsRow := tableBoxOverhead + minVisibleRows
	if jobsRowHeight < minJobsRow {
		jobsRowHeight = minJobsRow
	}
	// Never hand the row more height than the longest list actually needs
	// (capped at maxVisibleRows regardless) — leftover falls through to the
	// detail panel instead.
	idealRows := max(len(m.recurringJobs), len(m.pendingJobs), len(m.activeHistoryJobs()))
	if idealRows > maxVisibleRows {
		idealRows = maxVisibleRows
	}
	if idealJobsRow := max(tableBoxOverhead+idealRows, minJobsRow); jobsRowHeight > idealJobsRow {
		jobsRowHeight = idealJobsRow
	}

	detailBoxHeight := bodyHeight - jobsRowHeight
	if detailBoxHeight < detailBoxOverhead+minDetailLines {
		detailBoxHeight = detailBoxOverhead + minDetailLines
	}

	setTableVisibleRows(&m.recurringTable, jobsRowHeight-tableBoxOverhead)
	setTableVisibleRows(&m.pendingTable, jobsRowHeight-tableBoxOverhead)
	setTableVisibleRows(&m.historyTable, jobsRowHeight-tableBoxOverhead)

	m.detailMaxLines = detailBoxHeight - detailBoxOverhead
	if m.detailMaxLines < minDetailLines {
		m.detailMaxLines = minDetailLines
	}
}

func setTableVisibleRows(t *table.Model, rows int) {
	if rows < minVisibleRows {
		rows = minVisibleRows
	}
	// SetHeight subtracts the table's own header row internally, so pass
	// rows+1 to actually get `rows` data rows on screen.
	t.SetHeight(rows + 1)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sizeMsg.Width, sizeMsg.Height
		m.layout()
		return m, nil
	}

	switch m.mode {
	case modeEdit:
		return m.updateEdit(msg)
	case modeDelete:
		return m.updateDelete(msg)
	case modeCreate:
		return m.updateCreate(msg)
	default:
		return m.updateList(msg)
	}
}

func (m appModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(reloadMsg); ok {
		m.reloadJobs()
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m.forwardToFocusedTable(msg)
	}

	m.message = ""
	switch keyMsg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.reloadJobs()
		m.message = "List refreshed."
		return m, nil
	case "n":
		m.createForm = newCreateForm()
		m.mode = modeCreate
		return m, m.createForm.Init()
	// Neovim-style pane navigation: Ctrl+h/l move focus one panel left/right
	// among the three side-by-side panels, pivoting off selectedSide so it
	// still works from focusDetail; Ctrl+j/k move down into details and
	// back up. No-op when there's no pane in that direction, same as
	// vim-tmux-navigator.
	case "ctrl+h":
		switch m.selectedSide {
		case focusPending:
			m.focus, m.selectedSide = focusRecurring, focusRecurring
		case focusHistory:
			m.focus, m.selectedSide = focusPending, focusPending
		}
		return m, nil
	case "ctrl+l":
		switch m.selectedSide {
		case focusRecurring:
			m.focus, m.selectedSide = focusPending, focusPending
		case focusPending:
			m.focus, m.selectedSide = focusHistory, focusHistory
		}
		return m, nil
	case "ctrl+j":
		if m.focus != focusDetail {
			m.focus = focusDetail
		}
		return m, nil
	case "ctrl+k":
		if m.focus == focusDetail {
			m.focus = m.selectedSide
		}
		return m, nil
	case "e":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		if job.TimerPath == "" {
			m.message = fmt.Sprintf("'%s' has no timer to reschedule.", job.Name)
			return m, nil
		}
		jobCopy := *job
		m.editJob = &jobCopy
		m.editForm = newEditForm(jobCopy)
		m.mode = modeEdit
		return m, m.editForm.Init()
	case "t":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		if job.TimerPath == "" {
			m.message = fmt.Sprintf("'%s' has no timer.", job.Name)
			return m, nil
		}
		toggleJob(*job)
		wasEnabled := job.Enabled()
		m.reloadJobs()
		state := "paused"
		if !wasEnabled {
			state = "resumed"
		}
		m.message = fmt.Sprintf("'%s' %s.", job.Name, state)
		return m, nil
	case "x":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		if err := runJob(*job); err != nil {
			m.message = fmt.Sprintf("error running '%s': %v", job.Name, err)
			return m, nil
		}
		m.message = fmt.Sprintf("'%s' started.", job.Name)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return reloadMsg{} })
	case "d":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		jobCopy := *job
		archived := m.selectedSide == focusHistory && m.historyMode == historyModeArchived
		m.deleteJob = &jobCopy
		m.deleteFromArchive = archived
		m.deleteForm = newDeleteForm(jobCopy, archived)
		m.mode = modeDelete
		return m, m.deleteForm.Init()
	case "A":
		if m.historyMode == historyModeNormal {
			m.historyMode = historyModeArchived
			m.message = "Showing archived jobs."
		} else {
			m.historyMode = historyModeNormal
			m.message = "Showing history."
		}
		m.focus, m.selectedSide = focusHistory, focusHistory
		m.reloadJobs()
		return m, nil
	case "u":
		if m.historyMode != historyModeArchived {
			return m, nil
		}
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		if err := unarchiveJob(job.Name); err != nil {
			m.message = fmt.Sprintf("error unarchiving '%s': %v", job.Name, err)
			return m, nil
		}
		m.message = fmt.Sprintf("'%s' unarchived.", job.Name)
		m.reloadJobs()
		return m, nil
	}

	// Details has no table of its own — j/k (and the arrow keys) scroll its
	// text by one line instead of moving a cursor. Any other key while
	// focused there is simply ignored.
	if m.focus == focusDetail {
		switch keyMsg.String() {
		case "j", "down":
			if m.detailScroll < m.maxDetailScroll() {
				m.detailScroll++
			}
		case "k", "up":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
		}
		return m, nil
	}

	return m.forwardToFocusedTable(msg)
}

// forwardToFocusedTable forwards to whichever table selectedSide points at,
// resetting detailScroll if that moved the cursor to a different job — the
// old scroll offset otherwise makes no sense against new detail text.
func (m appModel) forwardToFocusedTable(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevName := jobName(m.focusedJobs(), m.focusedTable().Cursor())

	var cmd tea.Cmd
	switch m.selectedSide {
	case focusRecurring:
		m.recurringTable, cmd = m.recurringTable.Update(msg)
	case focusHistory:
		m.historyTable, cmd = m.historyTable.Update(msg)
	default:
		m.pendingTable, cmd = m.pendingTable.Update(msg)
	}

	if jobName(m.focusedJobs(), m.focusedTable().Cursor()) != prevName {
		m.detailScroll = 0
	}
	return m, cmd
}

func (m appModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.mode = modeList
		m.editForm = nil
		m.editJob = nil
		return m, nil
	}

	form, cmd := m.editForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.editForm = f
	}

	if m.editForm.State == huh.StateCompleted {
		dom, month, errD := parseDDMM(m.editForm.GetString("date"))
		hour, minute, errT := parseHHMM(m.editForm.GetString("time"))
		if errD == nil && errT == nil {
			if err := rescheduleJob(*m.editJob, minute, hour, dom, month, time.Now()); err == nil {
				m.message = fmt.Sprintf("'%s' rescheduled for %02d/%02d %02d:%02d.", m.editJob.Name, dom, month, hour, minute)
			}
		}
		m.mode = modeList
		m.editForm = nil
		m.editJob = nil
		m.reloadJobs()
		return m, nil
	}

	return m, cmd
}

func (m appModel) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.mode = modeList
		m.createForm = nil
		return m, nil
	}

	form, cmd := m.createForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.createForm = f
	}

	if m.createForm.State == huh.StateCompleted {
		name := m.createForm.GetString("name")
		commands := m.createForm.GetString("commands")
		notes := m.createForm.GetString("notes")
		kind, _ := m.createForm.Get("type").(RecurrenceKind)

		hour, minute, errTime := 0, 0, error(nil)
		if kind != recurManual {
			hour, minute, errTime = parseHHMM(m.createForm.GetString("time"))
		}

		sched := jobSchedule{Kind: kind, Hour: hour, Minute: minute}
		valid := errTime == nil
		switch kind {
		case recurManual:
			// Nothing to schedule — runs only when started manually.
		case recurDaily:
			// Hour/minute above is everything a daily schedule needs.
		case recurWeekly:
			weekdays, _ := m.createForm.Get("weekdays").([]time.Weekday)
			sched.Weekdays = weekdays
			valid = valid && len(weekdays) > 0
		case recurMonthly:
			dayOfMonth, errDOM := strconv.Atoi(strings.TrimSpace(m.createForm.GetString("dayOfMonth")))
			sched.DayOfMonth = dayOfMonth
			valid = valid && errDOM == nil
		default: // recurOneshot, recurCycle — both need an absolute start date
			dom, month, errDate := parseDDMM(m.createForm.GetString("date"))
			sched.DOM, sched.Month = dom, month
			valid = valid && errDate == nil
			if kind == recurCycle {
				cycle, errCycle := parseCycle(m.createForm.GetString("cycle"))
				sched.Cycle = cycle
				valid = valid && errCycle == nil
			}
		}

		m.mode = modeList
		m.createForm = nil

		if valid {
			if err := createJob(name, commands, notes, sched, time.Now()); err == nil {
				if kind == recurManual {
					m.message = fmt.Sprintf("'%s' created — run it with x.", name)
					m.focus, m.selectedSide = focusPending, focusPending
				} else {
					m.message = fmt.Sprintf("'%s' created and scheduled.", name)
					if kind == recurOneshot {
						m.focus, m.selectedSide = focusPending, focusPending
					} else {
						m.focus, m.selectedSide = focusRecurring, focusRecurring
					}
				}
			} else {
				m.message = fmt.Sprintf("error creating '%s': %v", name, err)
			}
		}
		m.reloadJobs()
		return m, nil
	}

	return m, cmd
}

func (m appModel) updateDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.mode = modeList
		m.deleteForm = nil
		m.deleteJob = nil
		m.deleteFromArchive = false
		return m, nil
	}

	form, cmd := m.deleteForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.deleteForm = f
	}

	if m.deleteForm.State == huh.StateCompleted {
		choice := m.deleteForm.GetString("choice")
		job := *m.deleteJob
		m.mode = modeList
		m.deleteForm = nil
		m.deleteJob = nil
		m.deleteFromArchive = false

		switch choice {
		case deleteChoiceArchive:
			if err := archiveJob(job); err == nil {
				m.message = fmt.Sprintf("'%s' archived.", job.Name)
			} else {
				m.message = fmt.Sprintf("error archiving '%s': %v", job.Name, err)
			}
			m.reloadJobs()
		case deleteChoiceForever:
			if err := deleteJob(job, true); err == nil {
				m.message = fmt.Sprintf("'%s' deleted forever.", job.Name)
			} else {
				m.message = fmt.Sprintf("error deleting '%s': %v", job.Name, err)
			}
			m.reloadJobs()
		}
		return m, nil
	}

	return m, cmd
}

func (m appModel) View() string {
	if m.mode == modeEdit {
		title := fmt.Sprintf("reschedule: %s", m.editJob.Name)
		return m.renderModal(title, m.editForm.View())
	}
	if m.mode == modeDelete {
		title := fmt.Sprintf("delete: %s", m.deleteJob.Name)
		return m.renderModal(title, m.deleteForm.View())
	}
	if m.mode == modeCreate {
		return m.renderModal("new job", m.createForm.View())
	}

	headerText := fmt.Sprintf("jobs — %d recurring, %d pending, %d in history — %s", len(m.recurringJobs), len(m.pendingJobs), len(m.historyJobs), jobsDir)
	if avail := m.width - 4; avail > 0 {
		headerText = strings.TrimRight(padLines(headerText, avail), " ")
	}
	header := headerStyle(m.width).Render(headerText)

	recurringView := colorizeStatusColumn(m.recurringTable.View(), m.recurringJobs, m.recurringTable.Cursor(), m.recurringStatusOffset, m.statusCellWidth)
	recurringBox := panelStyle(m.focus == focusRecurring).Render(padLines(
		titleStyle().Render("recurring")+"\n\n"+recurringView, m.recurringInnerWidth,
	))

	pendingView := colorizeStatusColumn(m.pendingTable.View(), m.pendingJobs, m.pendingTable.Cursor(), m.pendingStatusOffset, m.statusCellWidth)
	pendingBox := panelStyle(m.focus == focusPending).Render(padLines(
		titleStyle().Render("pending")+"\n\n"+pendingView, m.pendingInnerWidth,
	))

	// Archived jobs have no meaningful Status() (their timer is long gone),
	// so the archived view skips colorizeStatusColumn — the plain "archived"
	// label rendered by the table itself is all there is to show.
	historyTitle, historyView := "history", m.historyTable.View()
	if m.historyMode == historyModeArchived {
		historyTitle = "archived"
	} else {
		historyView = colorizeStatusColumn(historyView, m.historyJobs, m.historyTable.Cursor(), m.historyStatusOffset, m.statusCellWidth)
	}
	historyBox := panelStyle(m.focus == focusHistory).Render(padLines(
		titleStyle().Render(historyTitle)+"\n\n"+historyView, m.historyInnerWidth,
	))

	// Recurring, pending, and history sit side by side, equal width split.
	jobsRow := lipgloss.JoinHorizontal(lipgloss.Top, recurringBox, strings.Repeat(" ", panelGap), pendingBox, strings.Repeat(" ", panelGap), historyBox)

	detailTitle := "details"
	if total := len(m.currentDetailLines()); total > m.detailMaxLines {
		detailTitle = fmt.Sprintf("details (%d–%d/%d)", m.detailScroll+1, min(m.detailScroll+m.detailMaxLines, total), total)
	}
	detailBox := panelStyle(m.focus == focusDetail).Render(padLines(
		titleStyle().Render(detailTitle)+"\n\n"+m.renderDetailBody(), m.innerWidth,
	))

	help := "n new · e reschedule · t pause/resume · x run now · d delete/archive · A archived view · u unarchive · r refresh · ctrl+h/j/k/l navigate · j/k scroll details · q quit"
	footerText := help
	if m.message != "" {
		footerText = m.message + "   " + help
	}
	// Must be pre-truncated like headerText above: footerStyle sets Width(),
	// which word-wraps overflow instead of truncating it. An untruncated
	// wrap silently turns the footer into 2 lines, breaking layout()'s fixed
	// footerLines=1 budget and pushing everything above it (including the
	// jobs row) up past the top of the altscreen.
	if avail := m.width - 4; avail > 0 {
		footerText = strings.TrimRight(padLines(footerText, avail), " ")
	}
	footer := footerStyle(m.width).Render(footerText)

	return lipgloss.JoinVertical(lipgloss.Left, header, jobsRow, detailBox, footer)
}

// currentDetailLines is the full (unscrolled, unclipped) detail text for
// currentJob(), split into lines — shared by renderDetailBody, maxDetailScroll,
// and the title's scroll-position indicator so they never disagree about
// line count.
func (m appModel) currentDetailLines() []string {
	job := m.currentJob()
	if job == nil {
		return nil
	}
	kind, label := job.Status()
	styled := statusStyle(kind).Render(statusGlyph(kind) + " " + label)
	return strings.Split(job.DetailText(styled), "\n")
}

// maxDetailScroll is the highest detailScroll that still leaves the last
// line visible — scrolling past it would just show trailing blank space.
func (m appModel) maxDetailScroll() int {
	if n := len(m.currentDetailLines()) - m.detailMaxLines; n > 0 {
		return n
	}
	return 0
}

// renderDetailBody always returns exactly m.detailMaxLines lines (padding
// with blank ones when the job's detail text is shorter), so the details
// panel renders at a constant height regardless of how much text the
// current job has — otherwise the panel (and everything below it, i.e. the
// footer) would grow or shrink as you move between jobs.
func (m appModel) renderDetailBody() string {
	lines := m.currentDetailLines()
	if lines == nil {
		lines = []string{dimStyle().Render(fmt.Sprintf("No jobs in %s (recurring, pending, or history).", jobsDir))}
	}
	scroll := m.detailScroll
	if max := m.maxDetailScroll(); scroll > max {
		scroll = max
	}
	end := scroll + m.detailMaxLines
	if end > len(lines) {
		end = len(lines)
	}
	visible := append([]string{}, lines[scroll:end]...)
	for len(visible) < m.detailMaxLines {
		visible = append(visible, "")
	}
	return strings.Join(visible, "\n")
}

// colorizeStatusColumn re-colors the status column of an already-rendered
// table view, one line at a time. Coloring the cell *value* before it goes
// through bubbles/table's own rendering doesn't work: bubbles truncates
// each cell with runewidth.Truncate, which isn't ANSI-aware and counts
// escape-code bytes as visible width, mangling short colored strings well
// before they'd actually need truncating. Post-processing the plain
// rendered text instead sidesteps that entirely.
//
// Each line is matched back to its Job by the displayed name cell rather
// than by line position: bubbles/table renders rows from an internal
// viewport window (its unexported `start` + viewport `YOffset`) that isn't
// recoverable from the public API, so positional math drifts as soon as the
// table scrolls. Name matching is exact even for "…"-truncated names, and
// unambiguous because job names are unique. The row at `cursor` is left
// untouched — bubbles/table already wraps it in its own Selected style, and
// nesting another color in there would reset-terminate that highlight
// partway through the row.
func colorizeStatusColumn(view string, jobs []Job, cursor int, offset, width int) string {
	// The name column sits right before the when + status columns, so its
	// content width is recoverable from the status offset: name + 2 padding
	// + when + 2 padding = offset, and `when` is a fixed constant.
	nameWidth := offset - whenColWidth - 4

	type match struct {
		job  Job
		when string
	}
	byName := make(map[string][]match, len(jobs))
	for _, j := range jobs {
		displayed := runewidth.Truncate(j.Name, nameWidth, "…")
		byName[displayed] = append(byName[displayed], match{j, j.HistoryWhen()})
	}

	lines := strings.Split(view, "\n")
	for i := range lines {
		if i == 0 { // line 0 is the table's own header row
			continue
		}
		line := []rune(lines[i])
		if offset+width > len(line) {
			continue
		}
		// Strip ANSI up front so fixed-width slices line up even on the
		// selected row, which bubbles wraps in its Selected style.
		clean := []rune(ansi.Strip(lines[i]))
		if len(clean) < nameWidth+2 {
			continue
		}
		displayed := strings.TrimRight(string(clean[:nameWidth+2]), " ")
		cands := byName[displayed]
		if len(cands) == 0 {
			continue
		}
		var job *Job
		if len(cands) == 1 {
			job = &cands[0].job
		} else {
			// Truncated-name collision — disambiguate by the date column.
			when := strings.TrimRight(string(clean[nameWidth+2:nameWidth+2+whenColWidth+2]), " ")
			for k := range cands {
				if cands[k].when == when {
					job = &cands[k].job
					break
				}
			}
			if job == nil {
				continue
			}
		}
		if job.Name == jobName(jobs, cursor) {
			continue
		}
		cell := string(line[offset : offset+width])
		trimmed := strings.TrimRight(cell, " ")
		trailing := cell[len(trimmed):]
		kind, _ := job.Status()
		colored := statusStyle(kind).Render(trimmed)
		lines[i] = string(line[:offset]) + colored + trailing + string(line[offset+width:])
	}
	return strings.Join(lines, "\n")
}

func (m appModel) renderModal(title, body string) string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}
	box := modalBoxStyle().Render(titleStyle().Render(title) + "\n\n" + body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ianptkcs/tabelatuiui"
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
	// selected tracks the multi-select marks ("space") per side panel,
	// keyed by job name. Marks are scoped to the panel where they were made,
	// so 'd' never acts on a job selected in another panel; a job name is
	// unique across the whole app at any moment, so the set needs no extra
	// disambiguation. The history panel's set applies to whichever list is
	// currently displayed (history or archived).
	selected map[panelFocus]map[string]bool
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

	deleteJobs []Job
	deleteForm *huh.Form
	// deleteFromArchive records which variant of newDeleteForm is showing,
	// so updateDelete knows an "Archive" choice isn't even on offer.
	deleteFromArchive bool

	createForm *huh.Form

	// helpModal is the "?" overlay listing every keybinding; settingsModal is
	// the "," overlay that lets the user rebind them. Both read from reg, so
	// they're declared once here (not per-update) and reflect each other.
	helpModal     *tuiui.HelpModal
	settingsModal *tuiui.SettingsModal
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
	if err := reg.Load(); err != nil {
		// Corrupted/missing override file: keep pure defaults.
	}
	m := appModel{
		recurringTable: newJobTable(),
		pendingTable:   newJobTable(),
		historyTable:   newJobTable(),
		selected:       map[panelFocus]map[string]bool{},
		width:          100,
		height:         30,
		// Open on the pending panel: that's where a scheduled one-shot job
		// the user most likely wants to act on lives.
		focus:        focusPending,
		selectedSide: focusPending,
		helpModal: tuiui.NewHelpModal(tuiui.HelpSection{
			Title:      "Atalhos",
			BindingsFn: reg.Bindings,
		}),
		settingsModal: tuiui.NewSettingsModal(reg),
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

// toggleSelection marks/unmarks the job under the cursor in the focused
// panel ("space"). Marks are per-panel (see appModel.selected).
func (m *appModel) toggleSelection() {
	job := m.currentJob()
	if job == nil {
		return
	}
	set := m.selected[m.selectedSide]
	if set == nil {
		set = map[string]bool{}
		m.selected[m.selectedSide] = set
	}
	if set[job.Name] {
		delete(set, job.Name)
	} else {
		set[job.Name] = true
	}
}

// selectedJobs returns, in display order, the jobs of the given side that
// are currently marked. Empty when nothing is marked — callers fall back to
// the cursor job.
func (m *appModel) selectedJobs(side panelFocus) []Job {
	set := m.selected[side]
	if len(set) == 0 {
		return nil
	}
	var jobs []Job
	for _, j := range m.allJobs(side) {
		if set[j.Name] {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

// allJobs returns every job currently displayed in the given side panel, in
// display order.
func (m *appModel) allJobs(side panelFocus) []Job {
	switch side {
	case focusRecurring:
		return m.recurringJobs
	case focusPending:
		return m.pendingJobs
	default:
		return m.activeHistoryJobs()
	}
}

// startBulkDelete opens the delete confirmation modal over every job in the
// focused panel, with `choice` preselected — the mechanism behind the
// "archive all" / "delete all" shortcuts. Returns the (model, cmd) pair for
// Update to return.
func (m appModel) startBulkDelete(choice string) (tea.Model, tea.Cmd) {
	jobs := m.allJobs(m.selectedSide)
	if len(jobs) == 0 {
		m.message = "No jobs in this panel."
		return m, nil
	}
	archived := m.selectedSide == focusHistory && m.historyMode == historyModeArchived
	m.deleteJobs = jobs
	m.deleteFromArchive = archived
	m.deleteForm = newDeleteForm(jobs, archived, choice)
	m.mode = modeDelete
	return m, m.deleteForm.Init()
}

func (m *appModel) totalSelected() int {
	n := 0
	for _, set := range m.selected {
		n += len(set)
	}
	return n
}

// pruneSelection drops marks for jobs that no longer exist in their panel's
// current list (e.g. a one-shot that ran and left history, or a job that was
// archived/deleted). Called from reloadJobs after the lists are rebuilt.
func (m *appModel) pruneSelection() {
	for _, side := range []panelFocus{focusRecurring, focusPending, focusHistory} {
		set := m.selected[side]
		if len(set) == 0 {
			continue
		}
		list := m.allJobs(side)
		seen := make(map[string]bool, len(list))
		for _, j := range list {
			seen[j.Name] = true
		}
		for name := range set {
			if !seen[name] {
				delete(set, name)
			}
		}
	}
}

// nameCell returns the name column's cell value for j in the given panel,
// prefixed with the "*" selection marker when j is marked. The marker counts
// against the column width (colorizeStatusColumn strips it back off), so
// marked rows with long names show one rune less of the name — an accepted
// tradeoff for alignment staying exact.
func (m *appModel) nameCell(j Job, side panelFocus) string {
	if m.selected[side][j.Name] {
		return "*" + j.Name
	}
	return j.Name
}

// panelTitle appends the panel's mark count to its title, so "history (3)"
// makes a pending batch action visible at a glance. No suffix when nothing
// is marked, keeping the title short in the common case.
func (m *appModel) panelTitle(base string, side panelFocus) string {
	if n := len(m.selected[side]); n > 0 {
		return fmt.Sprintf("%s (%d)", base, n)
	}
	return base
}

// footerStatus prefers the transient action message, but falls back to a
// selection hint while marks are active — so the "space toggles / d acts"
// reminder shows instead of a blank footer.
func (m *appModel) footerStatus() string {
	if m.message != "" {
		return m.message
	}
	if n := m.totalSelected(); n > 0 {
		return fmt.Sprintf("%d selected — space toggles, d acts, esc clears", n)
	}
	return ""
}

// deleteSubject names what a delete form is about: a single job quoted by
// name, or a bare count for a batch.
func deleteSubject(jobs []Job) string {
	if len(jobs) == 1 {
		return fmt.Sprintf("'%s'", jobs[0].Name)
	}
	return fmt.Sprintf("%d jobs", len(jobs))
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
		recurringRows[i] = table.Row{m.nameCell(j, focusRecurring), j.NextRunHuman(), label}
	}
	m.recurringTable.SetRows(recurringRows)
	m.recurringTable.SetCursor(recurringSelected)

	pendingSelected := indexOfName(m.pendingJobs, prevPending)
	pendingRows := make([]table.Row, len(m.pendingJobs))
	for i, j := range m.pendingJobs {
		_, label := j.Status()
		pendingRows[i] = table.Row{m.nameCell(j, focusPending), j.ScheduleHuman(), label}
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
		historyRows[i] = table.Row{m.nameCell(j, focusHistory), j.HistoryWhen(), label}
	}
	m.historyTable.SetRows(historyRows)
	m.historyTable.SetCursor(historySelected)

	m.pruneSelection()
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
		m.helpModal.SetSize(sizeMsg.Width, sizeMsg.Height)
		m.settingsModal.SetSize(sizeMsg.Width, sizeMsg.Height)
		m.layout()
		return m, nil
	}

	// The settings/help modals swallow all keys while open — the app must
	// not act on them (so "q" closes the modal instead of quitting, etc.).
	if m.settingsModal.Update(msg) {
		return m, nil
	}
	if m.helpModal.Update(msg) {
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
	// Neovim-style pane navigation: Ctrl+h/l move focus one panel left/right
	// among the three side-by-side panels, pivoting off selectedSide so it
	// still works from focusDetail; Ctrl+j/k move down into details and
	// back up. No-op when there's no pane in that direction, same as
	// vim-tmux-navigator.
	switch {
	case key.Matches(keyMsg, resolve("quit")):
		return m, tea.Quit
	case key.Matches(keyMsg, resolve("help")):
		m.helpModal.Toggle()
		return m, nil
	case key.Matches(keyMsg, resolve("settings")):
		m.settingsModal.Toggle()
		return m, nil
	case key.Matches(keyMsg, resolve("refresh")):
		m.reloadJobs()
		m.message = "List refreshed."
		return m, nil
	case key.Matches(keyMsg, resolve("new")):
		m.createForm = newCreateForm()
		m.mode = modeCreate
		return m, m.createForm.Init()
	case key.Matches(keyMsg, resolve("nav")):
		// "nav" is a single action holding all four pane-navigation keys in
		// a fixed order ([0]=left, [1]=right, [2]=down, [3]=up); the pressed
		// key's position decides the direction, so rebinding nav keeps
		// working as long as the keys keep their order.
		navKeys := resolve("nav").Keys()
		switch {
		case len(navKeys) > 0 && keyMsg.String() == navKeys[0]:
			switch m.selectedSide {
			case focusPending:
				m.focus, m.selectedSide = focusRecurring, focusRecurring
			case focusHistory:
				m.focus, m.selectedSide = focusPending, focusPending
			}
		case len(navKeys) > 1 && keyMsg.String() == navKeys[1]:
			switch m.selectedSide {
			case focusRecurring:
				m.focus, m.selectedSide = focusPending, focusPending
			case focusPending:
				m.focus, m.selectedSide = focusHistory, focusHistory
			}
		case len(navKeys) > 2 && keyMsg.String() == navKeys[2]:
			if m.focus != focusDetail {
				m.focus = focusDetail
			}
		case len(navKeys) > 3 && keyMsg.String() == navKeys[3]:
			if m.focus == focusDetail {
				m.focus = m.selectedSide
			}
		}
		return m, nil
	case key.Matches(keyMsg, resolve("edit")):
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
	case key.Matches(keyMsg, resolve("toggle-pause")):
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
	case key.Matches(keyMsg, resolve("run-now")):
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
	case key.Matches(keyMsg, resolve("delete")):
		archived := m.selectedSide == focusHistory && m.historyMode == historyModeArchived
		jobs := m.selectedJobs(m.selectedSide)
		if len(jobs) == 0 {
			job := m.currentJob()
			if job == nil {
				return m, nil
			}
			jobCopy := *job
			m.deleteJobs = []Job{jobCopy}
		} else {
			m.deleteJobs = jobs
		}
		m.deleteFromArchive = archived
		m.deleteForm = newDeleteForm(m.deleteJobs, archived, "")
		m.mode = modeDelete
		return m, m.deleteForm.Init()
	case key.Matches(keyMsg, resolve("archive-all")):
		if m.selectedSide == focusHistory && m.historyMode == historyModeArchived {
			m.message = "These jobs are already archived."
			return m, nil
		}
		return m.startBulkDelete(deleteChoiceArchive)
	case key.Matches(keyMsg, resolve("delete-all")):
		return m.startBulkDelete(deleteChoiceForever)
	case key.Matches(keyMsg, resolve("select-toggle")):
		m.toggleSelection()
		return m, nil
	case key.Matches(keyMsg, resolve("select-clear")):
		if set := m.selected[m.selectedSide]; len(set) > 0 {
			clear(set)
			m.message = "Selection cleared."
		}
		return m, nil
	case key.Matches(keyMsg, resolve("toggle-archive")):
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
	case key.Matches(keyMsg, resolve("unarchive")):
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
		m.deleteJobs = nil
		m.deleteFromArchive = false
		return m, nil
	}

	form, cmd := m.deleteForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.deleteForm = f
	}

	if m.deleteForm.State == huh.StateCompleted {
		choice := m.deleteForm.GetString("choice")
		jobs := m.deleteJobs
		m.mode = modeList
		m.deleteForm = nil
		m.deleteJobs = nil
		m.deleteFromArchive = false

		for _, job := range jobs {
			switch choice {
			case deleteChoiceArchive:
				if err := archiveJob(job); err == nil {
					m.message = fmt.Sprintf("'%s' archived.", job.Name)
				} else {
					m.message = fmt.Sprintf("error archiving '%s': %v", job.Name, err)
				}
			case deleteChoiceForever:
				if err := deleteJob(job, true); err == nil {
					m.message = fmt.Sprintf("'%s' deleted forever.", job.Name)
				} else {
					m.message = fmt.Sprintf("error deleting '%s': %v", job.Name, err)
				}
			}
		}
		if len(jobs) > 1 {
			verb := "archived"
			if choice == deleteChoiceForever {
				verb = "deleted forever"
			}
			m.message = fmt.Sprintf("%d jobs %s.", len(jobs), verb)
		}
		m.reloadJobs()
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
		title := fmt.Sprintf("delete: %s", deleteSubject(m.deleteJobs))
		return m.renderModal(title, m.deleteForm.View())
	}
	if m.mode == modeCreate {
		return m.renderModal("new job", m.createForm.View())
	}

	headerText := fmt.Sprintf("jobs — %d recurring, %d pending, %d in history — %s", len(m.recurringJobs), len(m.pendingJobs), len(m.historyJobs), jobsDir)
	if avail := m.width - 4; avail > 0 {
		headerText = strings.TrimRight(padLines(headerText, avail), " ")
	}
	header := theme.Header(m.width).Render(headerText)

	recurringView := decorateTable(m.recurringTable.View(), m.recurringJobs, m.recurringTable.Cursor(), m.recurringStatusOffset, m.statusCellWidth, whenColWidth, m.selected[focusRecurring], true)
	recurringBox := theme.Panel(m.focus == focusRecurring).Render(padLines(
		theme.Title().Render(m.panelTitle("recurring", focusRecurring))+"\n\n"+recurringView, m.recurringInnerWidth,
	))

	pendingView := decorateTable(m.pendingTable.View(), m.pendingJobs, m.pendingTable.Cursor(), m.pendingStatusOffset, m.statusCellWidth, scheduleColWidth, m.selected[focusPending], true)
	pendingBox := theme.Panel(m.focus == focusPending).Render(padLines(
		theme.Title().Render(m.panelTitle("pending", focusPending))+"\n\n"+pendingView, m.pendingInnerWidth,
	))

	// Archived jobs have no meaningful Status() (their timer is long gone),
	// so the archived view skips status coloring — the plain "archived"
	// label rendered by the table itself is all there is to show. Marked
	// rows are still highlighted though.
	historyTitle := "history"
	historyView := decorateTable(m.historyTable.View(), m.historyJobs, m.historyTable.Cursor(), m.historyStatusOffset, m.statusCellWidth, whenColWidth, m.selected[focusHistory], true)
	if m.historyMode == historyModeArchived {
		historyTitle = "archived"
		historyView = decorateTable(m.historyTable.View(), m.archivedJobs, m.historyTable.Cursor(), m.historyStatusOffset, m.statusCellWidth, whenColWidth, m.selected[focusHistory], false)
	}
	historyBox := theme.Panel(m.focus == focusHistory).Render(padLines(
		theme.Title().Render(m.panelTitle(historyTitle, focusHistory))+"\n\n"+historyView, m.historyInnerWidth,
	))

	// Recurring, pending, and history sit side by side, equal width split.
	jobsRow := lipgloss.JoinHorizontal(lipgloss.Top, recurringBox, strings.Repeat(" ", panelGap), pendingBox, strings.Repeat(" ", panelGap), historyBox)

	detailTitle := "details"
	if total := len(m.currentDetailLines()); total > m.detailMaxLines {
		detailTitle = fmt.Sprintf("details (%d–%d/%d)", m.detailScroll+1, min(m.detailScroll+m.detailMaxLines, total), total)
	}
	detailBox := theme.Panel(m.focus == focusDetail).Render(padLines(
		theme.Title().Render(detailTitle)+"\n\n"+m.renderDetailBody(), m.innerWidth,
	))

	footer := tuiui.NewFooter(reg.Bindings()...).
		Status(m.footerStatus()).
		Render(m.width, theme)

	view := lipgloss.JoinVertical(lipgloss.Left, header, jobsRow, detailBox, footer)
	if m.settingsModal.Visible() {
		return m.settingsModal.View(theme)
	}
	if m.helpModal.Visible() {
		return m.helpModal.View(theme)
	}
	return view
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
		lines = []string{theme.Dim().Render(fmt.Sprintf("No jobs in %s (recurring, pending, or history).", jobsDir))}
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

// decorateTable post-processes an already-rendered table view, one line at
// a time, matching each line back to its Job by the displayed name cell.
// It re-colors the status column of every row (unless colorStatus is false)
// and, for marked rows (in `selected`), paints a whole-row background +
// bold so the "space" marks are unmistakable without relying on the cursor.
//
// Coloring the cell *value* before it goes through bubbles/table's own
// rendering doesn't work: bubbles truncates each cell with
// runewidth.Truncate, which isn't ANSI-aware and counts escape-code bytes
// as visible width, mangling short colored strings well before they'd
// actually need truncating. Post-processing the plain rendered text instead
// sidesteps that entirely.
//
// Name matching beats line position because bubbles/table renders rows from
// an internal viewport window (its unexported `start` + viewport `YOffset`)
// that isn't recoverable from the public API, so positional math drifts as
// soon as the table scrolls. Matching is exact even for "…"-truncated
// names, and unambiguous because job names are unique. midWidth is the
// *content* width of the middle column ("next run"/"scheduled for"/"date"),
// which differs per panel (whenColWidth vs scheduleColWidth) and is needed
// to recover the name column's own content width from the status offset.
// The row at `cursor` is left untouched — bubbles/table already wraps it in
// its own Selected style, and nesting another color in there would
// reset-terminate that highlight partway through the row.
func decorateTable(view string, jobs []Job, cursor int, offset, width int, midWidth int, selected map[string]bool, colorStatus bool) string {
	// Layout: [name cell: nameW+2][mid cell: midWidth+2][status cell:
	// width]. The status offset is therefore nameW + midWidth + 4, which
	// recovers nameW (each cell carries one space of padding per side).
	nameWidth := offset - midWidth - 4

	type match struct {
		job  Job
		when string
	}
	byName := make(map[string][]match, len(jobs)*2)
	for _, j := range jobs {
		displayed := runewidth.Truncate(j.Name, nameWidth, "…")
		byName[displayed] = append(byName[displayed], match{j, j.HistoryWhen()})
		// Marked rows prefix their name cell with the "*" marker, which
		// counts against the column width — so they display one rune less
		// of the name. Index that truncated shape too, or a long marked
		// name would fail to match and lose its highlight.
		if short := runewidth.Truncate(j.Name, nameWidth-1, "…"); short != displayed {
			byName[short] = append(byName[short], match{j, j.HistoryWhen()})
		}
	}

	markSGR := sgrBg(string(colSurface1))
	if markSGR != "" {
		markSGR = "\x1b[1m" + markSGR // bold + background for marked rows
	}

	lines := strings.Split(view, "\n")
	for i := range lines {
		if i == 0 { // line 0 is the table's own header row
			continue
		}
		// Strip ANSI up front so fixed-width slices line up even on the
		// selected row, which bubbles wraps in its Selected style.
		clean := []rune(ansi.Strip(lines[i]))
		if len(clean) < nameWidth+2 || offset >= len(clean) {
			continue
		}
		// The status cell is the last column; a narrow panel clips its
		// right padding (bubbles caps the line at the table width), so
		// clamp the end instead of bailing on the full-cell check.
		end := min(offset+width, len(clean))
		// TrimSpace drops both sides of the cell, including the leading
		// padding space bubbles' Cell style injects before content.
		displayed := strings.TrimSpace(string(clean[:nameWidth+2]))
		displayed = strings.TrimPrefix(displayed, "*")
		cands := byName[displayed]
		if len(cands) == 0 {
			continue
		}
		var job *Job
		if len(cands) == 1 {
			job = &cands[0].job
		} else {
			// Truncated-name collision — disambiguate by the middle column.
			when := strings.TrimSpace(string(clean[nameWidth+2 : nameWidth+2+midWidth+2]))
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

		kind, _ := job.Status()
		if selected[job.Name] && markSGR != "" {
			lines[i] = renderMarkedRow(clean, offset, end, kind, colorStatus, markSGR)
			continue
		}
		if !colorStatus {
			continue
		}
		cell := string(clean[offset:end])
		trimmed := strings.TrimRight(cell, " ")
		trailing := cell[len(trimmed):]
		colored := statusStyle(kind).Render(trimmed)
		lines[i] = string(clean[:offset]) + colored + trailing + string(clean[end:])
	}
	return strings.Join(lines, "\n")
}

// renderMarkedRow rebuilds one rendered table line as a single SGR
// sequence: bold + the highlight background across the whole row, with the
// status cell (when colorStatus) switching to its status foreground on top
// of that same background. clean is the ANSI-stripped line; offset/end
// delimit the status column (end may be clamped by a narrow panel).
func renderMarkedRow(clean []rune, offset, end int, kind jobStatusKind, colorStatus bool, markSGR string) string {
	b := strings.Builder{}
	b.WriteString(markSGR)
	b.WriteString(string(clean[:offset])) // name + middle columns
	if colorStatus {
		cell := string(clean[offset:end])
		trimmed := strings.TrimRight(cell, " ")
		trailing := cell[len(trimmed):]
		if c, ok := statusStyle(kind).GetForeground().(lipgloss.Color); ok {
			if fg := sgrFg(string(c)); fg != "" {
				b.WriteString(fg)
			}
		}
		b.WriteString(trimmed)
		b.WriteString(trailing)
	} else {
		b.WriteString(string(clean[offset:]))
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// sgrBg/sgrFg turn a "#rrggbb" color into the 24-bit SGR sequence that
// sets background/foreground, returning "" when the color isn't hex (so
// callers can degrade gracefully instead of emitting garbage).
func sgrBg(color string) string {
	if r, g, bl, ok := hexRGB(color); ok {
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, bl)
	}
	return ""
}

func sgrFg(color string) string {
	if r, g, bl, ok := hexRGB(color); ok {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, bl)
	}
	return ""
}

func hexRGB(color string) (r, g, b int, ok bool) {
	color = strings.TrimPrefix(strings.TrimSpace(color), "#")
	if len(color) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(color, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16), int(v>>8) & 0xff, int(v & 0xff), true
}

func (m appModel) renderModal(title, body string) string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}
	box := theme.Modal().Render(theme.Title().Render(title) + "\n\n" + body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

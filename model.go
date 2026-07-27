package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeList mode = iota
	modeEdit
	modeDelete
)

// panelFocus selects which of the two job tables Ctrl+j/Ctrl+k moves
// between (neovim-style pane navigation) and which one drives currentJob().
type panelFocus int

const (
	focusPending panelFocus = iota
	focusHistory
)

// Column widths (content width, before bubbles/table's default Padding(0,1)
// adds 2 on each side). "name" flexes to fill whatever's left of the panel.
const (
	scheduleColWidth = 13 // "agendado para" header, pending panel
	statusColWidth   = 10 // "concluído" is the longest status label
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
	// Of the space left after the detail panel, pendentes gets 2/3 and
	// histórico 1/3.
	jobsAreaPercent     = 42
	pendingSharePercent = 67
)

type appModel struct {
	pendingJobs []Job
	historyJobs []Job

	pendingTable table.Model
	historyTable table.Model
	focus        panelFocus

	mode           mode
	width          int
	height         int
	innerWidth     int // shared content width for all panels/header/footer
	detailMaxLines int
	message        string

	// Rune offset/width of the status column within a rendered table line,
	// used by colorizeStatusColumn — set by layout() alongside the column
	// widths they're derived from.
	pendingStatusOffset int
	historyStatusOffset int
	statusCellWidth     int

	editJob  *Job
	editForm *huh.Form

	deleteJob  *Job
	deleteForm *huh.Form
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
		pendingTable: newJobTable(),
		historyTable: newJobTable(),
		width:        100,
		height:       30,
	}
	m.reloadJobs()
	return m
}

func (m appModel) Init() tea.Cmd {
	return nil
}

// focusedTable/otherTable let most code work generically instead of
// switching on m.focus everywhere.
func (m *appModel) focusedTable() *table.Model {
	if m.focus == focusPending {
		return &m.pendingTable
	}
	return &m.historyTable
}

func (m *appModel) focusedJobs() []Job {
	if m.focus == focusPending {
		return m.pendingJobs
	}
	return m.historyJobs
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

// reloadJobs re-scans ~/jobs, splits jobs into "pendentes" (still on an
// active or paused timer) and "histórico" (resolved — completed, failed, or
// removed before ever running), and rebuilds both tables, keeping each
// one's cursor on its previously selected job by name instead of resetting
// to row 0.
func (m *appModel) reloadJobs() {
	prevPending, prevHistory := jobName(m.pendingJobs, m.pendingTable.Cursor()), jobName(m.historyJobs, m.historyTable.Cursor())

	all := discoverJobs()
	m.pendingJobs = nil
	m.historyJobs = nil
	for _, j := range all {
		if j.IsPending() {
			m.pendingJobs = append(m.pendingJobs, j)
		} else {
			m.historyJobs = append(m.historyJobs, j)
		}
	}
	// Most recently touched first — that's what makes a history useful.
	sort.SliceStable(m.historyJobs, func(i, k int) bool {
		return m.historyJobs[i].historyModTime().After(m.historyJobs[k].historyModTime())
	})

	// Columns must exist before SetRows below — bubbles/table indexes
	// m.cols per cell while rendering, and panics if that's still empty.
	m.layout()

	pendingSelected := indexOfName(m.pendingJobs, prevPending)
	pendingRows := make([]table.Row, len(m.pendingJobs))
	for i, j := range m.pendingJobs {
		_, label := j.Status()
		pendingRows[i] = table.Row{j.Name, j.ScheduleHuman(), label}
	}
	m.pendingTable.SetRows(pendingRows)
	m.pendingTable.SetCursor(pendingSelected)

	historySelected := indexOfName(m.historyJobs, prevHistory)
	historyRows := make([]table.Row, len(m.historyJobs))
	for i, j := range m.historyJobs {
		_, label := j.Status()
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

// layout recomputes column widths for both tables (so each one's rendered
// width exactly matches m.innerWidth, the same width used by the panel
// border around it — a mismatch there makes lipgloss hard-wrap rows
// mid-line) and the vertical space budget across all three panels, so
// header + pendentes + histórico + detalhes + footer never exceeds m.height.
func (m *appModel) layout() {
	m.innerWidth = m.width - 4
	if m.innerWidth < 40 {
		m.innerWidth = 40
	}

	pendingNameWidth := m.innerWidth - 3*2 - scheduleColWidth - statusColWidth
	if pendingNameWidth < minNameColWidth {
		pendingNameWidth = minNameColWidth
	}
	m.pendingTable.SetColumns([]table.Column{
		{Title: "job", Width: pendingNameWidth},
		{Title: "agendado para", Width: scheduleColWidth},
		{Title: "status", Width: statusColWidth},
	})
	m.pendingTable.SetWidth(m.innerWidth)
	m.pendingStatusOffset = (pendingNameWidth + 2) + (scheduleColWidth + 2)

	historyNameWidth := m.innerWidth - 3*2 - whenColWidth - statusColWidth
	if historyNameWidth < minNameColWidth {
		historyNameWidth = minNameColWidth
	}
	m.historyTable.SetColumns([]table.Column{
		{Title: "nome", Width: historyNameWidth},
		{Title: "data", Width: whenColWidth},
		{Title: "status", Width: statusColWidth},
	})
	m.historyTable.SetWidth(m.innerWidth)
	m.historyStatusOffset = (historyNameWidth + 2) + (whenColWidth + 2)
	m.statusCellWidth = statusColWidth + 2

	minBody := 2*tableBoxOverhead + 2*minVisibleRows + detailBoxOverhead + minDetailLines
	bodyHeight := m.height - headerLines - footerLines
	if bodyHeight < minBody {
		bodyHeight = minBody
	}

	jobsAreaHeight := bodyHeight * jobsAreaPercent / 100
	minJobsArea := 2*tableBoxOverhead + 2*minVisibleRows
	if jobsAreaHeight < minJobsArea {
		jobsAreaHeight = minJobsArea
	}

	// Each panel's actual ceiling: enough rows for every job it has, never
	// less than minVisibleRows. Neither panel should be handed more than
	// this even if the 2/3-1/3 split below would give it more.
	minPending := tableBoxOverhead + minVisibleRows
	idealPending := max(tableBoxOverhead+len(m.pendingJobs), minPending)
	minHistory := tableBoxOverhead + minVisibleRows
	idealHistory := max(tableBoxOverhead+len(m.historyJobs), minHistory)

	pendingBoxHeight := min(max(jobsAreaHeight*pendingSharePercent/100, minPending), idealPending)
	historyBoxHeight := min(max(jobsAreaHeight-pendingBoxHeight, minHistory), idealHistory)

	// A panel with fewer jobs than its share frees up space — offer that to
	// whichever job panel is still short of its own ideal before letting it
	// fall through to the detail panel.
	if leftover := jobsAreaHeight - pendingBoxHeight - historyBoxHeight; leftover > 0 {
		if grow := min(leftover, idealPending-pendingBoxHeight); grow > 0 {
			pendingBoxHeight += grow
			leftover -= grow
		}
		if grow := min(leftover, idealHistory-historyBoxHeight); grow > 0 {
			historyBoxHeight += grow
		}
	}

	detailBoxHeight := bodyHeight - pendingBoxHeight - historyBoxHeight
	if detailBoxHeight < detailBoxOverhead+minDetailLines {
		detailBoxHeight = detailBoxOverhead + minDetailLines
	}

	setTableVisibleRows(&m.pendingTable, pendingBoxHeight-tableBoxOverhead)
	setTableVisibleRows(&m.historyTable, historyBoxHeight-tableBoxOverhead)

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
	default:
		return m.updateList(msg)
	}
}

func (m appModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.message = "Lista atualizada."
		return m, nil
	// Neovim-style pane navigation: Ctrl+j/Ctrl+k move focus between the
	// pendentes and histórico panels (Ctrl+h/Ctrl+l are no-ops, same as
	// vim-tmux-navigator when there's no pane in that direction).
	case "ctrl+j":
		m.focus = focusHistory
		return m, nil
	case "ctrl+k":
		m.focus = focusPending
		return m, nil
	case "e":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		if job.TimerPath == "" {
			m.message = fmt.Sprintf("'%s' não tem timer pra reagendar.", job.Name)
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
			m.message = fmt.Sprintf("'%s' não tem timer.", job.Name)
			return m, nil
		}
		toggleJob(*job)
		wasEnabled := job.Enabled()
		m.reloadJobs()
		state := "pausado"
		if !wasEnabled {
			state = "ativado"
		}
		m.message = fmt.Sprintf("'%s' %s.", job.Name, state)
		return m, nil
	case "d":
		job := m.currentJob()
		if job == nil {
			return m, nil
		}
		jobCopy := *job
		m.deleteJob = &jobCopy
		m.deleteForm = newDeleteForm(jobCopy)
		m.mode = modeDelete
		return m, m.deleteForm.Init()
	}

	return m.forwardToFocusedTable(msg)
}

func (m appModel) forwardToFocusedTable(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.focus == focusPending {
		m.pendingTable, cmd = m.pendingTable.Update(msg)
	} else {
		m.historyTable, cmd = m.historyTable.Update(msg)
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
				m.message = fmt.Sprintf("'%s' reagendado para %02d/%02d %02d:%02d.", m.editJob.Name, dom, month, hour, minute)
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

func (m appModel) updateDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.mode = modeList
		m.deleteForm = nil
		m.deleteJob = nil
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

		switch choice {
		case deleteChoiceCron, deleteChoiceAll:
			if err := deleteJob(job, choice == deleteChoiceAll); err == nil {
				scope := "só agendamento"
				if choice == deleteChoiceAll {
					scope = "agendamento + arquivos"
				}
				m.message = fmt.Sprintf("'%s' removido (%s).", job.Name, scope)
			}
			m.reloadJobs()
		}
		return m, nil
	}

	return m, cmd
}

func (m appModel) View() string {
	if m.mode == modeEdit {
		title := fmt.Sprintf("reagendar: %s", m.editJob.Name)
		return m.renderModal(title, m.editForm.View())
	}
	if m.mode == modeDelete {
		title := fmt.Sprintf("apagar: %s", m.deleteJob.Name)
		return m.renderModal(title, m.deleteForm.View())
	}

	headerText := fmt.Sprintf("jobs — %d pendente(s), %d no histórico — %s", len(m.pendingJobs), len(m.historyJobs), jobsDir)
	if avail := m.width - 4; avail > 0 {
		headerText = strings.TrimRight(padLines(headerText, avail), " ")
	}
	header := headerStyle(m.width).Render(headerText)

	pendingView := colorizeStatusColumn(m.pendingTable.View(), m.pendingJobs, m.pendingTable.Cursor(), m.pendingStatusOffset, m.statusCellWidth)
	pendingBox := panelStyle(m.focus == focusPending).Render(padLines(
		titleStyle().Render("pendentes")+"\n\n"+pendingView, m.innerWidth,
	))

	historyView := colorizeStatusColumn(m.historyTable.View(), m.historyJobs, m.historyTable.Cursor(), m.historyStatusOffset, m.statusCellWidth)
	historyBox := panelStyle(m.focus == focusHistory).Render(padLines(
		titleStyle().Render("histórico")+"\n\n"+historyView, m.innerWidth,
	))

	detailBox := panelStyle(false).Render(padLines(
		titleStyle().Render("detalhes")+"\n\n"+m.renderDetailBody(), m.innerWidth,
	))

	help := "e reagendar · t pausar/retomar · d apagar · r atualizar · ctrl+j/k trocar painel · q sair"
	footerText := help
	if m.message != "" {
		footerText = m.message + "   " + help
	}
	footer := footerStyle(m.width).Render(footerText)

	return lipgloss.JoinVertical(lipgloss.Left, header, pendingBox, historyBox, detailBox, footer)
}

func (m appModel) renderDetailBody() string {
	job := m.currentJob()
	if job == nil {
		return dimStyle().Render(fmt.Sprintf("Nenhum job em %s (pendentes ou histórico).", jobsDir))
	}
	kind, label := job.Status()
	styled := statusStyle(kind).Render(statusGlyph(kind) + " " + label)
	return truncateLines(job.DetailText(styled), m.detailMaxLines)
}

// truncateLines caps body to at most max lines, marking the cut so it reads
// as "there's more, go look at the file" rather than a silent clip.
func truncateLines(body string, max int) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= max || max <= 1 {
		return body
	}
	lines = lines[:max-1]
	lines = append(lines, dimStyle().Render("… (cortado — veja o arquivo original)"))
	return strings.Join(lines, "\n")
}

// colorizeStatusColumn re-colors the status column of an already-rendered
// table view, one line at a time. Coloring the cell *value* before it goes
// through bubbles/table's own rendering doesn't work: bubbles truncates
// each cell with runewidth.Truncate, which isn't ANSI-aware and counts
// escape-code bytes as visible width, mangling short colored strings well
// before they'd actually need truncating. Post-processing the plain
// rendered text instead sidesteps that entirely. The row at `cursor` is
// left untouched — bubbles/table already wraps it in its own Selected
// style, and nesting another color in there would reset-terminate that
// highlight partway through the row.
func colorizeStatusColumn(view string, jobs []Job, cursor int, offset, width int) string {
	lines := strings.Split(view, "\n")
	for i := range lines {
		rowIdx := i - 1 // line 0 is the table's own header row
		if rowIdx < 0 || rowIdx >= len(jobs) || rowIdx == cursor {
			continue
		}
		line := []rune(lines[i])
		if offset+width > len(line) {
			continue
		}
		cell := string(line[offset : offset+width])
		trimmed := strings.TrimRight(cell, " ")
		trailing := cell[len(trimmed):]
		kind, _ := jobs[rowIdx].Status()
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

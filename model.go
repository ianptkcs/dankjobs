package main

import (
	"fmt"
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

// Fixed table columns other than "job", which flexes to fill the panel.
// scheduleColWidth/statusColWidth/logColWidth are content widths (before the
// +2 that bubbles/table's default Padding(0,1) adds on each side).
const (
	scheduleColWidth = 13 // "agendado para" header
	statusColWidth   = 15 // "rodou/removido" is the longest status label
	logColWidth      = 4
	minNameColWidth  = 12
)

// Fixed vertical overhead around the flexible parts of each panel, in
// lines: 2 border + 1 title + 1 blank line, plus the table's own header row
// for the jobs panel. Layout math in (*appModel).layout keeps total
// rendered height within the terminal's actual height — content that
// doesn't fit gets clipped on purpose (missing a line beats the terminal
// silently scrolling the header/table off the top).
const (
	headerLines       = 1
	footerLines       = 1
	tableBoxOverhead  = 2 + 1 + 1 + 1
	detailBoxOverhead = 2 + 1 + 1
	minVisibleRows    = 3
	minDetailLines    = 3
	tableBoxPercent   = 40
)

type appModel struct {
	jobs           []Job
	table          table.Model
	mode           mode
	width          int
	height         int
	innerWidth     int // shared content width for all panels/header/footer
	detailMaxLines int
	message        string

	editJob  *Job
	editForm *huh.Form

	deleteJob  *Job
	deleteForm *huh.Form
}

func newModel() appModel {
	t := table.New(table.WithFocused(true))

	styles := table.DefaultStyles()
	// Background is set explicitly (matching the panel's own background)
	// because Header's own reset code would otherwise blank it out to the
	// terminal default for every char it touches, same root cause as the
	// Cell/Selected issue above.
	styles.Header = styles.Header.Foreground(colText).Background(colBase).Bold(true).BorderForeground(colSurface1)
	styles.Selected = styles.Selected.Foreground(colBase).Background(colPink).Bold(true)
	// Cell intentionally has no Foreground: bubbles/table renders each cell
	// independently (with its own reset code) before joining the row, then
	// wraps the whole joined row in Selected for the cursor line. A colored
	// Cell style embeds a reset partway through that row, which cuts the
	// Selected background off after the first column. Leaving Cell plain
	// (as bubbles' own DefaultStyles does) avoids that.
	t.SetStyles(styles)

	m := appModel{table: t, width: 100, height: 30}
	m.layout()
	m.reloadJobs()
	return m
}

// layout recomputes both the table's columns/width (so its rendered width
// exactly matches m.innerWidth, the same width used by the panel border
// around it — a mismatch there makes lipgloss hard-wrap table rows mid-line)
// and the vertical space budget for the table box vs. the detail box, so
// header + table box + detail box + footer never exceeds m.height.
func (m *appModel) layout() {
	m.innerWidth = m.width - 4
	if m.innerWidth < 40 {
		m.innerWidth = 40
	}
	nameWidth := m.innerWidth - 4*2 - scheduleColWidth - statusColWidth - logColWidth
	if nameWidth < minNameColWidth {
		nameWidth = minNameColWidth
	}
	m.table.SetColumns([]table.Column{
		{Title: "job", Width: nameWidth},
		{Title: "agendado para", Width: scheduleColWidth},
		{Title: "status", Width: statusColWidth},
		{Title: "log", Width: logColWidth},
	})
	m.table.SetWidth(m.innerWidth)

	minBody := tableBoxOverhead + minVisibleRows + detailBoxOverhead + minDetailLines
	bodyHeight := m.height - headerLines - footerLines
	if bodyHeight < minBody {
		bodyHeight = minBody
	}

	tableBoxHeight := bodyHeight * tableBoxPercent / 100
	if tableBoxHeight < tableBoxOverhead+minVisibleRows {
		tableBoxHeight = tableBoxOverhead + minVisibleRows
	}
	// Don't reserve more table rows than there are jobs — give the leftover
	// space to the detail panel instead of padding the table with blank rows.
	if wanted := tableBoxOverhead + len(m.jobs); wanted >= tableBoxOverhead+minVisibleRows && tableBoxHeight > wanted {
		tableBoxHeight = wanted
	}

	detailBoxHeight := bodyHeight - tableBoxHeight
	if detailBoxHeight < detailBoxOverhead+minDetailLines {
		detailBoxHeight = detailBoxOverhead + minDetailLines
	}

	visibleRows := tableBoxHeight - tableBoxOverhead
	if visibleRows < minVisibleRows {
		visibleRows = minVisibleRows
	}
	// SetHeight subtracts the table's own header row internally, so pass
	// visibleRows+1 to actually get visibleRows data rows on screen.
	m.table.SetHeight(visibleRows + 1)

	m.detailMaxLines = detailBoxHeight - detailBoxOverhead
	if m.detailMaxLines < minDetailLines {
		m.detailMaxLines = minDetailLines
	}
}

func (m appModel) Init() tea.Cmd {
	return nil
}

func (m *appModel) currentJob() *Job {
	if len(m.jobs) == 0 {
		return nil
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.jobs) {
		return nil
	}
	return &m.jobs[idx]
}

// reloadJobs re-scans ~/jobs and rebuilds the table, keeping the cursor on
// the previously selected job (by name) instead of resetting to row 0.
func (m *appModel) reloadJobs() {
	var previousName string
	if cur := m.currentJob(); cur != nil {
		previousName = cur.Name
	}

	m.jobs = discoverJobs()
	rows := make([]table.Row, len(m.jobs))
	selectedIndex := 0
	for i, j := range m.jobs {
		_, label := j.Status()
		logMark := "—"
		if j.Log != "" {
			logMark = "sim"
		}
		rows[i] = table.Row{j.Name, j.ScheduleHuman(), label, logMark}
		if j.Name == previousName {
			selectedIndex = i
		}
	}
	m.table.SetRows(rows)
	m.table.SetCursor(selectedIndex)
	m.layout()
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
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}

	m.message = ""
	switch keyMsg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.reloadJobs()
		m.message = "Lista atualizada."
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

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
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

	headerText := fmt.Sprintf("jobs — %d job(s) em %s", len(m.jobs), jobsDir)
	if avail := m.width - 4; avail > 0 {
		headerText = strings.TrimRight(padLines(headerText, avail), " ")
	}
	header := headerStyle(m.width).Render(headerText)

	tableBox := panelStyle().Render(padLines(
		titleStyle().Render("jobs")+"\n\n"+m.table.View(), m.innerWidth,
	))

	detailBox := panelStyle().Render(padLines(
		titleStyle().Render("detalhes")+"\n\n"+m.renderDetailBody(), m.innerWidth,
	))

	help := "e reagendar · t pausar/retomar · d apagar · r atualizar · q sair"
	footerText := help
	if m.message != "" {
		footerText = m.message + "   " + help
	}
	footer := footerStyle(m.width).Render(footerText)

	return lipgloss.JoinVertical(lipgloss.Left, header, tableBox, detailBox, footer)
}

func (m appModel) renderDetailBody() string {
	job := m.currentJob()
	if job == nil {
		return dimStyle().Render(fmt.Sprintf("Nenhum job encontrado em %s", jobsDir))
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

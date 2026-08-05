package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ianptkcs/tabelatuiui"
)

var (
	jobsDir        = tuiui.EnvOr("DJOBS_JOBS_DIR", filepath.Join(tuiui.HomeDir(), "jobs"))
	systemdUserDir = tuiui.EnvOr("DJOBS_SYSTEMD_DIR", filepath.Join(tuiui.HomeDir(), ".config", "systemd", "user"))
)

var onCalendarRe = regexp.MustCompile(`(?m)^OnCalendar=(.+)$`)

// oneshotOnCalendarRe matches the absolute-timestamp OnCalendar= shape that
// computeOnCalendar produces for one-shot jobs — anything else (systemd's
// native "daily"/weekday-list/day-of-month expressions) is a recurring
// schedule. See Job.IsRecurring.
var oneshotOnCalendarRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

var jobNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// validateJobName is shared by the create form (as a huh.Validate callback)
// and createJob itself.
func validateJobName(name string) error {
	if !jobNameRe.MatchString(name) {
		return fmt.Errorf("use letters, numbers, - or _ (starting with a letter or number)")
	}
	if _, err := os.Stat(filepath.Join(jobsDir, name)); err == nil {
		return fmt.Errorf("a job named '%s' already exists", name)
	}
	return nil
}

// Job mirrors one ~/jobs/<name>/ directory paired with an optional
// <name>.timer / <name>.service systemd user unit of the same name.
type Job struct {
	Name          string
	Dir           string
	Script        string
	Log           string
	Body          string
	TimerPath     string
	ServicePath   string
	OnCalendar    string
	UnitFileState string
	NextElapse    string
	// ServiceActiveState is only populated when TimerPath != "": it's what
	// tells a job that's still scheduled apart from one whose timer already
	// fired and failed (see Status).
	ServiceActiveState string
	// RecurCyclePath is set when a <name>.recur sidecar exists next to the
	// script — see IsRecurring.
	RecurCyclePath string
}

func systemctlUser(args ...string) string {
	out, _ := exec.Command("systemctl", append([]string{"--user"}, args...)...).Output()
	return string(out)
}

func timerProperties(name string) map[string]string {
	out := systemctlUser("show", name+".timer", "--property=ActiveState,UnitFileState,NextElapseUSecRealtime")
	return parseProperties(out)
}

// serviceActiveState reports the ActiveState of <name>.service. A one-shot
// service that has actually run and failed shows "failed" here; one that
// hasn't run yet (or ran fine) shows "inactive" — that's the only reliable
// signal for telling "still pending" apart from "fired and failed", since a
// failed run never reaches the job script's self-cleanup step and so leaves
// its timer/service files behind just like a merely-paused job would.
func serviceActiveState(name string) string {
	out := systemctlUser("show", name+".service", "--property=ActiveState")
	return parseProperties(out)["ActiveState"]
}

func parseProperties(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			props[k] = v
		}
	}
	return props
}

func readOnCalendar(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := onCalendarRe.FindStringSubmatch(string(data))
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func globFirst(dir, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func discoverJobs() []Job {
	var jobs []Job
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		return jobs
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		dir := filepath.Join(jobsDir, name)
		body := globFirst(dir, "*body*.txt")
		if body == "" {
			body = globFirst(dir, "*.txt")
		}
		job := Job{
			Name:           name,
			Dir:            dir,
			Script:         globFirst(dir, "*.sh"),
			Log:            globFirst(dir, "*.log"),
			Body:           body,
			RecurCyclePath: globFirst(dir, "*.recur"),
		}

		servicePath := filepath.Join(systemdUserDir, name+".service")
		if _, err := os.Stat(servicePath); err == nil {
			job.ServicePath = servicePath
		}

		timerPath := filepath.Join(systemdUserDir, name+".timer")
		if _, err := os.Stat(timerPath); err == nil {
			job.TimerPath = timerPath
			job.OnCalendar = readOnCalendar(timerPath)
			props := timerProperties(name)
			job.UnitFileState = props["UnitFileState"]
			job.NextElapse = props["NextElapseUSecRealtime"]
			job.ServiceActiveState = serviceActiveState(name)
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func (j Job) Enabled() bool {
	return j.UnitFileState == "enabled"
}

func (j Job) ScheduleHuman() string {
	if j.OnCalendar == "" {
		return "—"
	}
	t, err := time.Parse("2006-01-02 15:04:05", j.OnCalendar)
	if err != nil {
		return j.OnCalendar
	}
	return fmt.Sprintf("%02d/%02d %02d:%02d", t.Day(), t.Month(), t.Hour(), t.Minute())
}

// nextElapseLayouts covers the timestamp shapes systemctl's
// NextElapseUSecRealtime property has been observed to use across systemd
// versions/locales. NextRunHuman falls back to the raw string when none
// match, same as ScheduleHuman does for OnCalendar.
var nextElapseLayouts = []string{
	"Mon 2006-01-02 15:04:05 -0700",
	"Mon 2006-01-02 15:04:05 -07",
	"Mon 2006-01-02 15:04:05 MST",
	"2006-01-02 15:04:05",
}

// NextRunHuman formats NextElapse (systemd's own computed next-occurrence
// timestamp) for the recurring panel — unlike ScheduleHuman, this works for
// every recurrence kind since systemd resolves "daily"/weekday-list/cycle
// expressions into a concrete next timestamp regardless of how OnCalendar=
// itself is spelled.
func (j Job) NextRunHuman() string {
	if j.NextElapse == "" {
		return "—"
	}
	for _, layout := range nextElapseLayouts {
		if t, err := time.Parse(layout, j.NextElapse); err == nil {
			return fmt.Sprintf("%02d/%02d %02d:%02d", t.Day(), t.Month(), t.Hour(), t.Minute())
		}
	}
	return j.NextElapse
}

type jobStatusKind int

const (
	statusActive jobStatusKind = iota
	statusPaused
	statusCompleted
	statusFailed
	statusRemoved
	statusManual
)

// IsManual reports whether j has a service unit but no timer — a job created
// without a schedule that only runs when started by hand from the TUI.
func (j Job) IsManual() bool {
	return j.TimerPath == "" && j.ServicePath != ""
}

// Status returns a coarse status kind (for styling) and a plain-text label.
//
// A job's timer/service unit files are only ever deleted by the job
// script's own self-cleanup step, which runs after everything else
// succeeds — so TimerPath's presence isn't just "still scheduled", it's
// also how a failed run is told apart from a completed one: a failure never
// reaches that cleanup line, leaving the units behind exactly like a merely
// paused job would. The one further signal needed is whether the service
// unit actually ran and failed (ActiveState == "failed") versus simply not
// having fired yet. A manual job (service unit, no timer) is never "done":
// it stays actionable so it can be started again.
func (j Job) Status() (jobStatusKind, string) {
	if j.TimerPath != "" {
		if j.ServiceActiveState == "failed" {
			return statusFailed, "failed"
		}
		if j.Enabled() {
			return statusActive, "active"
		}
		return statusPaused, "paused"
	}
	if j.IsManual() {
		return statusManual, "manual"
	}
	if j.Log != "" {
		return statusCompleted, "done"
	}
	return statusRemoved, "removed"
}

// IsPending reports whether j still has a live, unresolved schedule —
// i.e. belongs in the "pending" panel rather than "history".
func (j Job) IsPending() bool {
	kind, _ := j.Status()
	return kind == statusActive || kind == statusPaused || kind == statusManual
}

// IsRecurring reports whether j repeats instead of running once. A job with
// no timer never recurs. Otherwise: systemd-native recurring expressions
// (daily/weekly/monthly) don't match the absolute one-shot timestamp shape,
// so they're detected directly; a custom day-interval cycle job, though, has
// an OnCalendar= that *does* look like an absolute timestamp at any given
// snapshot (it's rewritten to the next concrete date after every run), so
// it's told apart by the presence of its .recur sidecar instead.
func (j Job) IsRecurring() bool {
	if j.TimerPath == "" {
		return false
	}
	if !oneshotOnCalendarRe.MatchString(j.OnCalendar) {
		return true
	}
	return j.RecurCyclePath != ""
}

// HistoryWhen returns when a non-pending job was last touched — the log's
// mtime (when it finished running) if there is one, else the job
// directory's own mtime — for display in the history panel. historyModTime
// returns the same thing as a time.Time, for sorting.
func (j Job) HistoryWhen() string {
	t := j.historyModTime()
	if t.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%02d/%02d %02d:%02d", t.Day(), t.Month(), t.Hour(), t.Minute())
}

func (j Job) historyModTime() time.Time {
	if j.Log != "" {
		if info, err := os.Stat(j.Log); err == nil {
			return info.ModTime()
		}
	}
	if info, err := os.Stat(j.Dir); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// DetailText renders the job's detail panel body. statusLabel is injected
// pre-styled by the caller so this stays presentation-agnostic otherwise.
func (j Job) DetailText(statusLabel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (%s)\n", j.Name, j.Dir)

	if j.TimerPath != "" {
		fmt.Fprintf(&b, "\ntimer: %s  [%s]\n", j.OnCalendar, statusLabel)
		if j.NextElapse != "" {
			fmt.Fprintf(&b, "next run: %s\n", j.NextElapse)
		}
		if j.Script != "" {
			fmt.Fprintf(&b, "command: %s\n", j.Script)
		}
	} else {
		fmt.Fprintf(&b, "\nstatus: %s   (%s)\n", statusLabel, j.HistoryWhen())
	}

	if j.Body != "" {
		if data, err := os.ReadFile(j.Body); err == nil {
			fmt.Fprintf(&b, "\n--- notes ---\n%s\n", strings.TrimSpace(string(data)))
		}
	}
	if j.Script != "" {
		if data, err := os.ReadFile(j.Script); err == nil {
			fmt.Fprintf(&b, "\n--- script ---\n%s\n", strings.TrimSpace(string(data)))
		}
	}
	if j.Log != "" {
		if data, err := os.ReadFile(j.Log); err == nil {
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			if len(lines) > 25 {
				lines = lines[len(lines)-25:]
			}
			fmt.Fprintf(&b, "\n--- log (tail) ---\n%s\n", strings.Join(lines, "\n"))
		}
	}
	return b.String()
}

func daemonReload() {
	systemctlUser("daemon-reload")
}

func enableNow(name string) {
	systemctlUser("enable", "--now", name+".timer")
}

func disableNow(name string) {
	systemctlUser("disable", "--now", name+".timer")
}

func toggleJob(j Job) {
	if j.Enabled() {
		disableNow(j.Name)
	} else {
		enableNow(j.Name)
	}
}

// computeOnCalendar builds a systemd OnCalendar= timestamp for a one-shot
// job. Year rolls over to next year if the given month/day already passed
// this year, mirroring how such a date is normally picked interactively.
func computeOnCalendar(minute, hour, dom, month int, now time.Time) string {
	year := now.Year()
	todayMonth, todayDay := int(now.Month()), now.Day()
	if month < todayMonth || (month == todayMonth && dom < todayDay) {
		year++
	}
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:00", year, month, dom, hour, minute)
}

// RecurrenceKind is how often a job runs. recurOneshot runs once and
// self-deletes its own timer/service (the original convention); the rest
// keep running forever on the given schedule.
type RecurrenceKind int

const (
	recurOneshot RecurrenceKind = iota
	recurDaily
	recurWeekly
	recurMonthly
	recurCycle
	// recurManual runs on demand only — no timer unit, the job fires when the
	// TUI's "x run now" starts its service.
	recurManual
)

var systemdWeekdayAbbr = map[time.Weekday]string{
	time.Monday:    "Mon",
	time.Tuesday:   "Tue",
	time.Wednesday: "Wed",
	time.Thursday:  "Thu",
	time.Friday:    "Fri",
	time.Saturday:  "Sat",
	time.Sunday:    "Sun",
}

// isoWeekday returns weekday number with Monday=1..Sunday=7, for sorting a
// weekday list into the conventional Mon..Sun display order.
func isoWeekday(d time.Weekday) int {
	if d == time.Sunday {
		return 7
	}
	return int(d)
}

// computeRecurringOnCalendar builds a native systemd OnCalendar= expression
// for the daily/weekly/monthly kinds — systemd itself resolves these into
// every future occurrence, so (unlike recurCycle) nothing about the job's
// own script needs to touch its timer unit again after creation.
func computeRecurringOnCalendar(kind RecurrenceKind, weekdays []time.Weekday, dayOfMonth, hour, minute int) string {
	switch kind {
	case recurDaily:
		return fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute)
	case recurWeekly:
		sorted := append([]time.Weekday(nil), weekdays...)
		sort.Slice(sorted, func(i, k int) bool { return isoWeekday(sorted[i]) < isoWeekday(sorted[k]) })
		abbrs := make([]string, len(sorted))
		for i, d := range sorted {
			abbrs[i] = systemdWeekdayAbbr[d]
		}
		return fmt.Sprintf("%s *-*-* %02d:%02d:00", strings.Join(abbrs, ","), hour, minute)
	case recurMonthly:
		return fmt.Sprintf("*-*-%02d %02d:%02d:00", dayOfMonth, hour, minute)
	default:
		return ""
	}
}

// rescheduleJob rewrites the OnCalendar= line of j's timer unit.
func rescheduleJob(j Job, minute, hour, dom, month int, now time.Time) error {
	onCalendar := computeOnCalendar(minute, hour, dom, month, now)

	data, err := os.ReadFile(j.TimerPath)
	if err != nil {
		return err
	}
	newData := onCalendarRe.ReplaceAllString(string(data), "OnCalendar="+onCalendar)
	if err := os.WriteFile(j.TimerPath, []byte(newData), 0o644); err != nil {
		return err
	}
	daemonReload()
	if j.Enabled() {
		enableNow(j.Name)
	}
	return nil
}

// jobSchedule bundles every recurrence-kind-specific parameter createJob
// needs. Which fields matter depends on Kind: oneshot/cycle use
// Minute/Hour/DOM/Month for an absolute date, weekly uses Weekdays, monthly
// uses DayOfMonth, daily only needs Hour/Minute.
type jobSchedule struct {
	Kind       RecurrenceKind
	Minute     int
	Hour       int
	DOM        int
	Month      int
	Weekdays   []time.Weekday
	DayOfMonth int
	Cycle      []int
}

// oneshotCleanupTail is the self-removal block a one-shot job's script ends
// with: without it, a successful run would leave the timer/service files
// behind and the job would never move from "pending" to "history".
const oneshotCleanupTail = `
# self-remove the systemd unit pair once done (one-shot, not recurring)
systemctl --user disable --now "${JOB_NAME}.timer" 2>/dev/null || true
rm -f %q %q
systemctl --user daemon-reload
`

// cycleRescheduleTail is a custom-cycle job's self-reschedule block: instead
// of deleting its unit, it advances to the next day-interval in the cycle
// (wrapping around) and rewrites its own OnCalendar= to that concrete date.
// Placeholders are substituted via strings.NewReplacer rather than
// fmt.Sprintf, since the bash itself is full of literal '%' (date format,
// modulo, printf) that would otherwise need doubling as verb-escapes.
const cycleRescheduleTail = `
# self-reschedule for the next point in the day-interval cycle (recurring, not one-shot)
RECUR_FILE="__RECUR_FILE__"
read -r -a CYCLE < <(sed -n 1p "$RECUR_FILE")
IDX=$(sed -n 2p "$RECUR_FILE")
NEXT_IDX=$(( (IDX + 1) % ${#CYCLE[@]} ))
NEXT_DATE=$(date -d "+${CYCLE[$IDX]} days" '+%Y-%m-%d %H:%M:00')
sed -i "s/^OnCalendar=.*/OnCalendar=${NEXT_DATE}/" "__TIMER_PATH__"
printf '%s\n%d\n' "${CYCLE[*]}" "$NEXT_IDX" > "$RECUR_FILE"
systemctl --user daemon-reload
systemctl --user enable --now "__TIMER_NAME__"
`

// createJob writes a new job directory + script, plus its paired systemd
// timer/service units, and enables the timer — the same convention
// discoverJobs expects (see instructions.md). What the script's tail does
// once commands finish depends on sched.Kind: a one-shot self-removes its
// unit pair (oneshotCleanupTail); daily/weekly/monthly need no tail at all,
// since systemd itself resolves those OnCalendar= expressions into every
// future occurrence; a custom cycle self-reschedules (cycleRescheduleTail)
// plus a <name>.recur sidecar recording the cycle and current position.
func createJob(name, commands, notes string, sched jobSchedule, now time.Time) error {
	if err := validateJobName(name); err != nil {
		return err
	}

	dir := filepath.Join(jobsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(systemdUserDir, 0o755); err != nil {
		return err
	}

	scriptPath := filepath.Join(dir, name+".sh")
	logPath := filepath.Join(dir, name+".log")
	timerPath := filepath.Join(systemdUserDir, name+".timer")
	servicePath := filepath.Join(systemdUserDir, name+".service")

	var onCalendar, tail string
	switch sched.Kind {
	case recurDaily, recurWeekly, recurMonthly:
		onCalendar = computeRecurringOnCalendar(sched.Kind, sched.Weekdays, sched.DayOfMonth, sched.Hour, sched.Minute)
	case recurCycle:
		onCalendar = computeOnCalendar(sched.Minute, sched.Hour, sched.DOM, sched.Month, now)
		recurPath := filepath.Join(dir, name+".recur")
		cycleFields := make([]string, len(sched.Cycle))
		for i, d := range sched.Cycle {
			cycleFields[i] = strconv.Itoa(d)
		}
		if err := os.WriteFile(recurPath, []byte(strings.Join(cycleFields, " ")+"\n0\n"), 0o644); err != nil {
			return err
		}
		tail = strings.NewReplacer(
			"__RECUR_FILE__", recurPath,
			"__TIMER_PATH__", timerPath,
			"__TIMER_NAME__", name+".timer",
		).Replace(cycleRescheduleTail)
	case recurManual:
		// No timer at all — the job runs only when started manually, so
		// there's nothing to schedule and no self-cleanup tail to add.
	default: // recurOneshot
		onCalendar = computeOnCalendar(sched.Minute, sched.Hour, sched.DOM, sched.Month, now)
		tail = fmt.Sprintf(oneshotCleanupTail, timerPath, servicePath)
	}

	script := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\n\nJOB_NAME=%q\n\n%s\n", name, strings.TrimSpace(commands)) + tail
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}

	if notes := strings.TrimSpace(notes); notes != "" {
		if err := os.WriteFile(filepath.Join(dir, name+"-body.txt"), []byte(notes+"\n"), 0o644); err != nil {
			return err
		}
	}

	service := fmt.Sprintf(`[Unit]
Description=%s

[Service]
Type=oneshot
ExecStart=%s
StandardOutput=append:%s
StandardError=append:%s
`, name, scriptPath, logPath, logPath)
	if err := os.WriteFile(servicePath, []byte(service), 0o644); err != nil {
		return err
	}

	timer := fmt.Sprintf(`[Unit]
Description=%s

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, name, onCalendar)
	if sched.Kind != recurManual {
		if err := os.WriteFile(timerPath, []byte(timer), 0o644); err != nil {
			return err
		}
	}

	daemonReload()
	if sched.Kind != recurManual {
		enableNow(name)
	}
	return nil
}

// runJob starts j's service unit right now, bypassing its timer — the
// mechanism behind the TUI's "x run now". Every job has a service unit (a
// manual one has just the service, no timer); starting it runs the script
// immediately, and a one-shot job's script then self-removes its units as
// usual, while recurring/manual jobs stay where they are.
func runJob(j Job) error {
	if j.ServicePath == "" {
		return fmt.Errorf("'%s' has no service unit to start", j.Name)
	}
	out, err := exec.Command("systemctl", "--user", "start", j.Name+".service").CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

// removeJobUnits disables and deletes j's timer/service unit files, if it
// has any — shared by deleteJob and archiveJob, since both need a job to
// stop firing before its files are touched. A manual job has no timer but
// does have a service unit, which must be removed too or it'd be orphaned.
func removeJobUnits(j Job) {
	if j.TimerPath != "" {
		disableNow(j.Name)
		os.Remove(j.TimerPath)
	}
	if j.ServicePath != "" {
		os.Remove(j.ServicePath)
	}
	if j.TimerPath != "" || j.ServicePath != "" {
		daemonReload()
	}
}

func deleteJob(j Job, removeFiles bool) error {
	removeJobUnits(j)
	if removeFiles {
		return os.RemoveAll(j.Dir)
	}
	return nil
}

// archiveDir is computed from jobsDir on every call (rather than cached at
// package-init time) because tests reassign jobsDir directly per fixture.
func archiveDir() string {
	return filepath.Join(jobsDir, ".archive")
}

// archiveJob stops j's schedule (if any) and moves its directory under
// jobsDir/.archive — discoverJobs already skips dot-prefixed directories,
// so an archived job is invisible to the normal scan for free.
func archiveJob(j Job) error {
	removeJobUnits(j)
	if err := os.MkdirAll(archiveDir(), 0o755); err != nil {
		return err
	}
	return os.Rename(j.Dir, filepath.Join(archiveDir(), j.Name))
}

// unarchiveJob moves a job's directory back out of jobsDir/.archive. Its
// timer/service were removed at archive time and are not restored — the job
// reappears in history until rescheduled.
func unarchiveJob(name string) error {
	return os.Rename(filepath.Join(archiveDir(), name), filepath.Join(jobsDir, name))
}

// discoverArchivedJobs mirrors discoverJobs but scans jobsDir/.archive.
// Archived jobs never have a timer/service (removed at archive time), so
// there's no systemctl lookup to do.
func discoverArchivedJobs() []Job {
	var jobs []Job
	dir := archiveDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return jobs
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		jobDir := filepath.Join(dir, name)
		body := globFirst(jobDir, "*body*.txt")
		if body == "" {
			body = globFirst(jobDir, "*.txt")
		}
		jobs = append(jobs, Job{
			Name:   name,
			Dir:    jobDir,
			Script: globFirst(jobDir, "*.sh"),
			Log:    globFirst(jobDir, "*.log"),
			Body:   body,
		})
	}
	return jobs
}

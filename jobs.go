package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	homeDir, _     = os.UserHomeDir()
	jobsDir        = envOr("DJOBS_JOBS_DIR", filepath.Join(homeDir, "jobs"))
	systemdUserDir = envOr("DJOBS_SYSTEMD_DIR", filepath.Join(homeDir, ".config", "systemd", "user"))
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var onCalendarRe = regexp.MustCompile(`(?m)^OnCalendar=(.+)$`)

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
			Name:   name,
			Dir:    dir,
			Script: globFirst(dir, "*.sh"),
			Log:    globFirst(dir, "*.log"),
			Body:   body,
		}

		timerPath := filepath.Join(systemdUserDir, name+".timer")
		if _, err := os.Stat(timerPath); err == nil {
			job.TimerPath = timerPath
			job.OnCalendar = readOnCalendar(timerPath)
			props := timerProperties(name)
			job.UnitFileState = props["UnitFileState"]
			job.NextElapse = props["NextElapseUSecRealtime"]
			job.ServiceActiveState = serviceActiveState(name)
			servicePath := filepath.Join(systemdUserDir, name+".service")
			if _, err := os.Stat(servicePath); err == nil {
				job.ServicePath = servicePath
			}
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

type jobStatusKind int

const (
	statusActive jobStatusKind = iota
	statusPaused
	statusCompleted
	statusFailed
	statusRemoved
)

// Status returns a coarse status kind (for styling) and a plain-text label.
//
// A job's timer/service unit files are only ever deleted by the job
// script's own self-cleanup step, which runs after everything else
// succeeds — so TimerPath's presence isn't just "still scheduled", it's
// also how a failed run is told apart from a completed one: a failure never
// reaches that cleanup line, leaving the units behind exactly like a merely
// paused job would. The one further signal needed is whether the service
// unit actually ran and failed (ActiveState == "failed") versus simply not
// having fired yet.
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
	if j.Log != "" {
		return statusCompleted, "done"
	}
	return statusRemoved, "removed"
}

// IsPending reports whether j still has a live, unresolved schedule —
// i.e. belongs in the "pending" panel rather than "history".
func (j Job) IsPending() bool {
	kind, _ := j.Status()
	return kind == statusActive || kind == statusPaused
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

// createJob writes a new job directory + script, plus its paired systemd
// timer/service units, and enables the timer — the same convention
// discoverJobs expects (see instructions.md). The generated script ends
// with the self-cleanup block that convention relies on: without it, a
// successful run would leave the timer/service files behind and the job
// would never move from "pending" to "history".
func createJob(name, commands, notes string, minute, hour, dom, month int, now time.Time) error {
	if err := validateJobName(name); err != nil {
		return err
	}

	dir := filepath.Join(jobsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	scriptPath := filepath.Join(dir, name+".sh")
	logPath := filepath.Join(dir, name+".log")
	timerPath := filepath.Join(systemdUserDir, name+".timer")
	servicePath := filepath.Join(systemdUserDir, name+".service")

	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

JOB_NAME=%q

%s

# self-remove the systemd unit pair once done (one-shot, not recurring)
systemctl --user disable --now "${JOB_NAME}.timer" 2>/dev/null || true
rm -f %q %q
systemctl --user daemon-reload
`, name, strings.TrimSpace(commands), timerPath, servicePath)
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
`, name, computeOnCalendar(minute, hour, dom, month, now))
	if err := os.WriteFile(timerPath, []byte(timer), 0o644); err != nil {
		return err
	}

	daemonReload()
	enableNow(name)
	return nil
}

func deleteJob(j Job, removeFiles bool) error {
	if j.TimerPath != "" {
		disableNow(j.Name)
		os.Remove(j.TimerPath)
		if j.ServicePath != "" {
			os.Remove(j.ServicePath)
		}
		daemonReload()
	}
	if removeFiles {
		return os.RemoveAll(j.Dir)
	}
	return nil
}

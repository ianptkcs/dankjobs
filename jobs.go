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
	jobsDir        = envOr("JOBS_TUI_JOBS_DIR", filepath.Join(homeDir, "jobs"))
	systemdUserDir = envOr("JOBS_TUI_SYSTEMD_DIR", filepath.Join(homeDir, ".config", "systemd", "user"))
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var onCalendarRe = regexp.MustCompile(`(?m)^OnCalendar=(.+)$`)

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
}

func systemctlUser(args ...string) string {
	out, _ := exec.Command("systemctl", append([]string{"--user"}, args...)...).Output()
	return string(out)
}

func timerProperties(name string) map[string]string {
	out := systemctlUser("show", name+".timer", "--property=ActiveState,UnitFileState,NextElapseUSecRealtime")
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
	statusRanOrRemoved
	statusNone
)

// Status returns a coarse status kind (for styling) and a plain-text label.
func (j Job) Status() (jobStatusKind, string) {
	if j.TimerPath == "" {
		if j.Log != "" {
			return statusRanOrRemoved, "rodou/removido"
		}
		return statusNone, "sem timer"
	}
	if j.Enabled() {
		return statusActive, "ativo"
	}
	return statusPaused, "pausado"
}

// DetailText renders the job's detail panel body. statusLabel is injected
// pre-styled by the caller so this stays presentation-agnostic otherwise.
func (j Job) DetailText(statusLabel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (%s)\n", j.Name, j.Dir)

	if j.TimerPath != "" {
		fmt.Fprintf(&b, "\ntimer: %s  [%s]\n", j.OnCalendar, statusLabel)
		if j.NextElapse != "" {
			fmt.Fprintf(&b, "próxima execução: %s\n", j.NextElapse)
		}
		if j.Script != "" {
			fmt.Fprintf(&b, "comando: %s\n", j.Script)
		}
	} else {
		fmt.Fprintf(&b, "\nstatus: %s\n", statusLabel)
	}

	if j.Body != "" {
		if data, err := os.ReadFile(j.Body); err == nil {
			fmt.Fprintf(&b, "\n--- corpo/notas ---\n%s\n", strings.TrimSpace(string(data)))
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
			fmt.Fprintf(&b, "\n--- log (fim) ---\n%s\n", strings.Join(lines, "\n"))
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

// rescheduleJob rewrites the OnCalendar= line of j's timer unit. Year rolls
// over to next year if the given month/day already passed this year,
// mirroring how a one-shot job's date is normally picked interactively.
func rescheduleJob(j Job, minute, hour, dom, month int, now time.Time) error {
	year := now.Year()
	todayMonth, todayDay := int(now.Month()), now.Day()
	if month < todayMonth || (month == todayMonth && dom < todayDay) {
		year++
	}
	onCalendar := fmt.Sprintf("%04d-%02d-%02d %02d:%02d:00", year, month, dom, hour, minute)

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

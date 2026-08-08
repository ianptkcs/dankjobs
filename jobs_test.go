package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupFixture points jobsDir at a throwaway temp dir but keeps
// systemdUserDir pointed at the real systemd --user unit directory, since
// systemctl only ever looks there. The fixture unit name is unique per test
// run and is torn down in t.Cleanup regardless of pass/fail, so this never
// touches a real scheduled job.
func setupFixture(t *testing.T, name string) Job {
	t.Helper()
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir

	dir := filepath.Join(jobsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(systemdUserDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".sh"), []byte("#!/usr/bin/env bash\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+"-body.txt"), []byte("fixture body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	serviceUnit := filepath.Join(systemdUserDir, name+".service")
	timerUnit := filepath.Join(systemdUserDir, name+".timer")
	serviceContent := "[Unit]\nDescription=" + name + " (test fixture)\n\n[Service]\nType=oneshot\nExecStart=/bin/true\n"
	timerContent := "[Unit]\nDescription=" + name + " (schedule)\n\n[Timer]\nOnCalendar=2099-01-01 00:00:00\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n"
	if err := os.WriteFile(serviceUnit, []byte(serviceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(timerUnit, []byte(timerContent), 0o644); err != nil {
		t.Fatal(err)
	}
	systemctlUser("daemon-reload")
	systemctlUser("enable", "--now", name+".timer")

	t.Cleanup(func() {
		systemctlUser("disable", "--now", name+".timer")
		os.Remove(timerUnit)
		os.Remove(serviceUnit)
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	jobs := discoverJobs()
	for _, j := range jobs {
		if j.Name == name {
			return j
		}
	}
	t.Fatalf("fixture job %q not found after setup", name)
	return Job{}
}

func hasRealSystemd(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not available")
	}
}

func TestDiscoverAndToggle(t *testing.T) {
	hasRealSystemd(t)
	job := setupFixture(t, "zz-jobs-tui-test-toggle")

	if job.TimerPath == "" {
		t.Fatal("expected fixture job to have a timer")
	}
	if !job.Enabled() {
		t.Fatal("expected fixture job to start enabled")
	}

	toggleJob(job)
	jobs := discoverJobs()
	after := findJob(jobs, job.Name)
	if after.Enabled() {
		t.Fatal("expected job to be disabled after one toggle")
	}

	toggleJob(after)
	jobs = discoverJobs()
	after = findJob(jobs, job.Name)
	if !after.Enabled() {
		t.Fatal("expected job to be enabled again after a second toggle")
	}
}

func TestReschedule(t *testing.T) {
	hasRealSystemd(t)
	job := setupFixture(t, "zz-jobs-tui-test-reschedule")

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	if err := rescheduleJob(job, 30, 9, 15, 6, now); err != nil {
		t.Fatal(err)
	}

	jobs := discoverJobs()
	after := findJob(jobs, job.Name)
	want := "2026-06-15 09:30:00"
	if after.OnCalendar != want {
		t.Fatalf("OnCalendar = %q, want %q", after.OnCalendar, want)
	}

	// A date already past this year should roll over to next year.
	if err := rescheduleJob(after, 0, 0, 1, 1, now); err != nil {
		t.Fatal(err)
	}
	jobs = discoverJobs()
	after = findJob(jobs, job.Name)
	want = "2027-01-01 00:00:00"
	if after.OnCalendar != want {
		t.Fatalf("OnCalendar = %q, want %q", after.OnCalendar, want)
	}
}

func TestDeleteScheduleOnly(t *testing.T) {
	hasRealSystemd(t)
	job := setupFixture(t, "zz-jobs-tui-test-delete-cron")

	if err := deleteJob(job, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job.Dir); err != nil {
		t.Fatal("job dir should still exist when only the schedule is deleted")
	}
	if _, err := os.Stat(job.TimerPath); !os.IsNotExist(err) {
		t.Fatal("timer unit should have been removed")
	}
}

func TestDeleteWithFiles(t *testing.T) {
	hasRealSystemd(t)
	job := setupFixture(t, "zz-jobs-tui-test-delete-all")

	if err := deleteJob(job, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job.Dir); !os.IsNotExist(err) {
		t.Fatal("job dir should have been removed")
	}
}

func TestCreateJob(t *testing.T) {
	hasRealSystemd(t)
	name := "zz-jobs-tui-test-create"

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir
	t.Cleanup(func() {
		systemctlUser("disable", "--now", name+".timer")
		os.Remove(filepath.Join(systemdUserDir, name+".timer"))
		os.Remove(filepath.Join(systemdUserDir, name+".service"))
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	sched := jobSchedule{Kind: recurOneshot, Minute: 30, Hour: 9, DOM: 15, Month: 6}
	if err := createJob(name, "echo hello", "some notes", sched, now); err != nil {
		t.Fatal(err)
	}

	jobs := discoverJobs()
	job := findJob(jobs, name)
	if job.TimerPath == "" {
		t.Fatal("expected created job to have a timer")
	}
	if !job.Enabled() {
		t.Fatal("expected created job to start enabled")
	}
	if want := "2026-06-15 09:30:00"; job.OnCalendar != want {
		t.Fatalf("OnCalendar = %q, want %q", job.OnCalendar, want)
	}
	if job.Body == "" {
		t.Fatal("expected notes to produce a body file")
	}

	data, err := os.ReadFile(job.Script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "echo hello") {
		t.Fatal("expected script to contain the provided command")
	}
	if !strings.Contains(string(data), "disable --now") {
		t.Fatal("expected script to contain the self-cleanup block")
	}

	if err := createJob(name, "echo hello again", "", sched, now); err == nil {
		t.Fatal("expected creating a duplicate-named job to fail")
	}
}

func TestIsRecurring(t *testing.T) {
	cases := []struct {
		name string
		job  Job
		want bool
	}{
		{"no timer at all", Job{}, false},
		{"oneshot absolute date", Job{TimerPath: "x", OnCalendar: "2026-08-05 14:00:00"}, false},
		{"daily", Job{TimerPath: "x", OnCalendar: "*-*-* 09:00:00"}, true},
		{"weekly", Job{TimerPath: "x", OnCalendar: "Mon,Wed,Fri *-*-* 09:00:00"}, true},
		{"monthly", Job{TimerPath: "x", OnCalendar: "*-*-15 09:00:00"}, true},
		{"custom cycle (looks like a oneshot date, but has a .recur sidecar)",
			Job{TimerPath: "x", OnCalendar: "2026-08-05 14:00:00", RecurCyclePath: "/tmp/whatever.recur"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.job.IsRecurring(); got != c.want {
				t.Fatalf("IsRecurring() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsManual(t *testing.T) {
	cases := []struct {
		name string
		job  Job
		want bool
	}{
		{"no units at all", Job{}, false},
		{"scheduled (timer + service)", Job{TimerPath: "x", ServicePath: "y"}, false},
		{"manual (service only)", Job{ServicePath: "y"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.job.IsManual(); got != c.want {
				t.Fatalf("IsManual() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestManualJobStatus(t *testing.T) {
	job := Job{ServicePath: "/tmp/x.service"}
	kind, label := job.Status()
	if kind != statusManual || label != "manual" {
		t.Fatalf("Status() = %v/%q, want manual", kind, label)
	}
	if !job.IsPending() {
		t.Fatal("manual job should be pending (actionable)")
	}
	if job.IsRecurring() {
		t.Fatal("manual job must not be recurring")
	}
}

func TestCreateManualJob(t *testing.T) {
	hasRealSystemd(t)
	name := "zz-jobs-tui-test-manual"

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir
	t.Cleanup(func() {
		os.Remove(filepath.Join(systemdUserDir, name+".service"))
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	sched := jobSchedule{Kind: recurManual}
	if err := createJob(name, "echo manual", "manual notes", sched, time.Now()); err != nil {
		t.Fatal(err)
	}

	job := findJob(discoverJobs(), name)
	if job.TimerPath != "" {
		t.Fatal("manual job must not have a timer")
	}
	if job.ServicePath == "" {
		t.Fatal("manual job must have a service unit")
	}
	if !job.IsManual() {
		t.Fatal("expected IsManual() = true")
	}
	kind, label := job.Status()
	if kind != statusManual || label != "manual" {
		t.Fatalf("Status() = %v/%q, want manual", kind, label)
	}
	if !job.IsPending() {
		t.Fatal("manual job should appear in the pending panel")
	}
	if job.Body == "" {
		t.Fatal("expected notes to produce a body file")
	}
}

func TestRunManualJob(t *testing.T) {
	hasRealSystemd(t)
	name := "zz-jobs-tui-test-run-manual"

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir
	marker := filepath.Join(t.TempDir(), "marker.txt")
	t.Cleanup(func() {
		os.Remove(filepath.Join(systemdUserDir, name+".service"))
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	sched := jobSchedule{Kind: recurManual}
	if err := createJob(name, "echo ran > "+marker, "", sched, time.Now()); err != nil {
		t.Fatal(err)
	}

	job := findJob(discoverJobs(), name)
	if err := runJob(job); err != nil {
		t.Fatalf("runJob: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not produce its marker file within 10s")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestRunJobWithoutService(t *testing.T) {
	job := Job{Name: "no-unit", Dir: "/tmp/nope"}
	if err := runJob(job); err == nil {
		t.Fatal("expected runJob to fail for a job without a service unit")
	}
}

func TestDeleteManualJobRemovesService(t *testing.T) {
	hasRealSystemd(t)
	name := "zz-jobs-tui-test-delete-manual"

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir
	t.Cleanup(func() {
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	sched := jobSchedule{Kind: recurManual}
	if err := createJob(name, "echo manual", "", sched, time.Now()); err != nil {
		t.Fatal(err)
	}
	job := findJob(discoverJobs(), name)

	if err := deleteJob(job, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(systemdUserDir, name+".service")); !os.IsNotExist(err) {
		t.Fatal("manual job's service unit should be removed on delete")
	}
}

func TestComputeRecurringOnCalendar(t *testing.T) {
	if got, want := computeRecurringOnCalendar(recurDaily, nil, 0, 9, 30), "*-*-* 09:30:00"; got != want {
		t.Fatalf("daily = %q, want %q", got, want)
	}
	// Weekdays given out of order should still render Mon..Sun.
	weekdays := []time.Weekday{time.Friday, time.Monday, time.Wednesday}
	if got, want := computeRecurringOnCalendar(recurWeekly, weekdays, 0, 9, 0), "Mon,Wed,Fri *-*-* 09:00:00"; got != want {
		t.Fatalf("weekly = %q, want %q", got, want)
	}
	if got, want := computeRecurringOnCalendar(recurMonthly, nil, 15, 9, 0), "*-*-15 09:00:00"; got != want {
		t.Fatalf("monthly = %q, want %q", got, want)
	}
}

func TestScheduleHuman(t *testing.T) {
	tests := []struct {
		onCalendar string
		want       string
	}{
		{"", "—"},
		// One-shot absolute timestamp.
		{"2026-08-14 09:00:00", "14/08 09:00"},
		// Daily recurring.
		{"*-*-* 09:30:00", "09:30"},
		// Weekly recurring.
		{"Mon,Tue *-*-* 09:00:00", "Mon,Tue 09:00"},
		{"Mon,Wed,Fri *-*-* 14:15:00", "Mon,Wed,Fri 14:15"},
		// Monthly recurring.
		{"*-*-15 09:00:00", "dia 15 09:00"},
		// Unknown pattern falls through to raw string.
		{"garbage", "garbage"},
	}
	for _, tt := range tests {
		j := Job{OnCalendar: tt.onCalendar}
		if got := j.ScheduleHuman(); got != tt.want {
			t.Errorf("ScheduleHuman(%q) = %q, want %q", tt.onCalendar, got, tt.want)
		}
	}
}

func TestArchiveAndUnarchive(t *testing.T) {
	hasRealSystemd(t)
	job := setupFixture(t, "zz-jobs-tui-test-archive")

	if err := archiveJob(job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(job.Dir); !os.IsNotExist(err) {
		t.Fatal("job dir should have moved out of jobsDir")
	}
	if _, err := os.Stat(job.TimerPath); !os.IsNotExist(err) {
		t.Fatal("timer unit should have been removed")
	}
	if findJob(discoverJobs(), job.Name).Name != "" {
		t.Fatal("archived job should not appear in discoverJobs")
	}
	if findJob(discoverArchivedJobs(), job.Name).Name == "" {
		t.Fatal("archived job should appear in discoverArchivedJobs")
	}

	if err := unarchiveJob(job.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(jobsDir, job.Name)); err != nil {
		t.Fatal("job dir should be back under jobsDir after unarchive")
	}
	if findJob(discoverArchivedJobs(), job.Name).Name != "" {
		t.Fatal("unarchived job should no longer appear in discoverArchivedJobs")
	}
}

func findJob(jobs []Job, name string) Job {
	for _, j := range jobs {
		if j.Name == name {
			return j
		}
	}
	return Job{}
}

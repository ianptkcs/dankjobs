package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

func findJob(jobs []Job, name string) Job {
	for _, j := range jobs {
		if j.Name == name {
			return j
		}
	}
	return Job{}
}

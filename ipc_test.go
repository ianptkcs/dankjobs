package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIPCJobsList(t *testing.T) {
	hasRealSystemd(t)
	active := setupFixture(t, "zz-jobs-tui-test-ipc-list-active")

	captureStdout := func(f func() int) (string, int) {
		orig := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w
		code := f()
		w.Close()
		os.Stdout = orig
		var buf bytes.Buffer
		buf.ReadFrom(r)
		return buf.String(), code
	}

	out, code := captureStdout(func() int { return ipcJobsList(map[string]string{"pending": "true"}) })
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var jobs []jobJSON
	if err := json.Unmarshal([]byte(out), &jobs); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if !containsName(jobs, active.Name) {
		t.Fatalf("expected %q in pending=true results, got %+v", active.Name, jobs)
	}

	out, code = captureStdout(func() int { return ipcJobsList(map[string]string{"pending": "false"}) })
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if err := json.Unmarshal([]byte(out), &jobs); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if containsName(jobs, active.Name) {
		t.Fatalf("did not expect %q in pending=false results, got %+v", active.Name, jobs)
	}
}

// TestIPCJobsNext creates three fixture jobs sharing a single jobsDir
// override — unlike setupFixture (which repoints jobsDir at a fresh temp
// dir on every call, so calling it more than once per test only leaves the
// last job visible to discoverJobs), createJob writes into whatever jobsDir
// is already set, so all three coexist.
func TestIPCJobsNext(t *testing.T) {
	hasRealSystemd(t)
	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	realSystemdDir := filepath.Join(realHome, ".config", "systemd", "user")

	origJobsDir, origSystemdDir := jobsDir, systemdUserDir
	jobsDir = t.TempDir()
	systemdUserDir = realSystemdDir

	names := []string{
		"zz-jobs-tui-test-ipc-next-paused",
		"zz-jobs-tui-test-ipc-next-soon",
		"zz-jobs-tui-test-ipc-next-later",
	}
	t.Cleanup(func() {
		for _, name := range names {
			systemctlUser("disable", "--now", name+".timer")
			os.Remove(filepath.Join(systemdUserDir, name+".timer"))
			os.Remove(filepath.Join(systemdUserDir, name+".service"))
		}
		systemctlUser("daemon-reload")
		jobsDir, systemdUserDir = origJobsDir, origSystemdDir
	})

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	if err := createJob(names[0], "true", "", 0, 9, 10, 3, now); err != nil {
		t.Fatal(err)
	}
	if err := createJob(names[1], "true", "", 0, 9, 5, 3, now); err != nil {
		t.Fatal(err)
	}
	if err := createJob(names[2], "true", "", 0, 9, 20, 3, now); err != nil {
		t.Fatal(err)
	}

	paused := findJob(discoverJobs(), names[0])
	toggleJob(paused) // now disabled — must be ignored by jobs.next
	soon := names[1]

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := ipcJobsNext()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	buf.ReadFrom(r)

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var got jobJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if got.Name != soon {
		t.Fatalf("jobs.next = %q, want %q (soonest active job)", got.Name, soon)
	}
}

func TestRunIPCErrors(t *testing.T) {
	if code := runIPC([]string{}); code != 1 {
		t.Fatalf("empty args: exit code = %d, want 1", code)
	}
	if code := runIPC([]string{"jobs.list"}); code != 1 {
		t.Fatalf("missing --json: exit code = %d, want 1", code)
	}
	if code := runIPC([]string{"not.a.method", "--json"}); code != 1 {
		t.Fatalf("unknown method: exit code = %d, want 1", code)
	}
	if code := runIPC([]string{"jobs.list", "not-a-kv", "--json"}); code != 1 {
		t.Fatalf("invalid filter arg: exit code = %d, want 1", code)
	}
}

func containsName(jobs []jobJSON, name string) bool {
	for _, j := range jobs {
		if j.Name == name {
			return true
		}
	}
	return false
}

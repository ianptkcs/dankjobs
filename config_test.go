package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeConfigFixture points XDG_CONFIG_HOME at a temp dir and writes
// config.toml into ~/.config/tjobs. An empty body leaves the file out.
func writeConfigFixture(t *testing.T, body string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	cfg = nil
	settings = defaultConfig()

	dir := filepath.Join(base, "tjobs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestLoadSettingsWithoutFileUsesDefaults(t *testing.T) {
	writeConfigFixture(t, "")
	if err := loadSettings(); err != nil {
		t.Fatalf("loadSettings() = %v, want nil", err)
	}
	if jobsRowPercent() != 45 || maxVisibleRows() != 8 || minPanelInnerWidth() != 20 {
		t.Fatalf("layout = %+v, want the defaults", settings.Layout)
	}
	if settings.Timing.RunNowReloadDelay.Duration != 3*time.Second {
		t.Fatalf("RunNowReloadDelay = %v, want 3s", settings.Timing.RunNowReloadDelay.Duration)
	}
}

func TestLoadSettingsOverridesOnlyWhatFileSets(t *testing.T) {
	writeConfigFixture(t, "[layout]\nmax_visible_rows = 20\n")
	if err := loadSettings(); err != nil {
		t.Fatal(err)
	}
	if maxVisibleRows() != 20 {
		t.Fatalf("maxVisibleRows() = %d, want 20", maxVisibleRows())
	}
	// Untouched keys keep their defaults.
	if jobsRowPercent() != 45 {
		t.Fatalf("jobsRowPercent() = %d, want the default 45", jobsRowPercent())
	}
	if settings.Timing.RunNowReloadDelay.Duration != 3*time.Second {
		t.Fatalf("RunNowReloadDelay = %v, want the default 3s", settings.Timing.RunNowReloadDelay.Duration)
	}
}

func TestRunNowReloadDelayParsesDuration(t *testing.T) {
	writeConfigFixture(t, "[timing]\nrun_now_reload_delay = \"500ms\"\n")
	if err := loadSettings(); err != nil {
		t.Fatal(err)
	}
	if got := settings.Timing.RunNowReloadDelay.Duration; got != 500*time.Millisecond {
		t.Fatalf("RunNowReloadDelay = %v, want 500ms", got)
	}
}

// The status-cell decorator derives byte offsets from the column widths, so a
// width narrower than its own header text would desynchronize what it paints
// from what's rendered. normalize floors them instead.
func TestNormalizeFloorsColumnWidthsAtHeaderText(t *testing.T) {
	got := normalize(config{Layout: layoutConfig{ScheduleColWidth: 3, StatusColWidth: 1}})
	if got.Layout.ScheduleColWidth != 13 {
		t.Fatalf("ScheduleColWidth = %d, want the 13 that fits \"scheduled for\"", got.Layout.ScheduleColWidth)
	}
	if got.Layout.StatusColWidth != 8 {
		t.Fatalf("StatusColWidth = %d, want the 8 that fits \"removed\"", got.Layout.StatusColWidth)
	}

	// Wider than the default is fine — only narrower is clamped.
	wide := normalize(config{Layout: layoutConfig{ScheduleColWidth: 30, StatusColWidth: 12}})
	if wide.Layout.ScheduleColWidth != 30 || wide.Layout.StatusColWidth != 12 {
		t.Fatalf("layout = %+v, want the wider values kept", wide.Layout)
	}
}

func TestNormalizeClampsOutOfRangeLayout(t *testing.T) {
	got := normalize(config{Layout: layoutConfig{JobsRowPercent: 150, MaxVisibleRows: 0, MinPanelWidth: 0}})
	if got.Layout.JobsRowPercent != 45 {
		t.Fatalf("JobsRowPercent = %d, want the default 45", got.Layout.JobsRowPercent)
	}
	if got.Layout.MaxVisibleRows != 8 {
		t.Fatalf("MaxVisibleRows = %d, want the default 8", got.Layout.MaxVisibleRows)
	}
	if got.Layout.MinPanelWidth != 20 {
		t.Fatalf("MinPanelWidth = %d, want the default 20", got.Layout.MinPanelWidth)
	}
}

func TestReloadSettingsKeepsValuesOnMalformedFile(t *testing.T) {
	path := writeConfigFixture(t, "[layout]\nmax_visible_rows = 15\n")
	if err := loadSettings(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("[layout\nmax_visible"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reloadSettings(); err == nil {
		t.Fatal("reloadSettings() on malformed TOML = nil, want a parse error")
	}
	if maxVisibleRows() != 15 {
		t.Fatalf("maxVisibleRows() = %d, want the previous 15", maxVisibleRows())
	}
}

func TestReloadSettingsPicksUpExternalEdit(t *testing.T) {
	path := writeConfigFixture(t, "[layout]\njobs_row_percent = 30\n")
	if err := loadSettings(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("[layout]\njobs_row_percent = 60\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := reloadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("reloadSettings() should report a change")
	}
	if jobsRowPercent() != 60 {
		t.Fatalf("jobsRowPercent() = %d, want 60", jobsRowPercent())
	}
}

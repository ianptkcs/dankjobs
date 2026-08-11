package main

import (
	"time"

	"github.com/ianptkcs/tabelatuiui"
)

// config is tjobs' settings schema, read from ~/.config/tjobs/config.toml
// (the config dir follows the binary name, same as keybindings.json). Every
// field falls back to defaultConfig when the file leaves it out.
//
// TJOBS_JOBS_DIR and TJOBS_SYSTEMD_DIR stay env vars deliberately: they point
// at system locations, not user preferences, and tests swap them per fixture.
type config struct {
	Layout layoutConfig `toml:"layout"`
	Timing timingConfig `toml:"timing"`
}

type layoutConfig struct {
	// ScheduleColWidth and StatusColWidth are content widths, before
	// bubbles/table's Padding(0,1) adds 2. The status-cell decorator derives
	// its byte offsets from these, so normalize floors them at the width of
	// the header text they were sized for.
	ScheduleColWidth int `toml:"schedule_col_width"`
	StatusColWidth   int `toml:"status_col_width"`
	// JobsRowPercent is the share of body height given to the
	// recurring/pending/history row.
	JobsRowPercent int `toml:"jobs_row_percent"`
	// MaxVisibleRows caps rows per side panel regardless of terminal height,
	// keeping the dashboard compact in a tall terminal.
	MaxVisibleRows int `toml:"max_visible_rows"`
	MinPanelWidth  int `toml:"min_panel_width"`
}

type timingConfig struct {
	// RunNowReloadDelay is how long to wait after starting a job before
	// reloading the list, so systemd has time to report the new state.
	RunNowReloadDelay duration `toml:"run_now_reload_delay"`
}

// duration wraps time.Duration so TOML can express it as "3s" instead of a
// raw nanosecond count.
type duration struct{ time.Duration }

func (d *duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func defaultConfig() config {
	return config{
		Layout: layoutConfig{
			ScheduleColWidth: 13, // "scheduled for"
			StatusColWidth:   8,  // "removed" is the longest status label
			JobsRowPercent:   45,
			MaxVisibleRows:   8,
			MinPanelWidth:    20,
		},
		Timing: timingConfig{RunNowReloadDelay: duration{3 * time.Second}},
	}
}

// configPath is resolved lazily, not in a package-level var: an init-time var
// would freeze XDG_CONFIG_HOME before main (or a test) could set it.
func configPath() string { return tuiui.ConfigPath("tjobs", "config.toml") }

var cfg *tuiui.Config[config]

// settings is the normalized snapshot the app renders from.
var settings = defaultConfig()

// normalize floors the values the table layout can't survive. The column
// widths are the sharp edge: recurringStatusOffset and friends are computed
// arithmetically from them and fed to decorateTable, so a width narrower than
// its own header text would desynchronize the decorator's byte offsets from
// what's actually rendered.
func normalize(c config) config {
	d := defaultConfig()
	if c.Layout.ScheduleColWidth < d.Layout.ScheduleColWidth {
		c.Layout.ScheduleColWidth = d.Layout.ScheduleColWidth
	}
	if c.Layout.StatusColWidth < d.Layout.StatusColWidth {
		c.Layout.StatusColWidth = d.Layout.StatusColWidth
	}
	if c.Layout.JobsRowPercent < 10 || c.Layout.JobsRowPercent > 90 {
		c.Layout.JobsRowPercent = d.Layout.JobsRowPercent
	}
	if c.Layout.MaxVisibleRows < 1 {
		c.Layout.MaxVisibleRows = d.Layout.MaxVisibleRows
	}
	if c.Layout.MinPanelWidth < 1 {
		c.Layout.MinPanelWidth = d.Layout.MinPanelWidth
	}
	if c.Timing.RunNowReloadDelay.Duration <= 0 {
		c.Timing.RunNowReloadDelay = d.Timing.RunNowReloadDelay
	}
	return c
}

// loadSettings reads config.toml once at startup. A missing file is not an
// error — the app runs on defaults.
func loadSettings() error {
	cfg = tuiui.NewConfig(configPath(), defaultConfig())
	err := cfg.Load()
	settings = normalize(cfg.Get())
	return err
}

// reloadSettings re-reads config.toml, reporting whether anything changed. On
// a parse error Config keeps the previous values, so settings stays valid.
func reloadSettings() (bool, error) {
	if cfg == nil {
		return true, loadSettings()
	}
	changed, err := cfg.Reload()
	settings = normalize(cfg.Get())
	return changed, err
}

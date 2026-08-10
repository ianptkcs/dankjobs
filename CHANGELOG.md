# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-08-09

### Added

- DMS plugin: a settings panel (Max name width + Refresh interval). Max
  name width caps how wide a job name can get in the bar pill before it's
  elided, so long names no longer stretch the pill.

- Multi-select: `space` marks/unmarks the job under the cursor, with the
  marked count shown in the panel title. A marked row is highlighted
  (bold, tinted background, `*` before the name). `d` then archives or
  deletes forever all marked jobs in that panel at once (still behind the
  confirmation modal); `esc` clears the marks. Marks are scoped per panel,
  so they never leak across recurring/pending/history.
- Bulk shortcuts without marking: `A` archives every job in the focused
  panel and `D` deletes them all forever — both preselect their choice in
  the same confirmation modal, so a single Enter confirms.

### Fixed

- Status column colors never applied after the 0.2.2 name-cell matching
  rework: bubbles pads every cell with a leading space and the pending
  panel's middle column is wider than the history one, both of which broke
  the name lookup (an unmarked row's status rendered plain). Matching now
  trims the cell padding and uses the panel's real middle-column width, and
  tolerates a narrow panel clipping the status cell's right padding.

## [0.2.2] - 2026-08-09

### Fixed

- History panel: status column colors (and the selection highlight) broke when
  scrolling past the visible window with `j`/`k`. `colorizeStatusColumn` mapped
  each rendered line to a job by line position, but `bubbles/table` scrolls an
  internal viewport window that isn't derivable from its public API, so colors
  drifted once the cursor left the first page. It now matches each line back to
  its job by the displayed name cell, which is exact even for truncated names —
  this also fixes the same latent bug in the recurring and pending panels.

## [0.2.1] - 2026-08-08

### Fixed

- DMS plugin popup: date+time of pending jobs was truncated. `ScheduleHuman()`
  now returns short human-friendly strings for recurring schedules (e.g.
  `Mon,Tue 09:00` instead of `Mon,Tue *-*-* 09:00:00`), and the plugin widget
  uses `RowLayout` so the status text always fits.

## [0.2.0] - 2026-08-05

### Added

- Manual jobs: creating a job with the new "Manual (no schedule)" recurrence
  writes the script + service unit but no timer, so it stays in the pending
  panel (status "manual") instead of ever firing on its own.
- `x` runs any job now: starts the job's service unit immediately, bypassing
  its timer. Works for scheduled jobs too — a one-shot run finishes and
  self-removes as usual, recurring/manual jobs stay in place.

## [0.2.0-beta.2] - 2026-08-01

### Changed

- Side panels (recurring/pending/history) now split the row equally
  (1:1:1), instead of favoring pending (2:3:2).

## [0.2.0-beta.1] - 2026-08-01

### Added

- Recurring jobs: Daily, Weekly (specific weekdays), Monthly (day of month),
  and a custom day-interval cycle (e.g. "2 4 5"), each with their own
  panel — a recurring job's timer keeps firing instead of self-removing.
- Archiving: `d` now offers Archive as an alternative to permanent deletion,
  moving a job under `~/jobs/.archive/<name>/`; `A` toggles the history
  panel into an archived view, `u` unarchives.
- A hard 8-row cap per side panel, so the dashboard stays compact regardless
  of terminal height.
- `recurring` field on `djobs ipc jobs.list` JSON output.
- Tagging a `-beta.N`/`-rc.N` version now publishes a GitHub prerelease
  instead of a "Latest" release.

### Fixed

- `djobs ipc jobs.next` no longer considers recurring jobs as candidates —
  their `OnCalendar` isn't a comparable absolute timestamp.

## [0.1.0] - 2026-08-01

### Added

- Initial release: TUI for browsing and managing one-shot jobs scheduled as
  systemd user timers, with pending/history/details panels, job creation,
  rescheduling, pause/resume, and deletion.
- `djobs ipc jobs.list` / `jobs.next` non-interactive subcommand for scripting
  and status-bar widgets.
- DankMaterialShell accent-color integration, with `DJOBS_ACCENT` fallback.

[0.3.0]: https://github.com/ianptkcs/djobs/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/ianptkcs/djobs/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/ianptkcs/djobs/compare/v0.2.0-beta.2...v0.2.1
[0.2.0-beta.2]: https://github.com/ianptkcs/djobs/compare/v0.2.0-beta.1...v0.2.0-beta.2
[0.2.0-beta.1]: https://github.com/ianptkcs/djobs/compare/v0.1.0...v0.2.0-beta.1
[0.1.0]: https://github.com/ianptkcs/djobs/releases/tag/v0.1.0

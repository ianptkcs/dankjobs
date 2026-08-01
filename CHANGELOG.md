# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/ianptkcs/djobs/compare/v0.2.0-beta.1...HEAD
[0.2.0-beta.1]: https://github.com/ianptkcs/djobs/compare/v0.1.0...v0.2.0-beta.1
[0.1.0]: https://github.com/ianptkcs/djobs/releases/tag/v0.1.0

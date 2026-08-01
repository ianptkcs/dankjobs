# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-08-01

### Added

- Initial release: TUI for browsing and managing one-shot jobs scheduled as
  systemd user timers, with pending/history/details panels, job creation,
  rescheduling, pause/resume, and deletion.
- `djobs ipc jobs.list` / `jobs.next` non-interactive subcommand for scripting
  and status-bar widgets.
- DankMaterialShell accent-color integration, with `DJOBS_ACCENT` fallback.

[Unreleased]: https://github.com/ianptkcs/djobs/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ianptkcs/djobs/releases/tag/v0.1.0

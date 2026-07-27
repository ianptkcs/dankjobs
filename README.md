# jobs-tui

A [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI for browsing and
managing one-shot "jobs" scheduled as systemd user timers — a lightweight
pattern for deferring CLI/git work (finish this branch tonight, open that PR
tomorrow at 9am) without needing a full task queue.

Styled with the official [Catppuccin](https://github.com/catppuccin/catppuccin)
Mocha palette, following the project's
[semantic color guidelines](https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md) —
red for errors, yellow for paused/warning states, green for active ones, mauve
as the primary accent — via [`catppuccin/go`](https://github.com/catppuccin/go)
for the panels/table and [`huh`](https://github.com/charmbracelet/huh)'s
built-in Catppuccin theme for the reschedule/delete dialogs. Layout is
patterned after [dgop](https://github.com/AvengeMedia/dgop)'s bordered-panel
Bubble Tea style.

![screenshot](screenshot.png)

## The job convention

Each job lives in its own directory under `~/jobs/<name>/`:

- `<name>.sh` — the script that does the actual work
- `<name>.log` — output log (populated by the systemd service's stdout/stderr redirect)
- `<name>*body*.txt` — optional freeform notes (e.g. a PR body) shown in the detail panel

Scheduling is a pair of **systemd user units**, `~/.config/systemd/user/<name>.timer`
and `<name>.service`, using `OnCalendar=` + `Persistent=true` — so a run missed
because the machine was asleep or off fires as soon as it's back, unlike plain cron.

jobs-tui discovers a job whenever a `~/jobs/<name>/` directory and a `<name>.timer`
unit share the same name — no extra tagging required. A job with no matching timer
(already run, or never scheduled) still shows up if it has a log or script, just
without a schedule.

```
~/jobs/kiwi-pr/
  kiwi-pr.sh
  kiwi-pr.log
  kiwi-pr-body.txt

~/.config/systemd/user/
  kiwi-pr.timer
  kiwi-pr.service
```

`JOBS_TUI_JOBS_DIR` / `JOBS_TUI_SYSTEMD_DIR` override the two directories
above, if you want to point jobs-tui somewhere other than `~/jobs` and
`~/.config/systemd/user`.

## Install

Requires Go 1.23+.

```bash
git clone https://github.com/ianptkcs/jobs-tui.git
cd jobs-tui
go build -o jobs-tui .
```

Drop the resulting binary on your `PATH`.

## Usage

```bash
./jobs-tui
```

| Key                 | Action                              |
| ------------------- | ------------------------------------ |
| `e`                 | Reschedule the selected job          |
| `t`                 | Pause / resume its timer             |
| `d`                 | Delete (schedule only, or + files)   |
| `r`                 | Refresh the list                     |
| `q`                 | Quit                                  |
| `j`/`k`, `↓`/`↑`     | Move down/up (bubbles/table default) |
| `g`/`G`             | Jump to first/last job                |
| `ctrl+u`/`ctrl+d`   | Half-page up/down                     |

`u`/`d` alone would also be half-page up/down in a plain bubbles table, but
`d` is remapped to delete here — use `ctrl+d` for half-page-down instead.

## Development

```bash
go test ./...
```

Job discovery, toggling, rescheduling and deletion are covered against a
disposable fixture job + systemd timer that each test creates and tears down
itself (`jobs_test.go`) — it never touches a real scheduled job.

## License

MIT — see [LICENSE](LICENSE).

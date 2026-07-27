# jobs-tui

A [Textual](https://github.com/Textualize/textual) TUI for browsing and managing
one-shot "jobs" scheduled as systemd user timers — a lightweight pattern for
deferring CLI/git work (finish this branch tonight, open that PR tomorrow at 9am)
without needing a full task queue.

Styled with the official [Catppuccin](https://github.com/catppuccin/catppuccin)
palette (Mocha by default, plus Latte/Frappé/Macchiato) following the project's
[semantic color guidelines](https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md) —
red for errors, yellow for paused/warning states, green for active ones, mauve as
the primary accent.

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

## Install

Requires Python 3.11+.

```bash
git clone https://github.com/ianptkcs/jobs-tui.git
cd jobs-tui
python3 -m venv .venv
.venv/bin/pip install textual
```

Optionally, drop a launcher on your `PATH`:

```bash
#!/usr/bin/env bash
exec /path/to/jobs-tui/.venv/bin/python /path/to/jobs-tui/app.py
```

## Usage

```bash
.venv/bin/python app.py
```

| Key      | Action                          |
| -------- | -------------------------------- |
| `e`      | Reschedule the selected job      |
| `t`      | Pause / resume its timer         |
| `d`      | Delete (schedule only, or + files) |
| `r`      | Refresh the list                 |
| `q`      | Quit                              |
| `ctrl+p` | Command palette — switch theme, among other things |

## Development

```bash
.venv/bin/python smoke_test.py
```

Runs an end-to-end [Textual pilot](https://textual.textualize.io/guide/testing/)
smoke test against a disposable fixture job + systemd timer created and torn
down by the test itself — it never touches your real jobs.

## License

MIT — see [LICENSE](LICENSE).

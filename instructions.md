# How to create a job for djobs

This document exists so an AI (or a person) can create a job compatible
with djobs by hand, without opening the TUI — for example inside an agent
that needs to schedule a terminal task for later. If you have the TUI at
hand, it's simpler to just press `n` inside it (see the end of this
document); what follows is the format it expects to find on disk.

## What djobs sees

djobs scans a jobs directory (`~/jobs` by default, configurable via
`DJOBS_JOBS_DIR`) looking for subdirectories, and matches each one by name
with an optional pair of `--user` systemd units in a second directory
(`~/.config/systemd/user` by default, configurable via `DJOBS_SYSTEMD_DIR`).
There's no separate metadata file — all state is inferred from the
filesystem + systemd.

Expected layout per job, at `<jobs-dir>/<job-name>/`:

- `<job-name>.sh` — the script that does the actual work.
- `<job-name>-body.txt` (or any `*body*.txt`, falling back to `*.txt`) —
  free-form notes/description, optional, shown in the details panel.
- `<job-name>.log` — optional. Writing here is what leaves a record in
  history: the presence of this file (with no timer left) is what marks a
  job as "done" instead of "removed".

While the job is still scheduled, the pair of units also exists at
`~/.config/systemd/user/`:

```
~/jobs/my-task/
  my-task.sh
  my-task.log
  my-task-body.txt

~/.config/systemd/user/
  my-task.timer
  my-task.service
```

## The five states djobs infers

- **active** — timer exists, unit enabled.
- **paused** — timer exists, unit disabled (but the service never got to
  run and fail).
- **done** — timer/service no longer exist, and there's a `.log`.
- **failed** — timer/service still exist, but the service's `ActiveState`
  is `failed` (it ran and exited with an error).
- **removed** — timer/service no longer exist, and there's no `.log`
  (schedule deleted, or it never existed, before ever running).

**The practical consequence of this**: the job script itself is
responsible for removing its own systemd units when it finishes
successfully. If it doesn't do this self-cleanup, the job stays in
"pending" forever even after it already ran — there's no other way for
djobs to know it finished well.

## Script template

```bash
#!/usr/bin/env bash
set -euo pipefail

JOB_NAME="my-task"

# ... the actual work goes here ...

# self-remove the systemd unit pair once done (one-shot, not recurring)
systemctl --user disable --now "${JOB_NAME}.timer" 2>/dev/null || true
rm -f "$HOME/.config/systemd/user/${JOB_NAME}.timer" "$HOME/.config/systemd/user/${JOB_NAME}.service"
systemctl --user daemon-reload
```

`set -euo pipefail` matters beyond the usual safety net: it's what makes
telling "failed" apart from "removed" possible. A script that dies partway
through never reaches the self-cleanup line, so it leaves the units behind
exactly like a merely-paused job would — and djobs uses the service's
`ActiveState` to tell the two cases apart.

## The two systemd units

```ini
# ~/.config/systemd/user/my-task.service
[Unit]
Description=my-task

[Service]
Type=oneshot
ExecStart=/home/user/jobs/my-task/my-task.sh
StandardOutput=append:/home/user/jobs/my-task/my-task.log
StandardError=append:/home/user/jobs/my-task/my-task.log
```

```ini
# ~/.config/systemd/user/my-task.timer
[Unit]
Description=my-task

[Timer]
OnCalendar=2026-08-05 14:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

`OnCalendar` takes an absolute timestamp for a one-shot job — worth
validating first with `systemd-analyze calendar "<timestamp>"`.
`Persistent=true` is what makes a missed run (machine off or asleep at the
scheduled time) fire as soon as the machine is back, instead of being
silently skipped like plain cron would.

After writing both files:

```bash
systemctl --user daemon-reload
systemctl --user enable --now my-task.timer
```

This depends on `loginctl` lingering being enabled for the user
(`loginctl show-user "$USER" --property=Linger` should return `yes`),
otherwise the unit won't fire without an active login session.

## Or just use djobs

If the TUI is available, press `n` — it asks for the name, date/time, and
the command(s) to run, and writes the script (with the self-cleanup block
above already baked in), both units, and enables the timer for you.

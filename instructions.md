# How to create a job for tjobs

This document exists so an AI (or a person) can create a job compatible
with tjobs by hand, without opening the TUI — for example inside an agent
that needs to schedule a terminal task for later. If you have the TUI at
hand, it's simpler to just press `n` inside it (see the end of this
document); what follows is the format it expects to find on disk.

## What tjobs sees

tjobs scans a jobs directory (`~/jobs` by default, configurable via
`TJOBS_JOBS_DIR`) looking for subdirectories, and matches each one by name
with an optional pair of `--user` systemd units in a second directory
(`~/.config/systemd/user` by default, configurable via `TJOBS_SYSTEMD_DIR`).
There's no separate metadata file — all state is inferred from the
filesystem + systemd.

Expected layout per job, at `<jobs-dir>/<job-name>/`:

- `<job-name>.sh` — the script that does the actual work.
- `<job-name>-body.txt` (or any `*body*.txt`, falling back to `*.txt`) —
  free-form notes/description, optional, shown in the details panel.
- `<job-name>.log` — optional. Writing here is what leaves a record in
  history: the presence of this file (with no timer left) is what marks a
  job as "done" instead of "removed".
- `<job-name>.recur` — only for a custom-cycle recurring job (see below).

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

## The five states tjobs infers

- **active** — timer exists, unit enabled.
- **paused** — timer exists, unit disabled (but the service never got to
  run and fail).
- **done** — timer/service no longer exist, and there's a `.log`.
- **failed** — timer/service still exist, but the service's `ActiveState`
  is `failed` (it ran and exited with an error).
- **removed** — timer/service no longer exist, and there's no `.log`
  (schedule deleted, or it never existed, before ever running).

**The practical consequence of this**: a one-shot job's script itself is
responsible for removing its own systemd units when it finishes
successfully. If it doesn't do this self-cleanup, the job stays in
"pending" forever even after it already ran — there's no other way for
tjobs to know it finished well. A recurring job (see below) is different:
its timer is meant to keep existing, so its script does *not* self-remove —
it lives in its own "recurring" panel instead of pending/history for as
long as the timer exists, regardless of whether the last run succeeded,
failed, or hasn't fired yet.

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
exactly like a merely-paused job would — and tjobs uses the service's
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

## Recurring jobs

A job repeats instead of running once by giving it an `OnCalendar=` that
isn't a single absolute timestamp. There are two ways to do that:

**Native systemd expressions** (daily/weekly/monthly) — systemd itself
resolves these into every future occurrence, so the script needs no tail at
all beyond the actual work:

```
OnCalendar=*-*-* 09:00:00          # daily, at 09:00
OnCalendar=Mon,Wed,Fri *-*-* 09:00:00   # weekly, on specific weekdays
OnCalendar=*-*-15 09:00:00         # monthly, on the 15th
```

Validate any of these first with `systemd-analyze calendar "<expression>"`.

**Custom day-interval cycle** — for a pattern like "run, wait 2 days, run,
wait 4 days, run, wait 5 days, repeat" that native `OnCalendar=` can't
express directly. This needs two things beyond the usual layout:

- `<job-name>.recur` — two lines: the cycle as space-separated day counts
  (`2 4 5`), then the current zero-based index into it (`0` initially).
- `OnCalendar=` starts as a normal absolute timestamp (the first run), and
  the script's tail *rewrites its own timer* to the next date in the cycle
  instead of self-deleting:

```bash
# self-reschedule for the next point in the day-interval cycle (recurring, not one-shot)
RECUR_FILE="$HOME/jobs/my-task/my-task.recur"
read -r -a CYCLE < <(sed -n 1p "$RECUR_FILE")
IDX=$(sed -n 2p "$RECUR_FILE")
NEXT_IDX=$(( (IDX + 1) % ${#CYCLE[@]} ))
NEXT_DATE=$(date -d "+${CYCLE[$IDX]} days" '+%Y-%m-%d %H:%M:00')
sed -i "s/^OnCalendar=.*/OnCalendar=${NEXT_DATE}/" "$HOME/.config/systemd/user/my-task.timer"
printf '%s\n%d\n' "${CYCLE[*]}" "$NEXT_IDX" > "$RECUR_FILE"
systemctl --user daemon-reload
systemctl --user enable --now my-task.timer
```

tjobs tells a custom-cycle job apart from a genuine one-shot (whose
`OnCalendar=` also looks like a plain absolute timestamp) purely by the
presence of the `.recur` file — there's no other marker.

## Archiving

Instead of deleting a job, move its whole directory under a `.archive`
subdirectory of the jobs dir:

```
~/jobs/.archive/my-task/
  my-task.sh
  my-task.log
```

tjobs already skips dot-prefixed directories when scanning `~/jobs`, so an
archived job is invisible to the normal pending/recurring/history panels
for free. If the job still had a timer, disable and remove its unit files
first (`systemctl --user disable --now my-task.timer`, then remove the
`.timer`/`.service` files and `daemon-reload`) — an archived job shouldn't
keep firing. To bring one back, just move the directory back out of
`.archive`; its timer is not restored automatically.

## Or just use tjobs

If the TUI is available, press `n` — it asks for the name, a recurrence
type (One-shot / Daily / Weekly / Monthly / Custom cycle) and its
schedule, and the command(s) to run, then writes the script (with whichever
tail that type needs already baked in), both units, and the `.recur`
sidecar if applicable, and enables the timer for you. `d` on a job offers
Archive or Delete forever; `A` toggles the history panel into an archived
view, where `u` unarchives the selected job.

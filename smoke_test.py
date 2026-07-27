import asyncio
import subprocess
import sys
from datetime import datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import app as app_module  # noqa: E402
from app import JobsApp  # noqa: E402

FIXTURE_NAME = "zz-smoketest-job"
FIXTURE_ONCALENDAR = "2099-01-01 00:00:00"


def systemctl_user(*args: str) -> None:
    subprocess.run(["systemctl", "--user", *args], capture_output=True, text=True, check=False)


def make_fixture() -> Path:
    """Cria um job + timer systemd descartaveis pra nao tocar em jobs reais do usuario."""
    job_dir = app_module.JOBS_DIR / FIXTURE_NAME
    job_dir.mkdir(parents=True, exist_ok=True)
    (job_dir / f"{FIXTURE_NAME}.sh").write_text("#!/usr/bin/env bash\ntrue\n")
    (job_dir / f"{FIXTURE_NAME}.log").write_text("fixture log\n")
    (job_dir / f"{FIXTURE_NAME}-body.txt").write_text("fixture body\n")

    unit_dir = app_module.SYSTEMD_USER_DIR
    unit_dir.mkdir(parents=True, exist_ok=True)
    (unit_dir / f"{FIXTURE_NAME}.service").write_text(
        f"[Unit]\nDescription={FIXTURE_NAME} (smoke test fixture)\n\n"
        "[Service]\nType=oneshot\nExecStart=/bin/true\n"
    )
    (unit_dir / f"{FIXTURE_NAME}.timer").write_text(
        f"[Unit]\nDescription={FIXTURE_NAME} (schedule)\n\n"
        f"[Timer]\nOnCalendar={FIXTURE_ONCALENDAR}\nPersistent=true\n\n"
        "[Install]\nWantedBy=timers.target\n"
    )
    systemctl_user("daemon-reload")
    systemctl_user("enable", "--now", f"{FIXTURE_NAME}.timer")
    return job_dir


def cleanup_fixture(job_dir: Path) -> None:
    systemctl_user("disable", "--now", f"{FIXTURE_NAME}.timer")
    (app_module.SYSTEMD_USER_DIR / f"{FIXTURE_NAME}.timer").unlink(missing_ok=True)
    (app_module.SYSTEMD_USER_DIR / f"{FIXTURE_NAME}.service").unlink(missing_ok=True)
    systemctl_user("daemon-reload")
    if job_dir.exists():
        for f in job_dir.iterdir():
            f.unlink()
        job_dir.rmdir()


async def run(job_dir: Path) -> None:
    app = JobsApp()
    async with app.run_test() as pilot:
        await pilot.pause()
        table = app.query_one("DataTable")
        print("rows:", table.row_count)
        print("jobs found:", [j.name for j in app.jobs])
        assert table.row_count == len(app.jobs) > 0

        table.move_cursor(row=table.get_row_index(FIXTURE_NAME))
        await pilot.pause()
        job = app.current_job()
        print("cursor on:", job.name, "has timer:", job.timer_path is not None)
        assert job.name == FIXTURE_NAME
        assert job.timer_path is not None
        enabled_before = job.enabled

        await pilot.press("e")
        await pilot.pause()
        stack = [type(s).__name__ for s in app.screen_stack]
        print("screen stack after 'e':", stack)
        assert "EditScheduleScreen" in stack
        date_input = app.screen.query_one("#date")
        time_input = app.screen.query_one("#time")
        dt = datetime.strptime(job.oncalendar, "%Y-%m-%d %H:%M:%S")
        expected_date = f"{dt.day:02d}/{dt.month:02d}"
        expected_time = f"{dt.hour:02d}:{dt.minute:02d}"
        print("prefilled date/time:", date_input.value, time_input.value)
        assert date_input.value == expected_date
        assert time_input.value == expected_time

        await pilot.press("escape")
        await pilot.pause()
        stack = [type(s).__name__ for s in app.screen_stack]
        print("screen stack after escape:", stack)
        assert "EditScheduleScreen" not in stack

        await pilot.press("d")
        await pilot.pause()
        stack = [type(s).__name__ for s in app.screen_stack]
        print("screen stack after 'd':", stack)
        assert "ConfirmScreen" in stack

        await pilot.press("escape")
        await pilot.pause()
        stack = [type(s).__name__ for s in app.screen_stack]
        print("screen stack after escape:", stack)
        assert "ConfirmScreen" not in stack

        await pilot.press("t")
        await pilot.pause()
        print("after toggle, enabled:", app.current_job().enabled)
        await pilot.press("t")
        await pilot.pause()
        print("after toggle back, enabled:", app.current_job().enabled)
        assert app.current_job().enabled == enabled_before, "timer enabled state should be back to original after toggling twice"

        await pilot.press("r")
        await pilot.pause()
        print("refresh ok, rows:", app.query_one("DataTable").row_count)

    print("SMOKE TEST OK")


def main() -> None:
    job_dir = make_fixture()
    try:
        asyncio.run(run(job_dir))
    finally:
        cleanup_fixture(job_dir)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""TUI para visualizar e manipular jobs agendados em ~/jobs (via timers systemd --user)."""

from __future__ import annotations

import re
import shutil
import subprocess
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from textual import on
from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.containers import Horizontal, Vertical
from textual.screen import ModalScreen
from textual.theme import Theme
from textual.widgets import Button, DataTable, Footer, Header, Input, Label, Static

JOBS_DIR = Path.home() / "jobs"
SYSTEMD_USER_DIR = Path.home() / ".config" / "systemd" / "user"

ONCALENDAR_RE = re.compile(r"^OnCalendar=(.+)$", re.MULTILINE)

# Paletas oficiais: https://github.com/catppuccin/catppuccin (docs/style-guide.md)
# Mapeamento semantico: red=erro, yellow=warning, green=success, mauve=primary,
# blue=secondary, peach=accent, text/subtext/overlay=hierarquia de texto,
# base/mantle/crust=fundos, surface0-2=superficies com prominencia crescente.

CATPPUCCIN_MOCHA = Theme(
    name="catppuccin-mocha",
    dark=True,
    primary="#cba6f7",  # mauve
    secondary="#89b4fa",  # blue
    warning="#f9e2af",  # yellow
    error="#f38ba8",  # red
    success="#a6e3a1",  # green
    accent="#fab387",  # peach
    foreground="#cdd6f4",  # text
    background="#1e1e2e",  # base
    surface="#313244",  # surface0
    panel="#45475a",  # surface1
    variables={
        "border": "#b4befe",  # lavender
        "border-blurred": "#6c7086",  # overlay0
        "footer-background": "#181825",  # mantle
        "block-cursor-foreground": "#11111b",  # crust
        "block-cursor-background": "#f5c2e7",  # pink
        "block-cursor-text-style": "bold",
        "input-cursor-foreground": "#11111b",  # crust
        "input-cursor-background": "#f5e0dc",  # rosewater
        "input-selection-background": "#585b70 60%",  # surface2
        "button-color-foreground": "#1e1e2e",  # base
        "scrollbar": "#45475a",  # surface1
        "scrollbar-hover": "#585b70",  # surface2
        "scrollbar-active": "#cba6f7",  # mauve
    },
)

CATPPUCCIN_LATTE = Theme(
    name="catppuccin-latte",
    dark=False,
    primary="#8839ef",  # mauve
    secondary="#1e66f5",  # blue
    warning="#df8e1d",  # yellow
    error="#d20f39",  # red
    success="#40a02b",  # green
    accent="#fe640b",  # peach
    foreground="#4c4f69",  # text
    background="#eff1f5",  # base
    surface="#ccd0da",  # surface0
    panel="#bcc0cc",  # surface1
    variables={
        "border": "#7287fd",  # lavender
        "border-blurred": "#9ca0b0",  # overlay0
        "footer-background": "#e6e9ef",  # mantle
        "block-cursor-foreground": "#eff1f5",  # base
        "block-cursor-background": "#ea76cb",  # pink
        "block-cursor-text-style": "bold",
        "input-cursor-foreground": "#eff1f5",  # base
        "input-cursor-background": "#dc8a78",  # rosewater
        "input-selection-background": "#acb0be 60%",  # surface2
        "button-color-foreground": "#eff1f5",  # base
        "scrollbar": "#bcc0cc",  # surface1
        "scrollbar-hover": "#acb0be",  # surface2
        "scrollbar-active": "#8839ef",  # mauve
    },
)


def systemctl_user(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["systemctl", "--user", *args], capture_output=True, text=True
    )


def timer_properties(name: str) -> dict[str, str]:
    result = systemctl_user(
        "show",
        f"{name}.timer",
        "--property=ActiveState,UnitFileState,NextElapseUSecRealtime",
    )
    props: dict[str, str] = {}
    for line in result.stdout.splitlines():
        if "=" in line:
            key, _, value = line.partition("=")
            props[key] = value
    return props


def read_oncalendar(timer_path: Path) -> str | None:
    try:
        text = timer_path.read_text()
    except OSError:
        return None
    match = ONCALENDAR_RE.search(text)
    return match.group(1).strip() if match else None


@dataclass
class Job:
    name: str
    dir: Path
    script: Path | None
    log: Path | None
    body: Path | None
    timer_path: Path | None
    service_path: Path | None
    oncalendar: str | None
    unit_file_state: str | None
    next_elapse: str | None

    @property
    def schedule_human(self) -> str:
        if self.oncalendar is None:
            return "—"
        try:
            dt = datetime.strptime(self.oncalendar, "%Y-%m-%d %H:%M:%S")
            return f"{dt.day:02d}/{dt.month:02d} {dt.hour:02d}:{dt.minute:02d}"
        except ValueError:
            return self.oncalendar

    @property
    def enabled(self) -> bool:
        return self.unit_file_state == "enabled"

    @property
    def status(self) -> str:
        if self.timer_path is None:
            return "sem timer (rodou/removido)" if self.log else "sem timer"
        return "ativo" if self.enabled else "pausado"

    @property
    def status_markup(self) -> str:
        # DataTable renderiza celulas via Rich puro (nao entende "$variavel" do
        # tema Textual), por isso usa nomes de cor padrao do Rich aqui.
        if self.timer_path is None:
            label = "rodou/removido" if self.log else "sem timer"
            return f"[dim]○ {label}[/]"
        if self.enabled:
            return "[bold green]● ativo[/]"
        return "[bold yellow]● pausado[/]"

    def detail_text(self) -> str:
        parts = [f"[b]{self.name}[/b]  ({self.dir})\n"]
        if self.timer_path is not None:
            parts.append(f"timer: {self.oncalendar}  [{self.status}]")
            if self.next_elapse:
                parts.append(f"próxima execução: {self.next_elapse}")
            if self.script:
                parts.append(f"comando: {self.script}\n")
        else:
            parts.append(f"status: {self.status}\n")
        if self.body:
            parts.append("--- corpo/notas ---")
            parts.append(self.body.read_text().strip())
            parts.append("")
        if self.script:
            parts.append("--- script ---")
            parts.append(self.script.read_text().strip())
            parts.append("")
        if self.log:
            parts.append("--- log (fim) ---")
            lines = self.log.read_text().splitlines()
            parts.append("\n".join(lines[-25:]))
        return "\n".join(parts)


def discover_jobs() -> list[Job]:
    jobs: list[Job] = []
    if not JOBS_DIR.exists():
        return jobs
    for d in sorted(JOBS_DIR.iterdir()):
        if not d.is_dir() or d.name.startswith((".", "_")):
            continue
        scripts = sorted(d.glob("*.sh"))
        logs = sorted(d.glob("*.log"))
        bodies = sorted(d.glob("*body*.txt")) or sorted(d.glob("*.txt"))

        timer_path = SYSTEMD_USER_DIR / f"{d.name}.timer"
        service_path = SYSTEMD_USER_DIR / f"{d.name}.service"
        if timer_path.exists():
            oncalendar = read_oncalendar(timer_path)
            props = timer_properties(d.name)
            unit_file_state = props.get("UnitFileState")
            next_elapse = props.get("NextElapseUSecRealtime") or None
        else:
            timer_path = None
            service_path = service_path if service_path.exists() else None
            oncalendar = None
            unit_file_state = None
            next_elapse = None

        jobs.append(
            Job(
                name=d.name,
                dir=d,
                script=scripts[0] if scripts else None,
                log=logs[0] if logs else None,
                body=bodies[0] if bodies else None,
                timer_path=timer_path,
                service_path=service_path,
                oncalendar=oncalendar,
                unit_file_state=unit_file_state,
                next_elapse=next_elapse,
            )
        )
    return jobs


class ConfirmScreen(ModalScreen[str]):
    """Modal genérico com um título e botões nomeados; devolve o id do botão clicado ou None."""

    BINDINGS = [Binding("escape", "cancel", "Cancelar")]

    CSS = """
    ConfirmScreen {
        align: center middle;
    }
    #dialog {
        width: auto;
        height: auto;
        padding: 1 2;
        border: round $warning;
        background: $surface;
    }
    #dialog Label {
        margin-bottom: 1;
    }
    #buttons {
        height: auto;
        align: center middle;
    }
    #buttons Button {
        margin: 0 1;
    }
    """

    def __init__(self, message: str, choices: list[tuple[str, str]]) -> None:
        super().__init__()
        self.message = message
        self.choices = choices  # (button_id, label)

    def compose(self) -> ComposeResult:
        with Vertical(id="dialog"):
            yield Label(self.message)
            with Horizontal(id="buttons"):
                for button_id, label in self.choices:
                    variant = "error" if button_id in ("delete-all", "delete") else "default"
                    yield Button(label, id=button_id, variant=variant)
                yield Button("Cancelar", id="cancel")

    @on(Button.Pressed)
    def handle_button(self, event: Button.Pressed) -> None:
        self.dismiss(None if event.button.id == "cancel" else event.button.id)

    def action_cancel(self) -> None:
        self.dismiss(None)


class EditScheduleScreen(ModalScreen[tuple[int, int, int, int] | None]):
    """Pede nova data (DD/MM) e hora (HH:MM) para o job. Devolve (minuto, hora, dia, mes) ou None."""

    BINDINGS = [Binding("escape", "cancel", "Cancelar")]

    CSS = """
    EditScheduleScreen {
        align: center middle;
    }
    #dialog {
        width: 40;
        height: auto;
        padding: 1 2;
        border: round $accent;
        background: $surface;
    }
    #dialog Label {
        margin-bottom: 1;
    }
    #dialog Input {
        margin-bottom: 1;
    }
    #buttons {
        height: auto;
        align: center middle;
    }
    #buttons Button {
        margin: 0 1;
    }
    """

    def __init__(self, job: Job) -> None:
        super().__init__()
        self.job = job

    def compose(self) -> ComposeResult:
        date_default, time_default = "", ""
        if self.job.oncalendar is not None:
            try:
                dt = datetime.strptime(self.job.oncalendar, "%Y-%m-%d %H:%M:%S")
                date_default = f"{dt.day:02d}/{dt.month:02d}"
                time_default = f"{dt.hour:02d}:{dt.minute:02d}"
            except ValueError:
                pass
        with Vertical(id="dialog"):
            yield Label(f"Reagendar: {self.job.name}")
            yield Label("Data (DD/MM):")
            yield Input(value=date_default, placeholder="17/07", id="date")
            yield Label("Hora (HH:MM):")
            yield Input(value=time_default, placeholder="14:00", id="time")
            with Horizontal(id="buttons"):
                yield Button("Salvar", id="save", variant="primary")
                yield Button("Cancelar", id="cancel")

    @on(Button.Pressed, "#cancel")
    def cancel(self) -> None:
        self.dismiss(None)

    def action_cancel(self) -> None:
        self.dismiss(None)

    @on(Button.Pressed, "#save")
    def save(self) -> None:
        date_str = self.query_one("#date", Input).value.strip()
        time_str = self.query_one("#time", Input).value.strip()
        try:
            dom_str, month_str = date_str.split("/")
            hour_str, minute_str = time_str.split(":")
            dom, month, hour, minute = int(dom_str), int(month_str), int(hour_str), int(minute_str)
            if not (1 <= dom <= 31 and 1 <= month <= 12 and 0 <= hour <= 23 and 0 <= minute <= 59):
                raise ValueError
        except (ValueError, IndexError):
            self.query_one("#date", Input).styles.border = ("heavy", "red")
            self.query_one("#time", Input).styles.border = ("heavy", "red")
            return
        self.dismiss((minute, hour, dom, month))


class JobsApp(App):
    TITLE = "jobs"
    SUB_TITLE = str(JOBS_DIR)
    CSS = """
    Screen {
        background: $background;
    }
    #table {
        height: 45%;
        margin: 1 1 0 1;
        border: round $panel;
        background: $surface;
    }
    #table > .datatable--header {
        background: $panel;
        color: $foreground;
        text-style: bold;
    }
    #detail {
        height: 1fr;
        margin: 1 1 1 1;
        border: round $panel;
        background: $surface;
        padding: 1 2;
    }
    """

    BINDINGS = [
        Binding("e", "edit_schedule", "Reagendar"),
        Binding("t", "toggle_enabled", "Pausar/Retomar"),
        Binding("d", "delete_job", "Apagar"),
        Binding("r", "refresh_jobs", "Atualizar"),
        Binding("q", "quit", "Sair"),
    ]

    def __init__(self) -> None:
        super().__init__()
        self.jobs: list[Job] = []
        self.register_theme(CATPPUCCIN_MOCHA)
        self.register_theme(CATPPUCCIN_LATTE)
        self.theme = "catppuccin-mocha"

    def compose(self) -> ComposeResult:
        yield Header()
        yield DataTable(id="table", cursor_type="row", zebra_stripes=True)
        yield Static(id="detail")
        yield Footer()

    def on_mount(self) -> None:
        table = self.query_one(DataTable)
        table.add_columns("job", "agendado para", "status", "log")
        table.border_title = "jobs"
        self.query_one("#detail", Static).border_title = "detalhes"
        self.load_jobs()

    def load_jobs(self) -> None:
        previous_job = self.current_job()
        previous_name = previous_job.name if previous_job else None

        self.jobs = discover_jobs()
        self.sub_title = f"{len(self.jobs)} job(s) em {JOBS_DIR}"
        table = self.query_one(DataTable)
        table.clear()
        for job in self.jobs:
            table.add_row(
                job.name,
                job.schedule_human,
                job.status_markup,
                "[dim]—[/]" if not job.log else "✓",
                key=job.name,
            )

        if previous_name is not None and previous_name in table.rows:
            table.move_cursor(row=table.get_row_index(previous_name))

        detail = self.query_one("#detail", Static)
        current = self.current_job()
        if current is not None:
            detail.update(current.detail_text())
        else:
            detail.update(f"Nenhum job encontrado em {JOBS_DIR}")

    def current_job(self) -> Job | None:
        table = self.query_one(DataTable)
        if table.row_count == 0:
            return None
        row_key = table.coordinate_to_cell_key(table.cursor_coordinate).row_key.value
        return next((j for j in self.jobs if j.name == row_key), None)

    @on(DataTable.RowHighlighted)
    def show_detail(self, event: DataTable.RowHighlighted) -> None:
        job = next((j for j in self.jobs if j.name == event.row_key.value), None)
        if job is not None:
            self.query_one("#detail", Static).update(job.detail_text())

    def action_refresh_jobs(self) -> None:
        self.load_jobs()
        self.notify("Lista atualizada.")

    def action_edit_schedule(self) -> None:
        job = self.current_job()
        if job is None:
            return
        if job.timer_path is None:
            self.notify(f"'{job.name}' não tem timer pra reagendar.", severity="warning")
            return

        def apply(result: tuple[int, int, int, int] | None) -> None:
            if result is None:
                return
            minute, hour, dom, month = result
            today = datetime.now()
            year = today.year
            if (month, dom) < (today.month, today.day):
                year += 1
            oncalendar = f"{year:04d}-{month:02d}-{dom:02d} {hour:02d}:{minute:02d}:00"

            text = job.timer_path.read_text()
            text = ONCALENDAR_RE.sub(f"OnCalendar={oncalendar}", text)
            job.timer_path.write_text(text)
            systemctl_user("daemon-reload")
            if job.enabled:
                systemctl_user("enable", "--now", f"{job.name}.timer")

            self.load_jobs()
            self.notify(f"'{job.name}' reagendado para {dom:02d}/{month:02d} {hour:02d}:{minute:02d}.")

        self.push_screen(EditScheduleScreen(job), apply)

    def action_toggle_enabled(self) -> None:
        job = self.current_job()
        if job is None:
            return
        if job.timer_path is None:
            self.notify(f"'{job.name}' não tem timer.", severity="warning")
            return
        if job.enabled:
            systemctl_user("disable", "--now", f"{job.name}.timer")
        else:
            systemctl_user("enable", "--now", f"{job.name}.timer")
        self.load_jobs()
        new_job = next((j for j in self.jobs if j.name == job.name), None)
        state = "ativado" if new_job and new_job.enabled else "pausado"
        self.notify(f"'{job.name}' {state}.")

    def action_delete_job(self) -> None:
        job = self.current_job()
        if job is None:
            return

        def handle(choice: str | None) -> None:
            if choice is None:
                return
            if job.timer_path is not None:
                systemctl_user("disable", "--now", f"{job.name}.timer")
                job.timer_path.unlink(missing_ok=True)
                if job.service_path is not None:
                    job.service_path.unlink(missing_ok=True)
                systemctl_user("daemon-reload")
            if choice == "delete-all":
                shutil.rmtree(job.dir)
            self.load_jobs()
            self.notify(f"'{job.name}' removido ({'agendamento + arquivos' if choice == 'delete-all' else 'só agendamento'}).")

        self.push_screen(
            ConfirmScreen(
                f"Apagar '{job.name}'?",
                [("delete-cron", "Só agendamento"), ("delete-all", "Agendamento + arquivos")],
            ),
            handle,
        )


def main() -> None:
    JobsApp().run()


if __name__ == "__main__":
    main()

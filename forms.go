package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
)

func validateDDMM(s string) error {
	dom, month, err := parseDDMM(s)
	if err != nil || dom < 1 || dom > 31 || month < 1 || month > 12 {
		return fmt.Errorf("formato esperado DD/MM")
	}
	return nil
}

func validateHHMM(s string) error {
	hour, minute, err := parseHHMM(s)
	if err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return fmt.Errorf("formato esperado HH:MM")
	}
	return nil
}

func parseDDMM(s string) (dom, month int, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("formato invalido")
	}
	dom, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	month, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	return dom, month, err
}

func parseHHMM(s string) (hour, minute int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("formato invalido")
	}
	hour, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	minute, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	return hour, minute, err
}

func newEditForm(j Job) *huh.Form {
	dateDefault, timeDefault := "", ""
	if j.OnCalendar != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", j.OnCalendar); err == nil {
			dateDefault = fmt.Sprintf("%02d/%02d", t.Day(), t.Month())
			timeDefault = fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
		}
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("date").
				Title("Data (DD/MM)").
				Placeholder("17/07").
				Value(&dateDefault).
				Validate(validateDDMM),
			huh.NewInput().
				Key("time").
				Title("Hora (HH:MM)").
				Placeholder("14:00").
				Value(&timeDefault).
				Validate(validateHHMM),
		),
	).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithWidth(40)
}

func validateNonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("obrigatório")
	}
	return nil
}

func newCreateForm() *huh.Form {
	name, date, timeStr, commands, notes := "", "", "", "", ""
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Nome do job").
				Placeholder("minha-tarefa").
				Value(&name).
				Validate(validateJobName),
			huh.NewInput().
				Key("date").
				Title("Data (DD/MM)").
				Placeholder("17/07").
				Value(&date).
				Validate(validateDDMM),
			huh.NewInput().
				Key("time").
				Title("Hora (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
			huh.NewText().
				Key("commands").
				Title("Comando(s) a executar (bash)").
				Placeholder("git add ...\ngit commit -m ...").
				Value(&commands).
				Validate(validateNonEmpty),
			huh.NewText().
				Key("notes").
				Title("Notas (opcional)").
				Value(&notes),
		),
	).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithWidth(60)
}

const (
	deleteChoiceCron   = "cron"
	deleteChoiceAll    = "all"
	deleteChoiceCancel = "cancel"
)

func newDeleteForm(j Job) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("choice").
				Title(fmt.Sprintf("Apagar '%s'?", j.Name)).
				Options(
					huh.NewOption("Só agendamento", deleteChoiceCron),
					huh.NewOption("Agendamento + arquivos", deleteChoiceAll),
					huh.NewOption("Cancelar", deleteChoiceCancel),
				),
		),
	).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithWidth(40)
}

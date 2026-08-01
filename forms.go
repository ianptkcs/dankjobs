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
		return fmt.Errorf("expected format DD/MM")
	}
	return nil
}

func validateHHMM(s string) error {
	hour, minute, err := parseHHMM(s)
	if err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return fmt.Errorf("expected format HH:MM")
	}
	return nil
}

func parseDDMM(s string) (dom, month int, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid format")
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
		return 0, 0, fmt.Errorf("invalid format")
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
				Title("Date (DD/MM)").
				Placeholder("17/07").
				Value(&dateDefault).
				Validate(validateDDMM),
			huh.NewInput().
				Key("time").
				Title("Time (HH:MM)").
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
		return fmt.Errorf("required")
	}
	return nil
}

func validateNonEmptyWeekdays(days []time.Weekday) error {
	if len(days) == 0 {
		return fmt.Errorf("select at least one day")
	}
	return nil
}

func validateDayOfMonth(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 31 {
		return fmt.Errorf("expected a number 1-31")
	}
	return nil
}

// parseCycle parses a custom recurrence cycle like "2 4 5" (run, wait 2
// days, run, wait 4, run, wait 5, repeat) into its day-interval sequence.
func parseCycle(s string) ([]int, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf(`expected one or more day counts, e.g. "2 4 5"`)
	}
	cycle := make([]int, len(fields))
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(`expected positive whole numbers, e.g. "2 4 5"`)
		}
		cycle[i] = n
	}
	return cycle, nil
}

func validateCycle(s string) error {
	_, err := parseCycle(s)
	return err
}

// newCreateForm builds the job-creation form. "date"/"time" (and, for the
// custom-cycle branch, "date" again) are reused as Keys across several
// mutually-exclusive groups rather than given per-kind names: huh's group
// navigation skips hidden groups entirely (see WithHideFunc below), so a
// hidden group's fields never write into the form's result set — only
// whichever branch the user actually walked through does.
func newCreateForm() *huh.Form {
	name, date, timeStr, commands, notes, cycleStr, dayOfMonth := "", "", "", "", "", "", ""
	kind := recurOneshot
	var weekdays []time.Weekday

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Job name").
				Placeholder("my-task").
				Value(&name).
				Validate(validateJobName),
			huh.NewSelect[RecurrenceKind]().
				Key("type").
				Title("Recurrence").
				Options(
					huh.NewOption("One-shot", recurOneshot),
					huh.NewOption("Daily", recurDaily),
					huh.NewOption("Weekly", recurWeekly),
					huh.NewOption("Monthly", recurMonthly),
					huh.NewOption("Custom cycle", recurCycle),
				).
				Value(&kind),
		),
		huh.NewGroup(
			huh.NewInput().
				Key("date").
				Title("Date (DD/MM)").
				Placeholder("17/07").
				Value(&date).
				Validate(validateDDMM),
			huh.NewInput().
				Key("time").
				Title("Time (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
		).WithHideFunc(func() bool { return kind != recurOneshot }),
		huh.NewGroup(
			huh.NewInput().
				Key("time").
				Title("Time (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
		).WithHideFunc(func() bool { return kind != recurDaily }),
		huh.NewGroup(
			huh.NewMultiSelect[time.Weekday]().
				Key("weekdays").
				Title("Days of the week").
				Options(
					huh.NewOption("Mon", time.Monday),
					huh.NewOption("Tue", time.Tuesday),
					huh.NewOption("Wed", time.Wednesday),
					huh.NewOption("Thu", time.Thursday),
					huh.NewOption("Fri", time.Friday),
					huh.NewOption("Sat", time.Saturday),
					huh.NewOption("Sun", time.Sunday),
				).
				Value(&weekdays).
				Validate(validateNonEmptyWeekdays),
			huh.NewInput().
				Key("time").
				Title("Time (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
		).WithHideFunc(func() bool { return kind != recurWeekly }),
		huh.NewGroup(
			huh.NewInput().
				Key("dayOfMonth").
				Title("Day of month (1-31)").
				Placeholder("15").
				Value(&dayOfMonth).
				Validate(validateDayOfMonth),
			huh.NewInput().
				Key("time").
				Title("Time (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
		).WithHideFunc(func() bool { return kind != recurMonthly }),
		huh.NewGroup(
			huh.NewInput().
				Key("cycle").
				Title(`Day-interval cycle (e.g. "2 4 5")`).
				Placeholder("2 4 5").
				Value(&cycleStr).
				Validate(validateCycle),
			huh.NewInput().
				Key("date").
				Title("First run date (DD/MM)").
				Placeholder("17/07").
				Value(&date).
				Validate(validateDDMM),
			huh.NewInput().
				Key("time").
				Title("First run time (HH:MM)").
				Placeholder("14:00").
				Value(&timeStr).
				Validate(validateHHMM),
		).WithHideFunc(func() bool { return kind != recurCycle }),
		huh.NewGroup(
			huh.NewText().
				Key("commands").
				Title("Command(s) to run (bash)").
				Placeholder("git add ...\ngit commit -m ...").
				Value(&commands).
				Validate(validateNonEmpty),
			huh.NewText().
				Key("notes").
				Title("Notes (optional)").
				Value(&notes),
		),
	).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithWidth(60)
}

const (
	deleteChoiceArchive = "archive"
	deleteChoiceForever = "forever"
	deleteChoiceCancel  = "cancel"
)

// newDeleteForm builds the "d" confirmation modal. archived jobs have
// already been moved out of the way, so "Archive" (which would just try to
// move a job into its own directory) is dropped from that case, leaving
// only Delete forever / Cancel.
func newDeleteForm(j Job, archived bool) *huh.Form {
	options := []huh.Option[string]{
		huh.NewOption("Archive", deleteChoiceArchive),
		huh.NewOption("Delete forever", deleteChoiceForever),
		huh.NewOption("Cancel", deleteChoiceCancel),
	}
	if archived {
		options = []huh.Option[string]{
			huh.NewOption("Delete forever", deleteChoiceForever),
			huh.NewOption("Cancel", deleteChoiceCancel),
		}
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("choice").
				Title(fmt.Sprintf("Delete '%s'?", j.Name)).
				Options(options...),
		),
	).
		WithTheme(huh.ThemeCatppuccin()).
		WithShowHelp(true).
		WithWidth(40)
}

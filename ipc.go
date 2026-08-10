package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/ianptkcs/tabelatuiui"
)

// jobJSON is the wire format for the ipc subcommand, reusing Job's own
// derived fields (Status, ScheduleHuman, HistoryWhen) rather than
// re-deriving anything from systemd state.
type jobJSON struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Pending       bool   `json:"pending"`
	Recurring     bool   `json:"recurring"`
	OnCalendar    string `json:"on_calendar,omitempty"`
	ScheduleHuman string `json:"schedule_human,omitempty"`
	HistoryWhen   string `json:"history_when,omitempty"`
	Dir           string `json:"dir"`
	Body          string `json:"body,omitempty"`
}

func (j Job) toIPC() jobJSON {
	_, label := j.Status()
	out := jobJSON{
		Name:      j.Name,
		Status:    label,
		Pending:   j.IsPending(),
		Recurring: j.IsRecurring(),
		Dir:       j.Dir,
		Body:      j.Body,
	}
	if j.TimerPath != "" {
		out.OnCalendar = j.OnCalendar
		out.ScheduleHuman = j.ScheduleHuman()
	} else {
		out.HistoryWhen = j.HistoryWhen()
	}
	return out
}

// runIPC implements `tjobs ipc <method> [key=value...] --json`, a
// scriptable data source mirroring dcal's `dcal ipc <method> --json`
// convention. Returns the process exit code.
func runIPC(args []string) int {
	parsed, err := tuiui.ParseIPCArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "uso: tjobs ipc <método> [key=value...] --json")
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	switch parsed.Method {
	case "jobs.list":
		return ipcJobsList(parsed.Filters)
	case "jobs.next":
		return ipcJobsNext()
	default:
		fmt.Fprintf(os.Stderr, "método desconhecido: %q\n", parsed.Method)
		return 1
	}
}

func ipcJobsList(filters map[string]string) int {
	jobs := discoverJobs()
	out := make([]jobJSON, 0, len(jobs))
	for _, j := range jobs {
		if pending, ok := filters["pending"]; ok {
			want := pending == "true"
			if j.IsPending() != want {
				continue
			}
		}
		out = append(out, j.toIPC())
	}
	return tuiui.WriteJSON(out)
}

func ipcJobsNext() int {
	jobs := discoverJobs()
	var candidates []Job
	for _, j := range jobs {
		// Recurring jobs are excluded: they're an ambient repeating
		// schedule rather than a one-off "next thing to do", and their
		// OnCalendar (e.g. "*-*-* 09:00:00") isn't a comparable timestamp
		// string against a one-shot job's absolute date.
		if j.IsPending() && !j.IsRecurring() && j.Enabled() && j.OnCalendar != "" {
			candidates = append(candidates, j)
		}
	}
	if len(candidates) == 0 {
		return tuiui.WriteJSON(nil)
	}
	sort.Slice(candidates, func(i, k int) bool {
		return candidates[i].OnCalendar < candidates[k].OnCalendar
	})
	out := candidates[0].toIPC()
	return tuiui.WriteJSON(out)
}

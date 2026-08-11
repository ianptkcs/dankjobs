package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := loadSettings(); err != nil {
		fmt.Fprintln(os.Stderr, "aviso: config.toml:", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "ipc" {
		os.Exit(runIPC(os.Args[2:]))
	}

	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

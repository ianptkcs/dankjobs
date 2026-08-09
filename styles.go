package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/ianptkcs/tabelatuiui"
)

// App-specific styles on top of tabelatuiui's shared chrome (called as
// theme.Header/Footer/Panel/Title/Dim/Modal directly). Colors live in the
// theme resolved in theme.go (Catppuccin Mocha + the DMS accent).

func padLines(s string, width int) string { return tuiui.PadLines(s, width) }

// statusStyle follows the Catppuccin style guide's semantics, the same
// meanings the lib's Success/Warning/Info/Error carry: green for
// success/active, yellow for a paused/warning state, red for a hard error,
// blue for a neutral "done, informational" state, and a dim overlay color
// for anything that just... isn't relevant anymore.
func statusStyle(kind jobStatusKind) lipgloss.Style {
	switch kind {
	case statusActive:
		return theme.Success()
	case statusPaused:
		return theme.Warning()
	case statusCompleted:
		return theme.Info()
	case statusFailed:
		return theme.Error()
	case statusManual:
		return theme.Info()
	default: // statusRemoved
		return theme.Dim()
	}
}

func statusGlyph(kind jobStatusKind) string {
	switch kind {
	case statusActive, statusPaused, statusCompleted:
		return "●"
	case statusFailed:
		return "✕"
	default: // statusManual, statusRemoved
		return "○"
	}
}

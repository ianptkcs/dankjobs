package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/ianptkcs/tabelatuiui"
)

// Thin wrappers over tabelatuiui's shared chrome, so the model/view code
// keeps calling the same short helpers it always did. Colors live in the
// theme resolved in theme.go (Catppuccin Mocha + the DMS accent).

func headerStyle(width int) lipgloss.Style { return theme.Header(width) }
func footerStyle(width int) lipgloss.Style { return theme.Footer(width) }
func panelStyle(focused bool) lipgloss.Style {
	return theme.Panel(focused)
}
func titleStyle() lipgloss.Style { return theme.Title() }
func dimStyle() lipgloss.Style   { return theme.Dim() }
func modalBoxStyle() lipgloss.Style {
	return theme.Modal()
}
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
	default: // statusRemoved
		return dimStyle()
	}
}

func statusGlyph(kind jobStatusKind) string {
	switch kind {
	case statusActive, statusPaused, statusCompleted:
		return "●"
	case statusFailed:
		return "✕"
	default: // statusRemoved
		return "○"
	}
}

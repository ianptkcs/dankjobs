package main

import (
	"strings"

	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Colors follow the official semantic guide:
// https://github.com/catppuccin/catppuccin/blob/main/docs/style-guide.md
var (
	mocha = catppuccin.Mocha

	colBase     = lipgloss.Color(mocha.Base().Hex)
	colMantle   = lipgloss.Color(mocha.Mantle().Hex)
	colSurface0 = lipgloss.Color(mocha.Surface0().Hex)
	colSurface1 = lipgloss.Color(mocha.Surface1().Hex)
	colOverlay0 = lipgloss.Color(mocha.Overlay0().Hex)
	colOverlay1 = lipgloss.Color(mocha.Overlay1().Hex)
	colText     = lipgloss.Color(mocha.Text().Hex)
	colSubtext0 = lipgloss.Color(mocha.Subtext0().Hex)
	colMauve    = lipgloss.Color(mocha.Mauve().Hex)
	colPink     = lipgloss.Color(mocha.Pink().Hex)
	colGreen    = lipgloss.Color(mocha.Green().Hex)
	colYellow   = lipgloss.Color(mocha.Yellow().Hex)
	colLavender = lipgloss.Color(mocha.Lavender().Hex)
)

func headerStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(colMantle).
		Foreground(colMauve).
		Bold(true).
		Width(width).
		Padding(0, 2)
}

func footerStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(colMantle).
		Foreground(colSubtext0).
		Width(width).
		Padding(0, 2)
}

// panelStyle intentionally has no Width(): calling Width() on a style makes
// lipgloss re-wrap its content, and that wrap logic miscounts lines that
// already carry their own nested ANSI (e.g. a table's selected-row
// highlight), breaking alignment. Content is pre-padded to a uniform width
// with padLines instead, so the border ends up sized correctly on its own.
func panelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colSurface1).
		Background(colBase).
		Padding(0, 1)
}

// padLines pads or truncates every line of s so its ANSI-aware visible
// width is exactly `width` — a border box only ends up sized (and
// positioned) correctly if every line it wraps is uniform. Long lines are
// expected to be plain, unstyled text (a path, a log line, a script line),
// so a plain rune-width truncate is safe here.
func padLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		switch w := lipgloss.Width(line); {
		case w < width:
			lines[i] = line + strings.Repeat(" ", width-w)
		case w > width:
			lines[i] = runewidth.Truncate(line, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}

// titleStyle sets Background explicitly for the same reason panelStyle's
// Header does: it's the first line inside a panel, so its own reset must not
// blank out the panel's background for that line.
func titleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(colMauve).Background(colBase)
}

func dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colOverlay0).Background(colBase)
}

func statusStyle(kind jobStatusKind) lipgloss.Style {
	switch kind {
	case statusActive:
		return lipgloss.NewStyle().Foreground(colGreen).Background(colBase).Bold(true)
	case statusPaused:
		return lipgloss.NewStyle().Foreground(colYellow).Background(colBase).Bold(true)
	default:
		return dimStyle()
	}
}

func statusGlyph(kind jobStatusKind) string {
	if kind == statusActive || kind == statusPaused {
		return "●"
	}
	return "○"
}

func modalBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colLavender).
		Background(colBase).
		Padding(1, 2)
}

package main

import (
	"github.com/ianptkcs/tabelatuiui"
)

// theme mirrors the installed DankMaterialShell's own configured accent
// (falling back to a manually chosen Catppuccin accent when DMS isn't
// installed/configured) — same lookup tjobs, tabelaradar and dcal use, kept
// in sync so every tool's chrome matches. TJOBS_DMS_SETTINGS/TJOBS_ACCENT env
// vars override the defaults; see tabelatuiui.NewThemeFromEnv.
var theme = tuiui.NewThemeFromEnv("TJOBS")

var (
	colBase     = theme.Base
	colMantle   = theme.Mantle
	colSurface0 = theme.Surface0
	colSurface1 = theme.Surface1
	colOverlay0 = theme.Overlay0
	colOverlay1 = theme.Overlay1
	colText     = theme.Text
	colSubtext0 = theme.Subtext0
	colPrimary  = theme.Primary
	colPink     = theme.Pink
	colGreen    = theme.Green
	colYellow   = theme.Yellow
	colRed      = theme.Red
	colBlue     = theme.Blue
	colLavender = theme.Lavender
)

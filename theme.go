package main

import (
	"path/filepath"

	"github.com/ianptkcs/tabelatuiui"
)

var (
	dmsSettingsPath = tuiui.EnvOr("DJOBS_DMS_SETTINGS", filepath.Join(tuiui.HomeDir(), ".config", "DankMaterialShell", "settings.json"))
	fallbackAccent  = tuiui.EnvOr("DJOBS_ACCENT", "mauve")
	// theme mirrors the installed DankMaterialShell's own configured accent
	// (falling back to a manually chosen Catppuccin accent when DMS isn't
	// installed/configured) — same lookup tabelaradar and dcal use, kept in
	// sync so every tool's chrome matches. See tabelatuiui.ResolveTheme.
	theme = tuiui.ResolveTheme(dmsSettingsPath, fallbackAccent)

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

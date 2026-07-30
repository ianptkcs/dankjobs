package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var (
	dmsSettingsPath = envOr("DJOBS_DMS_SETTINGS", filepath.Join(homeDir, ".config", "DankMaterialShell", "settings.json"))
	fallbackAccent  = envOr("DJOBS_ACCENT", "mauve")
)

type dmsThemeVariant struct {
	Flavor string `json:"flavor"`
	Accent string `json:"accent"`
}

type dmsSettings struct {
	CurrentThemeCategory  string `json:"currentThemeCategory"`
	CustomThemeFile       string `json:"customThemeFile"`
	RegistryThemeVariants map[string]struct {
		Dark dmsThemeVariant `json:"dark"`
	} `json:"registryThemeVariants"`
}

type catppuccinFlavorColors struct {
	Primary string `json:"primary"`
}

type catppuccinAccent struct {
	ID        string                 `json:"id"`
	Frappe    catppuccinFlavorColors `json:"frappe"`
	Latte     catppuccinFlavorColors `json:"latte"`
	Macchiato catppuccinFlavorColors `json:"macchiato"`
	Mocha     catppuccinFlavorColors `json:"mocha"`
}

func (a catppuccinAccent) primaryForFlavor(flavor string) string {
	switch flavor {
	case "frappe":
		return a.Frappe.Primary
	case "latte":
		return a.Latte.Primary
	case "macchiato":
		return a.Macchiato.Primary
	case "mocha":
		return a.Mocha.Primary
	default:
		return ""
	}
}

type dmsTheme struct {
	ID       string `json:"id"`
	Variants struct {
		Accents []catppuccinAccent `json:"accents"`
	} `json:"variants"`
}

// dmsAccentHex resolves the primary accent hex the installed
// DankMaterialShell is currently rendering, by reading its own settings.json
// + the theme.json it references — the same lookup DMS itself performs.
// Only understands DMS's Catppuccin registry theme (its accent/flavor grid
// is the one shape this parses); any other theme category, registry theme,
// or missing/malformed file returns "" so the caller falls back.
func dmsAccentHex() string {
	data, err := os.ReadFile(dmsSettingsPath)
	if err != nil {
		return ""
	}
	var settings dmsSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return ""
	}
	if settings.CurrentThemeCategory != "registry" || settings.CustomThemeFile == "" {
		return ""
	}

	themeData, err := os.ReadFile(settings.CustomThemeFile)
	if err != nil {
		return ""
	}
	var theme dmsTheme
	if err := json.Unmarshal(themeData, &theme); err != nil {
		return ""
	}
	if theme.ID != "catppuccin" {
		return ""
	}

	variant, ok := settings.RegistryThemeVariants[theme.ID]
	if !ok {
		return ""
	}
	for _, accent := range theme.Variants.Accents {
		if accent.ID == variant.Dark.Accent {
			return accent.primaryForFlavor(variant.Dark.Flavor)
		}
	}
	return ""
}

// catppuccinAccentHex maps a Catppuccin Mocha accent id to its hex, for the
// manual DJOBS_ACCENT fallback when DMS isn't installed/configured.
// Unknown ids fall back to mauve.
func catppuccinAccentHex(id string) string {
	switch id {
	case "rosewater":
		return mocha.Rosewater().Hex
	case "flamingo":
		return mocha.Flamingo().Hex
	case "pink":
		return mocha.Pink().Hex
	case "red":
		return mocha.Red().Hex
	case "maroon":
		return mocha.Maroon().Hex
	case "peach":
		return mocha.Peach().Hex
	case "yellow":
		return mocha.Yellow().Hex
	case "green":
		return mocha.Green().Hex
	case "teal":
		return mocha.Teal().Hex
	case "sky":
		return mocha.Sky().Hex
	case "sapphire":
		return mocha.Sapphire().Hex
	case "blue":
		return mocha.Blue().Hex
	case "lavender":
		return mocha.Lavender().Hex
	case "mauve":
		return mocha.Mauve().Hex
	default:
		return mocha.Mauve().Hex
	}
}

// resolvePrimaryHex is colPrimary's source: DMS's own live accent when
// available, else the manually configured (or default "mauve") fallback.
func resolvePrimaryHex() string {
	if hex := dmsAccentHex(); hex != "" {
		return hex
	}
	return catppuccinAccentHex(fallbackAccent)
}

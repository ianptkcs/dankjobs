package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDMSFixture(t *testing.T, settingsJSON, themeJSON string) {
	t.Helper()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	themePath := filepath.Join(dir, "theme.json")

	settingsJSON = strings.ReplaceAll(settingsJSON, "__THEME_PATH__", themePath)
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if themeJSON != "" {
		if err := os.WriteFile(themePath, []byte(themeJSON), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	origSettingsPath := dmsSettingsPath
	dmsSettingsPath = settingsPath
	t.Cleanup(func() { dmsSettingsPath = origSettingsPath })
}

const catppuccinThemeFixture = `{
  "id": "catppuccin",
  "variants": {
    "accents": [
      {"id": "mauve", "mocha": {"primary": "#cba6f7"}},
      {"id": "pink", "mocha": {"primary": "#f5c2e7"}},
      {"id": "blue", "mocha": {"primary": "#89b4fa"}}
    ]
  }
}`

func TestDMSAccentHex(t *testing.T) {
	tests := []struct {
		name         string
		settingsJSON string
		themeJSON    string
		want         string
	}{
		{
			name: "resolves configured accent+flavor",
			settingsJSON: `{
				"currentThemeCategory": "registry",
				"customThemeFile": "__THEME_PATH__",
				"registryThemeVariants": {"catppuccin": {"dark": {"flavor": "mocha", "accent": "pink"}}}
			}`,
			themeJSON: catppuccinThemeFixture,
			want:      "#f5c2e7",
		},
		{
			name: "non-registry category falls back",
			settingsJSON: `{
				"currentThemeCategory": "matugen",
				"customThemeFile": "__THEME_PATH__",
				"registryThemeVariants": {"catppuccin": {"dark": {"flavor": "mocha", "accent": "pink"}}}
			}`,
			themeJSON: catppuccinThemeFixture,
			want:      "",
		},
		{
			name: "non-catppuccin theme id falls back",
			settingsJSON: `{
				"currentThemeCategory": "registry",
				"customThemeFile": "__THEME_PATH__",
				"registryThemeVariants": {"catppuccin": {"dark": {"flavor": "mocha", "accent": "pink"}}}
			}`,
			themeJSON: `{"id": "somethingElse", "variants": {"accents": []}}`,
			want:      "",
		},
		{
			name: "unknown accent id falls back",
			settingsJSON: `{
				"currentThemeCategory": "registry",
				"customThemeFile": "__THEME_PATH__",
				"registryThemeVariants": {"catppuccin": {"dark": {"flavor": "mocha", "accent": "teal"}}}
			}`,
			themeJSON: catppuccinThemeFixture,
			want:      "",
		},
		{
			name:         "missing settings file falls back",
			settingsJSON: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.settingsJSON == "" {
				origSettingsPath := dmsSettingsPath
				dmsSettingsPath = filepath.Join(t.TempDir(), "does-not-exist.json")
				t.Cleanup(func() { dmsSettingsPath = origSettingsPath })
			} else {
				writeDMSFixture(t, tt.settingsJSON, tt.themeJSON)
			}
			if got := dmsAccentHex(); got != tt.want {
				t.Fatalf("dmsAccentHex() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCatppuccinAccentHex(t *testing.T) {
	ids := []string{
		"rosewater", "flamingo", "pink", "mauve", "red", "maroon", "peach",
		"yellow", "green", "teal", "sky", "sapphire", "blue", "lavender",
	}
	for _, id := range ids {
		if hex := catppuccinAccentHex(id); hex == "" {
			t.Errorf("catppuccinAccentHex(%q) returned empty hex", id)
		}
	}

	if got, want := catppuccinAccentHex("not-a-real-accent"), mocha.Mauve().Hex; got != want {
		t.Errorf("catppuccinAccentHex(unknown) = %q, want mauve fallback %q", got, want)
	}
}

func TestResolvePrimaryHex(t *testing.T) {
	origSettingsPath, origAccent := dmsSettingsPath, fallbackAccent
	t.Cleanup(func() { dmsSettingsPath, fallbackAccent = origSettingsPath, origAccent })

	dmsSettingsPath = filepath.Join(t.TempDir(), "does-not-exist.json")
	fallbackAccent = "blue"
	if got, want := resolvePrimaryHex(), mocha.Blue().Hex; got != want {
		t.Fatalf("resolvePrimaryHex() fallback = %q, want %q", got, want)
	}
}

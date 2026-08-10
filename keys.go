package main

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	"github.com/ianptkcs/tabelatuiui"
)

// reg is djobs' single source of truth for keybindings: defaults registered
// below, overrides persisted to ~/.config/djobs/keybindings.json (loaded in
// newModel via reg.Load()). Resolve() returns the effective binding, so the
// key dispatch, the footer (reg.Bindings()) and the help modal all agree —
// and a user rebind via the settings modal applies to all three at once.
var reg = tuiui.NewKeyRegistry(filepath.Join(tuiui.ConfigDir(), "djobs", "keybindings.json"))

func init() {
	reg.RegisterMany(
		tuiui.Action{ID: "quit", Help: "quit", Keys: []string{"q", "ctrl+c"}},
		tuiui.Action{ID: "help", Help: "keybindings", Keys: []string{"?"}},
		tuiui.Action{ID: "settings", Help: "rebind keys", Keys: []string{","}},
		tuiui.Action{ID: "refresh", Help: "refresh", Keys: []string{"r"}},
		tuiui.Action{ID: "new", Help: "new", Keys: []string{"n"}},
		tuiui.Action{ID: "nav-left", Help: "prev panel", Keys: []string{"ctrl+h"}},
		tuiui.Action{ID: "nav-right", Help: "next panel", Keys: []string{"ctrl+l"}},
		tuiui.Action{ID: "nav-down", Help: "details", Keys: []string{"ctrl+j"}},
		tuiui.Action{ID: "nav-up", Help: "back to panels", Keys: []string{"ctrl+k"}},
		tuiui.Action{ID: "edit", Help: "reschedule", Keys: []string{"e"}},
		tuiui.Action{ID: "toggle-pause", Help: "pause/resume", Keys: []string{"t"}},
		tuiui.Action{ID: "run-now", Help: "run now", Keys: []string{"x"}},
		tuiui.Action{ID: "delete", Help: "delete/archive", Keys: []string{"d"}},
		tuiui.Action{ID: "archive-all", Help: "archive all in panel", Keys: []string{"A"}},
		tuiui.Action{ID: "delete-all", Help: "delete all in panel", Keys: []string{"D"}},
		tuiui.Action{ID: "select-toggle", Help: "mark/unmark", Keys: []string{" "}, Label: "space"},
		tuiui.Action{ID: "select-clear", Help: "clear selection", Keys: []string{"esc"}},
		tuiui.Action{ID: "toggle-archive", Help: "archived view", Keys: []string{"a"}},
		tuiui.Action{ID: "unarchive", Help: "unarchive", Keys: []string{"u"}},
		tuiui.Action{ID: "scroll-detail", Help: "scroll details", Keys: []string{"j", "k", "up", "down"}, Label: "j/k"},
	)
}

// resolve is a short alias so Update reads like the old named keys.
func resolve(id string) key.Binding { return reg.Resolve(id) }

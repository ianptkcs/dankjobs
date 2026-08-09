package main

import (
	"github.com/charmbracelet/bubbles/key"
)

// djobs' keybindings, declared once and shared by three consumers: the key
// dispatch in updateList (key.Matches), the footer hints (tuiui.Footer) and
// the help modal (tuiui.HelpModal). The hints can never drift out of sync
// with what Update actually matches.
var (
	keyQuit          = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	keyHelp          = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keybindings"))
	keyRefresh       = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	keyNew           = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new"))
	keyNavLeft       = key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "prev panel"))
	keyNavRight      = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "next panel"))
	keyNavDown       = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "details"))
	keyNavUp         = key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "back to panels"))
	keyEdit          = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "reschedule"))
	keyTogglePause   = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "pause/resume"))
	keyRunNow        = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "run now"))
	keyDelete        = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete/archive"))
	keyToggleArchive = key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "archived view"))
	keyUnarchive     = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unarchive"))
	keyScrollDetail  = key.NewBinding(key.WithKeys("j", "k", "up", "down"), key.WithHelp("j/k", "scroll details"))
)

// appKeymap is the full list of bindings the footer hints and the help modal
// render from.
var appKeymap = []key.Binding{
	keyNew, keyEdit, keyTogglePause, keyRunNow, keyDelete, keyToggleArchive,
	keyUnarchive, keyRefresh,
	keyNavLeft, keyNavRight, keyNavDown, keyNavUp, keyScrollDetail,
	keyHelp, keyQuit,
}

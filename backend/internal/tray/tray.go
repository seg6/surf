// Package tray isolates Surf's small system-tray surface from its platform
// implementation.
package tray

import (
	"sync"

	systray "github.com/gogpu/systray"
)

// App owns Surf's tray icon, menu, and platform event loop.
type App struct {
	platform *systray.SystemTray
	menu     *systray.Menu
	quitOnce sync.Once
}

// Item is a mutable tray-menu item.
type Item struct {
	platform *systray.MenuItem
}

// New creates an empty tray. Add menu items before calling Run.
func New() *App {
	return &App{platform: systray.New(), menu: systray.NewMenu()}
}

// AddItem appends a menu item. Actions run outside the platform UI thread so
// backend restarts and management requests cannot stall the tray event loop.
func (a *App) AddItem(label string, action func()) *Item {
	var callback func()
	if action != nil {
		callback = func() { go action() }
	}
	return &Item{platform: a.menu.Add(label, callback)}
}

// AddSeparator appends a visual separator.
func (a *App) AddSeparator() {
	a.menu.AddSeparator()
}

// Run shows the configured tray and blocks until Quit is called.
func (a *App) Run(icon []byte, tooltip string) error {
	a.platform.SetIcon(icon).SetTooltip(tooltip).SetMenu(a.menu).Show()
	return a.platform.Run()
}

// Quit removes the tray and stops its event loop. It is safe from any
// goroutine and is idempotent.
func (a *App) Quit() {
	a.quitOnce.Do(a.platform.Remove)
}

// SetTitle changes a menu item's visible label.
func (i *Item) SetTitle(title string) {
	if i != nil {
		i.platform.SetLabel(title)
	}
}

// SetDisabled changes whether a menu item can be selected.
func (i *Item) SetDisabled(disabled bool) {
	if i != nil {
		i.platform.SetDisabled(disabled)
	}
}

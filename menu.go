package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/geekychris/mac_transformer/internal/app"
)

// installAppMenu builds a native macOS application menu with the standard
// AppMenu / File / Edit / View / Window / Help layout plus a Cmd+, hook to
// the frontend's settings tab. Wails handles the standard roles (Quit,
// Copy, Paste, Zoom, …) automatically.
func installAppMenu(a *application.App, svc *app.TransformerService, win *application.WebviewWindow) {
	menu := a.Menu.New()

	// Standard app menu (About / Services / Hide / Quit).
	menu.AddRole(application.AppMenu)

	// File menu: reveal-window shortcut + close.
	fileMenu := menu.AddSubmenu("File")
	fileMenu.Add("Show Window").SetAccelerator("CmdOrCtrl+0").OnClick(func(_ *application.Context) {
		if win != nil {
			win.Show()
			win.Focus()
		}
	})
	fileMenu.AddSeparator()
	fileMenu.Add("Reload Plugins").SetAccelerator("CmdOrCtrl+R").OnClick(func(_ *application.Context) {
		svc.ReloadPlugins()
	})
	fileMenu.AddSeparator()
	fileMenu.AddRole(application.CloseWindow)

	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)

	helpMenu := menu.AddSubmenu("Help")
	helpMenu.Add("Mac Transformer README").OnClick(func(_ *application.Context) {
		// Open the local README via the default URL handler. If the app is
		// installed globally the URL might not resolve — silently no-op.
		_ = a.Browser.OpenURL("https://github.com/geekychris/mac_transformer")
	})

	a.Menu.SetApplicationMenu(menu)
}

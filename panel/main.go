package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()
	app.screenWidth = detectScreenWidth()

	// Create application with options
	err := wails.Run(&options.App{
		// RULED OUT (2026-09-30): Frameless flipped to false to test whether
		// Wails' frameless mode was interacting badly with the raw X11
		// dock-type override. Result: with Frameless:false the window loses
		// its dock positioning entirely (becomes a normal shadowed floating
		// window, no longer spans the screen or pins to the top) - but the
		// WebKit content still never renders, identical to Frameless:true.
		// Not the cause. See PROJECT.md.
		Title:         "panel",
		Width:         int(app.screenWidth),
		Height:        panelHeight,
		Frameless:     true,
		DisableResize: true,
		AlwaysOnTop:   true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 31, B: 39, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

//go:build !preview

// SlotDrop: put games from your RomM library on a Slot SD card, with
// cartridge labels made from RomM's cartridge photos.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "SlotDrop",
		Width:     1180,
		Height:    780,
		MinWidth:  860,
		MinHeight: 560,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.imageHandler(),
		},
		BackgroundColour: &options.RGBA{R: 14, G: 14, B: 20, A: 255},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			DisableWindowIcon:    false,
			WebviewUserDataPath:  "", // default: %APPDATA%\<exe name>
			WebviewGpuIsDisabled: false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

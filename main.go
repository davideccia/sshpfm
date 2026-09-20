package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// icon is used for the macOS About panel and as the Linux window icon; on
// macOS and Windows the bundle icon itself comes from build/appicon.png.
//
//go:embed build/appicon.png
var icon []byte

// appID namespaces the single-instance lock. It matches CFBundleIdentifier
// in build/darwin/Info.plist.
const appID = "com.davideccia.sshpfm"

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "sshpfm",
		Width:  1024,
		Height: 768,
		// Below this the sidebar (300px) leaves the detail pane unusable.
		MinWidth:  820,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Matches --color-canvas in the frontend, so the native window does
		// not flash a different shade before the webview paints.
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 10, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		// A second instance would open the same SQLite file and race for the
		// same local ports, so it hands over to the running window instead.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               appID,
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		// The UI has no drop targets; without this a stray drag would make
		// the webview navigate away from the app.
		DragAndDrop: &options.DragAndDrop{
			DisableWebViewDrop: true,
		},
		Mac: &mac.Options{
			// The frontend declares `color-scheme: dark` unconditionally, so
			// the native chrome is pinned to dark rather than following the
			// system and clashing with it.
			Appearance: mac.NSAppearanceNameDarkAqua,
			About: &mac.AboutInfo{
				Title:   "sshpfm",
				Message: "SSH port-forwarding manager\n\nCopyright © 2026 davideccia",
				Icon:    icon,
			},
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Windows: &windows.Options{
			Theme:            windows.Dark,
			DisablePinchZoom: true,
		},
		Linux: &linux.Options{
			Icon:        icon,
			ProgramName: "sshpfm",
			// Wails applies this default itself when Linux options are nil;
			// keep it, as WebKitGTK acceleration is unreliable on VMs.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	})

	if err != nil {
		log.Fatalf("sshpfm: %v", err)
	}
}

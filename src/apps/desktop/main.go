// Command desktop is the Otter desktop app: a Wails window around the Angular
// frontend, with the Go services it calls.
package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

// MainWindowName names the only window.
const MainWindowName = "main"

func main() {
	app := application.New(application.Options{
		Name:        "Otter",
		Description: "Lightweight HTTP client for large collections",
		Services: []application.Service{
			service(NewAppService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		MarshalError: marshalError,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      MainWindowName,
		Title:     "Otter",
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 600,
		// Matches the dark --color-bg token so there is no flash before
		// Angular renders; the light theme repaints at once.
		BackgroundColour: application.NewRGB(0x0F, 0x12, 0x16),
		URL:              "/",
		// The app's top bar is the title bar: frameless on Windows and Linux
		// (it draws its own window buttons), native traffic lights over it on
		// macOS. It drags the window and maximises on double-click.
		Frameless: runtime.GOOS != "darwin",
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 40,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// service binds a service with our error marshaler: bound methods do not
// always pick up the app-level MarshalError, so it is set per service too.
func service[T any](instance *T) application.Service {
	return application.NewServiceWithOptions(instance, application.ServiceOptions{MarshalError: marshalError})
}

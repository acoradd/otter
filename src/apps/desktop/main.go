// Command desktop is the Otter desktop app: a Wails window around the Angular
// frontend, with the Go services it calls.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/acoradd/otter/src/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

// MainWindowName names the only window.
const MainWindowName = "main"

func init() {
	application.RegisterEvent[Settings](EventSettingsChanged)
	application.RegisterEvent[int64](EventWorkspacesChanged)
	application.RegisterEvent[int64](EventEnvironmentChanged)
	application.RegisterEvent[TreeChange](EventTreeChanged)
	application.RegisterEvent[int64](EventRequestChanged)
	application.RegisterEvent[int64](EventFolderChanged)
}

func main() {
	dataDir, err := appDataDir()
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(dataDir, store.FileName))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	emit := func(name string, data any) {
		if app := application.Get(); app != nil {
			app.Event.Emit(name, data)
		}
	}

	app := application.New(application.Options{
		Name:        "Otter",
		Description: "Lightweight HTTP client for large collections",
		Services: []application.Service{
			service(NewAppService()),
			service(NewSettingsService(db, emit)),
			service(NewWorkspaceService(db, emit)),
			service(NewTreeService(db, emit)),
			service(NewEditorService(db, emit, nativeDialogs{})),
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

// appDataDir is where otter.db lives: the user config folder (%AppData% on
// Windows, ~/Library/Application Support on macOS, ~/.config on Linux).
func appDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Otter"), nil
}

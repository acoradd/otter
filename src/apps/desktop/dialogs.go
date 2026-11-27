package main

import (
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// nativeDialogs shows the OS dialogs, attached to the main window.
type nativeDialogs struct{}

func (nativeDialogs) OpenFile(dir string) (string, error) {
	app := application.Get()
	d := app.Dialog.OpenFile().CanChooseFiles(true)
	if dir != "" {
		d.SetDirectory(dir)
	}
	if w, ok := app.Window.GetByName(MainWindowName); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// dirOf is the folder of a file path, "" for no path.
func dirOf(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

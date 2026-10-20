package main

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/rymdport/portal/filechooser"
)

// pickDirs lets the user pick directories through the portal, hands each to
// add and calls done once they're all in.
func (t *tray) pickDirs(title string, add func(path string), done func()) {
	home, _ := os.UserHomeDir()
	go func() {
		uris, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{Directory: true, Multiple: true, CurrentFolder: home})
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(fmt.Errorf("Unable to open the file chooser: %w", err), t.settings)
				return
			}
			for _, uri := range uris {
				add(strings.TrimPrefix(uri, "file://"))
			}
			done()
		})
	}()
}

//go:build !linux

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// pickDirs lets the user pick a directory in fyne's own dialog, hands it to
// add and calls done.
func (t *tray) pickDirs(title string, add func(path string), done func()) {
	d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, t.settings)
			return
		}
		if dir != nil {
			add(dir.Path())
			done()
		}
	}, t.settings)
	d.SetTitleText(title)
	d.Show()
}

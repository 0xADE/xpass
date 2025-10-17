package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

func Make(items []string) (fyne.App, *fyne.Container) {
	app := app.New()
	data := binding.BindStringList(&items)
	list := widget.NewListWithData(data,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			o.(*widget.Label).Bind(i.(binding.String))
		})

	filter := widget.NewEntry()
	add := widget.NewButton("Append", func() {
		val := fmt.Sprintf("Item %d", data.Length()+1)
		data.Append(val)
	})

	return app, container.NewBorder(filter, add, nil, nil, list)
}

func Run(ctx context.Context, app fyne.App, content *fyne.Container) {
	w := app.NewWindow("xpass")
	w.SetContent(content)
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeyEscape {
			w.Close()
		}
	})
	// ctrlTab := &desktop.CustomShortcut{KeyName: fyne.KeyEscape}
	// w.Canvas().AddShortcut(ctrlTab, func(shortcut fyne.Shortcut) {
	//	log.Println("We tapped Escape")
	//	os.Exit(0)
	// })
	w.SetContent(content)
	w.ShowAndRun()
}

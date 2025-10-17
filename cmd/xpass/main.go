package main

import (
	"context"

	"0xADE/xpass/config"
	"0xADE/xpass/storage"
	"0xADE/xpass/ui"

	"fyne.io/fyne/v2"
)

func main() {
	var err error
	ctx := context.Background()

	var cfg *config.Config
	if cfg, err = config.Get(); err != nil {
		fyne.LogError("can't read configuration properly", err)
	}

	storage.Init(cfg.PasswordStoreDir, cfg.PasswordStoreKey)
	items := []string{"Item 1", "Item 2", "Item 3", "Item 4", "Элемент 5"}
	app, content := ui.Make(items)
	ui.Run(ctx, app, content)
}

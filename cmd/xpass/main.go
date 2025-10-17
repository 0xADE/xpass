package main

import (
	"log"
	"os"

	"0xADE/xpass/config"
	"0xADE/xpass/storage"
	"0xADE/xpass/ui"
)

func main() {
	cfg, err := config.Get()
	if err != nil {
		log.Printf("Warning: can't read configuration properly: %v", err)
		cfg = &config.Config{}
	}

	store, err := storage.Init(cfg.PasswordStoreDir, cfg.PasswordStoreKey)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	app := ui.New(store)
	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
	
	os.Exit(0)
}

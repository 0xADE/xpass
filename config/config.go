package config

import (
	"github.com/kelseyhightower/envconfig"
)

// Config keeps configuration of `pass`. For details see `man pass`.
type Config struct {
	PasswordStoreDir         string `envconfig:"PASSWORD_STORE_DIR"`
	PasswordStoreKey         string `envconfig:"PASSWORD_STORE_KEY"`
	PasswordStoreGpgOpts     string `envconfig:"PASSWORD_STORE_GPG_OPTS"`
	PasswordStoreUmask       string `envconfig:"PASSWORD_STORE_KEY"`
	PasswordStoreClipSeconds int    `envconfig:"PASSWORD_STORE_CLIP_TIME" default:"30"` // in seconds
}

func Get() (*Config, error) {
	var p Config
	if err := envconfig.Process("", &p); err != nil {
		return nil, err
	}
	return &p, nil
}

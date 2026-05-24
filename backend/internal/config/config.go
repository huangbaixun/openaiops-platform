package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string
	ListenAddr  string
}

func FromEnv() (Config, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	addr := os.Getenv("GATEWAY_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return Config{DatabaseURL: dsn, ListenAddr: addr}, nil
}

package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	ShutdownTimeout time.Duration
	Database        DatabaseConfig
}

type DatabaseConfig struct {
	DSN        string
	MaxOpen    int
	MaxIdle    int
	MaxIdleFor time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getEnv("GOSYSTEM_HTTP_ADDR", ":8080"),
		ShutdownTimeout: getDuration("GOSYSTEM_SHUTDOWN_TIMEOUT", 10*time.Second),
		Database: DatabaseConfig{
			DSN:        getEnv("GOSYSTEM_DB_DSN", ""),
			MaxOpen:    getInt("GOSYSTEM_DB_MAX_OPEN", 20),
			MaxIdle:    getInt("GOSYSTEM_DB_MAX_IDLE", 10),
			MaxIdleFor: getDuration("GOSYSTEM_DB_MAX_IDLE_TIME", 5*time.Minute),
		},
	}

	if cfg.Database.DSN == "" {
		return cfg, errors.New("GOSYSTEM_DB_DSN is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

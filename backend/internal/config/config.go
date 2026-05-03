package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	DatabaseURL           string
	Port                  string
	ExpiryIntervalSeconds int
	LogLevel              string
}

// Load reads configuration from environment variables and returns a populated
// Config or an error if mandatory values are missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://enrique@localhost:5432/challengebeeyong_dev?sslmode=disable"),
		Port:        getEnv("PORT", "8080"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}

	intervalStr := getEnv("EXPIRY_INTERVAL_SECONDS", "5")
	interval, err := strconv.Atoi(intervalStr)
	if err != nil || interval <= 0 {
		return nil, fmt.Errorf("invalid EXPIRY_INTERVAL_SECONDS: %s", intervalStr)
	}
	cfg.ExpiryIntervalSeconds = interval

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

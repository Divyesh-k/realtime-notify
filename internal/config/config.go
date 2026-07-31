// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env             string // "development" | "production"
	Port            string
	RedisURL        string
	JWTSecret       string
	PublishAPIKey   string // shared secret for server-to-server /api/v1/publish
	PubSubDriver    string // "redis" | "memory" -- memory is single-instance/dev/test only
	AllowedOrigins  []string
	ReplayWindow    int // number of messages to retain per channel for reconnect replay
	ReplayTTL       time.Duration
	PingInterval    time.Duration
	WriteTimeout    time.Duration
	MaxMessageSize  int64
}

func Load() (*Config, error) {
	cfg := &Config{
		Env:            getEnv("APP_ENV", "development"),
		Port:           getEnv("PORT", "8080"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		PublishAPIKey:  getEnv("PUBLISH_API_KEY", ""),
		PubSubDriver:   getEnv("PUBSUB_DRIVER", "redis"),
		AllowedOrigins: getEnvList("ALLOWED_ORIGINS", nil),
		ReplayWindow:   getEnvInt("REPLAY_WINDOW", 50),
		ReplayTTL:      getEnvDuration("REPLAY_TTL", 5*time.Minute),
		PingInterval:   getEnvDuration("PING_INTERVAL", 30*time.Second),
		WriteTimeout:   getEnvDuration("WRITE_TIMEOUT", 10*time.Second),
		MaxMessageSize: int64(getEnvInt("MAX_MESSAGE_SIZE", 32*1024)), // 32KB
	}

	if cfg.PubSubDriver != "redis" && cfg.PubSubDriver != "memory" {
		return nil, fmt.Errorf("PUBSUB_DRIVER must be 'redis' or 'memory', got %q", cfg.PubSubDriver)
	}

	if cfg.Env == "production" {
		if cfg.JWTSecret == "" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		if cfg.PublishAPIKey == "" {
			return nil, fmt.Errorf("PUBLISH_API_KEY must be set in production")
		}
		if len(cfg.JWTSecret) < 32 {
			return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters in production")
		}
		if cfg.PubSubDriver == "memory" {
			return nil, fmt.Errorf("PUBSUB_DRIVER=memory cannot fan out across instances; use 'redis' in production")
		}
		if len(cfg.AllowedOrigins) == 0 {
			return nil, fmt.Errorf("ALLOWED_ORIGINS must be set in production (e.g. https://app.example.com)")
		}
	}

	// Dev default: wide open so the demo client and local tools work with
	// zero config. Production must set ALLOWED_ORIGINS explicitly -- see
	// the check above, which refuses to start without it.
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"*"}
	}

	// Sensible dev defaults so `docker compose up` works with zero config.
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "dev-only-secret-do-not-use-in-production"
	}
	if cfg.PublishAPIKey == "" {
		cfg.PublishAPIKey = "dev-only-publish-key"
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getEnvList(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

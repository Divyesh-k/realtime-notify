package config

import "testing"

func withEnv(t *testing.T, kv map[string]string, fn func()) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
	fn()
}

func TestLoadDevelopmentDefaults(t *testing.T) {
	withEnv(t, map[string]string{"APP_ENV": "development", "JWT_SECRET": "", "PUBLISH_API_KEY": ""}, func() {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.JWTSecret == "" || cfg.PublishAPIKey == "" {
			t.Fatal("expected dev defaults to be filled in")
		}
	})
}

func TestLoadProductionRequiresJWTSecret(t *testing.T) {
	withEnv(t, map[string]string{"APP_ENV": "production", "JWT_SECRET": "", "PUBLISH_API_KEY": "some-key-thats-long-enough"}, func() {
		if _, err := Load(); err == nil {
			t.Fatal("expected error when JWT_SECRET missing in production")
		}
	})
}

func TestLoadProductionRejectsShortSecret(t *testing.T) {
	withEnv(t, map[string]string{"APP_ENV": "production", "JWT_SECRET": "too-short", "PUBLISH_API_KEY": "k"}, func() {
		if _, err := Load(); err == nil {
			t.Fatal("expected error for JWT_SECRET under 32 characters in production")
		}
	})
}

func TestLoadProductionRejectsMemoryDriver(t *testing.T) {
	withEnv(t, map[string]string{
		"APP_ENV":         "production",
		"JWT_SECRET":      "a-secret-that-is-at-least-32-characters-long",
		"PUBLISH_API_KEY": "k",
		"PUBSUB_DRIVER":   "memory",
	}, func() {
		if _, err := Load(); err == nil {
			t.Fatal("expected error: memory driver can't fan out across instances in production")
		}
	})
}

func TestLoadRejectsInvalidDriver(t *testing.T) {
	withEnv(t, map[string]string{"PUBSUB_DRIVER": "kafka"}, func() {
		if _, err := Load(); err == nil {
			t.Fatal("expected error for unsupported PUBSUB_DRIVER value")
		}
	})
}

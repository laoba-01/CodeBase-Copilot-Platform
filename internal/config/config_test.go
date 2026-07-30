package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Save and restore env
	oldJWT := os.Getenv("JWT_SECRET")
	oldDev := os.Getenv("DEV_MODE")
	defer func() {
		os.Setenv("JWT_SECRET", oldJWT)
		os.Setenv("DEV_MODE", oldDev)
	}()

	// Test dev mode defaults
	os.Setenv("JWT_SECRET", "")
	os.Setenv("DEV_MODE", "true")
	cfg := Load()
	if cfg.JWTSecret != "dev-secret-dev-mode-only" {
		t.Errorf("dev mode JWT: got %q, want dev-secret-dev-mode-only", cfg.JWTSecret)
	}
	if !cfg.DevMode {
		t.Error("DevMode should be true by default")
	}
	if cfg.AllowedOrigin != "http://localhost:8080" {
		t.Errorf("AllowedOrigin: got %q", cfg.AllowedOrigin)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port: got %q", cfg.Port)
	}
}

func TestLoadWithEnvVars(t *testing.T) {
	oldJWT := os.Getenv("JWT_SECRET")
	oldOrigin := os.Getenv("CORS_ORIGIN")
	oldDev := os.Getenv("DEV_MODE")
	defer func() {
		os.Setenv("JWT_SECRET", oldJWT)
		os.Setenv("CORS_ORIGIN", oldOrigin)
		os.Setenv("DEV_MODE", oldDev)
	}()

	os.Setenv("JWT_SECRET", "my-production-secret-key-123")
	os.Setenv("CORS_ORIGIN", "https://example.com")
	os.Setenv("DEV_MODE", "false")
	cfg := Load()
	if cfg.JWTSecret != "my-production-secret-key-123" {
		t.Errorf("JWTSecret: got %q", cfg.JWTSecret)
	}
	if cfg.AllowedOrigin != "https://example.com" {
		t.Errorf("AllowedOrigin: got %q", cfg.AllowedOrigin)
	}
	if cfg.DevMode {
		t.Error("DevMode should be false")
	}
}

func TestLoadShortJWTSecretPanics(t *testing.T) {
	oldJWT := os.Getenv("JWT_SECRET")
	oldDev := os.Getenv("DEV_MODE")
	defer func() {
		os.Setenv("JWT_SECRET", oldJWT)
		os.Setenv("DEV_MODE", oldDev)
	}()

	os.Setenv("JWT_SECRET", "short")
	os.Setenv("DEV_MODE", "false")

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for short JWT_SECRET")
		}
	}()
	Load()
}

package config

import "os"

type Config struct {
	Port               string
	DatabaseURL        string
	RedisURL           string
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
	GiteeClientID      string
	GiteeClientSecret  string
	GiteeRedirectURI   string
	EmbeddingAddr      string
	LLMProvider        string
	LLMAPIKey          string
	LLMModel           string
	AllowedOrigin      string
	DevMode            bool
	GitSSLVerify       bool
}

func Load() *Config {
	cfg := &Config{
		Port:               env("PORT", "8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:          env("JWT_SECRET", ""),
		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		GiteeClientID:      env("GITEE_CLIENT_ID", ""),
		GiteeClientSecret:  env("GITEE_CLIENT_SECRET", ""),
		GiteeRedirectURI:   env("GITEE_REDIRECT_URI", "http://localhost:8080/auth/gitee/callback"),
		EmbeddingAddr:      env("EMBEDDING_ADDR", "localhost:50051"),
		LLMProvider:        env("LLM_PROVIDER", "claude"),
		LLMAPIKey:          env("LLM_API_KEY", ""),
		LLMModel:           env("LLM_MODEL", "claude-sonnet-5"),
		AllowedOrigin:      env("CORS_ORIGIN", "http://localhost:8080"),
		DevMode:            env("DEV_MODE", "true") == "true",
		GitSSLVerify:       env("GIT_SSL_VERIFY", "true") == "true",
	}
	// Validate required secrets at startup
	if cfg.JWTSecret == "" || cfg.JWTSecret == "change-me-in-production" || cfg.JWTSecret == "dev-secret" {
		if cfg.DevMode {
			cfg.JWTSecret = "dev-secret-dev-mode-only"
		} else {
			panic("JWT_SECRET is required and must not use default values")
		}
	}
	if len(cfg.JWTSecret) < 16 {
		panic("JWT_SECRET must be at least 16 characters")
	}
	return cfg
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

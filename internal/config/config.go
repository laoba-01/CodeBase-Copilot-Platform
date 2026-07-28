package config

import "os"

type Config struct {
	Port               string
	DatabaseURL        string
	RedisURL           string
	JWTSecret          string
	GitHubClientID     string
	GitHubClientSecret string
	EmbeddingAddr      string
	LLMProvider        string
	LLMAPIKey          string
	LLMModel           string
}

func Load() *Config {
	return &Config{
		Port:               env("PORT", "8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://copilot:copilot@localhost:5432/copilot?sslmode=disable"),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:          env("JWT_SECRET", "dev-secret"),
		GitHubClientID:     env("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
		EmbeddingAddr:      env("EMBEDDING_ADDR", "localhost:50051"),
		LLMProvider:        env("LLM_PROVIDER", "claude"),
		LLMAPIKey:          env("LLM_API_KEY", ""),
		LLMModel:           env("LLM_MODEL", "claude-sonnet-5"),
	}
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

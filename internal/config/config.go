package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                   string
	DefaultModel           string
	AllowedAPIKeys         []string
	LocalVLLMURL           string
	OpenAICompatibleURL    string
	OpenAICompatibleAPIKey string
	DeepSeekURL            string
	DeepSeekAPIKey         string
	ProviderTimeout        time.Duration
	EnableFakeProvider     bool
	LogLevel               slog.Level
}

func Load() Config {
	return Config{
		Addr:                   env("GATEWAY_ADDR", ":8080"),
		DefaultModel:           env("DEFAULT_MODEL", "honghu-fake-llm"),
		AllowedAPIKeys:         csvEnv("GATEWAY_API_KEYS"),
		LocalVLLMURL:           strings.TrimRight(os.Getenv("LOCAL_VLLM_URL"), "/"),
		OpenAICompatibleURL:    strings.TrimRight(os.Getenv("OPENAI_COMPATIBLE_URL"), "/"),
		OpenAICompatibleAPIKey: os.Getenv("OPENAI_COMPATIBLE_API_KEY"),
		DeepSeekURL:            strings.TrimRight(env("DEEPSEEK_URL", "https://api.deepseek.com"), "/"),
		DeepSeekAPIKey:         os.Getenv("DEEPSEEK_API_KEY"),
		ProviderTimeout:        durationEnv("PROVIDER_TIMEOUT", 60*time.Second),
		EnableFakeProvider:     boolEnv("ENABLE_FAKE_PROVIDER", false),
		LogLevel:               logLevelEnv("LOG_LEVEL", slog.LevelInfo),
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func csvEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err == nil {
		return value
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func logLevelEnv(key string, fallback slog.Level) slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return fallback
	default:
		return fallback
	}
}

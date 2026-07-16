package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains the runtime configuration for Nivora.
type Config struct {
	Address              string
	SharedSecret         string
	TenantID             string
	ProviderBaseURL      string
	ProviderSharedSecret string
	ArkAPIKey            string
	ArkModel             string
	ArkBaseURL           string
	RequestTimeout       time.Duration
	ProviderTimeout      time.Duration
	MaxHistoryTurns      int
	MaxQuestionBytes     int
	Version              string
	Commit               string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		Address:              env("NIVORA_ADDR", "127.0.0.1:3100"),
		TenantID:             env("NIVORA_TENANT_ID", "lumio"),
		ProviderBaseURL:      strings.TrimRight(env("NIVORA_PROVIDER_BASE_URL", "http://127.0.0.1:3000"), "/"),
		ArkBaseURL:           strings.TrimRight(env("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), "/"),
		RequestTimeout:       durationEnv("NIVORA_REQUEST_TIMEOUT", 90*time.Second),
		ProviderTimeout:      durationEnv("NIVORA_PROVIDER_TIMEOUT", 10*time.Second),
		MaxHistoryTurns:      intEnv("NIVORA_MAX_HISTORY_TURNS", 12),
		MaxQuestionBytes:     intEnv("NIVORA_MAX_QUESTION_BYTES", 16*1024),
		SharedSecret:         strings.TrimSpace(os.Getenv("NIVORA_SHARED_SECRET")),
		ProviderSharedSecret: strings.TrimSpace(os.Getenv("NIVORA_PROVIDER_SHARED_SECRET")),
		ArkAPIKey:            strings.TrimSpace(os.Getenv("ARK_API_KEY")),
		ArkModel:             strings.TrimSpace(os.Getenv("ARK_CHAT_MODEL")),
		Version:              env("NIVORA_VERSION", "dev"),
		Commit:               env("NIVORA_COMMIT", "unknown"),
	}

	if cfg.Address == "" {
		return Config{}, errors.New("NIVORA_ADDR must not be empty")
	}
	if cfg.TenantID == "" {
		return Config{}, errors.New("NIVORA_TENANT_ID must not be empty")
	}
	if cfg.ProviderBaseURL == "" {
		return Config{}, errors.New("NIVORA_PROVIDER_BASE_URL must not be empty")
	}
	if cfg.MaxHistoryTurns < 0 || cfg.MaxHistoryTurns > 100 {
		return Config{}, fmt.Errorf("NIVORA_MAX_HISTORY_TURNS must be between 0 and 100")
	}
	if cfg.MaxQuestionBytes < 256 {
		return Config{}, fmt.Errorf("NIVORA_MAX_QUESTION_BYTES must be at least 256")
	}
	return cfg, nil
}

// Ready reports whether the service has enough configuration to accept chat requests.
func (c Config) Ready() bool {
	return c.SharedSecret != "" && c.ArkAPIKey != "" && c.ArkModel != "" && c.ProviderBaseURL != ""
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

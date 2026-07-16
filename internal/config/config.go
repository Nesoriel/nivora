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
	ProviderMaxRetries   int
	ProviderRetryBackoff time.Duration
	ArkAPIKey            string
	ArkModels            []string
	ArkBaseURL           string
	RequestTimeout       time.Duration
	ProviderTimeout      time.Duration
	ReadinessTimeout     time.Duration
	ReadinessCacheTTL    time.Duration
	QueueTimeout         time.Duration
	SSEHeartbeat         time.Duration
	MaxConcurrentRuns    int
	MaxHistoryTurns      int
	MaxQuestionBytes     int
	Version              string
	Commit               string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	models := csvEnv("ARK_CHAT_MODELS")
	if len(models) == 0 {
		models = csv(strings.TrimSpace(os.Getenv("ARK_CHAT_MODEL")))
	}

	cfg := Config{
		Address:              env("NIVORA_ADDR", "127.0.0.1:3100"),
		TenantID:             env("NIVORA_TENANT_ID", "lumio"),
		ProviderBaseURL:      strings.TrimRight(env("NIVORA_PROVIDER_BASE_URL", "http://127.0.0.1:3000"), "/"),
		ArkBaseURL:           strings.TrimRight(env("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), "/"),
		RequestTimeout:       durationEnv("NIVORA_REQUEST_TIMEOUT", 90*time.Second),
		ProviderTimeout:      durationEnv("NIVORA_PROVIDER_TIMEOUT", 10*time.Second),
		ReadinessTimeout:     durationEnv("NIVORA_READINESS_TIMEOUT", 2*time.Second),
		ReadinessCacheTTL:    durationEnv("NIVORA_READINESS_CACHE_TTL", 10*time.Second),
		QueueTimeout:         durationEnv("NIVORA_QUEUE_TIMEOUT", 2*time.Second),
		SSEHeartbeat:         durationEnv("NIVORA_SSE_HEARTBEAT", 15*time.Second),
		ProviderRetryBackoff: durationEnv("NIVORA_PROVIDER_RETRY_BACKOFF", 150*time.Millisecond),
		ProviderMaxRetries:   intEnv("NIVORA_PROVIDER_MAX_RETRIES", 2),
		MaxConcurrentRuns:    intEnv("NIVORA_MAX_CONCURRENT_RUNS", 4),
		MaxHistoryTurns:      intEnv("NIVORA_MAX_HISTORY_TURNS", 12),
		MaxQuestionBytes:     intEnv("NIVORA_MAX_QUESTION_BYTES", 16*1024),
		SharedSecret:         strings.TrimSpace(os.Getenv("NIVORA_SHARED_SECRET")),
		ProviderSharedSecret: strings.TrimSpace(os.Getenv("NIVORA_PROVIDER_SHARED_SECRET")),
		ArkAPIKey:            strings.TrimSpace(os.Getenv("ARK_API_KEY")),
		ArkModels:            models,
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
		return Config{}, errors.New("NIVORA_MAX_HISTORY_TURNS must be between 0 and 100")
	}
	if cfg.MaxQuestionBytes < 256 {
		return Config{}, errors.New("NIVORA_MAX_QUESTION_BYTES must be at least 256")
	}
	if cfg.MaxConcurrentRuns < 1 || cfg.MaxConcurrentRuns > 1024 {
		return Config{}, errors.New("NIVORA_MAX_CONCURRENT_RUNS must be between 1 and 1024")
	}
	if cfg.ProviderMaxRetries < 0 || cfg.ProviderMaxRetries > 10 {
		return Config{}, errors.New("NIVORA_PROVIDER_MAX_RETRIES must be between 0 and 10")
	}
	if cfg.RequestTimeout <= 0 || cfg.ProviderTimeout <= 0 || cfg.ReadinessTimeout <= 0 {
		return Config{}, errors.New("request, provider, and readiness timeouts must be positive")
	}
	if cfg.ReadinessCacheTTL < 0 || cfg.QueueTimeout < 0 || cfg.SSEHeartbeat < 0 || cfg.ProviderRetryBackoff < 0 {
		return Config{}, fmt.Errorf("cache, queue, heartbeat, and retry durations must not be negative")
	}
	return cfg, nil
}

// Ready reports whether the service has enough configuration to accept chat requests.
func (c Config) Ready() bool {
	return c.SharedSecret != "" && c.ProviderSharedSecret != "" && c.ArkAPIKey != "" && len(c.ArkModels) > 0 && c.ProviderBaseURL != ""
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

func csvEnv(name string) []string {
	return csv(os.Getenv(name))
}

func csv(raw string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

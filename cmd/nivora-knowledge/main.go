package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Nesoriel/nivora/pkg/knowledge"
	"github.com/Nesoriel/nivora/pkg/knowledge/httpapi"
	knowledgeviking "github.com/Nesoriel/nivora/pkg/knowledge/vikingdb"
)

type config struct {
	Address           string
	SharedSecret      string
	Host              string
	Region            string
	AK                string
	SK                string
	Scheme            string
	Collection        string
	Index             string
	Partition         string
	ConnectionTimeout int64
	WithMultiModal    bool
	EmbeddingModel    string
	UseSparse         bool
	DenseWeight       float64
	Oversample        int
	MinimumScore      float64
	RequestTimeout    time.Duration
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("load knowledge configuration", "error", err)
		os.Exit(1)
	}

	backend, err := knowledgeviking.New(context.Background(), knowledgeviking.Config{
		Host:              cfg.Host,
		Region:            cfg.Region,
		AK:                cfg.AK,
		SK:                cfg.SK,
		Scheme:            cfg.Scheme,
		Collection:        cfg.Collection,
		Index:             cfg.Index,
		Partition:         cfg.Partition,
		ConnectionTimeout: cfg.ConnectionTimeout,
		WithMultiModal:    cfg.WithMultiModal,
		EmbeddingModel:    cfg.EmbeddingModel,
		UseSparse:         cfg.UseSparse,
		DenseWeight:       cfg.DenseWeight,
		Oversample:        cfg.Oversample,
	})
	if err != nil {
		logger.Error("initialize VikingDB knowledge backend", "error", err)
		os.Exit(1)
	}
	service, err := knowledge.NewService(backend, knowledge.WithMinimumScore(cfg.MinimumScore))
	if err != nil {
		logger.Error("create approved knowledge service", "error", err)
		os.Exit(1)
	}
	api, err := httpapi.New(service, cfg.SharedSecret)
	if err != nil {
		logger.Error("create approved knowledge HTTP API", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           http.TimeoutHandler(api.Handler(), cfg.RequestTimeout, `{"error":"request_timeout"}`),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Info("Nivora approved knowledge service started", "address", cfg.Address, "collection", cfg.Collection, "index", cfg.Index)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("knowledge HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("knowledge service graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("Nivora approved knowledge service stopped")
}

func loadConfig() (config, error) {
	cfg := config{
		Address:           env("NIVORA_KNOWLEDGE_ADDR", "127.0.0.1:3110"),
		SharedSecret:      strings.TrimSpace(os.Getenv("NIVORA_KNOWLEDGE_SHARED_SECRET")),
		Host:              strings.TrimSpace(os.Getenv("VIKINGDB_HOST")),
		Region:            strings.TrimSpace(os.Getenv("VIKINGDB_REGION")),
		AK:                strings.TrimSpace(os.Getenv("VIKINGDB_AK")),
		SK:                strings.TrimSpace(os.Getenv("VIKINGDB_SK")),
		Scheme:            env("VIKINGDB_SCHEME", "https"),
		Collection:        strings.TrimSpace(os.Getenv("VIKINGDB_COLLECTION")),
		Index:             strings.TrimSpace(os.Getenv("VIKINGDB_INDEX")),
		Partition:         env("VIKINGDB_PARTITION", "default"),
		ConnectionTimeout: int64(intEnv("VIKINGDB_CONNECTION_TIMEOUT_SECONDS", 5)),
		WithMultiModal:    boolEnv("VIKINGDB_WITH_MULTIMODAL", true),
		EmbeddingModel:    strings.TrimSpace(os.Getenv("VIKINGDB_EMBEDDING_MODEL")),
		UseSparse:         boolEnv("VIKINGDB_USE_SPARSE", true),
		DenseWeight:       floatEnv("VIKINGDB_DENSE_WEIGHT", 0.7),
		Oversample:        intEnv("NIVORA_KNOWLEDGE_OVERSAMPLE", 3),
		MinimumScore:      floatEnv("NIVORA_KNOWLEDGE_MIN_SCORE", 0.75),
		RequestTimeout:    durationEnv("NIVORA_KNOWLEDGE_REQUEST_TIMEOUT", 15*time.Second),
	}
	if cfg.SharedSecret == "" {
		return config{}, errors.New("NIVORA_KNOWLEDGE_SHARED_SECRET is required")
	}
	if cfg.Host == "" || cfg.Region == "" || cfg.Collection == "" || cfg.Index == "" {
		return config{}, errors.New("VIKINGDB_HOST, VIKINGDB_REGION, VIKINGDB_COLLECTION, and VIKINGDB_INDEX are required")
	}
	if cfg.AK == "" || cfg.SK == "" {
		return config{}, errors.New("VIKINGDB_AK and VIKINGDB_SK are required")
	}
	if !cfg.WithMultiModal && cfg.EmbeddingModel == "" {
		return config{}, errors.New("VIKINGDB_EMBEDDING_MODEL is required when VIKINGDB_WITH_MULTIMODAL=false")
	}
	if cfg.MinimumScore < 0 || cfg.MinimumScore > 1 {
		return config{}, errors.New("NIVORA_KNOWLEDGE_MIN_SCORE must be between 0 and 1")
	}
	if cfg.DenseWeight < 0.2 || cfg.DenseWeight > 1 {
		return config{}, errors.New("VIKINGDB_DENSE_WEIGHT must be between 0.2 and 1")
	}
	if cfg.RequestTimeout <= 0 {
		return config{}, errors.New("NIVORA_KNOWLEDGE_REQUEST_TIMEOUT must be positive")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func floatEnv(name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(name)), 64)
	if err != nil {
		return fallback
	}
	return value
}

func boolEnv(name string, fallback bool) bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

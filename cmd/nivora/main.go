package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einomodel "github.com/cloudwego/eino/components/model"

	"github.com/Nesoriel/nivora/internal/agent"
	"github.com/Nesoriel/nivora/internal/config"
	"github.com/Nesoriel/nivora/internal/model/failover"
	providerhttp "github.com/Nesoriel/nivora/internal/provider/httpclient"
	"github.com/Nesoriel/nivora/internal/telemetry"
	"github.com/Nesoriel/nivora/internal/transport/httpserver"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	providerClient, err := providerhttp.New(
		cfg.ProviderBaseURL,
		cfg.ProviderSharedSecret,
		&http.Client{Timeout: cfg.ProviderTimeout},
		providerhttp.WithRetry(cfg.ProviderMaxRetries, cfg.ProviderRetryBackoff),
	)
	if err != nil {
		logger.Error("create provider client", "error", err)
		os.Exit(1)
	}

	var runtime *agent.Service
	if cfg.ArkAPIKey != "" && len(cfg.ArkModels) > 0 {
		models := make([]einomodel.ToolCallingChatModel, 0, len(cfg.ArkModels))
		for _, modelID := range cfg.ArkModels {
			modelTimeout := cfg.RequestTimeout
			chatModel, modelErr := ark.NewChatModel(context.Background(), &ark.ChatModelConfig{
				APIKey:  cfg.ArkAPIKey,
				Model:   modelID,
				BaseURL: cfg.ArkBaseURL,
				Timeout: &modelTimeout,
			})
			if modelErr != nil {
				logger.Error("create Ark chat model", "error", modelErr)
				os.Exit(1)
			}
			models = append(models, chatModel)
		}

		var runtimeModel einomodel.ToolCallingChatModel = models[0]
		if len(models) > 1 {
			runtimeModel, err = failover.New(models...)
			if err != nil {
				logger.Error("create Ark failover model", "error", err)
				os.Exit(1)
			}
		}
		runtime, err = agent.New(runtimeModel, providerClient)
		if err != nil {
			logger.Error("create agent runtime", "error", err)
			os.Exit(1)
		}
		logger.Info("Ark runtime configured", "model_endpoints", len(models))
	} else {
		logger.Warn("Ark model is not configured; health endpoints are available but readiness will fail")
	}

	metrics := telemetry.New()
	transport := httpserver.New(cfg, runtime, providerClient, metrics, logger)
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           transport.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Nivora started", "address", cfg.Address, "version", cfg.Version, "commit", cfg.Commit)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("Nivora stopped")
}

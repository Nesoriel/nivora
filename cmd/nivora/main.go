package main

import (
	"context"
	"encoding/json"
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
	"github.com/Nesoriel/nivora/internal/promptpolicy"
	providerhttp "github.com/Nesoriel/nivora/internal/provider/httpclient"
	"github.com/Nesoriel/nivora/internal/requestctx"
	looptrace "github.com/Nesoriel/nivora/internal/runtrace/cozeloop"
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

	loopRuntime, loopErr := looptrace.New(cfg.CozeLoopEnabled, logger)
	if loopErr != nil {
		logger.Warn("CozeLoop initialization failed; tracing and remote prompts are disabled", "error", loopErr)
		loopRuntime = looptrace.Disabled(logger)
	}

	var policy promptpolicy.Source = promptpolicy.Static("", "bundled-v1", "bundled")
	if loopRuntime.Client() != nil && cfg.CozeLoopPromptKey != "" {
		remotePolicy, promptErr := promptpolicy.Remote(context.Background(), loopRuntime.Client(), promptpolicy.RemoteConfig{
			Key:             cfg.CozeLoopPromptKey,
			Version:         cfg.CozeLoopPromptVersion,
			RefreshInterval: cfg.CozeLoopPromptRefresh,
			RequestTimeout:  cfg.CozeLoopPromptTimeout,
			Fallback:        policy,
			Logger:          logger,
		})
		if promptErr != nil {
			logger.Warn("create CozeLoop prompt policy source", "error", promptErr)
		} else {
			policy = remotePolicy
		}
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
		runtime, err = agent.New(
			runtimeModel,
			providerClient,
			agent.WithPolicySource(policy),
			agent.WithTracer(loopRuntime.Tracer()),
			agent.WithBuildInfo(cfg.Version, cfg.Commit),
		)
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
	root := http.NewServeMux()
	root.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := policy.Current()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":          cfg.Version,
			"commit":           cfg.Commit,
			"prompt_key":       cfg.CozeLoopPromptKey,
			"prompt_version":   snapshot.Version,
			"prompt_source":    snapshot.Source,
			"cozeloop_enabled": cfg.CozeLoopEnabled && loopRuntime.Client() != nil,
		})
	})
	root.Handle("/", requestctx.Middleware(transport.Handler()))

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           root,
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
	policy.Close()
	loopRuntime.Close(ctx)
	logger.Info("Nivora stopped")
}

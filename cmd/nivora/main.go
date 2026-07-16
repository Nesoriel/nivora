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
	"github.com/Nesoriel/nivora/internal/conversation"
	conversationhttp "github.com/Nesoriel/nivora/internal/conversation/httpapi"
	"github.com/Nesoriel/nivora/internal/conversation/sqlstore"
	"github.com/Nesoriel/nivora/internal/dependency"
	"github.com/Nesoriel/nivora/internal/model/failover"
	"github.com/Nesoriel/nivora/internal/promptpolicy"
	"github.com/Nesoriel/nivora/internal/provider"
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
	serviceCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	loopRuntime, loopErr := looptrace.New(cfg.CozeLoopEnabled, logger)
	if loopErr != nil {
		logger.Warn("CozeLoop initialization failed; tracing and remote prompts are disabled", "error", loopErr)
		loopRuntime = looptrace.Disabled(logger)
	}

	var policy promptpolicy.Source = promptpolicy.Static("", "bundled-v1", "bundled")
	if loopRuntime.Client() != nil && cfg.CozeLoopPromptKey != "" {
		remotePolicy, promptErr := promptpolicy.Remote(serviceCtx, loopRuntime.Client(), promptpolicy.RemoteConfig{
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

	conversationStore := conversation.Nop()
	storageEnabled := cfg.StorageDriver != "" && cfg.StorageDSN != ""
	if storageEnabled {
		durableStore, storageErr := sqlstore.Open(serviceCtx, cfg.StorageDriver, cfg.StorageDSN)
		if storageErr != nil {
			logger.Error("open durable conversation store", "error", storageErr)
			os.Exit(1)
		}
		conversationStore = durableStore
		if cfg.StorageCleanupInterval > 0 {
			go retentionLoop(serviceCtx, conversationStore, cfg.StorageRetention, cfg.StorageCleanupInterval, logger)
		}
		logger.Info("durable conversation storage configured", "driver", cfg.StorageDriver)
	} else if cfg.StorageRequired {
		logger.Error("durable conversation storage is required but not configured")
		os.Exit(1)
	} else {
		logger.Warn("durable conversation storage is disabled; production acceptance requires enabling it")
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
	var runtimeProvider provider.Provider = providerClient
	if storageEnabled {
		recordedProvider, recordErr := conversation.NewProviderRecorder(runtimeProvider, conversationStore, cfg.TenantID)
		if recordErr != nil {
			logger.Error("create Provider audit recorder", "error", recordErr)
			os.Exit(1)
		}
		runtimeProvider = recordedProvider
	}

	var streamer conversation.Streamer
	if cfg.ArkAPIKey != "" && len(cfg.ArkModels) > 0 {
		models := make([]einomodel.ToolCallingChatModel, 0, len(cfg.ArkModels))
		for _, modelID := range cfg.ArkModels {
			modelTimeout := cfg.RequestTimeout
			chatModel, modelErr := ark.NewChatModel(serviceCtx, &ark.ChatModelConfig{
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
		runtime, runtimeErr := agent.New(
			runtimeModel,
			runtimeProvider,
			agent.WithPolicySource(policy),
			agent.WithTracer(loopRuntime.Tracer()),
			agent.WithBuildInfo(cfg.Version, cfg.Commit),
		)
		if runtimeErr != nil {
			logger.Error("create agent runtime", "error", runtimeErr)
			os.Exit(1)
		}
		streamer = runtime
		if storageEnabled {
			recorder, recordErr := conversation.NewRecorder(runtime, conversationStore, cfg.Version, cfg.Commit, func() (string, string) {
				snapshot := policy.Current()
				return snapshot.Version, snapshot.Source
			})
			if recordErr != nil {
				logger.Error("create durable run recorder", "error", recordErr)
				os.Exit(1)
			}
			streamer = recorder
		}
		logger.Info("Ark runtime configured", "model_endpoints", len(models))
	} else {
		logger.Warn("Ark model is not configured; health endpoints are available but readiness will fail")
	}

	checkers := []dependency.Checker{providerClient}
	if storageEnabled {
		checkers = append(checkers, conversationStore)
	}
	metrics := telemetry.New()
	transport := httpserver.New(cfg, streamer, dependency.New(checkers...), metrics, logger)
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
			"storage_enabled":  storageEnabled,
			"storage_driver":   cfg.StorageDriver,
		})
	})
	if storageEnabled {
		transcriptAPI, transcriptErr := conversationhttp.New(conversationStore, cfg.TenantID, cfg.SharedSecret)
		if transcriptErr != nil {
			logger.Error("create transcript API", "error", transcriptErr)
			os.Exit(1)
		}
		transcriptAPI.Register(root)
	}
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
	stopBackground()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	policy.Close()
	loopRuntime.Close(ctx)
	if err := conversationStore.Close(); err != nil {
		logger.Error("close conversation store", "error", err)
		os.Exit(1)
	}
	logger.Info("Nivora stopped")
}

func retentionLoop(ctx context.Context, store conversation.Store, retention, interval time.Duration, logger *slog.Logger) {
	run := func() {
		result, err := store.DeleteBefore(ctx, time.Now().UTC().Add(-retention))
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("conversation retention cleanup failed", "error", err)
			}
			return
		}
		logger.Info("conversation retention cleanup completed",
			"runs", result.Runs,
			"messages", result.Messages,
			"tool_audits", result.ToolAudits,
			"support_cases", result.SupportCases,
			"conversations", result.Conversations,
		)
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Nesoriel/nivora/internal/testprovider"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	address := env("NIVORA_TEST_PROVIDER_ADDR", "127.0.0.1:3120")
	secret := strings.TrimSpace(os.Getenv("NIVORA_TEST_PROVIDER_SHARED_SECRET"))
	bearer := strings.TrimSpace(os.Getenv("NIVORA_TEST_PROVIDER_BEARER_TOKEN"))
	if secret == "" || bearer == "" {
		logger.Error("NIVORA_TEST_PROVIDER_SHARED_SECRET and NIVORA_TEST_PROVIDER_BEARER_TOKEN are required")
		os.Exit(1)
	}
	provider := testprovider.New(testprovider.Config{
		SharedSecret: secret,
		BearerToken:  bearer,
		Delay:        durationEnv("NIVORA_TEST_PROVIDER_RESPONSE_DELAY", 0),
	})
	handler := testprovider.WithFaults(provider.Handler(), testprovider.FaultConfig{
		StatusCode: intEnv("NIVORA_TEST_PROVIDER_FAILURE_STATUS", 0),
		Count:      int64(intEnv("NIVORA_TEST_PROVIDER_FAILURE_COUNT", 0)),
		Delay:      durationEnv("NIVORA_TEST_PROVIDER_FAULT_DELAY", 0),
	})
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Warn("synthetic Provider started; never use this service with production customer traffic",
			"address", address,
			"failure_status", intEnv("NIVORA_TEST_PROVIDER_FAILURE_STATUS", 0),
			"failure_count", intEnv("NIVORA_TEST_PROVIDER_FAILURE_COUNT", 0),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("synthetic Provider stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()
	<-shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("synthetic Provider shutdown failed", "error", err)
		os.Exit(1)
	}
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

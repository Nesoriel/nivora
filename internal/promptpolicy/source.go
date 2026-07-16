package promptpolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	cozeloopgo "github.com/coze-dev/cozeloop-go"
	"github.com/coze-dev/cozeloop-go/entity"
)

const maxPolicyBytes = 32 * 1024

// Snapshot is the currently approved prompt-policy appendix.
type Snapshot struct {
	Text      string
	Version   string
	Source    string
	UpdatedAt time.Time
}

// Source supplies a safe, already-approved policy appendix for Agent runs.
type Source interface {
	Current() Snapshot
	Close()
}

// PromptClient is the narrow CozeLoop PromptHub surface used by Nivora.
type PromptClient interface {
	GetPrompt(context.Context, cozeloopgo.GetPromptParam, ...cozeloopgo.GetPromptOption) (*entity.Prompt, error)
	PromptFormat(context.Context, *entity.Prompt, map[string]any, ...cozeloopgo.PromptFormatOption) ([]*entity.Message, error)
}

type staticSource struct {
	snapshot Snapshot
}

// Static returns an immutable source. The bundled source normally has empty
// text because the mandatory safety policy remains compiled into Nivora.
func Static(text, version, source string) Source {
	return &staticSource{snapshot: Snapshot{
		Text:      strings.TrimSpace(text),
		Version:   defaultString(version, "bundled-v1"),
		Source:    defaultString(source, "bundled"),
		UpdatedAt: time.Now().UTC(),
	}}
}

func (s *staticSource) Current() Snapshot { return s.snapshot }
func (s *staticSource) Close()            {}

// RemoteConfig controls resilient remote prompt refresh behavior.
type RemoteConfig struct {
	Key             string
	Version         string
	RefreshInterval time.Duration
	RequestTimeout  time.Duration
	Fallback        Source
	Logger          *slog.Logger
}

type remoteSource struct {
	client   PromptClient
	config   RemoteConfig
	fallback Source
	logger   *slog.Logger

	mu        sync.RWMutex
	current   Snapshot
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// Remote creates a CozeLoop-backed policy source. A failed initial or periodic
// refresh never prevents Nivora from serving; the last approved snapshot or the
// bundled fallback remains active.
func Remote(parent context.Context, client PromptClient, config RemoteConfig) (Source, error) {
	if client == nil {
		return nil, errors.New("prompt client is required")
	}
	config.Key = strings.TrimSpace(config.Key)
	if config.Key == "" {
		return nil, errors.New("prompt key is required")
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 3 * time.Second
	}
	if config.RefreshInterval < 0 {
		return nil, errors.New("prompt refresh interval must not be negative")
	}
	fallback := config.Fallback
	if fallback == nil {
		fallback = Static("", "bundled-v1", "bundled")
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(parent)
	source := &remoteSource{
		client:   client,
		config:   config,
		fallback: fallback,
		logger:   logger,
		cancel:   cancel,
	}
	source.current = fallback.Current()
	if err := source.refresh(ctx); err != nil {
		logger.Warn("CozeLoop prompt refresh failed; using safe fallback", "error", err, "prompt_key", config.Key)
	}
	if config.RefreshInterval > 0 {
		go source.refreshLoop(ctx)
	}
	return source, nil
}

func (s *remoteSource) Current() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *remoteSource) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.fallback.Close()
	})
}

func (s *remoteSource) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refresh(ctx); err != nil {
				s.logger.Warn("CozeLoop prompt refresh failed; retaining last approved policy", "error", err, "prompt_key", s.config.Key)
			}
		}
	}
}

func (s *remoteSource) refresh(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, s.config.RequestTimeout)
	defer cancel()

	param := cozeloopgo.GetPromptParam{PromptKey: s.config.Key}
	if version := strings.TrimSpace(s.config.Version); version != "" {
		param.Version = version
	}
	prompt, err := s.client.GetPrompt(ctx, param)
	if err != nil {
		return fmt.Errorf("get CozeLoop prompt: %w", err)
	}
	if prompt == nil {
		return errors.New("CozeLoop prompt is empty")
	}
	messages, err := s.client.PromptFormat(ctx, prompt, map[string]any{})
	if err != nil {
		return fmt.Errorf("format CozeLoop prompt: %w", err)
	}
	text, err := extractSystemPolicy(messages)
	if err != nil {
		return err
	}
	version := strings.TrimSpace(prompt.Version)
	if version == "" {
		version = defaultString(s.config.Version, "latest")
	}
	snapshot := Snapshot{
		Text:      text,
		Version:   version,
		Source:    "cozeloop",
		UpdatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	s.current = snapshot
	s.mu.Unlock()
	s.logger.Info("CozeLoop prompt policy refreshed", "prompt_key", s.config.Key, "prompt_version", version)
	return nil
}

func extractSystemPolicy(messages []*entity.Message) (string, error) {
	var parts []string
	for _, message := range messages {
		if message == nil || message.Role != entity.RoleSystem || message.Content == nil {
			continue
		}
		content := strings.TrimSpace(*message.Content)
		if content != "" {
			parts = append(parts, content)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return "", errors.New("CozeLoop prompt must contain a non-empty system message")
	}
	if len(text) > maxPolicyBytes {
		return "", fmt.Errorf("CozeLoop prompt exceeds %d bytes", maxPolicyBytes)
	}
	return text, nil
}

func defaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

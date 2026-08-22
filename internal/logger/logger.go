// Package logger is the shared leveled logger for orbit.api.
// Console always; Better Stack when BETTERSTACK_SOURCE_TOKEN and
// BETTERSTACK_INGESTING_HOST are set (same env names as Choppr).
// Official Go setup: log/slog + github.com/samber/slog-betterstack
// https://betterstack.com/docs/logs/go/
package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	slogbetterstack "github.com/samber/slog-betterstack"

	"github.com/shivang-16/orbit.api/internal/config"
)

var (
	mu     sync.RWMutex
	base   *slog.Logger
	inited bool
)

func Init(cfg config.Config) {
	mu.Lock()
	if inited && base != nil {
		mu.Unlock()
		return
	}
	mu.Unlock()

	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	console := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	handlers := []slog.Handler{console}

	token := strings.TrimSpace(cfg.BetterStack.SourceToken)
	host := strings.TrimSpace(cfg.BetterStack.IngestingHost)
	if token != "" && host != "" {
		endpoint := host
		if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://") {
			endpoint = "https://" + endpoint
		}
		if !strings.HasSuffix(endpoint, "/") {
			endpoint += "/"
		}
		handlers = append(handlers, slogbetterstack.Option{
			Level:    level,
			Token:    token,
			Endpoint: endpoint,
		}.NewBetterstackHandler())
	}

	l := slog.New(newMultiHandler(handlers...)).With(
		"service", "orbit.api",
		"env", cfg.Env,
	)
	mu.Lock()
	base = l
	inited = true
	mu.Unlock()
	slog.SetDefault(l)

	if token != "" && host != "" {
		Info(context.Background(), "logger ready", "betterstack", true)
	} else {
		Info(context.Background(), "logger ready", "betterstack", false)
	}
}

func current() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if base != nil {
		return base
	}
	return slog.Default()
}

func Debug(ctx context.Context, msg string, args ...any) {
	current().DebugContext(ctxOrBG(ctx), msg, merge(ctx, args)...)
}

func Info(ctx context.Context, msg string, args ...any) {
	current().InfoContext(ctxOrBG(ctx), msg, merge(ctx, args)...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	current().WarnContext(ctxOrBG(ctx), msg, merge(ctx, args)...)
}

func Error(ctx context.Context, msg string, args ...any) {
	current().ErrorContext(ctxOrBG(ctx), msg, merge(ctx, args)...)
}

func Fatal(ctx context.Context, msg string, args ...any) {
	Error(ctx, msg, args...)
	// slog-betterstack Handle() returns immediately and POSTs in a
	// goroutine. Give that send a moment so the fatal line can land
	// before the process dies. Request-path logs are not affected.
	time.Sleep(2 * time.Second)
	os.Exit(1)
}

func Debugf(ctx context.Context, format string, args ...any) {
	Debug(ctx, fmt.Sprintf(format, args...))
}

func Infof(ctx context.Context, format string, args ...any) {
	Info(ctx, fmt.Sprintf(format, args...))
}

func Warnf(ctx context.Context, format string, args ...any) {
	Warn(ctx, fmt.Sprintf(format, args...))
}

func Errorf(ctx context.Context, format string, args ...any) {
	Error(ctx, fmt.Sprintf(format, args...))
}

func ctxOrBG(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func merge(ctx context.Context, args []any) []any {
	base := attrs(ctx)
	if len(args) == 0 {
		return base
	}
	out := make([]any, 0, len(base)+len(args))
	out = append(out, base...)
	out = append(out, args...)
	return out
}

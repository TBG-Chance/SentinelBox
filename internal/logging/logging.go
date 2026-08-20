package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/TBG-Chance/SentinelBox/internal/config"
)

func New(cfg config.LoggingConfig, output io.Writer) (*slog.Logger, error) {
	if output == nil {
		return nil, fmt.Errorf("logging output is required")
	}

	var level slog.Level
	switch strings.ToLower(strings.TrimSpace(cfg.Level)) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("unsupported logging level %q", cfg.Level)
	}

	options := &slog.HandlerOptions{
		AddSource: level == slog.LevelDebug,
		Level:     level,
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("unsupported logging format %q", cfg.Format)
	}

	return slog.New(handler).With("service", "sentinelbox"), nil
}

package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NewLogger constructs a *slog.Logger using the provided level and optional format.
// Supported levels: debug, info, warn, error.
// Supported formats: text (default), json.
func NewLogger(level, format string) (*slog.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	logger := slog.New(handler)
	return logger.With("component", "cherry-pick-action"), nil
}

func parseLevel(level string) (*slog.LevelVar, error) {
	var lvl slog.LevelVar

	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl.Set(slog.LevelDebug)
	case "info", "":
		lvl.Set(slog.LevelInfo)
	case "warn", "warning":
		lvl.Set(slog.LevelWarn)
	case "error":
		lvl.Set(slog.LevelError)
	default:
		return nil, fmt.Errorf("unsupported log level %q", level)
	}

	return &lvl, nil
}

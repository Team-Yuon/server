package logger

import (
	"context"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
}

func New(env string) *Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: getLogLevel(env),
	}

	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return &Logger{Logger: logger}
}

func getLogLevel(env string) slog.Level {
	if env == "production" {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{Logger: l.Logger.With()}
}

func (l *Logger) WithFields(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

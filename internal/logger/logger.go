package logger

import (
	"log/slog"
	"os"
)

// Logger is the application logging interface.
// Implementations can wrap slog, zerolog, zap, etc.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// SlogLogger wraps slog.Logger to implement the Logger interface.
type SlogLogger struct {
	inner *slog.Logger
}

func NewSlog(level string, format string) *SlogLogger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return &SlogLogger{inner: slog.New(handler)}
}

func (l *SlogLogger) Debug(msg string, args ...any) { l.inner.Debug(msg, args...) }
func (l *SlogLogger) Info(msg string, args ...any)  { l.inner.Info(msg, args...) }
func (l *SlogLogger) Warn(msg string, args ...any)  { l.inner.Warn(msg, args...) }
func (l *SlogLogger) Error(msg string, args ...any) { l.inner.Error(msg, args...) }

func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{inner: l.inner.With(args...)}
}

// Nop returns a logger that discards all output. Useful for tests.
func Nop() Logger {
	return &nopLogger{}
}

type nopLogger struct{}

func (n *nopLogger) Debug(string, ...any) {}
func (n *nopLogger) Info(string, ...any)  {}
func (n *nopLogger) Warn(string, ...any)  {}
func (n *nopLogger) Error(string, ...any) {}
func (n *nopLogger) With(...any) Logger   { return n }

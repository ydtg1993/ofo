package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

var (
	defaultLogger *slog.Logger
	logFile       *os.File
)

// Init initialises the global structured logger with dual output:
//   - console: human-readable text format
//   - file: machine-readable JSON format under logDir/app.log
//
// level must be one of "debug", "info", "warn", "error" (case-insensitive).
// Call this once at startup before any logging.
func Init(level, logDir string) error {
	lvl := parseLevel(level)

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("logger: create log dir %s: %w", logDir, err)
	}

	logPath := filepath.Join(logDir, "app.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("logger: open log file %s: %w", logPath, err)
	}
	logFile = f

	// Console handler: text format, colourless
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})

	// File handler: JSON format for machine readability
	fileHandler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: lvl,
	})

	defaultLogger = slog.New(&multiHandler{
		handlers: []slog.Handler{consoleHandler, fileHandler},
	})

	slog.SetDefault(defaultLogger)

	Info("logger initialised", "level", level, "path", logPath)
	return nil
}

// Close flushes and closes the log file. Call before process exit.
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

// ---- Convenience functions ----

func Info(msg string, args ...any) {
	logAt(slog.LevelInfo, msg, args...)
}

func Warn(msg string, args ...any) {
	logAt(slog.LevelWarn, msg, args...)
}

func Error(msg string, args ...any) {
	logAt(slog.LevelError, msg, args...)
}

func Debug(msg string, args ...any) {
	logAt(slog.LevelDebug, msg, args...)
}

func logAt(level slog.Level, msg string, args ...any) {
	if defaultLogger == nil {
		return
	}
	defaultLogger.Log(context.Background(), level, msg, args...)
}

// ---- Gin middleware ----

// GinLogger returns a Gin middleware that logs every request using slog.
// Log level is determined by HTTP status code:
//
//	5xx → Error
//	4xx → Warn
//	other → Info
//
// Requires middleware.RequestID() to be registered before this middleware.
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()
		reqID := requestid.Get(c)

		if query != "" {
			path = path + "?" + query
		}

		attrs := []any{
			"req_id", reqID,
			"status", statusCode,
			"latency", latency,
			"ip", clientIP,
			"method", method,
			"path", path,
		}

		// Append any errors that handlers may have stored
		if len(c.Errors) > 0 {
			errMsgs := make([]string, len(c.Errors))
			for i, e := range c.Errors {
				errMsgs[i] = e.Error()
			}
			attrs = append(attrs, "errors", errMsgs)
		}

		switch {
		case statusCode >= 500:
			logAt(slog.LevelError, "request", attrs...)
		case statusCode >= 400:
			logAt(slog.LevelWarn, "request", attrs...)
		default:
			logAt(slog.LevelInfo, "request", attrs...)
		}
	}
}

// ErrorWithContext logs an error message and attaches the request ID from the
// Gin context, making it easy to correlate error logs with the access log entry.
func ErrorWithContext(c *gin.Context, msg string, args ...any) {
	reqID := requestid.Get(c)
	allArgs := []any{"req_id", reqID}
	allArgs = append(allArgs, args...)
	logAt(slog.LevelError, msg, allArgs...)
}

// WarnWithContext logs a warning message with the request ID from Gin context.
func WarnWithContext(c *gin.Context, msg string, args ...any) {
	reqID := requestid.Get(c)
	allArgs := []any{"req_id", reqID}
	allArgs = append(allArgs, args...)
	logAt(slog.LevelWarn, msg, allArgs...)
}

// ---- multiHandler: fan-out to multiple handlers ----

type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			// Clone record so each handler gets its own copy
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				// Best-effort: log handler errors to stderr
				fmt.Fprintf(os.Stderr, "logger: handler error: %v\n", err)
			}
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// ---- Helpers ----

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// Default to info for unknown values
		fmt.Fprintf(os.Stderr, "logger: unknown level %q, defaulting to info\n", s)
		return slog.LevelInfo
	}
}

// Ensure io.Closer interface compliance for testing
var _ io.Closer = (*os.File)(nil)

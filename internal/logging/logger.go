package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// New создаёт JSON-логгер slog с заданным уровнем.
func New(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		lv = slog.LevelDebug
	case "WARN", "WARNING":
		lv = slog.LevelWarn
	case "ERROR":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})
	return slog.New(h)
}

// ShortBytes возвращает первые 4 байта в hex.
// Используется для сокращения NodeID (8 hex-символов) и request_id.
func ShortBytes(b []byte) string {
	if len(b) > 4 {
		b = b[:4]
	}
	return fmt.Sprintf("%x", b)
}

// WithComponent добавляет поле component=…, чтобы в общем логе
// можно было отделить transport от routing и rpc.
func WithComponent(l *slog.Logger, name string) *slog.Logger {
	return l.With("component", name)
}

// Discard — заглушка для тестов, чтобы не засорять вывод.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), nil))
}

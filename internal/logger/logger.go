package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// appLogger адаптирует zap под контракт interfaces.Logger.
type appLogger struct {
	sugar *zap.SugaredLogger
}

func (l appLogger) Info(msg string, kv ...any)  { l.sugar.Infow(msg, kv...) }
func (l appLogger) Error(msg string, kv ...any) { l.sugar.Errorw(msg, kv...) }

// New создаёт zap-логгер с двумя выводами: консоль и файл.
// Возвращает: логгер для main, логгер для сценариев (interfaces.Logger) и закрывашку.
func New(level string, filePath string) (*zap.Logger, interfaces.Logger, func(), error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("уровень логирования %q: %w", level, err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("папка логов: %w", err)
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("файл логов: %w", err)
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     moscowTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	console := zapcore.NewCore(zapcore.NewConsoleEncoder(encCfg), zapcore.AddSync(os.Stdout), lvl)
	toFile := zapcore.NewCore(zapcore.NewConsoleEncoder(encCfg), zapcore.AddSync(file), lvl)

	log := zap.New(zapcore.NewTee(console, toFile))
	zap.ReplaceGlobals(log) // delivery пишет через zap.S(), не таская логгер через конструкторы

	closer := func() {
		_ = log.Sync()
		_ = file.Close()
	}
	return log, appLogger{sugar: log.Sugar()}, closer, nil
}

// moscowTimeEncoder — время в сообщении лога по Москве, как привычно в README.
func moscowTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.In(domain.Moscow).Format("02.01 15:04:05"))
}

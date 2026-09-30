package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"krypto-proekt/model"
)

// setupLogging пишет журнал в logs/app.log и в stdout.
// Возвращает файл, чтобы run закрыл его при остановке.
func setupLogging() (io.Closer, error) {
	err := os.MkdirAll("logs", 0o755)
	if err != nil {
		return nil, fmt.Errorf("папка логов: %w", err)
	}
	file, err := os.OpenFile("logs/app.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("файл логов: %w", err)
	}
	// Используем MultiWriter для записи логов одновременно в файл и консоль.
	// Closer закрывает только файл, stdout не трогаем.
	// Text, не JSON: строку можно прочитать глазами.
	handler := slog.NewTextHandler(io.MultiWriter(os.Stdout, file), &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key != slog.TimeKey {
				return attr
			}
			stamp, ok := attr.Value.Any().(time.Time)
			if !ok {
				return attr
			}
			return slog.Time(slog.TimeKey, stamp.In(model.Moscow))
		},
	})
	slog.SetDefault(slog.New(handler))
	return file, nil
}

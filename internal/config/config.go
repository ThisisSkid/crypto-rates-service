package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config — все настройки сервиса в одной структуре.
// Значения по умолчанию заданы тегами envDefault, обязательные — required.
type Config struct {
	PostgresURL      string        `env:"POSTGRES_URL,required"`
	TelegramBotToken string        `env:"TELEGRAM_BOT_TOKEN"`
	BybitURL         string        `env:"BYBIT_URL" envDefault:"https://api.bybit.com/v5/market/tickers?category=spot&symbol="`
	HTTPAddr         string        `env:"HTTP_ADDR" envDefault:":8080"`
	UpdateInterval   time.Duration `env:"UPDATE_INTERVAL" envDefault:"1h"`
	LogLevel         string        `env:"LOG_LEVEL" envDefault:"info"`
	LogFile          string        `env:"LOG_FILE" envDefault:"logs/app.log"`
}

// Load читает .env (если файл есть) и переменные окружения процесса.
// В Docker .env внутри контейнера нет — там работают переменные из compose,
// поэтому ошибку отсутствия файла игнорируем.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("разбор переменных окружения: %w", err)
	}
	return &cfg, nil
}

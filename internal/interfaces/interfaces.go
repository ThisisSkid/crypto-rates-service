package interfaces

import (
	"context"

	"krypto-proekt/internal/domain"
)

// Store — контракт для хранилища (репозитория).
// UseCase знает только этот интерфейс, ему не важно, Postgres это или память.
type Store interface {
	SaveRate(ctx context.Context, rate domain.Rate) error
	LatestRate(ctx context.Context, coinSymbol string) (domain.Rate, error)
	DayMinMax(ctx context.Context, coinSymbol string) (minPrice float64, maxPrice float64, err error)
	PriceHourAgo(ctx context.Context, coinSymbol string) (float64, error)
}

// Exchange — контракт для получения курсов с биржи.
// Context внутри: Ctrl+C обязан уметь обрывать поход на биржу.
type Exchange interface {
	Fetch(ctx context.Context) ([]domain.Rate, error)
}

// Logger — контракт логирования для сценариев.
// UseCase не знает о конкретной библиотеке логов.
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

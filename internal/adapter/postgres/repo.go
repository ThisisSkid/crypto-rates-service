package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// PostgresRepo — склад. Только SQL. Биржу и Telegram он не знает.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// Жесткая проверка на этапе компиляции: PostgresRepo обязан реализовывать interfaces.Store.
var _ interfaces.Store = (*PostgresRepo)(nil)

// NewPostgresRepo принимает уже открытый и пинганутый пул соединений.
// Создание пула — ответственность main.go (Composition Root).
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{pool: pool}
}

// SaveRate пишет один снимок курса новой строкой в таблицу rates.
func (r *PostgresRepo) SaveRate(ctx context.Context, rate domain.Rate) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO rates (coin_symbol, price_usd, fetched_at) VALUES ($1, $2, $3)`,
		rate.CoinSymbol, rate.PriceUSD, rate.FetchedAt,
	)
	return err
}

// LatestRate читает последний снимок выбранной монеты.
func (r *PostgresRepo) LatestRate(ctx context.Context, coinSymbol string) (domain.Rate, error) {
	var rate domain.Rate
	err := r.pool.QueryRow(ctx,
		`SELECT coin_symbol, price_usd, fetched_at
		 FROM rates
		 WHERE coin_symbol = $1
		 ORDER BY fetched_at DESC
		 LIMIT 1`,
		coinSymbol,
	).Scan(&rate.CoinSymbol, &rate.PriceUSD, &rate.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rate{}, domain.ErrNotFound
	}
	return rate, err
}

// DayMinMax — минимум и максимум цены монеты с полуночи сегодня.
func (r *PostgresRepo) DayMinMax(ctx context.Context, coinSymbol string) (float64, float64, error) {
	var minPtr, maxPtr *float64
	err := r.pool.QueryRow(ctx,
		`SELECT MIN(price_usd), MAX(price_usd)
		 FROM rates
		 WHERE coin_symbol = $1
		   AND fetched_at >= date_trunc('day', NOW())`,
		coinSymbol,
	).Scan(&minPtr, &maxPtr)
	if err != nil {
		return 0, 0, err
	}
	return dayMinMax(minPtr, maxPtr)
}

// dayMinMax переводит ответ агрегатов в доменный результат.
// MIN/MAX по пустому набору — это NULL, не ноль. Scan в float64 падает, указатель остаётся nil.
func dayMinMax(minPtr *float64, maxPtr *float64) (float64, float64, error) {
	if minPtr == nil || maxPtr == nil {
		return 0, 0, domain.ErrNotFound
	}
	return *minPtr, *maxPtr, nil
}

// PriceHourAgo — самая свежая цена, которая уже старше часа.
func (r *PostgresRepo) PriceHourAgo(ctx context.Context, coinSymbol string) (float64, error) {
	var oldPrice float64
	err := r.pool.QueryRow(ctx,
		`SELECT price_usd
		 FROM rates
		 WHERE coin_symbol = $1
		   AND fetched_at <= NOW() - interval '1 hour'
		 ORDER BY fetched_at DESC
		 LIMIT 1`,
		coinSymbol,
	).Scan(&oldPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return oldPrice, err
}

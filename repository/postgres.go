package repository

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"krypto-proekt/model"
)

// Postgres — склад. Только SQL. Биржу и Telegram он не знает.
type Postgres struct {
	pool *pgxpool.Pool
}

// Open открывает пул соединений с БД.
// Адрес только из POSTGRES_URL: пароль не храним в исходниках.
func Open(ctx context.Context) (*Postgres, error) {
	postgresURL := os.Getenv("POSTGRES_URL")
	// Без POSTGRES_URL процесс останавливается сразу: пароля в коде нет.
	if postgresURL == "" {
		return nil, fmt.Errorf("не задана переменная POSTGRES_URL")
	}
	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть postgres: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Close закрывает пул и ждёт, пока завершатся текущие запросы.
func (store *Postgres) Close() {
	store.pool.Close()
}

// SaveRate пишет один снимок курса новой строкой в таблицу rates.
func (store *Postgres) SaveRate(ctx context.Context, rate model.Rate) error {
	_, err := store.pool.Exec(ctx,
		`INSERT INTO rates (coin_symbol, price_usd, fetched_at) VALUES ($1, $2, $3)`,
		rate.CoinSymbol,
		rate.PriceUSD,
		rate.FetchedAt,
	)
	return err
}

// LatestRate читает последний снимок выбранной монеты.
func (store *Postgres) LatestRate(ctx context.Context, coinSymbol string) (model.Rate, error) {
	var rate model.Rate
	err := store.pool.QueryRow(ctx,
		`SELECT coin_symbol, price_usd, fetched_at
		 FROM rates
		 WHERE coin_symbol = $1
		 ORDER BY fetched_at DESC
		 LIMIT 1`,
		coinSymbol,
	).Scan(&rate.CoinSymbol, &rate.PriceUSD, &rate.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Rate{}, model.ErrNotFound
	}
	return rate, err
}

// DayMinMax — минимум и максимум цены монеты с полуночи сегодня.
func (store *Postgres) DayMinMax(ctx context.Context, coinSymbol string) (minPrice float64, maxPrice float64, err error) {
	// date_trunc('day', NOW()) обрезает «сейчас» до полуночи.
	// 'day' в одинарных кавычках: это текст для Postgres, не код Go.
	var minPtr, maxPtr *float64
	err = store.pool.QueryRow(ctx,
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
func dayMinMax(minPtr *float64, maxPtr *float64) (minPrice float64, maxPrice float64, err error) {
	if minPtr == nil || maxPtr == nil {
		return 0, 0, model.ErrNotFound
	}
	return *minPtr, *maxPtr, nil
}

// PriceHourAgo — самая свежая цена, которая уже старше часа.
func (store *Postgres) PriceHourAgo(ctx context.Context, coinSymbol string) (float64, error) {
	var oldPrice float64
	err := store.pool.QueryRow(ctx,
		`SELECT price_usd
		 FROM rates
		 WHERE coin_symbol = $1
		   AND fetched_at <= NOW() - interval '1 hour'
		 ORDER BY fetched_at DESC
		 LIMIT 1`,
		coinSymbol,
	).Scan(&oldPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, model.ErrNotFound
	}
	return oldPrice, err
}

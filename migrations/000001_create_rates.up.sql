CREATE TABLE IF NOT EXISTS rates (
    id       BIGSERIAL PRIMARY KEY,
    coin_symbol TEXT NOT NULL,
    price_usd DOUBLE PRECISION NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL
);

-- Индекс покрывает фильтр по монете и сортировку по времени.
CREATE INDEX IF NOT EXISTS idx_rates_coin_time ON rates (coin_symbol, fetched_at DESC);

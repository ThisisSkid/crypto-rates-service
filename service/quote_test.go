package service

import (
	"context"
	"testing"
	"time"

	"krypto-proekt/model"
)

// memoryStore — склад в памяти: одна монета и одна цена час назад.
// Флаг miss превращает его в пустую таблицу.
type memoryStore struct {
	rate model.Rate
	old  float64
	miss bool
}

func (store memoryStore) SaveRate(context.Context, model.Rate) error { return nil }
func (store memoryStore) LatestRate(context.Context, string) (model.Rate, error) {
	if store.miss {
		return model.Rate{}, model.ErrNotFound
	}
	return store.rate, nil
}
func (store memoryStore) DayMinMax(context.Context, string) (float64, float64, error) {
	return 90, 110, nil
}
func (store memoryStore) PriceHourAgo(context.Context, string) (float64, error) {
	if store.old == 0 {
		return 0, model.ErrNotFound
	}
	return store.old, nil
}

// Quote — то, что уходит в JSON на GET /rates/{coin}.
func TestQuote(t *testing.T) {
	rates := New(memoryStore{
		rate: model.Rate{CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()},
		old:  100,
	}, nil)
	quote, err := rates.Quote(context.Background(), "BTC")
	if err != nil || quote.DayMin != 90 || quote.DayMax != 110 || quote.HourChangePercent == nil || *quote.HourChangePercent != 10 {
		t.Fatalf("quote %+v err %v", quote, err)
	}

	missing := New(memoryStore{miss: true}, nil)
	_, err = missing.Quote(context.Background(), "DOGE")
	if err != model.ErrNotFound {
		t.Fatalf("ждали отсутствие строки, получили %v", err)
	}
}

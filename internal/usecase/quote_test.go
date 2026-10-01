package usecase

import (
	"context"
	"testing"
	"time"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// memoryStore — склад в памяти для тестов.
type memoryStore struct {
	rate domain.Rate
	old  float64
	miss bool
}

// Проверка на этапе компиляции, что заглушка реализует интерфейс
var _ interfaces.Store = (*memoryStore)(nil)

func (s *memoryStore) SaveRate(context.Context, domain.Rate) error { return nil }
func (s *memoryStore) LatestRate(context.Context, string) (domain.Rate, error) {
	if s.miss {
		return domain.Rate{}, domain.ErrNotFound
	}
	return s.rate, nil
}
func (s *memoryStore) DayMinMax(context.Context, string) (float64, float64, error) {
	return 90, 110, nil
}
func (s *memoryStore) PriceHourAgo(context.Context, string) (float64, error) {
	if s.old == 0 {
		return 0, domain.ErrNotFound
	}
	return s.old, nil
}

func TestQuote(t *testing.T) {
	svc := NewService(&memoryStore{
		rate: domain.Rate{CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()},
		old:  100,
	}, nil)

	quote, err := svc.Quote(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.DayMin != 90 || quote.DayMax != 110 {
		t.Fatalf("wrong min/max: %+v", quote)
	}
	if quote.HourChangePercent == nil || *quote.HourChangePercent != 10 {
		t.Fatalf("wrong percent: %v", quote.HourChangePercent)
	}

	missing := NewService(&memoryStore{miss: true}, nil)
	_, err = missing.Quote(context.Background(), "DOGE")
	if err != domain.ErrNotFound {
		t.Fatalf("ждали отсутствие строки, получили %v", err)
	}
}

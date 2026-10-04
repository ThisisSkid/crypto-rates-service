package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// reportStore отдаёт заранее заданные ответы
type reportStore struct {
	rates      map[string]domain.Rate
	hourPrices map[string]float64
	dayErr     error
}

var _ interfaces.Store = (*reportStore)(nil)

func (s *reportStore) SaveRate(context.Context, domain.Rate) error { return nil }
func (s *reportStore) LatestRate(_ context.Context, coinSymbol string) (domain.Rate, error) {
	rate, ok := s.rates[coinSymbol]
	if !ok {
		return domain.Rate{}, domain.ErrNotFound
	}
	return rate, nil
}
func (s *reportStore) DayMinMax(context.Context, string) (float64, float64, error) {
	if s.dayErr != nil {
		return 0, 0, s.dayErr
	}
	return 90, 110, nil
}
func (s *reportStore) PriceHourAgo(_ context.Context, coinSymbol string) (float64, error) {
	price, ok := s.hourPrices[coinSymbol]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return price, nil
}

var (
	testBTC = domain.Rate{CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()}
	testETH = domain.Rate{CoinSymbol: "ETH", PriceUSD: 11, FetchedAt: time.Unix(0, 0).UTC()}
)

func TestReport(t *testing.T) {
	tests := []struct {
		name     string
		store    *reportStore
		symbols  []string
		contains []string
	}{
		{
			name: "курс, день и час на месте",
			store: &reportStore{
				rates:      map[string]domain.Rate{"BTC": testBTC},
				hourPrices: map[string]float64{"BTC": 100},
			},
			symbols:  []string{"BTC"},
			contains: []string{"BTC: 110 $", "за день: 90 … 110 $", "за час: +10.00%", "время: 01.01 03:00"},
		},
		{
			name:     "цены час назад ещё нет",
			store:    &reportStore{rates: map[string]domain.Rate{"BTC": testBTC}},
			symbols:  []string{"BTC"},
			contains: []string{"BTC: 110 $", "за час: мало данных"},
		},
		{
			name: "строк за сегодня ещё нет",
			store: &reportStore{
				rates:  map[string]domain.Rate{"BTC": testBTC},
				dayErr: domain.ErrNotFound,
			},
			symbols:  []string{"BTC"},
			contains: []string{"за день: нет данных"},
		},
		{
			name:     "монеты нет в таблице",
			store:    &reportStore{},
			symbols:  []string{"DOGE"},
			contains: []string{"DOGE: монета не найдена"},
		},
		{
			name:     "две монеты в одном ответе",
			store:    &reportStore{rates: map[string]domain.Rate{"BTC": testBTC, "ETH": testETH}},
			symbols:  []string{"BTC", "ETH"},
			contains: []string{"BTC: 110 $", "ETH: 11 $"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := NewService(test.store, nil, nil)
			text := svc.Report(context.Background(), test.symbols)
			for _, want := range test.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("в ответе нет %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestStatsText(t *testing.T) {
	store := &reportStore{
		rates:      map[string]domain.Rate{"BTC": testBTC},
		hourPrices: map[string]float64{"BTC": 100},
	}
	svc := NewService(store, nil, nil)
	text := svc.StatsText(context.Background(), testBTC)

	for _, want := range []string{"за день min=90 max=110", "за час изменение=10.00%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в строке нет %q:\n%s", want, text)
		}
	}
}

func TestQuotesSkipMissingCoin(t *testing.T) {
	store := &reportStore{rates: map[string]domain.Rate{"BTC": testBTC}}
	svc := NewService(store, nil, nil)

	quotes, err := svc.Quotes(context.Background(), []string{"BTC", "DOGE"})
	if err != nil {
		t.Fatalf("Quotes вернул ошибку: %v", err)
	}
	if len(quotes) != 1 || quotes[0].CoinSymbol != "BTC" {
		t.Fatalf("Quotes вернул %+v, ждали только BTC", quotes)
	}
}

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"krypto-proekt/model"
)

// reportStore отдаёт заранее заданные ответы по каждой монете отдельно,
// чтобы проверить все ветки текста: есть данные, нет строк за день, нет цены час назад.
type reportStore struct {
	rates      map[string]model.Rate
	hourPrices map[string]float64
	dayErr     error
}

func (store reportStore) SaveRate(context.Context, model.Rate) error { return nil }

func (store reportStore) LatestRate(_ context.Context, coinSymbol string) (model.Rate, error) {
	rate, ok := store.rates[coinSymbol]
	if !ok {
		return model.Rate{}, model.ErrNotFound
	}
	return rate, nil
}

func (store reportStore) DayMinMax(context.Context, string) (float64, float64, error) {
	if store.dayErr != nil {
		return 0, 0, store.dayErr
	}
	return 90, 110, nil
}

func (store reportStore) PriceHourAgo(_ context.Context, coinSymbol string) (float64, error) {
	price, ok := store.hourPrices[coinSymbol]
	if !ok {
		return 0, model.ErrNotFound
	}
	return price, nil
}

var (
	testBTC = model.Rate{CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()}
	testETH = model.Rate{CoinSymbol: "ETH", PriceUSD: 11, FetchedAt: time.Unix(0, 0).UTC()}
)

// Report — это то, что пользователь читает в Telegram.
// Отсутствие данных не должно выглядеть как ошибка сервиса.
func TestReport(t *testing.T) {
	tests := []struct {
		name     string
		store    reportStore
		symbols  []string
		contains []string
	}{
		{
			name: "курс, день и час на месте",
			store: reportStore{
				rates:      map[string]model.Rate{"BTC": testBTC},
				hourPrices: map[string]float64{"BTC": 100},
			},
			symbols:  []string{"BTC"},
			contains: []string{"BTC: 110 $", "за день: 90 … 110 $", "за час: +10.00%", "время: 01.01 03:00"},
		},
		{
			name:     "цены час назад ещё нет",
			store:    reportStore{rates: map[string]model.Rate{"BTC": testBTC}},
			symbols:  []string{"BTC"},
			contains: []string{"BTC: 110 $", "за час: мало данных"},
		},
		{
			name: "строк за сегодня ещё нет",
			store: reportStore{
				rates:  map[string]model.Rate{"BTC": testBTC},
				dayErr: model.ErrNotFound,
			},
			symbols:  []string{"BTC"},
			contains: []string{"за день: нет данных"},
		},
		{
			name:     "монеты нет в таблице",
			store:    reportStore{},
			symbols:  []string{"DOGE"},
			contains: []string{"DOGE: монета не найдена"},
		},
		{
			name:     "две монеты в одном ответе",
			store:    reportStore{rates: map[string]model.Rate{"BTC": testBTC, "ETH": testETH}},
			symbols:  []string{"BTC", "ETH"},
			contains: []string{"BTC: 110 $", "ETH: 11 $"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := New(test.store, nil).Report(context.Background(), test.symbols)
			for _, want := range test.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("в ответе нет %q:\n%s", want, text)
				}
			}
		})
	}
}

// StatsText печатается в консоль после каждого прохода тикера.
func TestStatsText(t *testing.T) {
	store := reportStore{
		rates:      map[string]model.Rate{"BTC": testBTC},
		hourPrices: map[string]float64{"BTC": 100},
	}
	text := New(store, nil).StatsText(context.Background(), testBTC)

	for _, want := range []string{"за день min=90 max=110", "за час изменение=10.00%"} {
		if !strings.Contains(text, want) {
			t.Fatalf("в строке нет %q:\n%s", want, text)
		}
	}
}

// Монета без строк не должна обрывать ответ по остальным монетам:
// GET /rates отдаст то, что есть, а не 500 из-за одной пропавшей монеты.
func TestQuotesSkipMissingCoin(t *testing.T) {
	store := reportStore{rates: map[string]model.Rate{"BTC": testBTC}}
	rates := New(store, nil)

	quotes, err := rates.Quotes(context.Background(), []string{"BTC", "DOGE"})
	if err != nil {
		t.Fatalf("Quotes вернул ошибку: %v", err)
	}
	if len(quotes) != 1 || quotes[0].CoinSymbol != "BTC" {
		t.Fatalf("Quotes вернул %+v, ждали только BTC", quotes)
	}
}

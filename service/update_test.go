package service

import (
	"context"
	"errors"
	"testing"

	"krypto-proekt/model"
)

// stubExchange подменяет Bybit: в тестах в сеть не ходим.
type stubExchange struct {
	rates []model.Rate
	err   error
}

func (exchange stubExchange) Fetch() ([]model.Rate, error) { return exchange.rates, exchange.err }

// saveStore считает записи и умеет ломаться на записи.
// Чтение берём у memoryStore, поэтому интерфейс Store выполнен целиком.
type saveStore struct {
	memoryStore
	saveErr error
	saved   int
}

func (store *saveStore) SaveRate(context.Context, model.Rate) error {
	if store.saveErr != nil {
		return store.saveErr
	}
	store.saved++
	return nil
}

// Update — единственное место, где обработка зависит и от биржи, и от склада.
// Проверяем, что ошибка любого из них поднимается наружу необёрнутой и запись прекращается.
func TestUpdate(t *testing.T) {
	errNetwork := errors.New("dial tcp 104.18.0.1:443: i/o timeout")
	errWrite := errors.New("write tcp: connection reset by peer")
	twoRates := []model.Rate{
		{CoinSymbol: "BTC", PriceUSD: 110},
		{CoinSymbol: "ETH", PriceUSD: 10},
	}

	tests := []struct {
		name      string
		exchange  stubExchange
		saveErr   error
		wantErr   error
		wantRates int
		wantSaved int
	}{
		{name: "биржа ответила, всё записано", exchange: stubExchange{rates: twoRates}, wantRates: 2, wantSaved: 2},
		{name: "сеть до биржи не дошла", exchange: stubExchange{err: errNetwork}, wantErr: errNetwork},
		{name: "биржа пустая, писать нечего", exchange: stubExchange{}},
		{name: "база отказала на записи", exchange: stubExchange{rates: twoRates}, saveErr: errWrite, wantErr: errWrite},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &saveStore{saveErr: test.saveErr}
			rates, err := New(store, test.exchange).Update(context.Background())

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("получили ошибку %v, ждали %v", err, test.wantErr)
			}
			if len(rates) != test.wantRates {
				t.Fatalf("получили %d курсов, ждали %d", len(rates), test.wantRates)
			}
			if store.saved != test.wantSaved {
				t.Fatalf("записали %d строк, ждали %d", store.saved, test.wantSaved)
			}
		})
	}
}

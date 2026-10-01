package usecase

import (
	"context"
	"errors"
	"testing"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// stubExchange подменяет Bybit
type stubExchange struct {
	rates []domain.Rate
	err   error
}

var _ interfaces.Exchange = (*stubExchange)(nil)

func (e *stubExchange) Fetch() ([]domain.Rate, error) { return e.rates, e.err }

// saveStore считает записи и умеет ломаться на записи
type saveStore struct {
	memoryStore
	saveErr error
	saved   int
}

func (s *saveStore) SaveRate(context.Context, domain.Rate) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved++
	return nil
}

func TestUpdate(t *testing.T) {
	errNetwork := errors.New("dial tcp 104.18.0.1:443: i/o timeout")
	errWrite := errors.New("write tcp: connection reset by peer")
	twoRates := []domain.Rate{
		{CoinSymbol: "BTC", PriceUSD: 110},
		{CoinSymbol: "ETH", PriceUSD: 10},
	}

	tests := []struct {
		name      string
		exchange  *stubExchange
		saveErr   error
		wantErr   error
		wantRates int
		wantSaved int
	}{
		{name: "биржа ответила, всё записано", exchange: &stubExchange{rates: twoRates}, wantRates: 2, wantSaved: 2},
		{name: "сеть до биржи не дошла", exchange: &stubExchange{err: errNetwork}, wantErr: errNetwork},
		{name: "биржа пустая, писать нечего", exchange: &stubExchange{}},
		{name: "база отказала на записи", exchange: &stubExchange{rates: twoRates}, saveErr: errWrite, wantErr: errWrite},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &saveStore{saveErr: test.saveErr}
			svc := NewService(store, test.exchange)

			rates, err := svc.Update(context.Background())
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

package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"krypto-proekt/model"
	"krypto-proekt/service"
)

// fakeStore — склад в памяти. Хендлеры знают только интерфейс service.Store,
// поэтому интеграционный тест маршрутов обходится без Postgres.
type fakeStore struct {
	rates map[string]model.Rate
}

func (store fakeStore) SaveRate(context.Context, model.Rate) error { return nil }

func (store fakeStore) LatestRate(_ context.Context, coinSymbol string) (model.Rate, error) {
	rate, ok := store.rates[coinSymbol]
	if !ok {
		return model.Rate{}, model.ErrNotFound
	}
	return rate, nil
}

func (store fakeStore) DayMinMax(context.Context, string) (float64, float64, error) {
	return 90, 110, nil
}

func (store fakeStore) PriceHourAgo(context.Context, string) (float64, error) {
	return 100, nil
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := fakeStore{rates: map[string]model.Rate{
		"BTC": {CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()},
		"ETH": {CoinSymbol: "ETH", PriceUSD: 11, FetchedAt: time.Unix(0, 0).UTC()},
	}}
	server := httptest.NewServer(New(service.New(store, nil)))
	t.Cleanup(server.Close)
	return server
}

// Коды ответов по маршрутам: монета, регистр, неизвестная монета и список.
func TestRatesStatusCodes(t *testing.T) {
	server := newTestServer(t)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "курс BTC", path: "/rates/BTC", wantStatus: http.StatusOK},
		{name: "нижний регистр приводится к верхнему", path: "/rates/btc", wantStatus: http.StatusOK},
		{name: "неизвестной монеты нет в таблице", path: "/rates/DOGE", wantStatus: http.StatusNotFound},
		{name: "список курсов", path: "/rates", wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := server.Client().Get(server.URL + test.path)
			if err != nil {
				t.Fatalf("запрос не ушёл: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("получили код %d, ждали %d", response.StatusCode, test.wantStatus)
			}
		})
	}
}

// GET /rates/BTC отдаёт 200, JSON-заголовок и все поля из openapi.yaml,
// включая min/max за день и процент за час.
func TestRateBTCJSON(t *testing.T) {
	server := newTestServer(t)

	response, err := server.Client().Get(server.URL + "/rates/BTC")
	if err != nil {
		t.Fatalf("запрос не ушёл: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("получили код %d, ждали 200", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("получили Content-Type %q, ждали application/json", contentType)
	}

	var quote service.Quote
	err = json.NewDecoder(response.Body).Decode(&quote)
	if err != nil {
		t.Fatalf("тело не разобралось: %v", err)
	}
	if quote.CoinSymbol != "BTC" || quote.PriceUSD != 110 {
		t.Fatalf("получили снимок %+v", quote.Rate)
	}
	if quote.DayMin != 90 || quote.DayMax != 110 {
		t.Fatalf("получили min/max %v..%v, ждали 90..110", quote.DayMin, quote.DayMax)
	}
	// Цена час назад 100, текущая 110, значит рост ровно на 10 процентов.
	if quote.HourChangePercent == nil || *quote.HourChangePercent != 10 {
		t.Fatalf("получили процент %v, ждали 10", quote.HourChangePercent)
	}
}

// Список отдаёт обе монеты.
func TestRatesListJSON(t *testing.T) {
	server := newTestServer(t)

	response, err := server.Client().Get(server.URL + "/rates")
	if err != nil {
		t.Fatalf("запрос не ушёл: %v", err)
	}
	defer response.Body.Close()

	var quotes []service.Quote
	err = json.NewDecoder(response.Body).Decode(&quotes)
	if err != nil {
		t.Fatalf("тело не разобралось: %v", err)
	}
	if len(quotes) != 2 || quotes[0].CoinSymbol != "BTC" || quotes[1].CoinSymbol != "ETH" {
		t.Fatalf("получили %+v, ждали BTC и ETH", quotes)
	}
}

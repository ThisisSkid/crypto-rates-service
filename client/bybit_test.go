package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient подменяет адрес биржи на локальный сервер: в тестах в интернет не ходим.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Bybit {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	bybit := New()
	bybit.baseURL = server.URL + "?symbol="
	return bybit
}

// Разбор ответа биржи: чужой JSON превращается в наши model.Rate.
func TestFetchParsesPrices(t *testing.T) {
	bybit := newTestClient(t, func(w http.ResponseWriter, request *http.Request) {
		price := map[string]string{
			"BTCUSDT": "86000.5",
			"ETHUSDT": "2100.25",
		}[request.URL.Query().Get("symbol")]
		fmt.Fprintf(w, `{"retCode":0,"retMsg":"OK","result":{"list":[{"lastPrice":"%s"}]}}`, price)
	})

	before := time.Now()
	rates, err := bybit.Fetch()
	if err != nil {
		t.Fatalf("Fetch вернул ошибку: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("получили %d курсов, ждали 2", len(rates))
	}
	if rates[0].CoinSymbol != "BTC" || rates[0].PriceUSD != 86000.5 {
		t.Fatalf("первый курс %+v, ждали BTC 86000.5", rates[0])
	}
	if rates[1].CoinSymbol != "ETH" || rates[1].PriceUSD != 2100.25 {
		t.Fatalf("второй курс %+v, ждали ETH 2100.25", rates[1])
	}
	// Время ставим сами, а не берём у биржи: оба снимка должны получить один момент.
	if !rates[0].FetchedAt.Equal(rates[1].FetchedAt) {
		t.Fatal("снимки одного прохода получили разное время")
	}
	if rates[0].FetchedAt.Before(before) {
		t.Fatal("время снимка раньше начала теста")
	}
}

// Биржа может ответить чем угодно, и это не должно ронять тикер паникой.
func TestFetchBadAnswers(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantErrPart string
	}{
		{
			name:        "лимит запросов исчерпан",
			status:      http.StatusTooManyRequests,
			body:        "rate limited",
			wantErrPart: "биржа ответила кодом 429",
		},
		{
			name:        "биржа сломалась",
			status:      http.StatusInternalServerError,
			body:        "oops",
			wantErrPart: "биржа ответила кодом 500",
		},
		{
			name:        "вместо JSON пришёл мусор",
			status:      http.StatusOK,
			body:        "<html>not json</html>",
			wantErrPart: "invalid character",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bybit := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})

			rates, err := bybit.Fetch()
			if err == nil {
				t.Fatalf("ждали ошибку, получили курсы %+v", rates)
			}
			if !strings.Contains(err.Error(), test.wantErrPart) {
				t.Fatalf("в ошибке нет %q: %v", test.wantErrPart, err)
			}
			if rates != nil {
				t.Fatalf("при ошибке курсы должны быть nil, получили %+v", rates)
			}
		})
	}
}

// Пустой список Bybit — это ошибка, нулевую цену в базу не пишем.
func TestFetchMissingCoin(t *testing.T) {
	bybit := newTestClient(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("symbol") == "ETHUSDT" {
			_, _ = w.Write([]byte(`{"retCode":0,"retMsg":"OK","result":{"list":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"retCode":0,"retMsg":"OK","result":{"list":[{"lastPrice":"86000.5"}]}}`))
	})

	rates, err := bybit.Fetch()
	if err == nil {
		t.Fatalf("ждали ошибку, получили %+v", rates)
	}
	if !strings.Contains(err.Error(), "в ответе нет цены") {
		t.Fatalf("в ошибке нет пропавшей цены: %v", err)
	}
}

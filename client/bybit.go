// Package client — клиенты внешних API. Здесь только поход в сеть и разбор ответа,
// ни SQL, ни бизнес-логики: сервис знает про нас только через интерфейс service.Exchange.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"krypto-proekt/model"
)

// bybitURL — публичные цены спота. В конец подставляется BTCUSDT или ETHUSDT.
const bybitURL = "https://api.bybit.com/v5/market/tickers?category=spot&symbol="

// Bybit ходит на биржу за текущими ценами BTC и ETH.
type Bybit struct {
	httpClient *http.Client
	baseURL    string
}

// New возвращает клиент биржи Bybit.
// Свой http.Client с таймаутом: клиент по умолчанию может ждать ответ бесконечно.
func New() *Bybit {
	return &Bybit{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    bybitURL,
	}
}

// bybitResponse повторяет кусок чужого JSON. Наружу он не уходит.
// lastPrice у Bybit строка, не число.
type bybitResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			LastPrice string `json:"lastPrice"`
		} `json:"list"`
	} `json:"result"`
}

// Fetch забирает BTC и ETH и возвращает наши бланки
// Оба снимка получают одно время: это один проход тикера, а не два разных.
func (bybit *Bybit) Fetch() ([]model.Rate, error) {
	now := time.Now()
	btc, err := bybit.lastPrice("BTCUSDT")
	if err != nil {
		return nil, fmt.Errorf("BTC: %w", err)
	}
	eth, err := bybit.lastPrice("ETHUSDT")
	if err != nil {
		return nil, fmt.Errorf("ETH: %w", err)
	}
	return []model.Rate{
		{CoinSymbol: "BTC", PriceUSD: btc, FetchedAt: now},
		{CoinSymbol: "ETH", PriceUSD: eth, FetchedAt: now},
	}, nil
}

// lastPrice делает один запрос по символу биржи и достаёт последнюю цену в долларах.
func (bybit *Bybit) lastPrice(symbol string) (float64, error) {
	response, err := bybit.httpClient.Get(bybit.baseURL + symbol)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("биржа ответила кодом %d", response.StatusCode)
	}

	var body bybitResponse
	err = json.NewDecoder(response.Body).Decode(&body)
	if err != nil {
		return 0, err
	}
	if body.RetCode != 0 {
		return 0, fmt.Errorf("биржа: %s", body.RetMsg)
	}
	if len(body.Result.List) == 0 {
		return 0, fmt.Errorf("в ответе нет цены")
	}
	price, err := strconv.ParseFloat(body.Result.List[0].LastPrice, 64)
	if err != nil {
		return 0, fmt.Errorf("цена %q: %w", body.Result.List[0].LastPrice, err)
	}
	return price, nil
}

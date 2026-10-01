package bybit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// BybitClient ходит на биржу за текущими ценами BTC и ETH.
type BybitClient struct {
	httpClient *http.Client
	baseURL    string
}

// Жесткая проверка на этапе компиляции: BybitClient обязан реализовывать interfaces.Exchange.
var _ interfaces.Exchange = (*BybitClient)(nil)

// NewBybitClient возвращает клиент биржи. baseURL приходит из конфига.
func NewBybitClient(baseURL string) *BybitClient {
	return &BybitClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

type bybitResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			LastPrice string `json:"lastPrice"`
		} `json:"list"`
	} `json:"result"`
}

func (c *BybitClient) Fetch() ([]domain.Rate, error) {
	now := time.Now()
	btc, err := c.lastPrice("BTCUSDT")
	if err != nil {
		return nil, fmt.Errorf("BTC: %w", err)
	}
	eth, err := c.lastPrice("ETHUSDT")
	if err != nil {
		return nil, fmt.Errorf("ETH: %w", err)
	}
	return []domain.Rate{
		{CoinSymbol: "BTC", PriceUSD: btc, FetchedAt: now},
		{CoinSymbol: "ETH", PriceUSD: eth, FetchedAt: now},
	}, nil
}

func (c *BybitClient) lastPrice(symbol string) (float64, error) {
	response, err := c.httpClient.Get(c.baseURL + symbol)
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

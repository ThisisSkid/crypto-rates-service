package bybit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// BybitClient ходит на биржу за текущими ценами монет из domain.SupportedCoins.
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

// Fetch забирает все монеты из domain.SupportedCoins одним проходом.
// Оба снимка получают одно время: это один проход тикера, а не два разных.
func (c *BybitClient) Fetch(ctx context.Context) ([]domain.Rate, error) {
	now := time.Now()
	rates := make([]domain.Rate, 0, len(domain.SupportedCoins))
	for _, coin := range domain.SupportedCoins {
		price, err := c.lastPrice(ctx, coin+"USDT")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", coin, err)
		}
		rates = append(rates, domain.Rate{CoinSymbol: coin, PriceUSD: price, FetchedAt: now})
	}
	return rates, nil
}

// lastPrice делает один запрос по символу биржи и достаёт последнюю цену в долларах.
// Запрос привязан к контексту: Ctrl+C обрывает поход на биржу, а не ждёт таймаут.
func (c *BybitClient) lastPrice(ctx context.Context, symbol string) (float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+symbol, nil)
	if err != nil {
		return 0, err
	}
	response, err := c.httpClient.Do(request)
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

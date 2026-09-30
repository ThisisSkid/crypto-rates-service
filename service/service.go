package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"krypto-proekt/model"
)

// Store — что обработке нужно от склада. Интерфейс объявлен здесь, у потребителя (отдельно не стал выносить)
// Postgres его выполняет, но service про конкретную базу не знает.
type Store interface {
	SaveRate(ctx context.Context, rate model.Rate) error
	LatestRate(ctx context.Context, coinSymbol string) (model.Rate, error)
	DayMinMax(ctx context.Context, coinSymbol string) (minPrice float64, maxPrice float64, err error)
	PriceHourAgo(ctx context.Context, coinSymbol string) (float64, error)
}

// Exchange — откуда брать свежие цены. В тесте сюда подставляют заглушку.
type Exchange interface {
	Fetch() ([]model.Rate, error)
}

// Service обрабатывает курсы: биржа, запись в базу, текст для человека.
type Service struct {
	store    Store
	exchange Exchange
}

// New собирает обработку из склада и биржи.
func New(store Store, exchange Exchange) *Service {
	return &Service{store: store, exchange: exchange}
}

// Update один раз забирает курсы у биржи и пишет их новыми строками на склад.
// Какие именно монеты — решает биржа, обработка в этот выбор не лезет.
func (service *Service) Update(ctx context.Context) ([]model.Rate, error) {
	rates, err := service.exchange.Fetch()
	if err != nil {
		slog.Error("тикер не получил курсы", "err", err)
		return nil, err
	}
	for _, rate := range rates {
		err = service.store.SaveRate(ctx, rate)
		if err != nil {
			slog.Error("тикер не записал курс", "coin", rate.CoinSymbol, "err", err)
			return nil, err
		}
	}
	slog.Info("тикер записал курсы", "count", len(rates))
	return rates, nil
}

// Quote — снимок плюс min/max за день и процент за час для HTTP.
type Quote struct {
	model.Rate
	DayMin            float64
	DayMax            float64
	HourChangePercent *float64
}

// Quote собирает один ответ. Процента нет, если нет цены час назад.
func (service *Service) Quote(ctx context.Context, symbol string) (Quote, error) {
	rate, err := service.store.LatestRate(ctx, symbol)
	if err != nil {
		return Quote{}, err
	}
	quote := Quote{Rate: rate}
	quote.DayMin, quote.DayMax, err = service.store.DayMinMax(ctx, symbol)
	// Нет строк за сегодня — это не обрыв базы. Снимок остаётся, min/max пустые.
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return Quote{}, err
	}
	oldPrice, err := service.store.PriceHourAgo(ctx, symbol)
	if errors.Is(err, model.ErrNotFound) {
		return quote, nil
	}
	if err != nil {
		return Quote{}, err
	}
	if percent, ok := changePercent(rate.PriceUSD, oldPrice); ok {
		quote.HourChangePercent = &percent
	}
	return quote, nil
}

// Quotes собирает ответы по списку. Монета без строк пропускается.
func (service *Service) Quotes(ctx context.Context, symbols []string) ([]Quote, error) {
	quotes := make([]Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quote, err := service.Quote(ctx, symbol)
		if errors.Is(err, model.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, nil
}

// StatsText — строки min/max и процента для уже известного снимка.
func (service *Service) StatsText(ctx context.Context, rate model.Rate) string {
	text := "  за день: нет данных"
	minPrice, maxPrice, err := service.store.DayMinMax(ctx, rate.CoinSymbol)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return "  min/max: " + err.Error()
	}
	if err == nil {
		text = fmt.Sprintf("  за день min=%v max=%v", minPrice, maxPrice)
	}

	oldPrice, err := service.store.PriceHourAgo(ctx, rate.CoinSymbol)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return text + "\n  за час: мало данных"
		}
		return text + "\n  процент за час: " + err.Error()
	}
	percent, ok := changePercent(rate.PriceUSD, oldPrice)
	if !ok {
		return text + "\n  за час: мало данных"
	}
	return text + fmt.Sprintf("\n  за час изменение=%.2f%%", percent)
}

// Report собирает текст по списку монет для бота.
func (service *Service) Report(ctx context.Context, symbols []string) string {
	var builder strings.Builder
	for _, symbol := range symbols {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(service.reportOne(ctx, symbol))
	}
	return builder.String()
}

func (service *Service) reportOne(ctx context.Context, symbol string) string {
	rate, err := service.store.LatestRate(ctx, symbol)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return symbol + ": монета не найдена"
		}
		return symbol + ": " + err.Error()
	}
	minPrice, maxPrice, err := service.store.DayMinMax(ctx, rate.CoinSymbol)
	day := "за день: нет данных"
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		day = "за день: " + err.Error()
	} else if err == nil {
		day = fmt.Sprintf("за день: %v … %v $", minPrice, maxPrice)
	}

	hour := "за час: мало данных"
	oldPrice, err := service.store.PriceHourAgo(ctx, rate.CoinSymbol)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		hour = "за час: " + err.Error()
	}
	if err == nil {
		if percent, ok := changePercent(rate.PriceUSD, oldPrice); ok {
			hour = fmt.Sprintf("за час: %+.2f%%", percent)
		}
	}
	return fmt.Sprintf("%s: %v $\n%s\n%s\nвремя: %s", rate.CoinSymbol, rate.PriceUSD, day, hour, model.ClockText(rate.FetchedAt))
}

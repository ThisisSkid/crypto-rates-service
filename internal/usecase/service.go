package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
)

// nopLogger — тихий логгер: подставляется, если логгер не передали (тесты).
type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Service обрабатывает курсы: биржа, запись в базу, текст для человека.
type Service struct {
	store    interfaces.Store
	exchange interfaces.Exchange
	log      interfaces.Logger
}

// NewService собирает обработку из склада, биржи и логгера.
func NewService(store interfaces.Store, exchange interfaces.Exchange, log interfaces.Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{store: store, exchange: exchange, log: log}
}

// Update один раз забирает курсы у биржи и пишет их новыми строками на склад.
func (s *Service) Update(ctx context.Context) ([]domain.Rate, error) {
	rates, err := s.exchange.Fetch(ctx)
	if err != nil {
		s.log.Error("тикер не получил курсы", "err", err)
		return nil, err
	}
	for _, rate := range rates {
		err = s.store.SaveRate(ctx, rate)
		if err != nil {
			s.log.Error("тикер не записал курс", "coin", rate.CoinSymbol, "err", err)
			return nil, err
		}
	}
	s.log.Info("тикер записал курсы", "count", len(rates))
	return rates, nil
}

// Start запускает фоновое обновление курсов с интервалом из конфига.
// Работает, пока не отменят контекст. Это метод сервиса, а не функция в main.
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Refresh(ctx); err != nil {
				s.log.Error("фоновое обновление", "err", err)
			}
		}
	}
}

// Refresh — один проход обновления: забрать курсы, записать, вывести сводку.
func (s *Service) Refresh(ctx context.Context) error {
	list, err := s.Update(ctx)
	if err != nil {
		return err
	}
	for _, rate := range list {
		s.log.Info("курс обновлён",
			"монета", rate.CoinSymbol,
			"цена", rate.PriceUSD,
			"время", domain.ClockText(rate.FetchedAt))
		s.log.Info(s.StatsText(ctx, rate))
	}
	return nil
}

// Quote — снимок плюс min/max за день и процент за час для HTTP.
// DayMin/DayMax — указатели: «нет данных за день» в JSON это null, а не ноль.
type Quote struct {
	domain.Rate
	DayMin            *float64
	DayMax            *float64
	HourChangePercent *float64
}

// Quote собирает один ответ. Процента нет, если нет цены час назад.
func (s *Service) Quote(ctx context.Context, symbol string) (Quote, error) {
	rate, err := s.store.LatestRate(ctx, symbol)
	if err != nil {
		return Quote{}, err
	}
	quote := Quote{Rate: rate}

	minPrice, maxPrice, err := s.store.DayMinMax(ctx, symbol)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return Quote{}, err
	}
	if err == nil {
		quote.DayMin = &minPrice
		quote.DayMax = &maxPrice
	}

	oldPrice, err := s.store.PriceHourAgo(ctx, symbol)
	if errors.Is(err, domain.ErrNotFound) {
		return quote, nil // Нет истории -> HourChangePercent останется nil
	}
	if err != nil {
		return Quote{}, err
	}

	quote.HourChangePercent = changePercent(rate.PriceUSD, oldPrice)
	return quote, nil
}

// Quotes собирает ответы по списку. Монета без строк пропускается.
func (s *Service) Quotes(ctx context.Context, symbols []string) ([]Quote, error) {
	quotes := make([]Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quote, err := s.Quote(ctx, symbol)
		if errors.Is(err, domain.ErrNotFound) {
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
func (s *Service) StatsText(ctx context.Context, rate domain.Rate) string {
	text := "  за день: нет данных"
	minPrice, maxPrice, err := s.store.DayMinMax(ctx, rate.CoinSymbol)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return "  min/max: " + err.Error()
	}
	if err == nil {
		text = fmt.Sprintf("  за день min=%v max=%v", minPrice, maxPrice)
	}

	oldPrice, err := s.store.PriceHourAgo(ctx, rate.CoinSymbol)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return text + "\n  за час: мало данных"
		}
		return text + "\n  процент за час: " + err.Error()
	}

	percent := changePercent(rate.PriceUSD, oldPrice)
	if percent == nil {
		return text + "\n  за час: мало данных"
	}
	return text + fmt.Sprintf("\n  за час изменение=%.2f%%", *percent)
}

// Report собирает текст по списку монет для бота.
func (s *Service) Report(ctx context.Context, symbols []string) string {
	var builder strings.Builder
	for _, symbol := range symbols {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(s.reportOne(ctx, symbol))
	}
	return builder.String()
}

func (s *Service) reportOne(ctx context.Context, symbol string) string {
	rate, err := s.store.LatestRate(ctx, symbol)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return symbol + ": монета не найдена"
		}
		return symbol + ": " + err.Error()
	}

	minPrice, maxPrice, err := s.store.DayMinMax(ctx, rate.CoinSymbol)
	day := "за день: нет данных"
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		day = "за день: " + err.Error()
	} else if err == nil {
		day = fmt.Sprintf("за день: %v … %v $", minPrice, maxPrice)
	}

	hour := "за час: мало данных"
	oldPrice, err := s.store.PriceHourAgo(ctx, rate.CoinSymbol)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		hour = "за час: " + err.Error()
	} else if err == nil {
		percent := changePercent(rate.PriceUSD, oldPrice)
		if percent != nil {
			hour = fmt.Sprintf("за час: %+.2f%%", *percent)
		}
	}

	return fmt.Sprintf("%s: %v $\n%s\n%s\nвремя: %s", rate.CoinSymbol, rate.PriceUSD, day, hour, domain.ClockText(rate.FetchedAt))
}

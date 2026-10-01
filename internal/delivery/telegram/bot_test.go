package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/interfaces"
	"krypto-proekt/internal/usecase"
)

// replyStore — минимальный склад: в таблице есть только BTC.
type replyStore struct{}

var _ interfaces.Store = replyStore{}

func (replyStore) SaveRate(context.Context, domain.Rate) error { return nil }

func (replyStore) LatestRate(_ context.Context, coinSymbol string) (domain.Rate, error) {
	if coinSymbol != "BTC" {
		return domain.Rate{}, domain.ErrNotFound
	}
	return domain.Rate{CoinSymbol: "BTC", PriceUSD: 110, FetchedAt: time.Unix(0, 0).UTC()}, nil
}

func (replyStore) DayMinMax(context.Context, string) (float64, float64, error) { return 90, 110, nil }

func (replyStore) PriceHourAgo(context.Context, string) (float64, error) { return 100, nil }

func testMessage(text string) *tgbotapi.Message {
	return &tgbotapi.Message{Text: text, Chat: &tgbotapi.Chat{ID: 42}}
}

// Маршрутизация команд из ТЗ. bot здесь не нужен: отправкой занимается вызывающая горутина.
func TestReplyRoutesCommands(t *testing.T) {
	rates := usecase.NewService(replyStore{}, nil)
	tests := []struct {
		name     string
		text     string
		contains string
	}{
		{name: "справка", text: "/start", contains: "/start_auto N"},
		{name: "курсы обеих монет", text: "/rates", contains: "BTC: 110 $"},
		{name: "курс одной монеты", text: "/rates BTC", contains: "BTC: 110 $"},
		{name: "монета в нижнем регистре", text: "/rates btc", contains: "BTC: 110 $"},
		{name: "монеты нет в таблице", text: "/rates DOGE", contains: "DOGE: монета не найдена"},
		{name: "только биткоин", text: "/rates_btc", contains: "BTC: 110 $"},
		{name: "неизвестная команда", text: "/погода", contains: "Команда не распознана"},
		{name: "выключение без включения", text: "/stop_auto", contains: "Авторассылка не была включена"},
		{name: "мусор вместо минут", text: "/start_auto пять", contains: "Пример: /start_auto 15"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
			text := reply(context.Background(), rates, nil, autos, testMessage(test.text))
			if !strings.Contains(text, test.contains) {
				t.Fatalf("в ответе нет %q:\n%s", test.contains, text)
			}
		})
	}
}

// Полный путь команды из ТЗ: /start_auto 15 включает рассылку с заданным периодом,
// /stop_auto её выключает. Первая отправка случилась бы через 15 минут, до неё тест не доживёт.
func TestReplyStartAuto15(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
	rates := usecase.NewService(replyStore{}, nil)

	started := reply(ctx, rates, nil, autos, testMessage("/start_auto 15"))
	if !strings.Contains(started, "каждые 15 мин") {
		t.Fatalf("ждали период 15 минут, получили: %s", started)
	}
	autos.mu.Lock()
	_, running := autos.cancel[42]
	autos.mu.Unlock()
	if !running {
		t.Fatal("рассылка не зарегистрирована для чата")
	}

	stopped := reply(ctx, rates, nil, autos, testMessage("/stop_auto"))
	if !strings.Contains(stopped, "Авторассылка выключена") {
		t.Fatalf("ждали выключение, получили: %s", stopped)
	}
	cancel()
	autos.workers.Wait()
}

// /start_auto_10 — та же рассылка с периодом по умолчанию.
func TestReplyStartAutoDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
	text := reply(ctx, usecase.NewService(replyStore{}, nil), nil, autos, testMessage("/start_auto_10"))
	if !strings.Contains(text, "каждые 10 мин") {
		t.Fatalf("ждали период 10 минут, получили: %s", text)
	}
	cancel()
	autos.workers.Wait()
}

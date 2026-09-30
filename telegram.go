package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
)

// startTelegram запускает бота в фоне, если задан TELEGRAM_BOT_TOKEN.
// Пустой токен не роняет HTTP: бот просто не стартует.
func startTelegram(ctx context.Context, conn *pgx.Conn) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		fmt.Println("бот выключен: нет переменной TELEGRAM_BOT_TOKEN")
		return
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		fmt.Println("бот:", err)
		return
	}
	fmt.Println("бот запущен:", bot.Self.UserName)

	// Long polling: сами спрашиваем Telegram «есть новые сообщения?», без своего URL в интернете.
	updatesConfig := tgbotapi.NewUpdate(0)
	updatesConfig.Timeout = 30
	updates := bot.GetUpdatesChan(updatesConfig)

	go func() {
		for update := range updates {
			if update.Message == nil {
				continue
			}
			text := replyTelegram(ctx, conn, update.Message.Text)
			message := tgbotapi.NewMessage(update.Message.Chat.ID, text)
			_, err := bot.Send(message)
			if err != nil {
				fmt.Println("бот отправка:", err)
			}
		}
	}()
}

// replyTelegram разбирает команду и готовит текст из таблицы, не с биржи.
func replyTelegram(ctx context.Context, conn *pgx.Conn, text string) string {
	command, arg := splitCommand(text)
	switch command {
	case "/start":
		return "Команды: /rates, /rates BTC, /rates ETH. Курсы из базы, биржа обновляет их раз в 5 минут."
	case "/rates":
		if arg == "" {
			return formatRates(ctx, conn, []string{"BTC", "ETH"})
		}
		return formatRates(ctx, conn, []string{strings.ToUpper(arg)})
	default:
		return "Не понял. Напиши /rates или /rates BTC"
	}
}

// splitCommand отделяет /rates от аргумента. Хвост @ИмяБота отбрасывается.
func splitCommand(text string) (command string, arg string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	command = fields[0]
	if at := strings.Index(command, "@"); at >= 0 {
		command = command[:at]
	}
	if len(fields) > 1 {
		arg = fields[1]
	}
	return command, arg
}

// formatRates собирает ответ по списку монет: цена, min/max дня, процент за час.
func formatRates(ctx context.Context, conn *pgx.Conn, symbols []string) string {
	var builder strings.Builder
	for _, symbol := range symbols {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(formatOneRate(ctx, conn, symbol))
	}
	return builder.String()
}

func formatOneRate(ctx context.Context, conn *pgx.Conn, symbol string) string {
	rate, err := getLatestRate(ctx, conn, symbol)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return symbol + ": монета не найдена"
		}
		return symbol + ": " + err.Error()
	}

	text := fmt.Sprintf("%s цена=%v время=%s", rate.CoinSymbol, rate.PriceUSD, rate.FetchedAt.Format("2006-01-02 15:04"))

	minPrice, maxPrice, err := dayMinMax(ctx, conn, symbol)
	if err != nil {
		return text + "\n  min/max: " + err.Error()
	}
	text += fmt.Sprintf("\n  за день min=%v max=%v", minPrice, maxPrice)

	percent, ok, err := hourChangePercent(ctx, conn, symbol, rate.PriceUSD)
	if err != nil {
		return text + "\n  процент за час: " + err.Error()
	}
	if !ok {
		return text + "\n  за час: мало данных"
	}
	return text + fmt.Sprintf("\n  за час изменение=%.2f%%", percent)
}

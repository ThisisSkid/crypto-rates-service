package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/usecase"
)

// defaultAutoMinutes — период авторассылки, если клиент не назвал своё число.
const defaultAutoMinutes = 10

// chatAuto хранит отмену авторассылки отдельно для каждого чата.
type chatAuto struct {
	mu      sync.Mutex
	cancel  map[int64]context.CancelFunc
	workers sync.WaitGroup
}

// Start запускает бота в фоне, если задан токен.
// Пустой токен не роняет HTTP.
// Возвращает ожидание: main дождётся выхода горутин бота, прежде чем закрывать пул и лог.
func Start(ctx context.Context, rates *usecase.Service, token string) (wait func()) {
	if token == "" {
		zap.S().Info("бот выключен: токен не задан")
		return func() {}
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		zap.S().Errorw("бот не запустился", "err", err)
		return func() {}
	}
	zap.S().Infow("бот запущен", "user", bot.Self.UserName)

	_, err = bot.Request(tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Список команд"},
		tgbotapi.BotCommand{Command: "rates", Description: "Курсы BTC и ETH"},
		tgbotapi.BotCommand{Command: "rates_btc", Description: "Только биткоин"},
		tgbotapi.BotCommand{Command: "rates_eth", Description: "Только эфир"},
		tgbotapi.BotCommand{Command: "start_auto", Description: "Авторассылка каждые N минут (пример: /start_auto 15)"},
		tgbotapi.BotCommand{Command: "start_auto_10", Description: "Авторассылка каждые 10 минут"},
		tgbotapi.BotCommand{Command: "stop_auto", Description: "Остановить авторассылку"},
	))
	if err != nil {
		zap.S().Errorw("команды бота", "err", err)
	}

	updatesConfig := tgbotapi.NewUpdate(0)
	updatesConfig.Timeout = 30
	updates := bot.GetUpdatesChan(updatesConfig)

	autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
	var poll sync.WaitGroup
	poll.Add(1)
	go func() {
		defer poll.Done()
		// Тот же ctx, что и у HTTP. По сигналу выходим из long poll, а не висим до убийства процесса.
		for {
			select {
			case <-ctx.Done():
				bot.StopReceivingUpdates()
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				if update.Message == nil {
					continue
				}
				command, _ := splitCommand(update.Message.Text)
				zap.S().Infow("бот команда", "command", command, "chat", update.Message.Chat.ID)
				text := reply(ctx, rates, bot, autos, update.Message)
				message := tgbotapi.NewMessage(update.Message.Chat.ID, text)
				if _, err := bot.Send(message); err != nil {
					zap.S().Errorw("бот отправка", "err", err)
				}
			}
		}
	}()

	return func() {
		poll.Wait()
		autos.workers.Wait()
	}
}

// start включает авторассылку для одного чата. Прежнюю рассылку этого чата сначала гасим,
// иначе клиент получал бы курсы дважды с двумя разными периодами.
func (a *chatAuto) start(ctx context.Context, rates *usecase.Service, bot *tgbotapi.BotAPI, chatID int64, minutes int) {
	a.stop(chatID)
	runCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel[chatID] = cancel
	a.mu.Unlock()
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ticker := time.NewTicker(time.Duration(minutes) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				text := rates.Report(runCtx, domain.SupportedCoins)
				message := tgbotapi.NewMessage(chatID, text)
				if _, err := bot.Send(message); err != nil {
					zap.S().Errorw("авторассылка", "chat", chatID, "err", err)
				}
			}
		}
	}()
}

// stop выключает авторассылку чата и сообщает, была ли она включена.
func (a *chatAuto) stop(chatID int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cancel, ok := a.cancel[chatID]
	if !ok {
		return false
	}
	cancel()
	delete(a.cancel, chatID)
	return true
}

// reply разбирает команду и возвращает текст ответа. Отправкой занимается вызывающая горутина,
// поэтому здесь нет ни сети, ни SQL — только выбор ветки и вызовы обработки.
func reply(ctx context.Context, rates *usecase.Service, bot *tgbotapi.BotAPI, autos *chatAuto, message *tgbotapi.Message) string {
	command, arg := splitCommand(message.Text)
	switch command {
	case "/start":
		return commandHelp()
	case "/rates":
		if arg == "" {
			return rates.Report(ctx, domain.SupportedCoins)
		}
		return rates.Report(ctx, []string{strings.ToUpper(arg)})
	case "/btc", "/rates-btc":
		return rates.Report(ctx, []string{"BTC"})
	case "/eth", "/rates-eth":
		return rates.Report(ctx, []string{"ETH"})
	case "/start-auto-10":
		arg = strconv.Itoa(defaultAutoMinutes)
		fallthrough
	case "/start-auto":
		minutes, ok := autoMinutes(arg)
		if !ok {
			return "Нужно целое число минут. Пример: /start_auto 15"
		}
		autos.start(ctx, rates, bot, message.Chat.ID, minutes)
		return fmt.Sprintf("Авторассылка включена, каждые %d мин.", minutes)
	case "/stop-auto":
		if autos.stop(message.Chat.ID) {
			return "Авторассылка выключена"
		}
		return "Авторассылка не была включена"
	default:
		return "Команда не распознана.\n" + commandHelp()
	}
}

// commandHelp — текст для /start и для нераспознанной команды.
func commandHelp() string {
	return strings.Join([]string{
		"/start — показать этот список",
		"/rates — курсы BTC и ETH",
		"/rates BTC — курс одной монеты",
		"/rates_btc — только биткоин",
		"/rates_eth — только эфир",
		"/start_auto N — присылать курсы каждые N минут (пример: /start_auto 15)",
		"/start_auto_10 — то же самое с периодом 10 минут",
		"/stop_auto — остановить авторассылку",
	}, "\n")
}

// autoMinutes читает аргумент /start_auto. Без аргумента берём период по умолчанию.
func autoMinutes(arg string) (minutes int, ok bool) {
	if arg == "" {
		return defaultAutoMinutes, true
	}
	parsed, err := strconv.Atoi(arg)
	if err != nil || parsed < 1 {
		return 0, false
	}
	return parsed, true
}

// splitCommand достаёт из сообщения команду и её аргумент.
func splitCommand(text string) (command string, arg string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	command = fields[0]
	if at := strings.Index(command, "@"); at >= 0 {
		command = command[:at]
	}
	// Telegram парсит только до дефиса, поэтому /start_auto тоже принимает.
	command = strings.ReplaceAll(command, "_", "-")
	if len(fields) > 1 {
		arg = fields[1]
	}
	return command, arg
}

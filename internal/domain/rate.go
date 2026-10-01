package domain

import "time"

// SupportedCoins — список монет, которые сервис отслеживает.
// Единственный источник истины для списка активов.
var SupportedCoins = []string{"BTC", "ETH"}

// Moscow — пояс для текста человеку. Фиксированный UTC+3.
// у Москвы нет перехода на лето, LoadLocation в контейнере может не найти Europe/Moscow.
var Moscow = time.FixedZone("MSK", 3*60*60)

// ClockText — время снимка в сообщении бота и в консоли: день.месяц часы:минуты по Москве.
func ClockText(at time.Time) string {
	return at.In(Moscow).Format("02.01 15:04")
}

// Rate — снимок курса одной монеты. Это общий бланк склада и обработки.
type Rate struct {
	CoinSymbol string
	PriceUSD   float64
	FetchedAt  time.Time
}

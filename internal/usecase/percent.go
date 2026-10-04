package usecase

// changePercent считает процент изменения цены.
// Если истории нет (первый запуск) или старая цена 0 -> вернуть nil.
func changePercent(currentPrice float64, oldPrice float64) *float64 {
	if oldPrice == 0 {
		return nil
	}
	percent := (currentPrice - oldPrice) / oldPrice * 100
	return &percent
}

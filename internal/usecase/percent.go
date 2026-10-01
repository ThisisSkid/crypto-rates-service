package usecase

// changePercent считает процент изменения цены.
// Если истории нет (первый запуск) или старая цена 0 -> вернуть nil, nil
// Обработка отсутствия данных происходит явно на уровне UseCase.
func changePercent(currentPrice float64, oldPrice float64) (*float64, error) {
	if oldPrice == 0 {
		return nil, nil
	}
	percent := (currentPrice - oldPrice) / oldPrice * 100
	return &percent, nil
}

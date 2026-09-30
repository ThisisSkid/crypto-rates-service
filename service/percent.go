package service

// changePercent считает процент изменения цены.
// ok=false, если старая цена 0: на ноль делить нельзя.
func changePercent(currentPrice float64, oldPrice float64) (percent float64, ok bool) {
	if oldPrice == 0 {
		return 0, false
	}
	percent = (currentPrice - oldPrice) / oldPrice * 100
	return percent, true
}

package service

import "testing"

// TestChangePercent проверяет корректность расчета процента изменения цены.
// Особое внимание уделяется граничному случаю деления на ноль.
func TestChangePercent(t *testing.T) {
	tests := []struct {
		name         string
		currentPrice float64
		oldPrice     float64
		wantPercent  float64
		wantOK       bool
	}{
		{name: "рост", currentPrice: 110, oldPrice: 100, wantPercent: 10, wantOK: true},
		{name: "падение", currentPrice: 90, oldPrice: 100, wantPercent: -10, wantOK: true},
		{name: "ноль", currentPrice: 100, oldPrice: 0, wantPercent: 0, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotPercent, gotOK := changePercent(test.currentPrice, test.oldPrice)
			// Проверяем оба возвращаемых значения: успех операции и сам процент
			if gotOK != test.wantOK || gotPercent != test.wantPercent {
				t.Fatalf("получили %v %v, ждали %v %v", gotPercent, gotOK, test.wantPercent, test.wantOK)
			}
		})
	}
}

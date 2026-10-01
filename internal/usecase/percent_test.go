package usecase

import "testing"

// Вспомогательная функция для создания указателя на float64 в тестах
func ptr(f float64) *float64 { return &f }

func TestChangePercent(t *testing.T) {
	tests := []struct {
		name         string
		currentPrice float64
		oldPrice     float64
		wantPercent  *float64
		wantErr      bool
	}{
		{name: "рост", currentPrice: 110, oldPrice: 100, wantPercent: ptr(10)},
		{name: "падение", currentPrice: 90, oldPrice: 100, wantPercent: ptr(-10)},
		{name: "ноль (нет данных)", currentPrice: 100, oldPrice: 0, wantPercent: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotPercent, err := changePercent(test.currentPrice, test.oldPrice)
			if (err != nil) != test.wantErr {
				t.Fatalf("ошибка: получили %v, ждали ошибку: %v", err, test.wantErr)
			}
			if test.wantPercent == nil && gotPercent != nil {
				t.Fatalf("ждали nil, получили %v", *gotPercent)
			}
			if test.wantPercent != nil && (gotPercent == nil || *gotPercent != *test.wantPercent) {
				t.Fatalf("получили %v, ждали %v", gotPercent, *test.wantPercent)
			}
		})
	}
}

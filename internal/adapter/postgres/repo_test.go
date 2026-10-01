package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"krypto-proekt/internal/domain"
)

// Агрегаты MIN/MAX по пустой выборке возвращают NULL, поэтому Scan идёт в *float64.
// Таблица проверяет все четыре комбинации NULL: одного nil достаточно, чтобы отдать ErrNotFound.
func TestDayMinMax(t *testing.T) {
	low, high := 90.0, 110.0
	tests := []struct {
		name    string
		minPtr  *float64
		maxPtr  *float64
		wantMin float64
		wantMax float64
		wantErr error
	}{
		{name: "есть строки за сегодня", minPtr: &low, maxPtr: &high, wantMin: 90, wantMax: 110},
		{name: "пустая выборка, оба NULL", wantErr: domain.ErrNotFound},
		{name: "NULL только в минимуме", maxPtr: &high, wantErr: domain.ErrNotFound},
		{name: "NULL только в максимуме", minPtr: &low, wantErr: domain.ErrNotFound},
		{name: "одна строка, минимум равен максимуму", minPtr: &high, maxPtr: &high, wantMin: 110, wantMax: 110},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minPrice, maxPrice, err := dayMinMax(test.minPtr, test.maxPtr)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("получили ошибку %v, ждали %v", err, test.wantErr)
			}
			if minPrice != test.wantMin || maxPrice != test.wantMax {
				t.Fatalf("получили %v..%v, ждали %v..%v", minPrice, maxPrice, test.wantMin, test.wantMax)
			}
		})
	}
}

// Тот же NULL, но от живого Postgres: монеты нет в таблице, значит выборка пустая.
// Запускается только с POSTGRES_TEST_URL, иначе go test ./... не требует базы.
func TestDayMinMaxOnRealPostgres(t *testing.T) {
	testURL := os.Getenv("POSTGRES_TEST_URL")
	if testURL == "" {
		t.Skip("нет POSTGRES_TEST_URL, пропускаем интеграционный тест")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatalf("не открылся пул: %v", err)
	}
	defer pool.Close()

	repo := NewPostgresRepo(pool)
	_, _, err = repo.DayMinMax(ctx, "МОНЕТЫ-С-ТАКИМ-ИМЕНЕМ-НЕТ")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("на пустой выборке ждали ErrNotFound, получили %v", err)
	}
}

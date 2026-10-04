package config

import (
	"os"
	"testing"
)

// Без POSTGRES_URL Load обязан вернуть ошибку: пароля в коде нет,
// процесс должен упасть сразу, а не молча работать без базы.
// Переменную на время теста убираем полностью и возвращаем в t.Cleanup,
func TestLoadRequiresPostgresURL(t *testing.T) {
	old, had := os.LookupEnv("POSTGRES_URL")
	os.Unsetenv("POSTGRES_URL")
	t.Cleanup(func() {
		if had {
			os.Setenv("POSTGRES_URL", old)
		} else {
			os.Unsetenv("POSTGRES_URL")
		}
	})

	if _, err := Load(); err == nil {
		t.Fatal("без POSTGRES_URL Load должен вернуть ошибку")
	}
}

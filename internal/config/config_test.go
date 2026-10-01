package config

import (
	"os"
	"testing"
)

func TestLoadRequiresPostgresURL(t *testing.T) {
	os.Unsetenv("POSTGRES_URL")
	_, err := Load()
	if err == nil {
		t.Fatal("без POSTGRES_URL Load должен вернуть ошибку")
	}
}

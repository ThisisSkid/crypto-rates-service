package domain

import (
	"testing"
	"time"
)

// была проблема в том, что бот отдавал содержимое по мировому времени, были не состыковки, заменено на +3 часа
func TestClockTextMoscow(t *testing.T) {
	at := time.Date(2026, 9, 28, 18, 5, 0, 0, time.UTC)
	got := ClockText(at)
	if got != "28.09 21:05" {
		t.Fatalf("время для человека: получили %q, ждали 28.09 21:05 (Москва)", got)
	}
}

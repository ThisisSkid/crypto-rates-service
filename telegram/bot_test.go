package telegram

import (
	"context"
	"testing"
)

// Telegram подсвечивает команду только до подчёркивания, поэтому принимаем оба написания,
// а из упоминания вида /rates@ИмяБота убираем хвост с собачкой.
func TestSplitCommand(t *testing.T) {
	tests := []struct {
		text    string
		command string
		arg     string
	}{
		{text: "/rates", command: "/rates"},
		{text: "/rates BTC", command: "/rates", arg: "BTC"},
		{text: "/start-auto 10", command: "/start-auto", arg: "10"},
		{text: "/start-auto 15", command: "/start-auto", arg: "15"},
		{text: "/rates_btc", command: "/rates-btc"},
		{text: "/start_auto_10", command: "/start-auto-10"},
		{text: "/start_auto 10", command: "/start-auto", arg: "10"},
		{text: "/start_auto 15", command: "/start-auto", arg: "15"},
		{text: "/rates@KryptoSkidBot ETH", command: "/rates", arg: "ETH"},
	}
	for _, test := range tests {
		command, arg := splitCommand(test.text)
		if command != test.command || arg != test.arg {
			t.Fatalf("%q: получили %s %s", test.text, command, arg)
		}
	}
}

// Аргумент /start_auto приходит строкой от пользователя, поэтому мусор
// не должен включать рассылку с нулевым или отрицательным периодом.
func TestAutoMinutes(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		wantMinutes int
		wantOK      bool
	}{
		{name: "пример из ТЗ", arg: "15", wantMinutes: 15, wantOK: true},
		{name: "минута тоже годится", arg: "1", wantMinutes: 1, wantOK: true},
		{name: "без аргумента берём период по умолчанию", arg: "", wantMinutes: defaultAutoMinutes, wantOK: true},
		{name: "ноль минут означал бы рассылку без паузы", arg: "0"},
		{name: "отрицательный период", arg: "-5"},
		{name: "не число", arg: "десять"},
		{name: "дробное число", arg: "1.5"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minutes, ok := autoMinutes(test.arg)
			if ok != test.wantOK || minutes != test.wantMinutes {
				t.Fatalf("получили %d %v, ждали %d %v", minutes, ok, test.wantMinutes, test.wantOK)
			}
		})
	}
}

// Карта отмен закрыта мьютексом, потому что в неё пишет горутина бота, а читают таймеры.
// stop обязан отменить контекст рассылки и забыть чат, повторный stop — вернуть false.
func TestChatAutoStop(t *testing.T) {
	autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
	runCtx, cancel := context.WithCancel(context.Background())
	autos.cancel[42] = cancel

	if !autos.stop(42) {
		t.Fatal("первый stop должен был выключить рассылку")
	}
	if runCtx.Err() == nil {
		t.Fatal("stop не отменил контекст рассылки")
	}
	if autos.stop(42) {
		t.Fatal("повторный stop должен вернуть false: рассылки уже нет")
	}
	if len(autos.cancel) != 0 {
		t.Fatalf("в карте осталось %d записей, ждали 0", len(autos.cancel))
	}
}

// Чужой чат выключать нельзя: рассылки живут независимо.
func TestChatAutoStopOtherChat(t *testing.T) {
	autos := &chatAuto{cancel: make(map[int64]context.CancelFunc)}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	autos.cancel[42] = cancel

	if autos.stop(100) {
		t.Fatal("stop чужого чата должен вернуть false")
	}
	if runCtx.Err() != nil {
		t.Fatal("контекст чужой рассылки отменять нельзя")
	}
}

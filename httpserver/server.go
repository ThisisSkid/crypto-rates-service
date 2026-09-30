package httpserver

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"krypto-proekt/model"
	"krypto-proekt/service"
)

// New собирает HTTP-маршруты. SQL здесь нет: только вызовы обработки.
func New(rates *service.Service) http.Handler {
	router := chi.NewRouter()
	router.Get("/rates", handleAllRates(rates))
	router.Get("/rates/{coin}", handleOneRate(rates))
	return router
}

func handleOneRate(rates *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		coin := strings.ToUpper(chi.URLParam(request, "coin"))
		quote, err := rates.Quote(request.Context(), coin)
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				http.Error(w, "монета не найдена", http.StatusNotFound)
				return
			}
			// Текст err уходит только в журнал. Клиенту не отдаем детал базы и путей.
			slog.Error("http курс", "coin", coin, "err", err)
			http.Error(w, "Не удалось получить данные", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(quote)
	}
}

func handleAllRates(rates *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		list, err := rates.Quotes(request.Context(), model.SupportedCoins)
		if err != nil {
			slog.Error("http курсы", "err", err)
			http.Error(w, "Не удалось получить данные", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

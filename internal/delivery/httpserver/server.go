package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"krypto-proekt/internal/domain"
	"krypto-proekt/internal/usecase"
)

// New собирает HTTP-маршруты. SQL здесь нет: только вызовы обработки.
func New(rates *usecase.Service) http.Handler {
	router := chi.NewRouter()
	router.Get("/rates", handleAllRates(rates))
	router.Get("/rates/{coin}", handleOneRate(rates))
	return router
}

func handleOneRate(rates *usecase.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		coin := strings.ToUpper(chi.URLParam(request, "coin"))
		quote, err := rates.Quote(request.Context(), coin)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				http.Error(w, "монета не найдена", http.StatusNotFound)
				return
			}
			// Текст err уходит только в журнал. Клиенту не отдаём детали базы и путей.
			zap.S().Errorw("http курс", "coin", coin, "err", err)
			http.Error(w, "Не удалось получить данные", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(quote)
	}
}

func handleAllRates(rates *usecase.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		list, err := rates.Quotes(request.Context(), domain.SupportedCoins)
		if err != nil {
			zap.S().Errorw("http курсы", "err", err)
			http.Error(w, "Не удалось получить данные", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}
}

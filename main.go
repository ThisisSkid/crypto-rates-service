package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"krypto-proekt/client"
	"krypto-proekt/httpserver"
	"krypto-proekt/model"
	"krypto-proekt/repository"
	"krypto-proekt/service"
	"krypto-proekt/telegram"
)

// main отвечает только за код выхода процесса.
// Вся работа в run: os.Exit прямо здесь пропустил бы defer и не закрыл бы пул с логом.
func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// run собирает три роли в один процесс: склад, обработка, отправка —
// и держит их живыми до сигнала остановки.
func run() error {
	logFile, err := setupLogging()
	if err != nil {
		return err
	}
	defer logFile.Close()

	// Один контекст на процесс. Сигнал вызывает cancel, его видят тикер, бот и запросы к базе.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			slog.Info("получен сигнал, останавливаемся")
			cancel()
		case <-ctx.Done():
		}
	}()

	store, err := repository.Open(ctx)
	if err != nil {
		return err
	}
	defer store.Close()

	rates := service.New(store, client.New())

	err = refresh(ctx, rates)
	if err != nil {
		return fmt.Errorf("первое обновление: %w", err)
	}

	// Фоновые роли считаем. Без ожидания store.Close закрыл бы пул под работающим тикером.
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		runTicker(ctx, rates)
	}()
	waitBot := telegram.Start(ctx, rates)

	// http.Server нужен ради Shutdown. ListenAndServe сам по сигналу не останавливается.
	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           httpserver.New(rates),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	slog.Info("сервис слушает", "http", ":8080", "лог", "logs/app.log")
	fmt.Println("сервис работает, курс каждый час, HTTP :8080, лог logs/app.log, Ctrl+C выход")

	var serveFailure error
	select {
	case <-ctx.Done():
	case serveFailure = <-serveErr:
		slog.Error("http упал", "err", serveFailure)
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	err = httpServer.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("http shutdown", "err", err)
	}

	// Сначала гасим тех, кто ходит в базу и пишет в лог, потом отработают defer'ы выше.
	waitBot()
	workers.Wait()
	if serveFailure != nil {
		return fmt.Errorf("http: %w", serveFailure)
	}
	return nil
}

// runTicker пишет курсы каждый час, пока не отменят корневой контекст.
func runTicker(ctx context.Context, rates *service.Service) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := refresh(ctx, rates)
			if err != nil {
				fmt.Println("фоновое обновление:", err)
			}
		}
	}
}

// refresh — один проход тикера и печать в консоль. Консоль тоже отправитель, рядом с HTTP и ботом.
func refresh(ctx context.Context, rates *service.Service) error {
	list, err := rates.Update(ctx)
	if err != nil {
		return err
	}
	for _, rate := range list {
		fmt.Printf("монета=%s цена=%v время=%s\n", rate.CoinSymbol, rate.PriceUSD, model.ClockText(rate.FetchedAt))
		fmt.Println(rates.StatsText(ctx, rate))
	}
	return nil
}

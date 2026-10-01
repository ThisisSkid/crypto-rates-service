package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"krypto-proekt/internal/adapter/bybit"
	"krypto-proekt/internal/adapter/postgres"
	"krypto-proekt/internal/config"
	"krypto-proekt/internal/delivery/httpserver"
	"krypto-proekt/internal/delivery/telegram"
	"krypto-proekt/internal/logger"
	"krypto-proekt/internal/usecase"
)

// main отвечает только за код выхода процесса.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run — Composition Root: собирает зависимости и держит процесс живым до сигнала.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("конфиг: %w", err)
	}

	log, closeLog, err := logger.New(cfg.LogLevel, cfg.LogFile)
	if err != nil {
		return err
	}
	defer closeLog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			log.Info("получен сигнал, останавливаемся")
			cancel()
		case <-ctx.Done():
		}
	}()

	// Пул соединений создаётся и пингуется здесь, в main.
	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("не удалось открыть postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("postgres не отвечает: %w", err)
	}
	defer pool.Close()

	svc := usecase.NewService(
		postgres.NewPostgresRepo(pool),
		bybit.NewBybitClient(cfg.BybitURL),
	)

	if err := svc.Refresh(ctx); err != nil {
		return fmt.Errorf("первое обновление: %w", err)
	}

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		svc.Start(ctx, cfg.UpdateInterval)
	}()

	waitBot := telegram.Start(ctx, svc, cfg.TelegramBotToken)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.New(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	log.Info("сервис слушает", zap.String("http", cfg.HTTPAddr), zap.String("лог", cfg.LogFile))

	var serveFailure error
	select {
	case <-ctx.Done():
	case serveFailure = <-serveErr:
		log.Error("http упал", zap.Error(serveFailure))
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", zap.Error(err))
	}

	waitBot()
	workers.Wait()

	if serveFailure != nil {
		return fmt.Errorf("http: %w", serveFailure)
	}
	return nil
}

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

// App держит всё собранное: зависимости и компоненты готовности к запуску.
type App struct {
	log        *zap.Logger
	pool       *pgxpool.Pool
	svc        *usecase.Service
	httpServer *http.Server
	botToken   string
	updateInt  time.Duration
}

// main отвечает только за код выхода процесса.
func main() {
	app := &App{}
	if err := app.newApp(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newApp - Composition Root: собирает все зависимости и компоненты.
// Здесь происходит вся инициализация: конфиг, логгер, пул БД, сервис, HTTP-сервер.
func (a *App) newApp() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("конфиг: %w", err)
	}

	log, appLog, closeLog, err := logger.New(cfg.LogLevel, cfg.LogFile)
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		closeLog()
		return fmt.Errorf("не удалось открыть postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		closeLog()
		return fmt.Errorf("postgres не отвечает: %w", err)
	}

	svc := usecase.NewService(
		postgres.NewPostgresRepo(pool),
		bybit.NewBybitClient(cfg.BybitURL),
		appLog,
	)

	if err := svc.Refresh(ctx); err != nil {
		pool.Close()
		closeLog()
		return fmt.Errorf("первое обновление: %w", err)
	}

	a.httpServer = &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.New(a.svc),
		ReadHeaderTimeout: 5 * time.Second,
	}

	a.log = log
	a.pool = pool
	a.botToken = cfg.TelegramBotToken
	a.updateInt = cfg.UpdateInterval
	log.Info("сервис собран", zap.String("http", cfg.HTTPAddr), zap.String("лог", cfg.LogFile))
	return nil
}

// run - только запуск компонентов и graceful stop по сигналу.
// Инициализации нет: всё собрано в newApp.
func (a *App) run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			a.log.Info("получен сигнал, останавливаемся")
			cancel()
		case <-ctx.Done():
		}
	}()

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		a.svc.Start(ctx, a.updateInt)
	}()

	waitBot := telegram.Start(ctx, a.svc, a.botToken)

	serveErr := make(chan error, 1)
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var serveFailure error
	select {
	case <-ctx.Done():
	case serveFailure = <-serveErr:
		a.log.Error("http упал", zap.Error(serveFailure))
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		a.log.Error("http shutdown", zap.Error(err))
	}

	waitBot()
	workers.Wait()

	a.pool.Close()

	if serveFailure != nil {
		return fmt.Errorf("http: %w", serveFailure)
	}
	return nil
}

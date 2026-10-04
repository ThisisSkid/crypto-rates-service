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
	app, err := newApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(app); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newApp - Composition Root: собирает все зависимости и компоненты.
// Здесь происходит вся инициализация: конфиг, логгер, пул БД, сервис, HTTP-сервер.
func newApp() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("конфиг: %w", err)
	}

	log, appLog, closeLog, err := logger.New(cfg.LogLevel, cfg.LogFile)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.PostgresURL)
	if err != nil {
		closeLog()
		return nil, fmt.Errorf("не удалось открыть postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		closeLog()
		return nil, fmt.Errorf("postgres не отвечает: %w", err)
	}

	svc := usecase.NewService(
		postgres.NewPostgresRepo(pool),
		bybit.NewBybitClient(cfg.BybitURL),
		appLog,
	)

	if err := svc.Refresh(ctx); err != nil {
		pool.Close()
		closeLog()
		return nil, fmt.Errorf("первое обновление: %w", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.New(svc),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Info("сервис собран", zap.String("http", cfg.HTTPAddr), zap.String("лог", cfg.LogFile))

	return &App{
		log:        log,
		pool:       pool,
		svc:        svc,
		httpServer: httpServer,
		botToken:   cfg.TelegramBotToken,
		updateInt:  cfg.UpdateInterval,
	}, nil
}

// run - только запуск компонентов и graceful stop по сигналу.
// Инициализации нет: всё собрано в newApp.
func run(app *App) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			app.log.Info("получен сигнал, останавливаемся")
			cancel()
		case <-ctx.Done():
		}
	}()

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		app.svc.Start(ctx, app.updateInt)
	}()

	waitBot := telegram.Start(ctx, app.svc, app.botToken)

	serveErr := make(chan error, 1)
	go func() {
		if err := app.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	var serveFailure error
	select {
	case <-ctx.Done():
	case serveFailure = <-serveErr:
		app.log.Error("http упал", zap.Error(serveFailure))
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := app.httpServer.Shutdown(shutdownCtx); err != nil {
		app.log.Error("http shutdown", zap.Error(err))
	}

	waitBot()
	workers.Wait()

	app.pool.Close()

	if serveFailure != nil {
		return fmt.Errorf("http: %w", serveFailure)
	}
	return nil
}

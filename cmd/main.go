package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astroapi/config"
	"astroapi/internal/database"
	"astroapi/internal/handlers"

	"astroapi/internal/logger"
	astromidware "astroapi/internal/middleware"

	natsadapter "astroapi/internal/nats"
	natsinfra "astroapi/internal/repositories/nats"
	"astroapi/internal/rules"
	"astroapi/internal/usecases"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

func main() {
	// Загружаем переменные из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	cfg := config.Load()

	if cfg.AdminToken == "" {
		log.Println("Warning: ADMIN_TOKEN is not set, admin endpoints will reject all requests")
	}

	// Инициализируем логгер
	logger, err := logger.NewLogger(cfg.LogServiceName, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// Инициализируем NATS
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := []nats.Option{
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Info("Disconnected from NATS", zap.Error(err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("Reconnected to NATS", zap.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
	}
	natsurl := fmt.Sprintf("nats://%s:%s", cfg.NATSHost, cfg.NATSPort)
	nc, err := nats.Connect(natsurl, opts...)
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer func() {
		err := nc.Drain()
		if err != nil {
			logger.Error("Failed to drain NATS connection", zap.Error(err))
		}
		nc.Close()
	}()

	js, err := jetstream.New(nc)
	if err != nil {
		logger.Fatal("Failed to create JetStream context", zap.Error(err))
	}

	// Dependency injection
	streamRepo := natsinfra.NewJetStreamRepository(js)
	streamUC := usecases.NewStreamUseCase(streamRepo)
	streamManager := natsadapter.NewStreamManager(streamUC, logger)

	// Инициализируем стримы
	jsctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := streamManager.Initialize(jsctx); err != nil {
		logger.Fatal("Failed to initialize streams", zap.Error(err))
	}

	// Инициализируем базу данных
	if err := database.InitDB(cfg); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	defer func() {
		if err := database.DB.Close(); err != nil {
			logger.Error("Error closing database connection", zap.Error(err))
		}
	}()

	rulesRepository := rules.NewPostgresRepository(database.DB.DB)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepository)

	// Настраиваем маршруты
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(astromidware.RequestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Get("/api/v1/", handlers.HelloWorldHandler)
	handlers.RegisterAdminRulesRoutes(r, cfg.AdminToken, adminRulesHandler)

	// Создаем HTTP сервер с таймаутами
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Запускаем сервер в горутине
	go func() {
		logger.Info("App starting on port 8080")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Ожидаем сигнал для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	downctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	if err := srv.Shutdown(downctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Fatal("Server forced close error", zap.Error(closeErr))
		}
	}

	logger.Info("Server exited")

}

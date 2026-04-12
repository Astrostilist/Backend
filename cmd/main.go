package main

import (
	"context"
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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"astroapi/internal/rules"

	"github.com/joho/godotenv"
)

func main() {
	// Загружаем переменные из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	cfg := config.Load()

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
	if cfg.AdminToken == "" {
		log.Println("Warning: ADMIN_TOKEN is not set, admin endpoints will reject all requests")
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

	// Настраиваем маршруты
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(astromidware.RequestLogger(logger))
	r.Use(middleware.Recoverer)

	r.Get("/api/v1/", handlers.HelloWorldHandler)

	// Создаем HTTP сервер с таймаутами
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	rulesRepository := rules.NewPostgresRepository(database.DB.DB)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepository)

	// Настраиваем маршруты
	router := chi.NewRouter()
	router.Get("/api/v1/", handlers.HelloWorldHandler)
	handlers.RegisterAdminRulesRoutes(router, cfg.AdminToken, adminRulesHandler)

	// Создаем HTTP сервер с таймаутами
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Fatal("Server forced close error", zap.Error(closeErr))
		}
	}

	logger.Info("Server exited")
}

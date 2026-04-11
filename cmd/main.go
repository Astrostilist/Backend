package main

import (
	"astroapi/internal/messaging"
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
	"astroapi/internal/rules"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
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

	// Инициализируем базу данных
	if err := database.InitDB(cfg); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	nc, err := messaging.InitNATS(cfg)
	if err != nil {
		log.Fatal("Failed to initialize NATS:", err)
	}
	defer nc.Close()

	defer func() {
		if err := database.DB.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	// Настраиваем маршруты
	http.HandleFunc("/api/v1/", handlers.HelloWorldHandler)
	http.HandleFunc("/api/v1/astro/profile", handlers.ProfileHandler)
	http.HandleFunc("/api/v1/astro/recommend", handlers.RecommendHandler)

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
		log.Printf("App starting on port 8080")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// Ожидаем сигнал для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")

}

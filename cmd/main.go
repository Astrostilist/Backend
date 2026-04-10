package main

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astroapi/config"
	"astroapi/internal/database"
	"astroapi/internal/handlers"

	"github.com/joho/godotenv"
)

func main() {
	// Загружаем переменные из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	//Паникуем при отсутвие ключа
	encodedKey := os.Getenv("ENCRYPTION_KEY")
	if encodedKey == "" {
		log.Fatal("ENCRYPTION_KEY is not set")
	}
	//Декодируем из base64
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		log.Fatal("Invalid base64 key:", err)
	}
	_ = key

	cfg := config.Load()

	// Инициализируем базу данных
	if err := database.InitDB(cfg); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	defer func() {
		if err := database.DB.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	// Настраиваем маршруты
	http.HandleFunc("/api/v1/", handlers.HelloWorldHandler)

	// Создаем HTTP сервер с таймаутами
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      nil,
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

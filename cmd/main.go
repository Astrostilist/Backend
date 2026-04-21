package main

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"astroapi/config"
	"astroapi/internal/alisa"
	"astroapi/internal/database"
	"astroapi/internal/handlers"
	natsinfra "astroapi/internal/infrastructure/nats"
	"astroapi/internal/logger"
	"astroapi/internal/messaging"
	astromidware "astroapi/internal/middleware"
	"astroapi/internal/models"

	rules "astroapi/internal/ruleengine"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	// Загружаем переменные из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	}

	// Паникуем при отсутствии ключа
	encodedKey := os.Getenv("ENCRYPTION_KEY")
	if encodedKey == "" {
		log.Fatal("ENCRYPTION_KEY is not set")
	}

	// Декодируем из base64
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		log.Fatal("Invalid base64 key:", err)
	}
	_ = key

	cfg := config.Load()

	if cfg.AdminToken == "" {
		log.Println("Warning: ADMIN_TOKEN is not set, admin endpoints will reject all requests")
	}

	// Инициализируем логгер
	appLogger, err := logger.NewLogger(cfg.LogServiceName, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := appLogger.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// Инициализируем NATS
	nc, err := natsinfra.InitNATS(ctx, appLogger, cfg)
	if err != nil {
		appLogger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer nc.DrainNATS()

	js, err := jetstream.New(nc.Conn)
	if err != nil {
		appLogger.Fatal("Failed to create JetStream context", zap.Error(err))
	}

	// Делаем JetStream доступным для HTTP handlers (/astro/profile и /astro/recommend)
	messaging.JS = js

	// Клиент Astro API для worker flow
	astroClient := alisa.NewAstroAPIClientFromConfig(cfg, js, appLogger)

	// Клиент AlisaAI для recommendation flow
	alisaClient := alisa.NewClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModelURL)

	// Конфигурируем message handlers с зависимостями
	handlers.ConfigureMessageHandlers(astroClient, alisaClient, nil, appLogger)

	jsadapter := natsinfra.NewJetStreamRepository(js, appLogger)

	// Инициализируем стримы
	jsctx, jscancel := context.WithTimeout(ctx, 10*time.Second)
	defer jscancel()

	if err := jsadapter.InitializeStreams(jsctx); err != nil {
		appLogger.Fatal("Failed to initialize streams", zap.Error(err))
	}

	// Инициализируем базу данных
	if err := database.InitDB(cfg); err != nil {
		appLogger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer func() {
		if err := database.DB.Close(); err != nil {
			appLogger.Error("Error closing database connection", zap.Error(err))
		}
	}()

	// Адаптер для публикации исходящих сообщений в NATS
	_ = natsinfra.NewMessagePublisher(jsadapter, appLogger)

	// Роутер обработки входящих сообщений
	msgRouter := handlers.NewMsgRouter(appLogger)
	msgRouter.Register(models.MsgRecommendSubj, handlers.HandlerFunc(handlers.HandleRecommend))
	msgRouter.Register(models.MsgProfileSubj, handlers.HandlerFunc(handlers.HandleProfile))

	consumer := natsinfra.NewMessageConsumer(jsadapter, appLogger)

	wg := sync.WaitGroup{}

	// Запускаем два воркера в отдельных горутинах
	wg.Go(func() {
		err := consumer.ConsumeWithHandler(
			jsctx,
			models.MsgStreamEvents,
			models.MsgProfileWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			},
		)
		if err != nil {
			appLogger.Error("Profile worker failed", zap.Error(err))
		}
	})

	wg.Go(func() {
		err := consumer.ConsumeWithHandler(
			jsctx,
			models.MsgStreamEvents,
			models.MsgRecommendWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			},
		)
		if err != nil {
			appLogger.Error("Recommend worker failed", zap.Error(err))
		}
	})

	rulesRepository := rules.NewPostgresRepository(database.DB.DB)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepository)

	helloService := &handlers.RealHelloService{}
	helloHandler := handlers.NewHelloHandler(helloService)

	// Настраиваем роутер chi и middleware
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(astromidware.RequestLogger(appLogger))
	r.Use(middleware.Recoverer)

	// Регистрируем ВСЕ маршруты
	r.Get("/api/v1/", helloHandler.HelloWorldHandler)
	r.Post("/api/v1/astro/profile", handlers.ProfileHandler)
	r.Post("/api/v1/astro/recommend", handlers.RecommendHandler)

	handlers.RegisterAdminRulesRoutes(r, cfg.AdminToken, adminRulesHandler)

	// Создаем HTTP сервер
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Запускаем сервер в горутине
	go func() {
		appLogger.Info("App starting on port 8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// Ожидаем сигнал для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	appLogger.Info("Shutting down server...")

	downctx, cancelDown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelDown()

	if err := srv.Shutdown(downctx); err != nil {
		appLogger.Fatal("Server forced to shutdown", zap.Error(err))
		if closeErr := srv.Close(); closeErr != nil {
			appLogger.Fatal("Server forced close error", zap.Error(closeErr))
		}
	}

	jscancel()
	wg.Wait()

	appLogger.Info("Server exited")
}

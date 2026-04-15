package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"astroapi/config"
	"astroapi/internal/database"
	"astroapi/internal/handlers"
	"astroapi/internal/models"

	natsinfra "astroapi/internal/infrastructure/nats"

	"astroapi/internal/logger"
	astromidware "astroapi/internal/middleware"

	"astroapi/internal/rules"

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
	nc, err := natsinfra.InitNATS(ctx, logger, cfg)
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))

	}
	defer nc.DrainNATS()

	js, err := jetstream.New(nc.Conn)
	if err != nil {
		logger.Fatal("Failed to create JetStream context", zap.Error(err))
	}

	jsadapter := natsinfra.NewJetStreamRepository(js, logger)

	// Инициализируем стримы
	jsctx, jscancel := context.WithTimeout(ctx, 10*time.Second)
	defer jscancel()
	if err := jsadapter.InitializeStreams(jsctx); err != nil {
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

	// Адаптер для публикации исходящих сообщений в NATS
	// TODO: подключить в соответствующие API-endpoint хэндлеры
	_ = natsinfra.NewMessagePublisher(jsadapter, logger)

	// Роутер обработки входящих сообщений
	msgRouter := handlers.NewMsgRouter(logger)
	msgRouter.Register(models.MsgRecommendSubj, handlers.HandlerFunc(handlers.HandleRecommend))
	msgRouter.Register(models.MsgProfileSubj, handlers.HandlerFunc(handlers.HandleProfile))

	consumer := natsinfra.NewMessageConsumer(jsadapter, logger)

	wg := sync.WaitGroup{}
	// Запускаем два воркера в отдельных горутинах
	wg.Go(func() {
		err := consumer.ConsumeWithHandler(jsctx,
			models.MsgStreamEvents,
			models.MsgProfileWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			})
		if err != nil {
			logger.Error("Profile worker failed", zap.String("error", err.Error()))
		}
	})

	wg.Go(func() {
		err := consumer.ConsumeWithHandler(jsctx,
			models.MsgStreamEvents,
			models.MsgRecommendWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			})
		if err != nil {
			logger.Error("Recommend worker failed", zap.String("error", err.Error()))
		}
	})

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

	if err := srv.Shutdown(downctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
		if closeErr := srv.Close(); closeErr != nil {
			logger.Fatal("Server forced close error", zap.Error(closeErr))
		}
	}

	jscancel() // стопаем консьюмеры
	wg.Wait()

	logger.Info("Server exited")
}

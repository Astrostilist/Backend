package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"astroapi/config"
	"astroapi/internal/admin"
	"astroapi/internal/adminlogs"
	"astroapi/internal/alisa"
	"astroapi/internal/database"
	"astroapi/internal/handlers"
	infra "astroapi/internal/infrastructure"
	health "astroapi/internal/infrastructure/health"
	natsinfra "astroapi/internal/infrastructure/nats"
	"astroapi/internal/logger"
	"astroapi/internal/metrics"
	astromidware "astroapi/internal/middleware"
	"astroapi/internal/models"
	"astroapi/internal/products"
	feedbackrepo "astroapi/internal/repositories"
	"astroapi/internal/requests"
	rules "astroapi/internal/ruleengine"
	"astroapi/internal/usecases"
	repositories "astroapi/internal/usecases/repositories"
	"astroapi/internal/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	httpReadTimeout  = 15 * time.Second
	httpWriteTimeout = 15 * time.Second
	httpIdleTimeout  = 60 * time.Second
	initTimeout      = 15 * time.Second
	shutdownTimeout  = 30 * time.Second
	cacheTTL         = 5 * time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Загрузка конфигурации
	cfg := config.Load()

	encryptionKey, err := decodeEncryptionKey(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	// 2. Инициализация логгера
	zapLogger, err := logger.NewLogger(cfg.LogServiceName, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() { _ = zapLogger.Sync() }()

	// 3. Метрики
	metrics.Initialize(cfg)
	metricsReporter := metrics.CircuitBreakerReporter{}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// 4. База данных
	db, err := database.New(rootCtx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			zapLogger.Error("failed to close database", zap.Error(closeErr))
		}
	}()

	// 5. NATS JetStream
	natsConn, err := natsinfra.InitNATS(rootCtx, zapLogger, cfg)
	if err != nil {
		return err
	}
	defer natsConn.DrainNATS()

	js, err := jetstream.New(natsConn.Conn)
	if err != nil {
		return err
	}

	jsAdapter := natsinfra.NewJetStreamRepository(js, zapLogger)
	initCtx, initCancel := context.WithTimeout(rootCtx, initTimeout)
	if err := jsAdapter.InitializeStreams(initCtx); err != nil {
		initCancel()
		return err
	}
	initCancel()

	publisher := natsinfra.NewMessagePublisher(jsAdapter)

	// 6. Инициализация репозиториев
	userRepo := user.NewPostgresRepository(db.DB, encryptionKey)
	requestsRepo := requests.NewPostgresRepository(db.DB)
	adminLogsRepo := adminlogs.NewPostgresRepository(db.DB)
	rulesRepo := rules.NewPostgresRepository(db.DB, zapLogger)
	productsRepo := products.NewPostgresRepository(db.DB)
	adminRepo := admin.NewPostgresRepository(db.DB)

	dbRepo := repositories.NewDBPersonalDataRepository(db.DB, encryptionKey)
	cacheRepo := repositories.NewCacheRepo(cacheTTL, []string{cfg.MemcachedHost})
	personalDataUC := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	healthRepo := health.NewHealthServiceRepo(db, natsConn)
	monitor := infra.NewMonitorService(jsAdapter, healthRepo, zapLogger)

	// 7. Внешние клиенты
	aiClient := alisa.NewClientWithOptions(
		cfg.AIBaseURL,
		cfg.AIAPIKey,
		cfg.AIModelURL,
		alisa.ClientOptions{
			Logger:     zapLogger,
			Metrics:    metricsReporter,
			MaxRetries: 3,
		},
	)

	// 8. Фоновые воркеры (JetStream)
	profileProcessor := handlers.NewProfileProcessor(userRepo, requestsRepo, zapLogger)
	recommendProcessor := handlers.NewRecommendProcessor(userRepo, requestsRepo, rulesRepo, aiClient, zapLogger)

	msgRouter := handlers.NewMsgRouter(zapLogger)
	msgRouter.Register(models.MsgProfileSubj, profileProcessor)
	msgRouter.Register(models.MsgRecommendSubj, recommendProcessor)

	consumer := natsinfra.NewMessageConsumer(jsAdapter)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if consumeErr := consumer.ConsumeWithHandler(
			rootCtx,
			models.MsgStreamEvents,
			models.MsgProfileWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			},
		); consumeErr != nil {
			zapLogger.Error("profile worker failed", zap.Error(consumeErr))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := consumer.ConsumeWithHandler(rootCtx, models.MsgStreamEvents, models.MsgRecommendWrk, func(ctx context.Context, msg jetstream.Msg) error {
			return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
		}); err != nil && !errors.Is(err, context.Canceled) {
			zapLogger.Error("recommend worker failed", zap.Error(err))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		monitor.StartInfraMonitor(rootCtx)
	}()

	// 9. HTTP Хендлеры
	helloHandler := handlers.NewHelloHandler(handlers.NewRealHelloService(db))
	healthHandler := handlers.NewHealthHandler(healthRepo)
	profileHandler := handlers.NewProfileHandler(publisher, requestsRepo, personalDataUC, zapLogger)
	recommendHandler := handlers.NewRecommendHandler(publisher, userRepo, rulesRepo, aiClient, requestsRepo, zapLogger)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepo)
	adminProductsHandler := handlers.NewAdminProductsHandler(productsRepo, nil)
	adminLogsHandler := handlers.NewAdminLogsHandler(adminLogsRepo)
	authHandler := handlers.NewAuthHandler(adminRepo, cfg.AdminToken)

	// Передача обеих зависимостей для задачи #77
	feedbackHandler := handlers.NewFeedbackHandler(feedbackrepo.NewFeedbackRepository(db.DB), requestsRepo)

	dlqReader := natsinfra.NewDLQReader(jsAdapter, zapLogger)
	dlqViewerHandler := handlers.NewDLQViewerHandler(dlqReader, zapLogger)

	// 10. Роутинг
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(astromidware.RequestMetricsMiddleware())
	r.Use(astromidware.RequestLogger(zapLogger))
	r.Use(middleware.Recoverer)

	r.Get("/api/v1/", helloHandler.HelloWorldHandler)

	r.Group(func(r chi.Router) {
		r.Use(handlers.ClientAuthMiddleware(cfg.BotAPIKey))

		// Обновленные пути (без /astro)
		r.Post("/api/v1/profile", profileHandler.HandleProfile)
		r.Post("/api/v1/recommend", recommendHandler.Handle)
		r.Post("/api/v1/feedback", feedbackHandler.CreateFeedback)
	})

	r.Post("/api/v1/auth/login", authHandler.Login)
	r.Post("/api/v1/admin/catalog/import", handlers.NewImportHandler(db))

	handlers.RegisterAdminRulesRoutes(r, cfg.AdminToken, adminRulesHandler)
	handlers.RegisterAdminProductsRoutes(r, cfg.AdminToken, adminProductsHandler)
	handlers.RegisterAdminLogsRoutes(r, cfg.AdminToken, adminLogsHandler)

	r.Group(func(r chi.Router) {
		r.Use(handlers.AdminAuthMiddleware(cfg.AdminToken))
		r.Get("/api/v1/admin/dlq", dlqViewerHandler.ListMessages)
	})

	r.Handle("/metrics", metrics.NewHandler())
	r.Get("/api/v1/health", healthHandler.HandleHealth)

	// 11. Запуск сервера
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		zapLogger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 12. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		zapLogger.Info("shutdown signal received")
	case err := <-serverErr:
		return err
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zapLogger.Error("server shutdown error", zap.Error(err))
	}

	rootCancel() // Останавливаем воркеры

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		zapLogger.Info("all workers stopped successfully")
	case <-time.After(15 * time.Second):
		zapLogger.Warn("shutdown timed out for workers")
	}

	zapLogger.Info("application exited")
	return nil
}

func decodeEncryptionKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("ENCRYPTION_KEY is not set")
	}

	clean := strings.TrimSpace(encoded)
	key, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(clean)
	}

	if err != nil {
		return nil, errors.New("invalid base64 for ENCRYPTION_KEY")
	}

	if len(key) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must decode to 32 bytes for AES-256")
	}

	return key, nil
}

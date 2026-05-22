package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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
	"astroapi/internal/metrics"
	astromidware "astroapi/internal/middleware"
	"astroapi/internal/models"
	"astroapi/internal/products"
	repositories "astroapi/internal/repositories"
	"astroapi/internal/requests"
	rules "astroapi/internal/ruleengine"
	"astroapi/internal/usecases"
	"astroapi/internal/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go/jetstream"

	astrologger "astroapi/internal/infrastructure/logger"
	tracer "astroapi/internal/infrastructure/tracer"

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
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	encryptionKey, err := decodeEncryptionKey(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	zapLogger, err := astrologger.NewLogger(cfg.LogServiceName, cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}

	defer func() {
		if syncErr := zapLogger.Sync(); syncErr != nil {
			log.Printf("failed to sync logger: %v", syncErr)
		}
	}()

	// Переходим на глобальный логгер
	astrologger.SetGlobalLogger(zapLogger)

	if cfg.AdminToken == "" {
		astrologger.Warn(context.Background(), "config validation: ADMIN_TOKEN is not set, admin endpoints will reject all requests")
	}
	if cfg.BotAPIKey == "" {
		astrologger.Warn(context.Background(), "config validation: BOT_API_KEY is not set, client endpoints will reject all requests")
	}

	metrics.Initialize(cfg)
	metricsReporter := metrics.CircuitBreakerReporter{}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	err = tracer.InitTracer(rootCtx, cfg)
	if err != nil {
		return fmt.Errorf("failed to init tracer: %w", err)
	}

	db, err := database.New(rootCtx, cfg)
	if err != nil {
		return fmt.Errorf("failed to init db: %w", err)
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			astrologger.Error(context.Background(), "failed to close database", zap.Error(closeErr))
		}
	}()

	natsConn, err := natsinfra.InitNATS(rootCtx, zapLogger, cfg)
	if err != nil {
		return fmt.Errorf("failed to init nats: %w", err)
	}

	defer natsConn.DrainNATS()

	js, err := jetstream.New(natsConn.Conn)
	if err != nil {
		return fmt.Errorf("failed to init nats jet stream: %w", err)
	}

	jsAdapter := natsinfra.NewJetStreamRepository(js, zapLogger)

	initCtx, initCancel := context.WithTimeout(rootCtx, initTimeout)
	defer initCancel()
	if err := jsAdapter.InitializeStreams(initCtx); err != nil {
		return fmt.Errorf("failed to init streams: %w", err)
	}

	publisher := natsinfra.NewMessagePublisher(jsAdapter)

	userRepo := user.NewPostgresRepository(db.DB, encryptionKey)
	requestsRepo := requests.NewPostgresRepository(db.DB)
	adminLogsRepo := adminlogs.NewPostgresRepository(db.DB)
	rulesRepo := rules.NewPostgresRepository(db.DB, zapLogger)
	productsRepo := products.NewPostgresRepository(db.DB)
	adminRepo := admin.NewPostgresRepository(db.DB)

	dbRepo := repositories.NewDBPersonalDataRepository(db.DB, encryptionKey)
	cacheRepo := repositories.NewCacheRepo(cacheTTL, []string{cfg.MemcachedHost})
	personalDataUC := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	userRepositoryDelete := repositories.NewUserRepository(db.DB)
	h := &handlers.Handler{
		Repo:   userRepositoryDelete,
		Logger: zapLogger,
	}

	healthRepo := health.NewHealthServiceRepo(db, natsConn)
	monitor := infra.NewMonitorService(jsAdapter, healthRepo, zapLogger)

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

	astroClient := alisa.NewAstroAPIClientFromConfig(cfg, js, zapLogger, metricsReporter)

	profileProcessor := handlers.NewProfileProcessor(userRepo, requestsRepo, astroClient, zapLogger)
	recommendProcessor := handlers.NewRecommendProcessor(userRepo, requestsRepo, rulesRepo, aiClient, astroClient, zapLogger)

	msgRouter := handlers.NewMsgRouter(zapLogger)
	msgRouter.Register(models.MsgProfileSubj, profileProcessor)
	msgRouter.Register(models.MsgRecommendSubj, recommendProcessor)

	consumer := natsinfra.NewMessageConsumer(jsAdapter)

	var wg sync.WaitGroup

	wg.Go(func() {
		if consumeErr := consumer.ConsumeWithHandler(
			rootCtx,
			models.MsgStreamEvents,
			models.MsgProfileWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			},
		); consumeErr != nil {
			astrologger.Error(rootCtx, "profile worker failed", zap.Error(consumeErr))
		}
		<-rootCtx.Done()
	})

	wg.Go(func() {
		if consumeErr := consumer.ConsumeWithHandler(
			rootCtx,
			models.MsgStreamEvents,
			models.MsgRecommendWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			},
		); consumeErr != nil {
			astrologger.Error(rootCtx, "recommend worker failed", zap.Error(consumeErr))
		}

		<-rootCtx.Done()
	})

	wg.Go(func() {
		monitor.StartInfraMonitor(rootCtx)
		<-rootCtx.Done()
	})

	helloHandler := handlers.NewHelloHandler(handlers.NewRealHelloService(db))
	healthHandler := handlers.NewHealthHandler(healthRepo)
	profileHandler := handlers.NewProfileHandler(publisher, requestsRepo, personalDataUC, zapLogger)
	recommendHandler := handlers.NewRecommendHandler(publisher, userRepo, rulesRepo, aiClient, astroClient, requestsRepo, zapLogger)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepo)
	adminProductsHandler := handlers.NewAdminProductsHandler(productsRepo, nil, zapLogger)
	adminLogsHandler := handlers.NewAdminLogsHandler(adminLogsRepo)
	authHandler := handlers.NewAuthHandler(adminRepo, cfg.AdminToken)
	feedbackHandler := handlers.NewFeedbackHandler(repositories.NewFeedbackRepository(db.DB))

	dlqReader := natsinfra.NewDLQReader(jsAdapter, zapLogger)
	dlqViewerHandler := handlers.NewDLQViewerHandler(dlqReader, zapLogger)

	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequestID)
		r.Use(astromidware.TracingMiddleware)
		r.Use(astromidware.RequestLogger)
		r.Use(astromidware.RequestMetricsMiddleware)
		r.Use(middleware.Recoverer)

		r.Get("/api/v1/", helloHandler.HelloWorldHandler)

		r.Group(func(r chi.Router) {
			r.Use(handlers.ClientAuthMiddleware(cfg.BotAPIKey))
			r.Post("/api/v1/astro/profile", profileHandler.HandleProfile)
			r.Post("/api/v1/astro/recommend", recommendHandler.Handle)
			r.Post("/api/v1/feedback", feedbackHandler.CreateFeedback)
			r.Post("/api/v1/astro/feedback", feedbackHandler.CreateFeedback)
			r.Delete("/api/v1/user/{user_id}", h.OblivionHandler)
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
	})

	r.Handle("/metrics", metrics.NewHandler())
	r.Get("/api/v1/health", healthHandler.HandleHealth)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

	serverErr := make(chan error, 1)

	go func() {
		astrologger.Info(rootCtx, "app starting", zap.String("addr", srv.Addr))
		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			serverErr <- listenErr
		}
		close(serverErr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		astrologger.Info(rootCtx, "shutdown signal received")
	case serverRunErr := <-serverErr:
		if serverRunErr != nil {
			return serverRunErr
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	defer func() {
		if err := tracer.Shutdown(shutdownCtx); err != nil {
			astrologger.Error(shutdownCtx, "error shutting down tracer provider: %w", zap.Error(err))
		}
	}()

	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		astrologger.Error(shutdownCtx, "server shutdown error", zap.Error(shutdownErr))
	}

	rootCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		astrologger.Info(context.Background(), "all workers stopped")
	case <-time.After(15 * time.Second):
		astrologger.Warn(context.Background(), "timeout waiting workers shutdown")
	}
	astrologger.Info(context.Background(), "server exited")

	return nil
}

func decodeEncryptionKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("ENCRYPTION_KEY is not set")
	}

	key, err := func() ([]byte, error) {
		clean := strings.TrimSpace(encoded)

		decoded, decodeErr := base64.StdEncoding.DecodeString(clean)
		if decodeErr == nil {
			return decoded, nil
		}

		return base64.RawStdEncoding.DecodeString(clean)
	}()
	if err != nil {
		return nil, errors.New("ENCRYPTION_KEY must be valid base64")
	}

	if len(key) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must decode to 32 bytes")
	}

	return key, nil
}

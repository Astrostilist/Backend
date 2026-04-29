package main

import (
	"context"
	"encoding/base64"
	"errors"
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
	astromidware "astroapi/internal/middleware"
	"astroapi/internal/models"
	"astroapi/internal/products"
	"astroapi/internal/requests"
	rules "astroapi/internal/ruleengine"
	"astroapi/internal/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go/jetstream"
  "github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

const (
	httpReadTimeout  = 15 * time.Second
	httpWriteTimeout = 15 * time.Second
	httpIdleTimeout  = 60 * time.Second
	initTimeout      = 15 * time.Second
	shutdownTimeout  = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg := config.Load()

	encryptionKey, err := decodeEncryptionKey(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	if cfg.AdminToken == "" {
		log.Println("warning: ADMIN_TOKEN is not set, admin endpoints will reject all requests")
	}

	zapLogger, err := logger.NewLogger(cfg.LogServiceName, cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() {
		if syncErr := zapLogger.Sync(); syncErr != nil {
			log.Printf("failed to sync logger: %v", syncErr)
		}
	}()

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	db, err := database.New(rootCtx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			zapLogger.Error("failed to close database", zap.Error(closeErr))
		}
	}()

	runMigrations(db, zapLogger)

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

	publisher := natsinfra.NewMessagePublisher(jsAdapter, zapLogger)

	userRepo := user.NewPostgresRepository(db.DB, encryptionKey)
	requestsRepo := requests.NewPostgresRepository(db.DB)
	rulesRepo := rules.NewPostgresRepository(db.DB)
	productsRepo := products.NewPostgresRepository(db.DB)

	aiClient := alisa.NewClient(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModelURL)

	profileProcessor := handlers.NewProfileProcessor(userRepo, requestsRepo, zapLogger)
	recommendProcessor := handlers.NewRecommendProcessor(userRepo, requestsRepo, rulesRepo, aiClient, zapLogger)

	msgRouter := handlers.NewMsgRouter(zapLogger)
	msgRouter.Register(models.MsgProfileSubj, profileProcessor)
	msgRouter.Register(models.MsgRecommendSubj, recommendProcessor)

	consumer := natsinfra.NewMessageConsumer(jsAdapter, zapLogger)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := consumer.ConsumeWithHandler(rootCtx, models.MsgStreamEvents, models.MsgProfileWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			}); err != nil {
			zapLogger.Error("profile worker failed", zap.Error(err))
		}
		<-rootCtx.Done()
	}()
	go func() {
		defer wg.Done()
		if err := consumer.ConsumeWithHandler(rootCtx, models.MsgStreamEvents, models.MsgRecommendWrk,
			func(ctx context.Context, msg jetstream.Msg) error {
				return msgRouter.Dispatch(ctx, msg.Subject(), msg.Data())
			}); err != nil {
			zapLogger.Error("recommend worker failed", zap.Error(err))
		}
		<-rootCtx.Done()
	}()

	// --- HTTP Handlers ---
	helloHandler := handlers.NewHelloHandler(handlers.NewRealHelloService(db))

	// Здесь теперь передаются все 3 нужных аргумента
	profileHandler := handlers.NewProfileHandler(publisher, requestsRepo, zapLogger)
	recommendHandler := handlers.NewRecommendHandler(publisher, userRepo, rulesRepo, aiClient, requestsRepo, zapLogger)
	adminRulesHandler := handlers.NewAdminRulesHandler(rulesRepo)
	adminProductsHandler := handlers.NewAdminProductsHandler(productsRepo, nil)

	dlqreader := natsinfra.NewDLQReader(jsAdapter, zapLogger)
	dlqViewerHandler := handlers.NewDLQViewerHandler(dlqreader, zapLogger)

	// Настраиваем роутер chi и middleware (единый блок)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(astromidware.RequestLogger(zapLogger))
	r.Use(middleware.Recoverer)


	r.Get("/api/v1/", helloHandler.HelloWorldHandler)
	r.Get("/api/v1/products", handlers.SearchProductsHandler(database.DB))
	r.Post("/api/v1/astro/profile", handlers.ProfileHandler)
	r.Post("/api/v1/astro/recommend", handlers.RecommendHandler)
  r.Post("/api/v1/admin/catalog/import", handlers.ImportCatalogHandler)

	handlers.RegisterAdminRulesRoutes(r, cfg.AdminToken, adminRulesHandler)
	handlers.RegisterAdminProductsRoutes(r, cfg.AdminToken, adminProductsHandler)

	r.Group(func(r chi.Router) {
		r.Use(handlers.AdminAuthMiddleware(cfg.AdminToken))
		r.Get("/api/v1/admin/dlq", dlqViewerHandler.ListMessages)
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  httpReadTimeout,
		WriteTimeout: httpWriteTimeout,
		IdleTimeout:  httpIdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		zapLogger.Info("app starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		zapLogger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zapLogger.Error("server shutdown error", zap.Error(err))
	}
	rootCancel()
	wg.Wait()

	zapLogger.Info("server exited")
	return nil
}

func runMigrations(db *database.PostgresDB, zapLogger *zap.Logger) {
    if err := goose.SetDialect("postgres"); err != nil {
        log.Fatal("Failed to set goose dialect:", err)
    }
    if err := goose.Up(db.DB, "./migrations"); err != nil {
        log.Fatal("Failed to apply migrations:", err)
    }
    zapLogger.Info("Migrations applied")
}

func decodeEncryptionKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("ENCRYPTION_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("ENCRYPTION_KEY must be valid base64")
	}
	if len(key) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must decode to 32 bytes")
	}
	return key, nil
}

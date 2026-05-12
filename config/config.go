package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// App
	LogServiceName string
	LogLevel       string
	Environment    string
	AdminToken     string
	BotAPIKey      string
	EncryptionKey  string

	MemcachedHost string

	JaegerEndpoint    string
	JaegerServiceName string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Pool settings
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration

	// NATS
	NATSHost      string
	NATSPort      string
	NATSClusterID string
	NATSClientID  string

	// AI
	AIBaseURL   string
	AIAPIKey    string
	AIModelURL  string
	AstroAPIURL string
}

// Load читает переменные окружения. .env-файл подхватывается один раз здесь,
// если он есть; в продакшене переменные приходят из окружения/секретов.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env file not found, using environment variables")
	}

	return &Config{
		LogServiceName: getEnv("LOG_SERVICE_NAME", "astro-backend"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		Environment:    getEnv("ENVIRONMENT", "dev"),

		JaegerEndpoint:    getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
		JaegerServiceName: getEnv("JAEGER_SERVICE_NAME", "astro-backend"),

		AdminToken:    getEnv("ADMIN_TOKEN", ""),
		BotAPIKey:     getEnv("BOT_API_KEY", ""),
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),

		MemcachedHost: getEnv("MEMCACHED_HOST", "localhost:11211"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "astrobackend"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),

		DBMaxConns:          getEnvAsInt32("DB_MAX_CONNS", 50),
		DBMinConns:          getEnvAsInt32("DB_MIN_CONNS", 20),
		DBMaxConnLifetime:   getEnvAsDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:   getEnvAsDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBHealthCheckPeriod: getEnvAsDuration("DB_HEALTH_CHECK_PERIOD", time.Minute),

		NATSHost:      getEnv("NATS_HOST", "localhost"),
		NATSPort:      getEnv("NATS_PORT", "4222"),
		NATSClusterID: getEnv("NATS_CLUSTER_ID", "test-cluster"),
		NATSClientID:  getEnv("NATS_CLIENT_ID", "astro-backend"),

		AIBaseURL:   getEnv("AI_BASE_URL", "https://ai.api.cloud.yandex.net/v1"),
		AIAPIKey:    getEnv("AI_API_KEY", ""),
		AIModelURL:  getEnv("AI_MODEL_URL", ""),
		AstroAPIURL: getEnv("ASTRO_API_URL", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt32(key string, defaultValue int32) int32 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 32); err == nil {
			return int32(intVal)
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// App
	LogServiceName   string
	LogLevel         string
	Environment      string
	SecretTokenAdmin string
	BotAPIKey        string
	EncryptionKey    string

	MemcachedHost string

	// Tracing
	JaegerEndpoint     string
	JaegerServiceName  string
	JaegerOtelEndpoint string
	JaegerInsecure     bool
	JaegerSamplingRate float64
	JaegerSendTimeout  time.Duration

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
	AIBaseURL     string
	AIAPIKey      string
	AIModelURL    string
	AstroProvider string
	AstroAPIURL   string
	AstroAPIKey   string

	// CRM
	CRMBaseUrl string
	CRMApiKey  string
}

// Load читает переменные окружения. .env-файл подхватывается один раз здесь,
// если он есть; в продакшене переменные приходят из окружения/секретов.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env file not found, using environment variables")
	}

	return &Config{
		LogServiceName: getEnv("LOG_SERVICE_NAME", "astro-backend"),
		LogLevel:       getEnv("LOG_LEVEL", "INFO"),
		Environment:    getEnv("ENVIRONMENT", "dev"),

		JaegerEndpoint:     getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
		JaegerServiceName:  getEnv("JAEGER_SERVICE_NAME", "astro-backend"),
		JaegerOtelEndpoint: getEnv("JAEGER_OTEL_ENDPOINT", "jaeger:4318"),
		JaegerInsecure:     getEnvAsBool("JAEGER_INSECURE", false),
		JaegerSamplingRate: getEnvAsFloat64("JAEGER_SAMPLING_RATE", 0.1),
		JaegerSendTimeout:  getEnvAsDuration("JAEGER_SEND_TIMEOUT", 10*time.Second),

		SecretTokenAdmin: getEnv("SECRET_TOKEN_ADMIN", ""),
		BotAPIKey:        getEnv("BOT_API_KEY", ""),
		EncryptionKey:    getEnv("ENCRYPTION_KEY", ""),

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

		AIBaseURL:     getEnv("AI_BASE_URL", "https://ai.api.cloud.yandex.net/v1"),
		AIAPIKey:      getEnv("AI_API_KEY", ""),
		AIModelURL:    getEnv("AI_MODEL_URL", ""),
		AstroProvider: getEnv("ASTRO_PROVIDER", "external"),
		AstroAPIURL:   getEnv("ASTRO_API_URL", "https://api.freeastroapi.com"),
		AstroAPIKey:   getEnv("ASTRO_API_KEY", getEnv("FREE_ASTRO_API_KEY", "")),

		CRMBaseUrl: getEnv("CRM_BASE_URL", "https://iline-group.retailcrm.ru"),
		CRMApiKey:  getEnv("CRM_API_KEY", "Ixr2gNhj2Xlzb6HKp3nHvwYBbjsZb6t8"),
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

func getEnvAsFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if fVal, err := strconv.ParseFloat(value, 64); err == nil {
			return float64(fVal)
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

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		switch value {
		case "true":
			return true
		case "false":
			return false
		default:
			return defaultValue
		}
	}
	return defaultValue
}

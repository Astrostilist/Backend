package config

import (
	"log"
	"os"
	"time"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	JaegerEndpoint    string
	JaegerServiceName string

	// Database
    DBHost            string
    DBPort            string
    DBUser            string
    DBPassword        string
    DBName            string
    DBSSLMode         string

    // Pool settings
    DBMaxConns        int32
    DBMinConns        int32
    DBMaxConnLifetime time.Duration
    DBMaxConnIdleTime time.Duration
    DBHealthCheckPeriod time.Duration

    // NATS
    NATSHost          string
    NATSPort          string
    NATSClusterID     string
    NATSClientID      string
}

func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	return &Config{
        JaegerEndpoint:    getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
        JaegerServiceName: getEnv("JAEGER_SERVICE_NAME", "astro-backend"),

        DBHost:            getEnv("DB_HOST", "localhost"),
        DBPort:            getEnv("DB_PORT", "5432"),
        DBUser:            getEnv("DB_USER", "postgres"),
        DBPassword:        getEnv("DB_PASSWORD", ""),
        DBName:            getEnv("DB_NAME", "myapi_db"),
        DBSSLMode:         getEnv("DB_SSL_MODE", "disable"),

		DBMaxConns:          getEnvAsInt32("DB_MAX_CONNS", 50),
		DBMinConns:          getEnvAsInt32("DB_MIN_CONNS", 20),
		DBMaxConnLifetime:   getEnvAsDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:   getEnvAsDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBHealthCheckPeriod: getEnvAsDuration("DB_HEALTH_CHECK_PERIOD", time.Minute),

        NATSHost:          getEnv("NATS_HOST", "localhost"),
        NATSPort:          getEnv("NATS_PORT", "4222"),
        NATSClusterID:     getEnv("NATS_CLUSTER_ID", "test-cluster"),
        NATSClientID:      getEnv("NATS_CLIENT_ID", "astro-backend"),
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

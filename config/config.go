package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	MemcachedHost   string
	JaegerEndpoint  string
	JaegerServiceName string

	// Database
    DBHost            string
    DBPort            string
    DBUser            string
    DBPassword        string
    DBName            string
    DBSSLMode         string
    DBTimezone        string
}

func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	return &Config{
		MemcachedHost:   getEnv("MEMCACHED_HOST", "localhost:11211"),
		JaegerEndpoint:  getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
		JaegerServiceName: getEnv("JAEGER_SERVICE_NAME", "astro-backend"),
        DBHost:            getEnv("DB_HOST", "localhost"),
        DBPort:            getEnv("DB_PORT", "5432"),
        DBUser:            getEnv("DB_USER", "postgres"),
        DBPassword:        getEnv("DB_PASSWORD", ""),
        DBName:            getEnv("DB_NAME", "myapi_db"),
        DBSSLMode:         getEnv("DB_SSL_MODE", "disable"),
        DBTimezone:        getEnv("DB_TIMEZONE", "UTC"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

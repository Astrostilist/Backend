package messaging

import (
	"fmt"
	"log"

	"astroapi/config" // путь к пакету конфигурации

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JS — глобальная переменная для доступа к интерфейсу JetStream.
var JS jetstream.JetStream

// InitNATS устанавливает соединение с брокером NATS.
func InitNATS(cfg *config.Config) (*nats.Conn, error) {
	// Формируем URL из конфигурации
	url := fmt.Sprintf("nats://%s:%s", cfg.NATSHost, cfg.NATSPort)

	// Подключаемся к серверу NATS
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to init JetStream: %w", err)
	}

	JS = js
	log.Println("Successfully connected to NATS JetStream")

	return nc, nil
}

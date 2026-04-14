//go:build integration

package integration

import (
	"context"
	"testing"

	models "astroapi/internal/models"
	repo "astroapi/internal/repositories"
	natsinfra "astroapi/internal/repositories/nats"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// NatsRepoTestSuite объединяет тесты и предоставляет методы очистки
type NatsRepoTestSuite struct {
	suite.Suite
	repo repo.StreamRepository
	js   jetstream.JetStream
	nc   *nats.Conn
	ctx  context.Context
}

// SetupSuite выполняется один раз перед всеми тестами в сюите
func (s *NatsRepoTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Подключаемся к NATS, который уже запущен в TestMain
	var err error
	s.nc, err = nats.Connect(testEnv.NatsURL)
	require.NoError(s.T(), err)

	// Создаем JetStream контекст
	js, err := jetstream.New(s.nc)
	require.NoError(s.T(), err)
	s.js = js

	// Инициализируем репозиторий
	s.repo = natsinfra.NewJetStreamRepository(js)
}

// TearDownTest выполняется после КАЖДОГО теста для очистки данных
func (s *NatsRepoTestSuite) TearDownTest() {
	// Получаем список всех стримов и удаляем их, чтобы тесты были изолированы
	streams := s.js.StreamNames(s.ctx)
	for name := range streams.Name() {
		err := s.js.DeleteStream(s.ctx, name)
		if err != nil {
			s.T().Logf("Failed to delete stream %s: %v", name, err)
		}
	}
	if err := streams.Err(); err != nil {
		s.T().Logf("Error iterating streams: %v", err)
	}
}

// --- Тесты для Stream ---

func (s *NatsRepoTestSuite) TestCreateStream_Success() {
	streamCfg := &models.StreamCfg{
		Name:        "TEST_STREAM_CREATE",
		Subjects:    []string{"test.create.>"},
		Retention:   models.RetentionPolicy(0),
		Storage:     models.FileStorage,
		MaxMessages: 1000,
	}

	err := s.repo.CreateOrUpdateStream(s.ctx, streamCfg)
	require.NoError(s.T(), err)

	// Проверяем, что стрим действительно создан и конфиг совпадает
	stream, err := s.js.Stream(s.ctx, "TEST_STREAM_CREATE")
	require.NoError(s.T(), err)
	info, err := stream.Info(s.ctx)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "TEST_STREAM_CREATE", info.Config.Name)
	require.Equal(s.T(), int64(1000), info.Config.MaxMsgs)
}

func (s *NatsRepoTestSuite) TestUpdateStream_Exists() {
	// 1. Создаем стрим с одними параметрами
	initialCfg := &models.StreamCfg{
		Name:        "TEST_STREAM_UPDATE",
		Subjects:    []string{"test.update.>"},
		MaxMessages: 100,
		Storage:     models.FileStorage,
	}
	err := s.repo.CreateOrUpdateStream(s.ctx, initialCfg)
	require.NoError(s.T(), err)

	// 2. Обновляем параметры (например, увеличиваем лимит сообщений)
	updatedCfg := &models.StreamCfg{
		Name:        "TEST_STREAM_UPDATE",
		Subjects:    []string{"test.update.>"},
		MaxMessages: 5000,
		Storage:     models.FileStorage,
	}
	err = s.repo.CreateOrUpdateStream(s.ctx, updatedCfg)
	require.NoError(s.T(), err)

	// 3. Проверяем, что изменения применились
	stream, err := s.js.Stream(s.ctx, "TEST_STREAM_UPDATE")
	require.NoError(s.T(), err)
	info, err := stream.Info(s.ctx)
	require.Equal(s.T(), int64(5000), info.Config.MaxMsgs)
	require.Len(s.T(), info.Config.Subjects, 1)
}

func (s *NatsRepoTestSuite) TestGetStream_Success() {
	// Подготовка
	cfg := &models.StreamCfg{
		Name:     "TEST_STREAM_GET",
		Subjects: []string{"test.get.>"},
		Storage:  models.MemoryStorage,
	}
	err := s.repo.CreateOrUpdateStream(s.ctx, cfg)
	require.NoError(s.T(), err)

	// Действие
	retrieved, err := s.repo.GetStream(s.ctx, "TEST_STREAM_GET")
	require.NoError(s.T(), err)

	// Проверка
	require.NotNil(s.T(), retrieved)
	require.Equal(s.T(), "TEST_STREAM_GET", retrieved.Name)
	require.Equal(s.T(), models.MemoryStorage, retrieved.Storage)
}

func (s *NatsRepoTestSuite) TestGetStream_NotFound() {
	_, err := s.repo.GetStream(s.ctx, "NON_EXISTENT_STREAM")
	require.Error(s.T(), err)
}

// --- Тесты для Consumer ---

func (s *NatsRepoTestSuite) TestCreateConsumer_Success() {
	// Сначала нужен стрим
	streamCfg := &models.StreamCfg{
		Name:     "STREAM_FOR_CONSUMER",
		Subjects: []string{"events.>"},
		Storage:  models.FileStorage,
	}
	err := s.repo.CreateOrUpdateStream(s.ctx, streamCfg)
	require.NoError(s.T(), err)

	// Создаем консьюмер
	consumerCfg := &models.Consumer{
		Name:         "MY_CONSUMER",
		StreamName:   "STREAM_FOR_CONSUMER",
		AckPolicy:    models.AckExplicit,
		MaxDeliver:   3,
		Durable:      true, // Важно для CreateOrUpdate
		ReplayPolicy: models.ReplayInstant,
	}

	err = s.repo.CreateOrUpdateConsumer(s.ctx, consumerCfg)
	require.NoError(s.T(), err)

	// Проверка через JS API
	consumer, err := s.js.Consumer(s.ctx, "STREAM_FOR_CONSUMER", "MY_CONSUMER")
	require.NoError(s.T(), err)
	info, err := consumer.Info(s.ctx)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "MY_CONSUMER", info.Name)
	require.Equal(s.T(), int(3), info.Config.MaxDeliver)
}

func (s *NatsRepoTestSuite) TestUpdateConsumer_Exists() {
	// Подготовка: Стрим и Консьюмер
	s.repo.CreateOrUpdateStream(s.ctx, &models.StreamCfg{Name: "STREAM_UPD_C", Subjects: []string{"upd.>"}, Storage: models.FileStorage})

	initialConsumer := &models.Consumer{
		Name:       "CONSUMER_UPD",
		StreamName: "STREAM_UPD_C",
		AckPolicy:  models.AckExplicit,
		MaxDeliver: 1,
		Durable:    true,
	}
	err := s.repo.CreateOrUpdateConsumer(s.ctx, initialConsumer)
	require.NoError(s.T(), err)

	// Обновление: меняем MaxDeliver
	updatedConsumer := &models.Consumer{
		Name:       "CONSUMER_UPD",
		StreamName: "STREAM_UPD_C",
		AckPolicy:  models.AckExplicit,
		MaxDeliver: 10, // Было 1, стало 10
		Durable:    true,
	}
	err = s.repo.CreateOrUpdateConsumer(s.ctx, updatedConsumer)
	require.NoError(s.T(), err)

	// Проверка
	consumer, err := s.js.Consumer(s.ctx, "STREAM_UPD_C", "CONSUMER_UPD")
	require.NoError(s.T(), err)
	info, err := consumer.Info(s.ctx)
	require.NoError(s.T(), err)
	require.Equal(s.T(), int(10), info.Config.MaxDeliver)
}

func (s *NatsRepoTestSuite) TestGetConsumer_Success() {
	// Подготовка
	s.repo.CreateOrUpdateStream(s.ctx, &models.StreamCfg{Name: "STREAM_GET_C", Subjects: []string{"get.c.>"}, Storage: models.FileStorage})

	consumerCfg := &models.Consumer{
		Name:       "CONSUMER_GET",
		StreamName: "STREAM_GET_C",
		AckPolicy:  models.AckAll,
		MaxDeliver: 5,
		Durable:    true,
	}
	err := s.repo.CreateOrUpdateConsumer(s.ctx, consumerCfg)
	require.NoError(s.T(), err)

	// Действие
	retrieved, err := s.repo.GetConsumer(s.ctx, "STREAM_GET_C", "CONSUMER_GET")
	require.NoError(s.T(), err)

	// Проверка
	require.NotNil(s.T(), retrieved)
	require.Equal(s.T(), "CONSUMER_GET", retrieved.Name)
	require.Equal(s.T(), models.AckAll, retrieved.AckPolicy)
	require.Equal(s.T(), 5, retrieved.MaxDeliver)
}

// Запуск сюита
func TestNatsRepositoryIntegration(t *testing.T) {
	suite.Run(t, new(NatsRepoTestSuite))
}

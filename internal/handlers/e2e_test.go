package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"astroapi/config"
	"astroapi/internal/alisa"
	"astroapi/internal/astro"
	"astroapi/internal/handlers"
	natsinfra "astroapi/internal/infrastructure/nats"
	"astroapi/internal/metrics"
	"astroapi/internal/models"
	"astroapi/internal/repositories/domain"
	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/user"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---- in-memory fakes ------------------------------------------------

type memUserRepo struct {
	mu    sync.Mutex
	users map[string]user.User
}

func newMemUserRepo() *memUserRepo { return &memUserRepo{users: map[string]user.User{}} }

func (m *memUserRepo) Save(_ context.Context, u user.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.UserID] = u
	return nil
}

func (m *memUserRepo) Get(_ context.Context, id string) (user.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

type memRequestsRepo struct {
	mu    sync.Mutex
	items map[string]requests.Request
}

func newMemRequestsRepo() *memRequestsRepo {
	return &memRequestsRepo{items: map[string]requests.Request{}}
}

func (m *memRequestsRepo) Create(_ context.Context, r requests.Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[r.RequestID] = r
	return nil
}

func (m *memRequestsRepo) StartProcessing(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.items[id]
	if !ok {
		return false, requests.ErrNotFound
	}
	canStart := len(r.Result) == 0 && (r.Status == requests.StatusPending ||
		r.Status == requests.StatusRetry ||
		(r.Status == requests.StatusFailed && r.AttemptCount < requests.MaxProcessingAttempts))
	if !canStart {
		return false, nil
	}
	r.Status = requests.StatusProcessing
	m.items[id] = r
	return true, nil
}

func (m *memRequestsRepo) UpdateStatus(_ context.Context, id, status string, result []byte, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.items[id]
	if !ok {
		return requests.ErrNotFound
	}
	r.Status = status
	r.Result = result
	r.ErrorReason = reason
	r.AttemptCount++
	m.items[id] = r
	return nil
}

func (m *memRequestsRepo) Get(_ context.Context, id string) (requests.Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.items[id]
	if !ok {
		return requests.Request{}, requests.ErrNotFound
	}
	return r, nil
}

func (m *memRequestsRepo) lookup(id string) (requests.Request, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.items[id]
	return r, ok
}

type memRulesRepo struct{ tags []string }

func (m *memRulesRepo) Match(context.Context, []string) ([]string, error) { return m.tags, nil }

type stubAI struct{ reply string }

func (s stubAI) Generate(context.Context, string) (string, error) { return s.reply, nil }

type stubAstroClient struct {
	natalData astro.NatalData
	err       error
}

func (s stubAstroClient) GetNatalChart(context.Context, astro.DateOfBirth, float64, float64) (astro.NatalData, error) {
	return s.natalData, s.err
}

type memPersonalDataRepo struct {
	mu    sync.Mutex
	items map[string]domain.PersonalData
}

func newMemPersonalDataRepo() *memPersonalDataRepo {
	return &memPersonalDataRepo{items: map[string]domain.PersonalData{}}
}

func (m *memPersonalDataRepo) Save(_ context.Context, data domain.PersonalData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[data.UserID] = data
	return nil
}

type memPersonalDataCache struct {
	mu    sync.Mutex
	items map[string]domain.PersonalData
}

func newMemPersonalDataCache() *memPersonalDataCache {
	return &memPersonalDataCache{items: map[string]domain.PersonalData{}}
}

func (m *memPersonalDataCache) Save(_ context.Context, data domain.PersonalData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[data.UserID] = data
	return nil
}

// ---- embedded NATS --------------------------------------------------

func startNATS(t *testing.T) (host, port string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, portStr, _ := net.SplitHostPort(l.Addr().String())
	require.NoError(t, l.Close())

	p, err := net.LookupPort("tcp", portStr)
	require.NoError(t, err)

	ns, err := server.NewServer(&server.Options{
		Host: host, Port: p, JetStream: true,
		StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	require.NoError(t, err)
	go ns.Start()
	t.Cleanup(func() { ns.Shutdown(); ns.WaitForShutdown() })
	require.Truef(t, ns.ReadyForConnections(5*time.Second), "NATS not ready")
	return host, portStr
}

// ---- e2e ------------------------------------------------------------

// TestE2E_ProfilePipeline: HTTP POST /astro/profile → NATS → ProfileProcessor
// сохраняет пользователя и апдейтит результат генерации в статус completed.
func TestE2E_ProfilePipeline(t *testing.T) {
	host, port := startNATS(t)

	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{NATSHost: host, NATSPort: port}
	conn, err := natsinfra.InitNATS(ctx, logger, cfg)
	require.NoError(t, err)
	defer conn.DrainNATS()

	js, err := jetstream.New(conn.Conn)
	require.NoError(t, err)
	adapter := natsinfra.NewJetStreamRepository(js, logger)
	require.NoError(t, adapter.InitializeStreams(ctx))

	publisher := natsinfra.NewMessagePublisher(adapter)
	userRepo := newMemUserRepo()
	reqRepo := newMemRequestsRepo()
	personalDataRepo := newMemPersonalDataRepo()
	personalDataCache := newMemPersonalDataCache()
	personalDataUC := usecases.NewProcessPersonalDataUseCase(personalDataRepo, personalDataCache)
	astroClient := stubAstroClient{natalData: testE2ENatalData()}

	profileProc := handlers.NewProfileProcessor(userRepo, reqRepo, astroClient, logger)
	router := handlers.NewMsgRouter(logger)
	router.Register(models.MsgProfileSubj, profileProc)

	consumer := natsinfra.NewMessageConsumer(adapter)
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgProfileWrk,
		func(c context.Context, msg jetstream.Msg) error {
			return router.Dispatch(c, msg.Subject(), msg.Data())
		}))

	h := handlers.NewProfileHandler(publisher, reqRepo, personalDataUC, logger)
	body := []byte(`{
		"user_id":"123e4567-e89b-12d3-a456-426614174000",
		"birth_date":"1990-01-01",
		"birth_time":"10:30",
		"lat":55.75,
		"lon":37.61,
		"timezone":"Europe/Moscow",
		"consent_given":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.HandleProfile(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	requestID := resp["request_id"]
	require.NotEmpty(t, requestID)

	waitFor(t, 3*time.Second, func() bool {
		r, ok := reqRepo.lookup(requestID)
		return ok && r.Status == requests.StatusCompleted
	})

	final, ok := reqRepo.lookup(requestID)
	require.True(t, ok)
	var natalData astro.NatalData
	require.NoError(t, json.Unmarshal(final.Result, &natalData))
	require.Equal(t, "external", natalData.Provider)
	require.Contains(t, natalData.Triggers, "sun:capricorn")
}

// TestE2E_RecommendPipeline: async POST /astro/recommend → NATS → RecommendProcessor
// вызывает AI, пишет completed в результат генерации.
func TestE2E_RecommendPipeline(t *testing.T) {
	host, port := startNATS(t)

	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{NATSHost: host, NATSPort: port}
	conn, err := natsinfra.InitNATS(ctx, logger, cfg)
	require.NoError(t, err)
	defer conn.DrainNATS()

	js, err := jetstream.New(conn.Conn)
	require.NoError(t, err)
	adapter := natsinfra.NewJetStreamRepository(js, logger)
	require.NoError(t, adapter.InitializeStreams(ctx))

	publisher := natsinfra.NewMessagePublisher(adapter)
	userRepo := newMemUserRepo()
	require.NoError(t, userRepo.Save(ctx, user.User{
		UserID:       validUserID,
		BirthDate:    "1990-01-01",
		ConsentGiven: true,
	}))

	reqRepo := newMemRequestsRepo()
	rulesRepo := &memRulesRepo{tags: []string{"luxury"}}
	ai := stubAI{reply: "e2e response"}

	astroClient := stubAstroClient{natalData: testE2ENatalData()}
	proc := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, astroClient, logger)
	router := handlers.NewMsgRouter(logger)
	router.Register(models.MsgRecommendSubj, proc)

	consumer := natsinfra.NewMessageConsumer(adapter)
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgRecommendWrk,
		func(c context.Context, msg jetstream.Msg) error {
			return router.Dispatch(c, msg.Subject(), msg.Data())
		}))

	h := handlers.NewRecommendHandler(publisher, userRepo, rulesRepo, ai, astroClient, reqRepo, logger)

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "async",
		"context":  map[string]any{"triggers": []string{"Полнолуние"}, "lat": 55.75, "lon": 37.61},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/recommend", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	requestID := resp["request_id"]

	waitFor(t, 3*time.Second, func() bool {
		r, ok := reqRepo.lookup(requestID)
		return ok && r.Status == requests.StatusCompleted && len(r.Result) > 0
	})

	final, _ := reqRepo.lookup(requestID)
	var res handlers.RecommendationResult
	require.NoError(t, json.Unmarshal(final.Result, &res))
	require.Equal(t, "e2e response", res.Text)
	require.Equal(t, []string{"luxury"}, res.Tags)
}

// TestE2E_ProfileFailureAfterFiveAttemptsGoesToDLQ проверяет полный retry/DLQ pipeline:
// HTTP POST /astro/profile → NATS → ProfileProcessor получает 5 ошибок Astro API →
// requests_log переходит в failed, а исходное сообщение появляется в astro.dlq.profile.
func TestE2E_ProfileFailureAfterFiveAttemptsGoesToDLQ(t *testing.T) {
	metrics.Initialize(&config.Config{Environment: "test", LogServiceName: "handlers-e2e"})

	host, port := startNATS(t)

	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{NATSHost: host, NATSPort: port}
	conn, err := natsinfra.InitNATS(ctx, logger, cfg)
	require.NoError(t, err)
	defer conn.DrainNATS()

	js, err := jetstream.New(conn.Conn)
	require.NoError(t, err)
	adapter := natsinfra.NewJetStreamRepository(js, logger)
	require.NoError(t, adapter.InitializeStreams(ctx))

	publisher := natsinfra.NewMessagePublisher(adapter)
	userRepo := newMemUserRepo()
	reqRepo := newMemRequestsRepo()
	personalDataRepo := newMemPersonalDataRepo()
	personalDataCache := newMemPersonalDataCache()
	personalDataUC := usecases.NewProcessPersonalDataUseCase(personalDataRepo, personalDataCache)
	astroErr := errors.New("astro upstream returned 500")
	astroClient := stubAstroClient{err: astroErr}

	profileProc := handlers.NewProfileProcessor(userRepo, reqRepo, astroClient, logger)
	router := handlers.NewMsgRouter(logger)
	router.Register(models.MsgProfileSubj, profileProc)

	consumer := natsinfra.NewMessageConsumer(adapter)
	consumer.SetBackOffForTest([4]time.Duration{
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
		10 * time.Millisecond,
	})
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgProfileWrk,
		func(c context.Context, msg jetstream.Msg) error {
			return router.Dispatch(c, msg.Subject(), msg.Data())
		}))

	h := handlers.NewProfileHandler(publisher, reqRepo, personalDataUC, logger)
	body := []byte(`{
		"user_id":"123e4567-e89b-12d3-a456-426614174001",
		"birth_date":"1991-02-03",
		"birth_time":"10:30",
		"lat":55.75,
		"lon":37.61,
		"timezone":"Europe/Moscow",
		"consent_given":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.HandleProfile(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	requestID := resp["request_id"]
	require.NotEmpty(t, requestID)

	waitFor(t, 5*time.Second, func() bool {
		r, ok := reqRepo.lookup(requestID)
		return ok && r.Status == requests.StatusFailed && r.AttemptCount == requests.MaxProcessingAttempts
	})

	final, ok := reqRepo.lookup(requestID)
	require.True(t, ok)
	require.Equal(t, requests.StatusFailed, final.Status)
	require.Equal(t, requests.MaxProcessingAttempts, final.AttemptCount)
	require.Contains(t, final.ErrorReason, astroErr.Error())
	require.Empty(t, final.Result)

	dlqReader := natsinfra.NewDLQReader(adapter, logger)
	waitFor(t, 5*time.Second, func() bool {
		messages, err := dlqReader.GetMessages(ctx)
		if err != nil {
			return false
		}
		for _, message := range messages {
			if message.Subject != "astro.dlq.profile" {
				continue
			}
			if len(message.Headers["original_subject"]) == 0 || message.Headers["original_subject"][0] != models.MsgProfileSubj {
				continue
			}
			if len(message.Headers["failure_reason"]) == 0 {
				continue
			}
			if !bytes.Contains([]byte(message.Data), []byte(requestID)) {
				continue
			}
			return true
		}
		return false
	})
}

// waitFor поллит условие с коротким шагом, чтобы не зависеть от фиксированных sleep.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func testE2ENatalData() astro.NatalData {
	return astro.NatalData{
		Provider: "external",
		Planets:  []astro.PlanetPosition{{Name: "Sun", Sign: "Capricorn"}},
		Triggers: []string{"sun:capricorn"},
	}
}

var (
	_ handlers.RuleMatcher   = (*memRulesRepo)(nil)
	_ handlers.AstroProvider = stubAstroClient{}
	_ alisa.Generator        = stubAI{}
)

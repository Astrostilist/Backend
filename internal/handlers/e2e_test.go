package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"astroapi/config"
	"astroapi/internal/alisa"
	"astroapi/internal/handlers"
	natsinfra "astroapi/internal/infrastructure/nats"
	"astroapi/internal/models"
	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/usecases/repositories/domain"
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

type memPersonalDataRepo struct {
	userRepo *memUserRepo
}

func (m *memPersonalDataRepo) Save(ctx context.Context, data domain.PersonalData) error {
	return m.userRepo.Save(ctx, user.User{
		UserID:       data.UserID,
		BirthDate:    data.DOB,
		ConsentGiven: data.ConsentGiven,
	})
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
// сохраняет пользователя и апдейтит requests_log в статус completed.
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

	publisher := natsinfra.NewMessagePublisher(adapter, logger)
	userRepo := newMemUserRepo()
	reqRepo := newMemRequestsRepo()

	dbRepo := &memPersonalDataRepo{userRepo: userRepo}
	cacheRepo := &memPersonalDataRepo{userRepo: userRepo}
	uc := usecases.NewProcessPersonalDataUseCase(dbRepo, cacheRepo)

	profileProc := handlers.NewProfileProcessor(userRepo, reqRepo, logger)
	router := handlers.NewMsgRouter(logger)
	router.Register(models.MsgProfileSubj, profileProc)

	consumer := natsinfra.NewMessageConsumer(adapter, logger)
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgProfileWrk,
		func(c context.Context, msg jetstream.Msg) error {
			return router.Dispatch(c, msg.Subject(), msg.Data())
		}))

	h := handlers.NewProfileHandler(publisher, reqRepo, uc, logger)

	body := []byte(`{
		"user_id":"123e4567-e89b-12d3-a456-426614174000",
		"birth_date":"1990-01-01",
		"birth_place":"Moscow",
		"consent_given":true
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/astro/profile", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Handle(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	requestID := resp["request_id"]
	require.NotEmpty(t, requestID)

	waitFor(t, 3*time.Second, func() bool {
		r, ok := reqRepo.lookup(requestID)
		return ok && r.Status == requests.StatusCompleted
	})

	u, err := userRepo.Get(ctx, "123e4567-e89b-12d3-a456-426614174000")
	require.NoError(t, err)
	require.Equal(t, "1990-01-01", u.BirthDate)
	require.True(t, u.ConsentGiven)
}

// TestE2E_RecommendPipeline: async POST /astro/recommend → NATS → RecommendProcessor
// вызывает AI, пишет completed в requests_log.
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

	publisher := natsinfra.NewMessagePublisher(adapter, logger)
	userRepo := newMemUserRepo()
	require.NoError(t, userRepo.Save(ctx, user.User{
		UserID:       validUserID,
		BirthDate:    "1990-01-01",
		ConsentGiven: true,
	}))

	reqRepo := newMemRequestsRepo()
	rulesRepo := &memRulesRepo{tags: []string{"luxury"}}
	ai := stubAI{reply: "e2e response"}

	proc := handlers.NewRecommendProcessor(userRepo, reqRepo, rulesRepo, ai, logger)
	router := handlers.NewMsgRouter(logger)
	router.Register(models.MsgRecommendSubj, proc)

	consumer := natsinfra.NewMessageConsumer(adapter, logger)
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgRecommendWrk,
		func(c context.Context, msg jetstream.Msg) error {
			return router.Dispatch(c, msg.Subject(), msg.Data())
		}))

	h := handlers.NewRecommendHandler(publisher, userRepo, rulesRepo, ai, reqRepo, logger)

	body, _ := json.Marshal(map[string]any{
		"user_id":  validUserID,
		"scenario": "personal_style",
		"mode":     "async",
		"context":  map[string]any{"triggers": []string{"Полнолуние"}},
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

var (
	_ handlers.RuleMatcher = (*memRulesRepo)(nil)
	_ alisa.Generator      = stubAI{}
)

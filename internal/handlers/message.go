package handlers

import (
	"astroapi/internal/alisa"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
)

type EventHandler interface {
	Handle(ctx context.Context, payload []byte) error
}

type MsgRouter struct {
	handlers map[string]EventHandler
	logger   *zap.Logger
}

// MsgPublisher - интерфейс для отправки сообщений
type MsgPublisher interface {
	PublishMessage(ctx context.Context, streamName, subject string, payload any) error
}

type HandlerFunc func(ctx context.Context, payload []byte) error

func (f HandlerFunc) Handle(ctx context.Context, payload []byte) error {
	return f(ctx, payload)
}

func NewMsgRouter(logger *zap.Logger) *MsgRouter {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MsgRouter{
		handlers: make(map[string]EventHandler),
		logger:   logger,
	}
}

func (r *MsgRouter) Register(subject string, handler EventHandler) {
	r.handlers[subject] = handler
	r.logger.Info("Handler registered", zap.String("event_type", subject))
}

// Dispatch принимает сырые данные и вызывает нужный хендлер для обработки
func (r *MsgRouter) Dispatch(ctx context.Context, subject string, data []byte) error {
	handler, ok := r.handlers[subject]
	if !ok {
		return fmt.Errorf("no handler found for subject: %s", subject)
	}

	r.logger.Debug("Dispatching message", zap.String("subject", subject))
	return handler.Handle(ctx, data)
}

type astroProfileFetcher interface {
	GetAstroProfile(birthDate, birthPlace string) (alisa.AstroProfile, error)
}

type recommendationGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type astroProfileStore interface {
	Save(ctx context.Context, profile alisa.AstroProfile) error
	Get(ctx context.Context, userID string) (alisa.AstroProfile, bool, error)
}

type memoryAstroProfileStore struct {
	mu       sync.RWMutex
	profiles map[string]alisa.AstroProfile
}

func newMemoryAstroProfileStore() *memoryAstroProfileStore {
	return &memoryAstroProfileStore{
		profiles: make(map[string]alisa.AstroProfile),
	}
}

func (s *memoryAstroProfileStore) Save(_ context.Context, profile alisa.AstroProfile) error {
	if strings.TrimSpace(profile.UserID) == "" {
		return errors.New("astro profile user_id is empty")
	}

	s.mu.Lock()
	s.profiles[profile.UserID] = profile
	s.mu.Unlock()

	return nil
}

func (s *memoryAstroProfileStore) Get(_ context.Context, userID string) (alisa.AstroProfile, bool, error) {
	var profile alisa.AstroProfile
	var found bool

	s.mu.RLock()
	profile, found = s.profiles[userID]
	s.mu.RUnlock()

	return profile, found, nil
}

type MessageHandlers struct {
	astroClient  astroProfileFetcher
	generator    recommendationGenerator
	profileStore astroProfileStore
	logger       *zap.Logger
}

func NewMessageHandlers(
	astroClient astroProfileFetcher,
	generator recommendationGenerator,
	profileStore astroProfileStore,
	logger *zap.Logger,
) *MessageHandlers {
	if profileStore == nil {
		profileStore = newMemoryAstroProfileStore()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MessageHandlers{
		astroClient:  astroClient,
		generator:    generator,
		profileStore: profileStore,
		logger:       logger,
	}
}

var (
	defaultMessageHandlersMu sync.RWMutex
	defaultMessageHandlers   = NewMessageHandlers(nil, nil, newMemoryAstroProfileStore(), zap.NewNop())
)

func ConfigureMessageHandlers(
	astroClient astroProfileFetcher,
	generator recommendationGenerator,
	profileStore astroProfileStore,
	logger *zap.Logger,
) {
	defaultMessageHandlersMu.Lock()
	defaultMessageHandlers = NewMessageHandlers(astroClient, generator, profileStore, logger)
	defaultMessageHandlersMu.Unlock()
}

func HandleProfile(ctx context.Context, payload []byte) error {
	defaultMessageHandlersMu.RLock()
	handlers := defaultMessageHandlers
	defaultMessageHandlersMu.RUnlock()

	return handlers.HandleProfile(ctx, payload)
}

func HandleRecommend(ctx context.Context, payload []byte) error {
	defaultMessageHandlersMu.RLock()
	handlers := defaultMessageHandlers
	defaultMessageHandlersMu.RUnlock()

	return handlers.HandleRecommend(ctx, payload)
}

func (h *MessageHandlers) HandleProfile(ctx context.Context, payload []byte) error {
	var req ProfileRequest
	var profile alisa.AstroProfile
	var err error

	if h.astroClient == nil {
		return errors.New("astro client is not configured")
	}
	if h.profileStore == nil {
		return errors.New("astro profile store is not configured")
	}

	err = json.Unmarshal(payload, &req)
	if err != nil {
		return fmt.Errorf("decode profile payload: %w", err)
	}

	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		return fmt.Errorf("profile request validation failed: %s", formatValidationErrors(validationErrors))
	}

	profile, err = h.astroClient.GetAstroProfile(req.BirthDate, req.BirthPlace)
	if err != nil {
		return fmt.Errorf("fetch astro profile: %w", err)
	}

	profile = enrichAstroProfile(profile, req)

	err = h.profileStore.Save(ctx, profile)
	if err != nil {
		return fmt.Errorf("save astro profile: %w", err)
	}

	h.logger.Info(
		"astro profile processed",
		zap.String("user_id", profile.UserID),
		zap.String("birth_date", profile.BirthDate),
		zap.String("birth_place", profile.BirthPlace),
	)

	return nil
}

func (h *MessageHandlers) HandleRecommend(ctx context.Context, payload []byte) error {
	var req RecommendRequest
	var profile alisa.AstroProfile
	var prompt string
	var result string
	var found bool
	var err error

	if h.profileStore == nil {
		return errors.New("astro profile store is not configured")
	}
	if h.generator == nil {
		return errors.New("AlisaAI client is not configured")
	}

	err = json.Unmarshal(payload, &req)
	if err != nil {
		return fmt.Errorf("decode recommend payload: %w", err)
	}

	if validationErrors := req.Validate(); len(validationErrors) > 0 {
		return fmt.Errorf("recommend request validation failed: %s", formatValidationErrors(validationErrors))
	}

	profile, found, err = h.profileStore.Get(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("load astro profile: %w", err)
	}
	if !found {
		return fmt.Errorf("astro profile not found for user_id: %s", req.UserID)
	}

	prompt = alisa.BuildPrompt(req.Scenario, profile, req.Context)
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("build prompt for scenario %q: empty prompt", req.Scenario)
	}

	result, err = h.generator.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("generate recommendation: %w", err)
	}

	h.logger.Info(
		"recommendation generated",
		zap.String("user_id", req.UserID),
		zap.String("scenario", req.Scenario),
	)
	h.logger.Debug(
		"recommendation result",
		zap.String("user_id", req.UserID),
		zap.String("scenario", req.Scenario),
		zap.String("result", result),
	)

	return nil
}

func enrichAstroProfile(profile alisa.AstroProfile, req ProfileRequest) alisa.AstroProfile {
	if profile.UserID == "" {
		profile.UserID = req.UserID
	}
	if profile.BirthDate == "" {
		profile.BirthDate = req.BirthDate
	}
	if profile.BirthTime == "" {
		profile.BirthTime = req.BirthTime
	}
	if profile.BirthPlace == "" {
		profile.BirthPlace = req.BirthPlace
	}

	return profile
}

func formatValidationErrors(validationErrors map[string]string) string {
	keys := make([]string, 0, len(validationErrors))
	for key := range validationErrors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, validationErrors[key]))
	}

	return strings.Join(parts, ", ")
}

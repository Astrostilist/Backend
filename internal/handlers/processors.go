package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"astroapi/internal/alisa"
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"astroapi/internal/requests"
	"astroapi/internal/user"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
)

// RuleMatcher — узкий интерфейс: handlers нужны только триггеры → тэги.
// Реализуется ruleengine.PostgresRepository (и моками в тестах).
type RuleMatcher interface {
	Match(ctx context.Context, triggers []string) ([]string, error)
}

// AstroProfileGetter — узкий интерфейс внешнего Astro API клиента.
// Реализуется alisa.AstroAPIClient; интерфейс оставлен маленьким для тестов воркеров.
type AstroProfileGetter interface {
	GetAstroProfileContext(ctx context.Context, birthDate, birthPlace string) (alisa.AstroProfile, error)
}

// RecommendationResult — результат построения рекомендации (общий для sync и async).
type RecommendationResult struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

// buildRecommendation собирает профиль, триггеры, prompt и дергает AlisaAI.
// Если в context передан birth_place, профиль дополнительно обогащается через Astro API.
func buildRecommendation(
	ctx context.Context,
	req RecommendRequest,
	userRepo user.Repository,
	rulesRepo RuleMatcher,
	ai alisa.Generator,
	astroClient AstroProfileGetter,
	logger *zap.Logger,
) (RecommendationResult, error) {
	u, err := userRepo.Get(ctx, req.UserID)
	if err != nil {
		return RecommendationResult{}, err
	}

	triggers, _ := triggersFromContext(req.Context)
	tags, err := rulesRepo.Match(ctx, triggers)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("match rules: %w", err)
	}

	astroProfile, err := buildAstroProfile(ctx, req, u, astroClient)
	if err != nil {
		return RecommendationResult{}, err
	}

	enrichedCtx := map[string]any{"tags": tags}
	for k, v := range req.Context {
		enrichedCtx[k] = v
	}

	prompt := alisa.BuildPrompt(req.Scenario, astroProfile, enrichedCtx, logger)
	if prompt == "" {
		return RecommendationResult{}, fmt.Errorf("validation: cannot build prompt for scenario %q", req.Scenario)
	}

	text, err := ai.Generate(ctx, prompt)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("ai generate: %w", err)
	}

	return RecommendationResult{Text: text, Tags: tags}, nil
}

func buildAstroProfile(
	ctx context.Context,
	req RecommendRequest,
	u user.User,
	astroClient AstroProfileGetter,
) (alisa.AstroProfile, error) {
	birthPlace, _ := stringFromContext(req.Context, "birth_place")
	profile := alisa.AstroProfile{
		UserID:     u.UserID,
		BirthDate:  u.BirthDate,
		BirthPlace: birthPlace,
	}

	if astroClient == nil || birthPlace == "" {
		return profile, nil
	}

	apiProfile, err := astroClient.GetAstroProfileContext(ctx, u.BirthDate, birthPlace)
	if err != nil {
		return profile, fmt.Errorf("astro api profile: %w", err)
	}

	if apiProfile.UserID == "" {
		apiProfile.UserID = u.UserID
	}
	if apiProfile.BirthDate == "" {
		apiProfile.BirthDate = u.BirthDate
	}
	if apiProfile.BirthPlace == "" {
		apiProfile.BirthPlace = birthPlace
	}

	return apiProfile, nil
}

// triggersFromContext достаёт список триггеров из context.
// Ожидается формат `{"triggers": ["Полнолуние", ...]}`.
func triggersFromContext(ctx map[string]any) ([]string, bool) {
	raw, ok := ctx["triggers"]
	if !ok {
		return nil, false
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	triggers := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			triggers = append(triggers, s)
		}
	}
	return triggers, true
}

func stringFromContext(ctx map[string]any, key string) (string, bool) {
	raw, ok := ctx[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	return value, value != ""
}

func workerPayload(message []byte) (json.RawMessage, map[string]string, error) {
	var wrapped models.MessageWithTrace
	if err := json.Unmarshal(message, &wrapped); err != nil {
		return nil, nil, err
	}

	if len(wrapped.Payload) == 0 {
		return json.RawMessage(message), nil, nil
	}

	return wrapped.Payload, wrapped.TraceContext, nil
}

func traceContextFromMessage(ctx context.Context, traceContext map[string]string) context.Context {
	if len(traceContext) == 0 {
		return ctx
	}

	carrier := propagation.MapCarrier(traceContext)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func beginRequestProcessing(
	ctx context.Context,
	repo requests.Repository,
	logger *zap.Logger,
	requestID string,
) (bool, error) {
	req, err := repo.Get(ctx, requestID)
	if err != nil {
		return false, fmt.Errorf("get request before processing: %w", err)
	}
	if len(req.Result) > 0 {
		logger.Info("skipping duplicate request_id", zap.String("request_id", requestID))
		return false, nil
	}
	if req.Status == requests.StatusFailed && req.AttemptCount >= requests.MaxProcessingAttempts {
		logger.Info("request already exhausted attempts, skip processing",
			zap.String("request_id", requestID),
			zap.Int("attempt_count", req.AttemptCount))
		return false, nil
	}

	started, err := repo.StartProcessing(ctx, requestID)
	if err != nil {
		return false, fmt.Errorf("start request processing: %w", err)
	}
	if !started {
		current, getErr := repo.Get(ctx, requestID)
		if getErr != nil {
			return false, fmt.Errorf("get request after start conflict: %w", getErr)
		}
		if len(current.Result) > 0 {
			logger.Info("skipping duplicate request_id", zap.String("request_id", requestID))
			return false, nil
		}
		if current.Status == requests.StatusFailed && current.AttemptCount >= requests.MaxProcessingAttempts {
			logger.Info("request already exhausted attempts, skip processing",
				zap.String("request_id", requestID),
				zap.Int("attempt_count", current.AttemptCount))
			return false, nil
		}
		return false, fmt.Errorf("request %s has no result_payload and cannot start processing: status=%s attempt_count=%d",
			requestID, current.Status, current.AttemptCount)
	}
	return true, nil
}

// ProfileProcessor обрабатывает сообщения astro.events.profile.
// Воркер вызывает внешний Astro API, пишет результат в requests_log и завершает задачу.
type ProfileProcessor struct {
	requestsRepo requests.Repository
	astroClient  AstroProfileGetter
	logger       *zap.Logger
}

func NewProfileProcessor(
	_ user.Repository,
	requestsRepo requests.Repository,
	astroClient AstroProfileGetter,
	logger *zap.Logger,
) *ProfileProcessor {
	return &ProfileProcessor{requestsRepo: requestsRepo, astroClient: astroClient, logger: logger}
}

func (p *ProfileProcessor) Handle(ctx context.Context, message []byte) error {
	payload, traceContext, err := workerPayload(message)
	if err != nil {
		return fmt.Errorf("validation: invalid profile payload: %w", err)
	}

	wctx := traceContextFromMessage(ctx, traceContext)

	tracer := otel.Tracer("worker-profile")
	spanctx, span := tracer.Start(wctx, "worker.handle-profile")
	defer span.End()

	tctx, cancel := context.WithTimeout(spanctx, 30*time.Second)
	defer cancel()

	var msg profilePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		span.RecordError(err)
		astrologger.Error(tctx, "invalid payload", zap.Error(err))
		return fmt.Errorf("validation: invalid profile payload: %w", err)
	}

	if errs := msg.Profile.Validate(); len(errs) > 0 {
		err := fmt.Errorf("validation: %v", errs)
		span.RecordError(err)
		return err
	}

	shouldProcess, err := beginRequestProcessing(tctx, p.requestsRepo, p.logger, msg.RequestID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if !shouldProcess {
		return nil
	}

	if p.astroClient == nil {
		err := errors.New("astro profile client is not configured")
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return err
	}

	profile, err := p.astroClient.GetAstroProfileContext(tctx, msg.Profile.BirthDate, msg.Profile.BirthPlace)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return fmt.Errorf("astro api profile: %w", err)
	}

	profile.UserID = msg.Profile.UserID
	if profile.BirthDate == "" {
		profile.BirthDate = msg.Profile.BirthDate
	}
	if profile.BirthTime == "" {
		profile.BirthTime = msg.Profile.BirthTime
	}
	if profile.BirthPlace == "" {
		profile.BirthPlace = msg.Profile.BirthPlace
	}

	resultJSON, err := json.Marshal(profile)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return fmt.Errorf("marshal astro profile: %w", err)
	}

	if err := p.requestsRepo.UpdateStatus(tctx, msg.RequestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		span.RecordError(err)
		astrologger.Error(tctx, "failed to mark profile request as completed", zap.Error(err))
		return err
	}

	astrologger.Info(tctx, "astro profile generated",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", msg.Profile.UserID))
	return nil
}

func (p *ProfileProcessor) markRetryOrFailed(ctx context.Context, requestID string, err error, worker string) {
	status := p.nextFailureStatus(ctx, requestID, worker)
	if updateErr := p.requestsRepo.UpdateStatus(ctx, requestID, status, nil, err.Error()); updateErr != nil {
		p.logger.Error("failed to mark request after processing error",
			zap.String("worker", worker),
			zap.String("status", status),
			zap.Error(updateErr))
	}
}

func (p *ProfileProcessor) nextFailureStatus(ctx context.Context, requestID string, worker string) string {
	status := requests.StatusRetry
	req, err := p.requestsRepo.Get(ctx, requestID)
	if err != nil {
		p.logger.Warn("failed to read attempt_count before retry update",
			zap.String("worker", worker),
			zap.Error(err))
		return status
	}
	if req.AttemptCount+1 >= requests.MaxProcessingAttempts {
		status = requests.StatusFailed
	}
	return status
}

// RecommendProcessor обрабатывает сообщения astro.events.recommend.
// Строит рекомендацию через Astro API + AlisaAI и пишет результат в requests_log.
type RecommendProcessor struct {
	userRepo     user.Repository
	requestsRepo requests.Repository
	rulesRepo    RuleMatcher
	aiClient     alisa.Generator
	astroClient  AstroProfileGetter
	logger       *zap.Logger
}

func NewRecommendProcessor(
	userRepo user.Repository,
	requestsRepo requests.Repository,
	rulesRepo RuleMatcher,
	aiClient alisa.Generator,
	astroClient AstroProfileGetter,
	logger *zap.Logger,
) *RecommendProcessor {
	return &RecommendProcessor{
		userRepo:     userRepo,
		requestsRepo: requestsRepo,
		rulesRepo:    rulesRepo,
		aiClient:     aiClient,
		astroClient:  astroClient,
		logger:       logger,
	}
}

func (p *RecommendProcessor) markRetryOrFailed(ctx context.Context, requestID string, err error, worker string) {
	status := p.nextFailureStatus(ctx, requestID, worker)
	if updateErr := p.requestsRepo.UpdateStatus(ctx, requestID, status, nil, err.Error()); updateErr != nil {
		p.logger.Error("failed to mark request after processing error",
			zap.String("worker", worker),
			zap.String("status", status),
			zap.Error(updateErr))
	}
}

func (p *RecommendProcessor) nextFailureStatus(ctx context.Context, requestID string, worker string) string {
	status := requests.StatusRetry
	req, err := p.requestsRepo.Get(ctx, requestID)
	if err != nil {
		p.logger.Warn("failed to read attempt_count before retry update",
			zap.String("worker", worker),
			zap.Error(err))
		return status
	}
	if req.AttemptCount+1 >= requests.MaxProcessingAttempts {
		status = requests.StatusFailed
	}
	return status
}

func (p *RecommendProcessor) Handle(ctx context.Context, message []byte) error {
	payload, traceContext, err := workerPayload(message)
	if err != nil {
		return fmt.Errorf("validation: invalid recommend payload: %w", err)
	}

	wctx := traceContextFromMessage(ctx, traceContext)

	tracer := otel.Tracer("worker-recommend")
	spanctx, span := tracer.Start(wctx, "worker.handle-recommend")
	defer span.End()

	tctx, cancel := context.WithTimeout(spanctx, 30*time.Second)
	defer cancel()

	var msg recommendPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		span.RecordError(err)
		return fmt.Errorf("validation: invalid recommend payload: %w", err)
	}

	shouldProcess, err := beginRequestProcessing(tctx, p.requestsRepo, p.logger, msg.RequestID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if !shouldProcess {
		return nil
	}

	result, err := buildRecommendation(tctx, msg.Recommend, p.userRepo, p.rulesRepo, p.aiClient, p.astroClient, p.logger)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")
		return fmt.Errorf("recommend processing failed: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")
		return fmt.Errorf("marshal recommendation: %w", err)
	}

	if err := p.requestsRepo.UpdateStatus(tctx, msg.RequestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		span.RecordError(err)
		astrologger.Error(tctx, "failed to mark recommend request as completed", zap.Error(err))
		return err
	}

	astrologger.Info(tctx, "recommendation generated",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", msg.Recommend.UserID))
	return nil
}

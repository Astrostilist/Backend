package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"astroapi/internal/alisa"
	"astroapi/internal/requests"
	"astroapi/internal/user"

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

func beginRequestProcessing(
	ctx context.Context,
	repo requests.Repository,
	logger *zap.Logger,
	requestID string,
) (bool, error) {
	req, err := repo.Get(ctx, requestID)
	if err != nil {
		return false, err
	}
	if req.Status == requests.StatusCompleted {
		logger.Info("skipping duplicate request_id", zap.String("request_id", requestID))
		return false, nil
	}
	if req.Status != requests.StatusPending {
		logger.Info("request is not pending, skip processing",
			zap.String("request_id", requestID), zap.String("status", req.Status))
		return false, nil
	}

	started, err := repo.StartProcessing(ctx, requestID)
	if err != nil {
		return false, err
	}
	if !started {
		current, getErr := repo.Get(ctx, requestID)
		if getErr != nil {
			return false, getErr
		}
		if current.Status == requests.StatusCompleted {
			logger.Info("skipping duplicate request_id", zap.String("request_id", requestID))
			return false, nil
		}
		logger.Info("request was locked by another worker",
			zap.String("request_id", requestID), zap.String("status", current.Status))
		return false, nil
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

func (p *ProfileProcessor) Handle(ctx context.Context, payload []byte) error {
	var msg profilePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		return fmt.Errorf("validation: invalid profile payload: %w", err)
	}
	if errs := msg.Profile.Validate(); len(errs) > 0 {
		return fmt.Errorf("validation: %v", errs)
	}

	shouldProcess, err := beginRequestProcessing(ctx, p.requestsRepo, p.logger, msg.RequestID)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}

	if p.astroClient == nil {
		err := errors.New("astro profile client is not configured")
		p.markFailed(ctx, msg.RequestID, err, "profile")
		return err
	}

	profile, err := p.astroClient.GetAstroProfileContext(ctx, msg.Profile.BirthDate, msg.Profile.BirthPlace)
	if err != nil {
		p.markFailed(ctx, msg.RequestID, err, "profile")
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
		p.markFailed(ctx, msg.RequestID, err, "profile")
		return fmt.Errorf("marshal astro profile: %w", err)
	}

	if err := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		p.logger.Error("failed to mark profile request as completed", zap.Error(err))
		return err
	}

	p.logger.Info("astro profile generated",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", msg.Profile.UserID))
	return nil
}

func (p *ProfileProcessor) markFailed(ctx context.Context, requestID string, err error, worker string) {
	if updateErr := p.requestsRepo.UpdateStatus(ctx, requestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
		p.logger.Error("failed to mark request as failed", zap.String("worker", worker), zap.Error(updateErr))
	}
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

func (p *RecommendProcessor) Handle(ctx context.Context, payload []byte) error {
	var msg recommendPayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		return fmt.Errorf("validation: invalid recommend payload: %w", err)
	}

	shouldProcess, err := beginRequestProcessing(ctx, p.requestsRepo, p.logger, msg.RequestID)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}

	result, err := buildRecommendation(ctx, msg.Recommend, p.userRepo, p.rulesRepo, p.aiClient, p.astroClient, p.logger)
	if err != nil {
		if updateErr := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
			p.logger.Error("failed to mark recommend request as failed", zap.Error(updateErr))
		}
		return err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		if updateErr := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
			p.logger.Error("failed to mark recommend request as failed", zap.Error(updateErr))
		}
		return fmt.Errorf("marshal recommendation: %w", err)
	}
	if err := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		p.logger.Error("failed to mark recommend request as completed", zap.Error(err))
		return err
	}
	p.logger.Info("recommendation generated",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", msg.Recommend.UserID))
	return nil
}

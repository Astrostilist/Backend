package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"astroapi/internal/alisa"
	astroproc "astroapi/internal/astroproc"
	"astroapi/internal/requests"
	"astroapi/internal/user"

	"go.uber.org/zap"
)

// RuleMatcher — узкий интерфейс: handlers нужны только триггеры → тэги.
// Реализуется ruleengine.PostgresRepository (и моками в тестах).
type RuleMatcher interface {
	Match(ctx context.Context, triggers []string) ([]string, error)
}

// RecommendationResult — результат построения рекомендации (общий для sync и async).
type RecommendationResult struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

// buildRecommendation собирает профиль, триггеры, prompt и дергает AlisaAI.
// Возвращает готовый RecommendationResult или ошибку (в т.ч. обёрнутую validation).
func buildRecommendation(
	ctx context.Context,
	req RecommendRequest,
	userRepo user.Repository,
	rulesRepo RuleMatcher,
	ai alisa.Generator, logger *zap.Logger) (RecommendationResult, error) {
	u, err := userRepo.Get(ctx, req.UserID)
	if err != nil {
		return RecommendationResult{}, err
	}

	triggers, _ := triggersFromContext(req.Context)
	tags, err := rulesRepo.Match(ctx, triggers)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("match rules: %w", err)
	}

	astroProfile := alisa.AstroProfile{
		UserID:    u.UserID,
		BirthDate: u.BirthDate,
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
		// TODO:
		// разобраться с процессом. Обработчик должен либо обрабатывать запрос до состояния completed,
		// либо возвращать ошибку. Иначе у нас и запрос не обработан и сообщение из очереди удалено
		return false, fmt.Errorf("request is not pending, skip processing")
		//logger.Info("request is not pending, skip processing",
		//	zap.String("request_id", requestID), zap.String("status", req.Status))
		//return false, nil
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
// Сохраняет астропрофиль пользователя и обновляет requests_log.

type ProfileProcessor struct {
	userRepo     user.Repository
	requestsRepo requests.Repository
	astroproc    astroproc.AstroProc
	logger       *zap.Logger
}

func NewProfileProcessor(userRepo user.Repository, requestsRepo requests.Repository,
	astroproc astroproc.AstroProc, logger *zap.Logger) *ProfileProcessor {
	return &ProfileProcessor{userRepo: userRepo, requestsRepo: requestsRepo, astroproc: astroproc, logger: logger}
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

	// TODO: get astroprofile from cache or astro API call instead of user save
	// user had saved in (h *ProfileHandler) HandleProfile
	profile, err := p.astroproc.GetAstroProfile(ctx, msg.Profile.UserID)
	if err != nil {
		return fmt.Errorf("failed to get astro profile: %v", err)
	}

	p.logger.Info("got astro profile", zap.String("userID", msg.Profile.UserID), zap.Any("data", profile))

	if err := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusCompleted, nil, ""); err != nil {
		p.logger.Error("failed to mark profile request as completed", zap.Error(err))
	}
	/*// consent_given=false: дата рождения не сохраняется в БД (GDPR / ФЗ-152).
	if msg.Profile.ConsentGiven {
		if err := p.userRepo.Save(ctx, user.User{
			UserID:       msg.Profile.UserID,
			BirthDate:    msg.Profile.BirthDate,
			ConsentGiven: msg.Profile.ConsentGiven,
		}); err != nil {
			p.markFailed(ctx, msg.RequestID, err)
			return err
		}
		p.logger.Info("profile saved", zap.String("request_id", msg.RequestID), zap.String("user_id", msg.Profile.UserID))
	} else {
		p.logger.Info("profile skipped: no consent", zap.String("request_id", msg.RequestID), zap.String("user_id", msg.Profile.UserID))
	}

	if err := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusCompleted, nil, ""); err != nil {
		p.logger.Error("failed to mark profile request as completed", zap.Error(err))
	}*/
	return nil
}

func (p *ProfileProcessor) markFailed(ctx context.Context, requestID string, err error) {
	if updateErr := p.requestsRepo.UpdateStatus(ctx, requestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
		p.logger.Error("failed to mark profile request as failed", zap.Error(updateErr))
	}
}

// RecommendProcessor обрабатывает сообщения astro.events.recommend.
// Строит рекомендацию через AlisaAI и пишет результат в requests_log.
type RecommendProcessor struct {
	userRepo     user.Repository
	requestsRepo requests.Repository
	rulesRepo    RuleMatcher
	aiClient     alisa.Generator
	logger       *zap.Logger
}

func NewRecommendProcessor(
	userRepo user.Repository,
	requestsRepo requests.Repository,
	rulesRepo RuleMatcher,
	aiClient alisa.Generator,
	logger *zap.Logger) *RecommendProcessor {
	return &RecommendProcessor{
		userRepo:     userRepo,
		requestsRepo: requestsRepo,
		rulesRepo:    rulesRepo,
		aiClient:     aiClient,
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

	result, err := buildRecommendation(ctx, msg.Recommend, p.userRepo, p.rulesRepo, p.aiClient, p.logger)
	if err != nil {
		if updateErr := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusFailed, nil, err.Error()); updateErr != nil {
			p.logger.Error("failed to mark recommend request as failed", zap.Error(updateErr))
		}
		return err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal recommendation: %w", err)
	}
	if err := p.requestsRepo.UpdateStatus(ctx, msg.RequestID, requests.StatusCompleted, resultJSON, ""); err != nil {
		p.logger.Error("failed to mark recommend request as completed", zap.Error(err))
	}
	p.logger.Info("recommendation generated",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", msg.Recommend.UserID))
	return nil
}

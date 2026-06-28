package handlers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"astroapi/internal/astro"
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"astroapi/internal/repositories/domain"
	"astroapi/internal/requests"
	"astroapi/internal/usecases"
	"astroapi/internal/user"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
)

var ZodiacMapping = map[string]string{
	astro.AriesID:       domain.AriesID,
	astro.TaurusID:      domain.TaurusID,
	astro.GeminiID:      domain.GeminiID,
	astro.CancerID:      domain.CancerID,
	astro.LeoID:         domain.LeoID,
	astro.VirgoID:       domain.VirgoID,
	astro.LibraID:       domain.LibraID,
	astro.ScorpioID:     domain.ScorpioID,
	astro.SagittariusID: domain.SagittariusID,
	astro.CapricornID:   domain.CapricornID,
	astro.AquariusID:    domain.AquariusID,
	astro.PiscesID:      domain.PiscesID,
}

var PlanetMapping = map[string]string{
	astro.SunID:     domain.Sun,
	astro.MoonID:    domain.Moon,
	astro.MercuryID: domain.Mercury,
	astro.VenusID:   domain.Venus,
	astro.MarsID:    domain.Mars,
	astro.JupiterID: domain.Jupiter,
	astro.SaturnID:  domain.Saturn,
	astro.UranusID:  domain.Uranus,
	astro.NeptuneID: domain.Neptune,
	astro.PlutoID:   domain.Pluto,
}

// RuleMatcher — узкий интерфейс: handlers нужны только триггеры → тэги.
// Реализуется ruleengine.PostgresRepository (и моками в тестах).
type RuleMatcher interface {
	Match(ctx context.Context, triggers []string) ([]string, error)
}

// AstroProvider — узкий интерфейс внешнего провайдера натальной карты.
// Бизнес-логика зависит только от внутреннего формата astro.NatalData.
type AstroProvider interface {
	GetNatalChart(ctx context.Context, dob astro.DateOfBirth, lat float64, lon float64) (astro.NatalData, error)
}

// RecommendationResult — результат построения рекомендации (общий для sync и async).
type RecommendationResult struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

// buildRecommendation получает натальную карту через AstroProvider,
// строит astro-триггеры, подбирает теги через ruleengine и вызывает AlisaAI.
func buildRecommendation(
	ctx context.Context,
	req RecommendRequest,
	userRepo user.Repository,
	rulesRepo RuleMatcher,
	astroProvider AstroProvider,
	logger *zap.Logger,
) (RecommendationResult, error) {
	u, err := userRepo.Get(ctx, req.UserID)
	if err != nil {
		return RecommendationResult{}, err
	}

	triggers, _ := triggersFromContext(req.Context)
	natalData, err := buildNatalData(ctx, req, u, astroProvider)
	if err != nil {
		return RecommendationResult{}, err
	}
	triggers = appendUniqueTriggers(triggers, natalData.Triggers)
	tags, err := rulesRepo.Match(ctx, triggers)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("match rules: %w", err)
	}

	enrichedCtx := map[string]any{"tags": tags}
	for k, v := range req.Context {
		enrichedCtx[k] = v
	}

	// TODO: search products
	/*prompt := alisa.BuildPrompt(req.Scenario, natalData, enrichedCtx, logger)
	if prompt == "" {
		return RecommendationResult{}, fmt.Errorf("validation: cannot build prompt for scenario %q", req.Scenario)
	}

	text, err := ai.Generate(ctx, prompt)
	if err != nil {
		return RecommendationResult{}, fmt.Errorf("ai generate: %w", err)
	}

	return RecommendationResult{Text: text, Tags: tags}, nil*/
	return RecommendationResult{}, nil

}

func buildNatalData(
	ctx context.Context,
	req RecommendRequest,
	u user.User,
	astroProvider AstroProvider,
) (astro.NatalData, error) {
	var natalData astro.NatalData
	if astroProvider == nil {
		return natalData, errors.New("astro provider is not configured")
	}
	dob, lat, lon, err := natalInputFromRequest(req.Context, u.BirthDate)
	if err != nil {
		return natalData, err
	}
	natalData, err = astroProvider.GetNatalChart(ctx, dob, lat, lon)
	if err != nil {
		return natalData, fmt.Errorf("astro natal chart: %w", err)
	}
	return natalData, nil
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

func natalInputFromRequest(ctx map[string]any, birthDate string) (astro.DateOfBirth, float64, float64, error) {
	parsedDate, err := time.Parse("2006-01-02", birthDate)
	if err != nil {
		return astro.DateOfBirth{}, 0, 0, fmt.Errorf("validation: birth_date must be YYYY-MM-DD: %w", err)
	}
	lat, hasLat := floatFromContext(ctx, "lat")
	if !hasLat {
		lat, hasLat = floatFromContext(ctx, "birth_lat")
	}
	lon, hasLon := floatFromContext(ctx, "lon")
	if !hasLon {
		lon, hasLon = floatFromContext(ctx, "birth_lon")
	}
	if !hasLat || !hasLon {
		return astro.DateOfBirth{}, 0, 0, errors.New("validation: natal chart requires lat/lon or birth_lat/birth_lon")
	}
	hour, minute := timeFromContext(ctx, "birth_time")
	timezone, _ := stringFromContext(ctx, "timezone")
	dob := astro.DateOfBirth{
		Year:     parsedDate.Year(),
		Month:    int(parsedDate.Month()),
		Day:      parsedDate.Day(),
		Hour:     hour,
		Minute:   minute,
		Timezone: timezone,
	}
	return dob, lat, lon, nil
}

func appendUniqueTriggers(existing []string, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	result := make([]string, 0, len(existing)+len(incoming))
	for _, trigger := range existing {
		if _, ok := seen[trigger]; !ok {
			result = append(result, trigger)
			seen[trigger] = struct{}{}
		}
	}
	for _, trigger := range incoming {
		if _, ok := seen[trigger]; !ok {
			result = append(result, trigger)
			seen[trigger] = struct{}{}
		}
	}
	return result
}

func floatFromContext(ctx map[string]any, key string) (float64, bool) {
	raw, ok := ctx[key]
	if !ok {
		return 0, false
	}
	result := 0.0
	success := true
	switch value := raw.(type) {
	case float64:
		result = value
	case float32:
		result = float64(value)
	case int:
		result = float64(value)
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			success = false
		} else {
			result = parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			success = false
		} else {
			result = parsed
		}
	default:
		success = false
	}
	return result, success
}

func natalInputFromProfile(profile ProfileRequest) (astro.DateOfBirth, float64, float64, error) {
	parsedDate, err := time.Parse("2006-01-02", profile.BirthDate)
	if err != nil {
		return astro.DateOfBirth{}, 0, 0, fmt.Errorf("validation: birth_date must be YYYY-MM-DD: %w", err)
	}
	if profile.Lat == nil || profile.Lon == nil {
		return astro.DateOfBirth{}, 0, 0, errors.New("validation: natal chart requires lat and lon")
	}
	hour, minute := timeFromValue(profile.BirthTime)
	dob := astro.DateOfBirth{
		Year:     parsedDate.Year(),
		Month:    int(parsedDate.Month()),
		Day:      parsedDate.Day(),
		Hour:     hour,
		Minute:   minute,
		Timezone: profile.Timezone,
	}
	return dob, *profile.Lat, *profile.Lon, nil
}

func timeFromContext(ctx map[string]any, key string) (int, int) {
	value, ok := stringFromContext(ctx, key)
	if !ok {
		return 0, 0
	}
	return timeFromValue(value)
}

func timeFromValue(value string) (int, int) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, 0
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil {
		return 0, 0
	}
	return hour, minute
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
		return false, fmt.Errorf("request %s allready in processing by another worker: status=%s attempt_count=%d",
			requestID, current.Status, current.AttemptCount)
	}
	return true, nil
}

// ProfileProcessor обрабатывает сообщения astro.events.profile.
// Воркер вызывает внешний Astro API, пишет результат в requests_log и завершает задачу.
type ProfileProcessor struct {
	astroRepo     AstroRepo
	requestsRepo  requests.Repository
	astroProvider AstroProvider
	crmClient     CrmClient
	logger        *zap.Logger
}
type AstroRepo interface {
	ExecuteSave(ctx context.Context, profile domain.AstroProfile) error
	ExecuteReceivingByHash(ctx context.Context, hash string) (*domain.AstroProfile, error)
}

type CrmClient interface {
	SendProfile(ctx context.Context, profile domain.AstroProfile) error
	SendRecommend(ctx context.Context, recommend string) error
}

func NewProfileProcessor(
	astroRepo AstroRepo,
	requestsRepo requests.Repository,
	astroProvider AstroProvider,
	crmClient CrmClient,
	logger *zap.Logger,
) *ProfileProcessor {
	return &ProfileProcessor{astroRepo: astroRepo, requestsRepo: requestsRepo, astroProvider: astroProvider, crmClient: crmClient, logger: logger}
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

	if p.astroProvider == nil {
		err := errors.New("astro provider is not configured")
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return err
	}

	// Create a hash from UserID, BirthDate and BirthTime fields for storage
	hash := ProfileHash(msg.Profile.UserID, msg.Profile.BirthDate, msg.Profile.BirthTime)
	astroProfile, err := p.astroRepo.ExecuteReceivingByHash(tctx, hash)
	if err != nil && !errors.Is(usecases.ErrNotFound, err) {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return err
	}

	if astroProfile == nil {

		dob, lat, lon, err := natalInputFromProfile(msg.Profile)
		if err != nil {
			span.RecordError(err)
			p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
			return err
		}

		natalData, err := p.astroProvider.GetNatalChart(tctx, dob, lat, lon)
		if err != nil {
			span.RecordError(err)
			p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
			return fmt.Errorf("astro natal chart: %w", err)
		}

		profileData := natalDataToAstroProfile(natalData)
		profile := domain.AstroProfile{
			ID:           msg.RequestID,
			UserID:       msg.Profile.UserID,
			ProfileHash:  hash,
			DOB:          msg.Profile.BirthDate,
			DOBTime:      msg.Profile.BirthTime,
			ConsentGiven: msg.Profile.ConsentGiven,
			ProfileData:  profileData,
		}

		// store db or cache
		err = p.astroRepo.ExecuteSave(tctx, profile)
		if err != nil {
			span.RecordError(err)
			p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
			return fmt.Errorf("store astro profile error: %w", err)
		}

		/*resultJSON, err := json.Marshal(natalData)
		if err != nil {
			span.RecordError(err)
			p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
			return fmt.Errorf("marshal natal chart: %w", err)
		}*/

		astrologger.Info(tctx, "astro profile generated",
			zap.String("request_id", msg.RequestID),
			zap.String("user_id", msg.Profile.UserID))
		astroProfile = &profile
	} else {
		astrologger.Info(tctx, "astro profile got from storage",
			zap.String("request_id", msg.RequestID),
			zap.String("user_id", msg.Profile.UserID))
	}

	// post CRM webhook_url with astroprofile
	p.crmClient.SendProfile(tctx, *astroProfile)

	if err := p.requestsRepo.UpdateStatus(tctx, msg.RequestID, requests.StatusCompleted, nil, ""); err != nil {
		span.RecordError(err)
		astrologger.Error(tctx, "failed to mark profile request as completed", zap.Error(err))
		return err
	}

	return nil

}

func natalDataToAstroProfile(data astro.NatalData) domain.ProfileData {
	var profileData domain.ProfileData

	// Assigning planet signs to profile fields using mappings
	profileData.Sun = ZodiacMapping[PlanetMapping[astro.SunID]]
	profileData.Moon = ZodiacMapping[PlanetMapping[astro.MoonID]]
	profileData.Mercury = ZodiacMapping[PlanetMapping[astro.MercuryID]]
	profileData.Venus = ZodiacMapping[PlanetMapping[astro.VenusID]]
	profileData.Mars = ZodiacMapping[PlanetMapping[astro.MarsID]]
	profileData.Jupiter = ZodiacMapping[PlanetMapping[astro.JupiterID]]
	profileData.Saturn = ZodiacMapping[PlanetMapping[astro.SaturnID]]
	profileData.Uranus = ZodiacMapping[PlanetMapping[astro.UranusID]]
	profileData.Neptune = ZodiacMapping[PlanetMapping[astro.NeptuneID]]
	profileData.Pluto = ZodiacMapping[PlanetMapping[astro.PlutoID]]

	// TODO: Handle special cases like Ascendant

	return profileData
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
	astroRepo     AstroRepo
	crmClient     CrmClient
	requestsRepo  requests.Repository
	rulesRepo     RuleMatcher
	astroProvider AstroProvider
	logger        *zap.Logger
}

func NewRecommendProcessor(
	astroRepo AstroRepo,
	requestsRepo requests.Repository,
	rulesRepo RuleMatcher,
	astroProvider AstroProvider,
	crmClient CrmClient,
	logger *zap.Logger,
) *RecommendProcessor {
	return &RecommendProcessor{
		astroRepo:     astroRepo,
		requestsRepo:  requestsRepo,
		rulesRepo:     rulesRepo,
		astroProvider: astroProvider,
		crmClient:     crmClient,
		logger:        logger,
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

	result, err := buildRecommendation(tctx, msg.Recommend, nil, p.rulesRepo, p.astroProvider, p.logger)
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

func ProfileHash(userID, birthDate, birthTime string) string {
	// Create a hash from UserID, BirthDate and BirthTime fields for storage
	var hashInput strings.Builder
	hashInput.WriteString(userID)
	hashInput.WriteString(birthDate)
	hashInput.WriteString(birthTime)

	return fmt.Sprintf("%x", md5.Sum([]byte(hashInput.String())))
}

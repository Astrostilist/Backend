package handlers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"astroapi/internal/astro"
	astrologger "astroapi/internal/infrastructure/logger"
	"astroapi/internal/models"
	"astroapi/internal/products"
	"astroapi/internal/repositories/domain"
	"astroapi/internal/requests"
	"astroapi/internal/usecases"
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

// AstroProvider — узкий интерфейс внешнего провайдера натальной карты.
// Бизнес-логика зависит только от внутреннего формата astro.NatalData.
type AstroProvider interface {
	GetNatalChart(ctx context.Context, dob astro.DateOfBirth, lat float64, lon float64) (astro.NatalData, error)
}

// RecommendationResult — результат построения рекомендации (общий для sync и async).
type RecommendationResult struct {
	RequestId string                  `json:"request_id"`
	Text      string                  `json:"text"`
	Tags      []string                `json:"tags"`
	Recommend []models.CatalogProduct `json:"recommended_items"` // TODO: поменять на DTO модель
}

func natalInputFromProfile(profile ProfileRequest) (astro.DateOfBirth, float64, float64, error) {
	parsedDate, err := time.Parse("2006-01-02", profile.BirthDate)
	if err != nil {
		return astro.DateOfBirth{}, 0, 0, fmt.Errorf("validation: birth_date must be YYYY-MM-DD: %w", err)
	}
	if profile.Lat == nil || profile.Lon == nil {
		return astro.DateOfBirth{}, 0, 0, errors.New("validation: natal chart requires lat and lon")
	}
	dob := astro.DateOfBirth{
		Year:     parsedDate.Year(),
		Month:    int(parsedDate.Month()),
		Day:      parsedDate.Day(),
		Timezone: profile.Timezone,
	}
	return dob, *profile.Lat, *profile.Lon, nil
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
		return false, fmt.Errorf("request %s already in processing by another worker: status=%s attempt_count=%d",
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

	// Create a hash from UserID andBirthDate fields for storage
	hash := ProfileHash(msg.Profile.UserID, msg.Profile.BirthDate)
	astroProfile, err := p.astroRepo.ExecuteReceivingByHash(tctx, hash)
	if err != nil && !errors.Is(err, usecases.ErrNotFound) {
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
		astrologger.Debug(tctx, "got natal data", zap.Any("natalData", natalData))

		profileData := natalDataToAstroProfile(natalData)
		profile := domain.AstroProfile{
			ID:           msg.RequestID,
			UserID:       msg.Profile.UserID,
			ProfileHash:  hash,
			DOB:          msg.Profile.BirthDate,
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
	err = p.crmClient.SendProfile(tctx, *astroProfile)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "profile")
		return fmt.Errorf("send to crm profile error: %w", err)
	}

	if err := p.requestsRepo.UpdateStatus(tctx, msg.RequestID, requests.StatusCompleted, nil, ""); err != nil {
		span.RecordError(err)
		astrologger.Error(tctx, "failed to mark profile request as completed", zap.Error(err))
		return err
	}

	return nil

}

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

var PlanetSetters = map[string]func(*domain.ProfileData, string){
	astro.SunID:     func(pd *domain.ProfileData, s string) { pd.Sun = s },
	astro.MoonID:    func(pd *domain.ProfileData, s string) { pd.Moon = s },
	astro.MercuryID: func(pd *domain.ProfileData, s string) { pd.Mercury = s },
	astro.VenusID:   func(pd *domain.ProfileData, s string) { pd.Venus = s },
	astro.MarsID:    func(pd *domain.ProfileData, s string) { pd.Mars = s },
	astro.JupiterID: func(pd *domain.ProfileData, s string) { pd.Jupiter = s },
	astro.SaturnID:  func(pd *domain.ProfileData, s string) { pd.Saturn = s },
	astro.UranusID:  func(pd *domain.ProfileData, s string) { pd.Uranus = s },
	astro.NeptuneID: func(pd *domain.ProfileData, s string) { pd.Neptune = s },
	astro.PlutoID:   func(pd *domain.ProfileData, s string) { pd.Pluto = s },
}

func natalDataToAstroProfile(data astro.NatalData) domain.ProfileData {
	var profileData domain.ProfileData

	for _, p := range data.Planets {
		if setter, ok := PlanetSetters[p.Id]; ok {
			setter(&profileData, ZodiacMapping[p.SignId])
		}
	}

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
	userRepo      user.Repository
	productRepo   products.Repository
	crmClient     CrmClient
	requestsRepo  requests.Repository
	rulesRepo     RuleMatcher
	astroProvider AstroProvider
	logger        *zap.Logger
}

func NewRecommendProcessor(
	astroRepo AstroRepo,
	userRepo user.Repository,
	productRepo products.Repository,
	requestsRepo requests.Repository,
	rulesRepo RuleMatcher,
	astroProvider AstroProvider,
	crmClient CrmClient,
	logger *zap.Logger,
) *RecommendProcessor {
	return &RecommendProcessor{
		astroRepo:     astroRepo,
		userRepo:      userRepo,
		productRepo:   productRepo,
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

	if p.astroProvider == nil {
		err := errors.New("astro provider is not configured")
		return err
	}

	u, err := p.userRepo.Get(ctx, msg.Recommend.UserID)
	if err != nil {
		return err
	}

	// Create a hash from UserID and BirthDate fields for storage
	hash := ProfileHash(u.UserID, u.BirthDate)
	astroProfile, err := p.astroRepo.ExecuteReceivingByHash(tctx, hash)
	// в пользователе не хранится место рождения, поэтому рассчитать заново не получится
	// TODOЖ обработать ошибку errors.Is(usecases.ErrNotFound, err) отдельно, чтобы сообщить пользователю
	if err != nil {
		astrologger.Error(tctx, "errror fetching astro profile from storage",
			zap.String("request_id", msg.RequestID),
			zap.String("user_id", u.UserID))
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")
		return err
	}

	astrologger.Info(tctx, "astro profile got from storage",
		zap.String("request_id", msg.RequestID),
		zap.String("user_id", u.UserID))

	scen := msg.Recommend.Scenario
	gen := msg.Recommend.Gender
	pref := msg.Recommend.Preferences
	planets := astroProfile.ProfileData

	triggers := buildTriggers(planets)
	tags, err := p.rulesRepo.Match(ctx, triggers)
	if err != nil {
		astrologger.Error(tctx, "error match rules",
			zap.String("request_id", msg.RequestID),
			zap.String("user_id", u.UserID))
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")

		return fmt.Errorf("error match rules: %w", err)
	}

	prods, err := p.productRepo.Recommend(ctx, tags, gen, scen, pref)
	if err != nil {

		astrologger.Error(tctx, "error search products",
			zap.String("request_id", msg.RequestID),
			zap.String("user_id", u.UserID))
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")

		return fmt.Errorf("error search products: %w", err)

	}

	resp := RecommendationResult{
		RequestId: msg.RequestID,
		Recommend: prods,
	}

	resultJSON, err := json.Marshal(resp)
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")
		return fmt.Errorf("marshal recommendation: %w", err)
	}

	err = p.crmClient.SendRecommend(ctx, string(resultJSON))
	if err != nil {
		span.RecordError(err)
		p.markRetryOrFailed(tctx, msg.RequestID, err, "recommend")
		return fmt.Errorf("send to crm recommend error: %w", err)
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

func buildTriggers(planets domain.ProfileData) []string {
	var triggers []string
	planetMap := map[string]string{
		domain.Sun:     planets.Sun,
		domain.Moon:    planets.Moon,
		domain.Venus:   planets.Venus,
		domain.Mars:    planets.Mars,
		domain.Jupiter: planets.Jupiter,
		domain.Saturn:  planets.Saturn,
		domain.Mercury: planets.Mercury,
		domain.Neptune: planets.Neptune,
		domain.Uranus:  planets.Uranus,
		domain.Pluto:   planets.Pluto,
	}

	for planet, sign := range planetMap {
		if sign != "" {
			triggers = append(triggers, fmt.Sprintf(`{"sign": %q, "planet": %q}`, sign, planet))
		}
	}
	return triggers
}

func ProfileHash(userID, birthDate string) string {
	// Create a hash from UserID and BirthDate fields for storage
	var hashInput strings.Builder
	hashInput.WriteString(userID)
	hashInput.WriteString(birthDate)

	return fmt.Sprintf("%x", md5.Sum([]byte(hashInput.String())))
}

package astro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultExternalAstroAPIBaseURL = "https://api.freeastroapi.com"
	externalChartPath              = "/api/v2/vedic/chart"
	defaultHTTPTimeout             = 10 * time.Second
)

type ExternalAstroProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type ExternalAstroProviderOptions struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewExternalAstroProvider(opts ExternalAstroProviderOptions) *ExternalAstroProvider {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultExternalAstroAPIBaseURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &ExternalAstroProvider{baseURL: baseURL, apiKey: opts.APIKey, httpClient: httpClient}
}

func (p *ExternalAstroProvider) GetNatalChart(ctx context.Context, dob DateOfBirth, lat float64, lon float64) (NatalData, error) {
	var data NatalData
	if p == nil {
		return data, errors.New("external astro provider is not configured")
	}
	if strings.TrimSpace(p.apiKey) == "" {
		return data, errors.New("ASTRO_API_KEY is not configured")
	}
	requestBody, err := json.Marshal(externalChartRequest{
		Year:        dob.Year,
		Month:       dob.Month,
		Day:         dob.Day,
		Hour:        dob.Hour,
		Minute:      dob.Minute,
		Latitude:    lat,
		Longitude:   lon,
		Timezone:    defaultTimezone(dob.Timezone),
		Ayanamsha:   "lahiri",
		HouseSystem: "whole_sign",
		NodeType:    "mean",
	})
	if err != nil {
		return data, fmt.Errorf("encode external astro natal chart request: %w", err)
	}
	url := p.baseURL + externalChartPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return data, fmt.Errorf("create external astro natal chart request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", p.apiKey)

	response, err := p.httpClient.Do(request)
	if err != nil {
		return data, fmt.Errorf("external astro natal chart request failed: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return data, fmt.Errorf("read external astro natal chart response: %w", err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return data, fmt.Errorf("external astro natal chart unavailable: status=%d body=%s", response.StatusCode, message)
	}
	data, err = mapExternalAstroResponse(body)
	if err != nil {
		return data, err
	}
	return data, nil
}

type externalChartRequest struct {
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	Hour        int     `json:"hour"`
	Minute      int     `json:"minute"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lng"`
	Timezone    string  `json:"tz_str"`
	Ayanamsha   string  `json:"ayanamsha"`
	HouseSystem string  `json:"house_system"`
	NodeType    string  `json:"node_type"`
}

type externalChartResponse struct {
	Ascendant *externalAstroBody  `json:"ascendant"`
	Planets   []externalAstroBody `json:"planets"`
	Data      *struct {
		Ascendant *externalAstroBody  `json:"ascendant"`
		Planets   []externalAstroBody `json:"planets"`
	} `json:"data"`
}

type externalAstroBody struct {
	Name       string             `json:"name"`
	Sign       string             `json:"sign"`
	SignName   string             `json:"sign_name"`
	Rashi      string             `json:"rashi"`
	House      int                `json:"house"`
	Degree     float64            `json:"degree"`
	Position   float64            `json:"pos"`
	Retrograde bool               `json:"retrograde"`
	IsRetro    bool               `json:"is_retrograde"`
	SignObject *externalAstroSign `json:"sign_info"`
}

type externalAstroSign struct {
	Name string `json:"name"`
}

func mapExternalAstroResponse(payload []byte) (NatalData, error) {
	var response externalChartResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return NatalData{}, fmt.Errorf("decode external astro natal chart response: %w", err)
	}
	if response.Data != nil {
		if response.Ascendant == nil {
			response.Ascendant = response.Data.Ascendant
		}
		if len(response.Planets) == 0 {
			response.Planets = response.Data.Planets
		}
	}
	data := NatalData{Provider: "external", Planets: make([]PlanetPosition, 0, len(response.Planets))}
	for _, planet := range response.Planets {
		position := mapExternalAstroBody(planet)
		if position.Name != "" && position.Sign != "" {
			data.Planets = append(data.Planets, position)
		}
	}
	if response.Ascendant != nil {
		ascendant := mapExternalAstroBody(*response.Ascendant)
		if ascendant.Sign != "" {
			if ascendant.Name == "" {
				ascendant.Name = "Ascendant"
			}
			data.Ascendant = &ascendant
		}
	}
	if len(data.Planets) == 0 && data.Ascendant == nil {
		return data, errors.New("external astro natal chart response does not contain planets or ascendant")
	}
	data.Triggers = BuildTriggers(data)
	return data, nil
}

func mapExternalAstroBody(body externalAstroBody) PlanetPosition {
	degree := body.Degree
	if degree == 0 {
		degree = body.Position
	}
	return PlanetPosition{
		Name:       strings.TrimSpace(body.Name),
		Sign:       firstNonEmpty(body.Sign, body.SignName, body.Rashi, signNameFromObject(body.SignObject)),
		House:      body.House,
		Degree:     degree,
		Retrograde: body.Retrograde || body.IsRetro,
	}
}

func defaultTimezone(timezone string) string {
	if strings.TrimSpace(timezone) == "" {
		return "UTC"
	}
	return timezone
}

func firstNonEmpty(values ...string) string {
	result := ""
	for _, value := range values {
		if strings.TrimSpace(value) != "" && result == "" {
			result = strings.TrimSpace(value)
		}
	}
	return result
}

func signNameFromObject(sign *externalAstroSign) string {
	if sign == nil {
		return ""
	}
	return sign.Name
}

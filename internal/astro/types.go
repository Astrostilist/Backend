package astro

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// DateOfBirth describes birth date/time for a natal chart request.
type DateOfBirth struct {
	Year     int    `json:"year"`
	Month    int    `json:"month"`
	Day      int    `json:"day"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Timezone string `json:"timezone,omitempty"`
}

// PlanetPosition is the normalized planet-in-sign format consumed by
// rule matching, prompt building and astro_triggers logic.
type PlanetPosition struct {
	Id         string  `json:"id"`
	Name       string  `json:"name"`
	Sign       string  `json:"sign"`
	SignId     string  `json:"sign_id"`
	House      int     `json:"house,omitempty"`
	Degree     float64 `json:"degree,omitempty"`
	Retrograde bool    `json:"retrograde,omitempty"`
}

// NatalData is the provider-agnostic natal chart representation.
type NatalData struct {
	Provider  string           `json:"provider"`
	Planets   []PlanetPosition `json:"planets"`
	Ascendant *PlanetPosition  `json:"ascendant,omitempty"`
	Triggers  []string         `json:"triggers,omitempty"`
}

// AstroProvider hides a concrete natal chart API behind a small interface.
// Implementations can be swapped without touching handlers or rule matching.
type AstroProvider interface {
	GetNatalChart(ctx context.Context, dob DateOfBirth, lat float64, lon float64) (NatalData, error)
}

func BuildNatalChartCacheKey(dob DateOfBirth, lat float64, lon float64) string {
	return fmt.Sprintf(
		//"natal_chart:%04d-%02d-%02dT%02d:%02d:%s:%.5f:%.5f",
		"natal_chart_%04d-%02d-%02dT%02d-%02d_%s_%.5f_%.5f",
		dob.Year,
		dob.Month,
		dob.Day,
		dob.Hour,
		dob.Minute,
		normalizeToken(dob.Timezone),
		roundCoordinate(lat),
		roundCoordinate(lon),
	)
}

func BuildTriggers(data NatalData) []string {
	triggers := make([]string, 0, len(data.Planets)*4+3)
	seen := make(map[string]struct{})
	for _, planet := range data.Planets {
		planetName := normalizeToken(planet.Name)
		signName := normalizeToken(planet.Sign)
		if planetName != "" && signName != "" {
			appendTrigger(&triggers, seen, fmt.Sprintf("%s:%s", planetName, signName))
			appendTrigger(&triggers, seen, fmt.Sprintf("%s_in_%s", planetName, signName))
			appendTrigger(&triggers, seen, fmt.Sprintf("%s in %s", planetName, signName))
			appendRussianPlanetTrigger(&triggers, seen, planetName, signName)
		}
	}
	if data.Ascendant != nil {
		signName := normalizeToken(data.Ascendant.Sign)
		if signName != "" {
			appendTrigger(&triggers, seen, fmt.Sprintf("ascendant:%s", signName))
			appendTrigger(&triggers, seen, fmt.Sprintf("ascendant in %s", signName))
			appendRussianAscendantTrigger(&triggers, seen, signName)
		}
	}
	return triggers
}

func appendRussianPlanetTrigger(dst *[]string, seen map[string]struct{}, planetName string, signName string) {
	russianPlanet, planetOk := russianPlanetNames()[planetName]
	russianSign, signOk := russianSignPrepositionalNames()[signName]
	if planetOk && signOk {
		appendTrigger(dst, seen, fmt.Sprintf("%s в %s", russianPlanet, russianSign))
	}
}

func appendRussianAscendantTrigger(dst *[]string, seen map[string]struct{}, signName string) {
	russianSign, ok := russianSignPrepositionalNames()[signName]
	if ok {
		appendTrigger(dst, seen, fmt.Sprintf("Асцендент в %s", russianSign))
	}
}

func appendTrigger(dst *[]string, seen map[string]struct{}, value string) {
	if _, ok := seen[value]; !ok {
		*dst = append(*dst, value)
		seen[value] = struct{}{}
	}
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func roundCoordinate(value float64) float64 {
	return math.Round(value*100000) / 100000
}

func russianPlanetNames() map[string]string {
	return map[string]string{
		"sun":     "Солнце",
		"moon":    "Луна",
		"mercury": "Меркурий",
		"venus":   "Венера",
		"mars":    "Марс",
		"jupiter": "Юпитер",
		"saturn":  "Сатурн",
		"uranus":  "Уран",
		"neptune": "Нептун",
		"pluto":   "Плутон",
	}
}

func russianSignPrepositionalNames() map[string]string {
	return map[string]string{
		"aries":       "Овне",
		"taurus":      "Тельце",
		"gemini":      "Близнецах",
		"cancer":      "Раке",
		"leo":         "Льве",
		"virgo":       "Деве",
		"libra":       "Весах",
		"scorpio":     "Скорпионе",
		"sagittarius": "Стрельце",
		"capricorn":   "Козероге",
		"aquarius":    "Водолее",
		"pisces":      "Рыбах",
	}
}

// Planet IDs for the natal chart
const (
	SunID     = "sun"
	MoonID    = "moon"
	MercuryID = "mercury"
	VenusID   = "venus"
	MarsID    = "mars"
	JupiterID = "jupiter"
	SaturnID  = "saturn"
	UranusID  = "uranus"
	NeptuneID = "neptune"
	PlutoID   = "pluto"
)

// Zodiac Sign IDs for the natal chart
const (
	AriesID       = "aries"
	TaurusID      = "taurus"
	GeminiID      = "gemini"
	CancerID      = "cancer"
	LeoID         = "leo"
	VirgoID       = "virgo"
	LibraID       = "libra"
	ScorpioID     = "scorpio"
	SagittariusID = "sagittarius"
	CapricornID   = "capricorn"
	AquariusID    = "aquarius"
	PiscesID      = "pisces"
)

package astro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExternalAstroProviderGetNatalChartMapsPlanetsAndTriggers(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v2/vedic/chart" {
			t.Fatalf("path = %s, want /api/v2/vedic/chart", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatalf("missing x-api-key header")
		}
		var request externalChartRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Year != 1992 || request.Latitude != 55.75 {
			t.Fatalf("unexpected request: %+v", request)
		}
		_, _ = w.Write([]byte(`{
			"ascendant": {"sign": "Aries", "degree": 12.1},
			"planets": [
				{"name": "Sun", "sign": "Cancer", "house": 4, "degree": 5.4},
				{"name": "Moon", "sign_name": "Taurus", "house": 2, "degree": 27.0, "is_retrograde": false}
			]
		}`))
	}))
	defer server.Close()

	provider := NewExternalAstroProvider(ExternalAstroProviderOptions{BaseURL: server.URL, APIKey: "secret"})
	data, err := provider.GetNatalChart(context.Background(), DateOfBirth{Year: 1992, Month: 6, Day: 26}, 55.75, 37.61)

	if err != nil {
		t.Fatalf("GetNatalChart error: %v", err)
	}
	if data.Provider != "external" {
		t.Fatalf("provider = %s", data.Provider)
	}
	if len(data.Planets) != 2 {
		t.Fatalf("len(planets) = %d", len(data.Planets))
	}
	if data.Planets[0].Name != "Sun" || data.Planets[0].Sign != "Cancer" {
		t.Fatalf("unexpected first planet: %+v", data.Planets[0])
	}
	if data.Ascendant == nil {
		t.Fatalf("ascendant is nil")
	}
	assertContains(t, data.Triggers, "sun:cancer")
	assertContains(t, data.Triggers, "moon:taurus")
	assertContains(t, data.Triggers, "ascendant:aries")
	assertContains(t, data.Triggers, "Солнце в Раке")
	assertContains(t, data.Triggers, "Луна в Тельце")
	assertContains(t, data.Triggers, "Асцендент в Овне")
}

func TestBuildTriggersAddsRussianAliasesForLegacyRuleNames(t *testing.T) {
	t.Parallel()
	data := NatalData{
		Planets: []PlanetPosition{
			{Name: "Venus", Sign: "Taurus"},
			{Name: "Mars", Sign: "Sagittarius"},
		},
	}

	triggers := BuildTriggers(data)

	assertContains(t, triggers, "venus:taurus")
	assertContains(t, triggers, "Венера в Тельце")
	assertContains(t, triggers, "mars:sagittarius")
	assertContains(t, triggers, "Марс в Стрельце")
}

func TestExternalAstroProviderReturnsClearUnavailableError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream down"}`))
	}))
	defer server.Close()

	provider := NewExternalAstroProvider(ExternalAstroProviderOptions{BaseURL: server.URL, APIKey: "secret"})
	_, err := provider.GetNatalChart(context.Background(), DateOfBirth{Year: 1992, Month: 6, Day: 26}, 55.75, 37.61)

	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "external astro natal chart unavailable") {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "status=502") {
		t.Fatalf("error = %q", err.Error())
	}
}

func assertContains(t *testing.T, values []string, expected string) {
	t.Helper()
	for _, value := range values {
		if value == expected {
			return
		}
	}
	t.Fatalf("%q not found in %v", expected, values)
}

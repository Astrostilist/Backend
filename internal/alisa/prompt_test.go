package alisa

import (
	"testing"

	"astroapi/internal/astro"
	"astroapi/internal/infrastructure/logger"

	"github.com/stretchr/testify/require"
)

func TestBuildPrompt(t *testing.T) {
	zapLogger, err := logger.NewLogger("tests", "debug")
	if err != nil {
		return
	}

	natalData := astro.NatalData{
		Provider: "external",
		Planets: []astro.PlanetPosition{
			{Name: "Sun", Sign: "Capricorn"},
		},
		Triggers: []string{"sun:capricorn"},
	}

	tests := []struct {
		name     string
		scenario string
	}{
		{name: "personal_style", scenario: scenarioPersonalStyle},
		{name: "perfect_gift", scenario: scenarioPerfectGift},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := BuildPrompt(tt.scenario, natalData, map[string]any{
				"budget": "10000",
				"season": "winter",
			}, zapLogger)

			require.NotEmpty(t, prompt)
			require.Contains(t, prompt, "external")
			require.Contains(t, prompt, "Capricorn")
			require.Contains(t, prompt, "winter")
		})
	}
}

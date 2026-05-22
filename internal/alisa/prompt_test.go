package alisa

import (
	"astroapi/internal/infrastructure/logger"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPrompt(t *testing.T) {
	zapLogger, err := logger.NewLogger("tests", "debug")
	if err != nil {
		return
	}

	profile := AstroProfile{
		UserID:     "123e4567-e89b-12d3-a456-426614174000",
		BirthDate:  "1990-01-01",
		BirthTime:  "10:30",
		BirthPlace: "Moscow",
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
			prompt := BuildPrompt(tt.scenario, profile, map[string]any{
				"budget": "10000",
				"season": "winter",
			},
				zapLogger)

			require.NotEmpty(t, prompt)
			require.Contains(t, prompt, profile.BirthDate)
			require.Contains(t, prompt, profile.BirthTime)
			require.Contains(t, prompt, profile.BirthPlace)
			require.Contains(t, prompt, "winter")
		})
	}
}

package astro

import (
	"fmt"
	"strings"

	"astroapi/config"
)

const (
	// ProviderExternal is the first supported concrete natal chart provider.
	ProviderExternal = "external"
)

// NormalizeProviderName converts an optional config value into a stable provider id.
func NormalizeProviderName(value string) string {
	provider := strings.ToLower(strings.TrimSpace(value))
	if provider == "" {
		provider = ProviderExternal
	}
	return provider
}

// NewProviderFromConfig is the only place that knows which concrete adapter is used.
// Business code should depend on AstroProvider and NatalData, not on ExternalAstroProvider.
func NewProviderFromConfig(cfg *config.Config) (AstroProvider, string, error) {
	providerName := ProviderExternal
	baseURL := ""
	apiKey := ""
	if cfg != nil {
		providerName = NormalizeProviderName(cfg.AstroProvider)
		baseURL = cfg.AstroAPIURL
		apiKey = cfg.AstroAPIKey
	}

	switch providerName {
	case ProviderExternal:
		provider := NewExternalAstroProvider(ExternalAstroProviderOptions{BaseURL: baseURL, APIKey: apiKey})
		return provider, providerName, nil
	default:
		return nil, providerName, fmt.Errorf("unsupported ASTRO_PROVIDER %q; supported values: %s", providerName, ProviderExternal)
	}
}

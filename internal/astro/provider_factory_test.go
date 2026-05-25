package astro

import (
	"testing"

	"astroapi/config"
)

func TestNewProviderFromConfigDefaultsToExternalAstroProvider(t *testing.T) {
	provider, providerName, err := NewProviderFromConfig(&config.Config{})
	if err != nil {
		t.Fatalf("NewProviderFromConfig returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider to be configured")
	}
	if providerName != ProviderExternal {
		t.Fatalf("expected provider name %q, got %q", ProviderExternal, providerName)
	}
}

func TestNewProviderFromConfigRejectsUnsupportedProvider(t *testing.T) {
	provider, providerName, err := NewProviderFromConfig(&config.Config{AstroProvider: "unknown"})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if provider != nil {
		t.Fatal("expected nil provider for unsupported value")
	}
	if providerName != "unknown" {
		t.Fatalf("expected normalized provider name unknown, got %q", providerName)
	}
}

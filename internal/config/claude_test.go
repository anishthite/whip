package config

import "testing"

func TestUpsertClaudeAddsSelectableRoute(t *testing.T) {
	cfg := &Config{
		DefaultModel: "existing",
		Providers:    map[string]Provider{"existing": {Name: "Existing"}},
		Models:       map[string]Model{"existing": {Providers: []string{"existing"}}},
	}

	cfg.UpsertClaude()

	provider := cfg.Providers[ClaudeProviderName]
	if provider.Name != "Claude Code" || provider.BaseURL != ClaudeBaseURL || provider.API != "anthropic-messages" || provider.Auth != "claude" {
		t.Fatalf("provider = %+v", provider)
	}
	route := cfg.Models[ClaudeDefaultModel]
	if len(route.Providers) != 1 || route.Providers[0] != ClaudeProviderName || route.Context != ClaudeDefaultContext || route.MaxOut != ClaudeDefaultMaxOut {
		t.Fatalf("route = %+v", route)
	}
	if cfg.DefaultModel != "existing" {
		t.Fatalf("default changed to %q", cfg.DefaultModel)
	}
}

func TestUpsertClaudePreservesRouteOverrides(t *testing.T) {
	cfg := &Config{
		Providers: map[string]Provider{ClaudeProviderName: {Name: "Old"}},
		Models: map[string]Model{ClaudeDefaultModel: {
			Providers: []string{"alternate", ClaudeProviderName},
			Context:   123,
			MaxOut:    456,
		}},
	}

	cfg.UpsertClaude()

	route := cfg.Models[ClaudeDefaultModel]
	if len(route.Providers) != 2 || route.Context != 123 || route.MaxOut != 456 {
		t.Fatalf("route = %+v", route)
	}
}

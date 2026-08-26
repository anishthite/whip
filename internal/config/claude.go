package config

const (
	ClaudeProviderName   = "claude"
	ClaudeBaseURL        = "https://api.anthropic.com"
	ClaudeDefaultModel   = "claude-sonnet-4-6"
	ClaudeDefaultContext = 200000
	ClaudeDefaultMaxOut  = 64000
)

// UpsertClaude registers the Claude Code subscription route without changing
// a user's selected default or an explicit per-model limit override.
func (c *Config) UpsertClaude() {
	if c.Providers == nil {
		c.Providers = map[string]Provider{}
	}
	c.Providers[ClaudeProviderName] = Provider{
		Name:    "Claude Code",
		BaseURL: ClaudeBaseURL,
		API:     "anthropic-messages",
		Auth:    "claude",
	}
	if c.Models == nil {
		c.Models = map[string]Model{}
	}
	m := c.Models[ClaudeDefaultModel]
	if !hasProvider(m.Providers, ClaudeProviderName) {
		m.Providers = append(m.Providers, ClaudeProviderName)
	}
	if m.Context == 0 && m.MaxTokens == 0 {
		m.Context = ClaudeDefaultContext
	}
	if m.MaxOut == 0 {
		m.MaxOut = ClaudeDefaultMaxOut
	}
	c.Models[ClaudeDefaultModel] = m
	logf("config.claude", "upserted Claude provider and %s route", ClaudeDefaultModel)
}
